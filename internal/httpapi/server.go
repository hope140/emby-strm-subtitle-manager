// Package httpapi exposes the intentionally small, gated D1/D2/D3 HTTP surface.
package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hope140/subbridge/internal/auth"
	"github.com/hope140/subbridge/internal/config"
	"github.com/hope140/subbridge/internal/d2"
	"github.com/hope140/subbridge/internal/d3"
	"github.com/hope140/subbridge/internal/domain"
	"github.com/hope140/subbridge/internal/embyclient"
	"github.com/hope140/subbridge/internal/inventory"
	"github.com/hope140/subbridge/internal/media"
	"github.com/hope140/subbridge/internal/pathmap"
	"github.com/hope140/subbridge/internal/version"
)

const (
	readinessProbeTimeout = 3 * time.Second
	readinessSuccessTTL   = 15 * time.Second
	readinessFailureTTL   = 5 * time.Second
	defaultStartIndex     = 0
	defaultLimit          = 50
	maxHTTPItemsLimit     = 200
	maxD2RequestBody      = 8 << 10
	maxUploadRequestBody  = (4 << 20) + (64 << 10)
	maxUploadFieldBytes   = 1024
	defaultHistoryLimit   = 50
	maxHistoryLimit       = 100
	adminSessionCookie    = "subbridge_admin_session"
)

// EmbyReader is the smallest interface needed by the HTTP layer. Keeping the
// interface here allows a later Media/Inventory handler to use the same
// client while keeping HTTP tests independent from a live Emby instance.
type EmbyReader interface {
	ListLibraries(context.Context) ([]domain.Library, error)
	ListItems(context.Context, string, int, int) (domain.ItemPage, error)
	GetItem(context.Context, string) (domain.EmbyItem, error)
}

// Services contains the dependencies needed by the D1 media slice and gated
// D2/D3 operations. The
// fields are interfaces/pointers so HTTP tests can inject a fake Emby client
// without creating a filesystem or exposing server paths.
type Services struct {
	Emby            EmbyReader
	D2              *d2.Service
	D3              *d3.Service
	Mapper          *pathmap.Mapper
	Guard           *pathmap.PathGuard
	Inventory       *inventory.Service
	AuthToken       string
	AuthTokenScopes []string
	AdminAuth       *auth.Authenticator
	UI              http.Handler
}

type Server struct {
	cfg             config.Config
	ver             version.Info
	logger          *slog.Logger
	emby            EmbyReader
	d2              *d2.Service
	d3              *d3.Service
	readiness       *readinessProbe
	mapper          *pathmap.Mapper
	guard           *pathmap.PathGuard
	inventory       *inventory.Service
	authToken       []byte
	authTokenScopes map[string]struct{}
	adminAuth       *auth.Authenticator
	ui              http.Handler
}

// NewServer creates a D1 HTTP server. The optional client keeps the small
// constructor convenient for health-only callers; production always passes an
// authenticated EmbyReader from cmd/server.
func NewServer(cfg config.Config, ver version.Info, logger *slog.Logger, clients ...EmbyReader) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	var client EmbyReader
	if len(clients) > 0 {
		client = clients[0]
	}
	return &Server{cfg: cfg, ver: ver, logger: logger, emby: client, readiness: &readinessProbe{client: client}}
}

// NewServerWithServices constructs the full D1/D2/D3 server.
func NewServerWithServices(cfg config.Config, ver version.Info, logger *slog.Logger, services Services) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	scopes := services.AuthTokenScopes
	if len(scopes) == 0 {
		scopes = config.DefaultAPIAuthScopes()
	}
	authTokenScopes := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		authTokenScopes[scope] = struct{}{}
	}
	return &Server{
		cfg: cfg, ver: ver, logger: logger, emby: services.Emby,
		d2: services.D2, d3: services.D3,
		readiness: &readinessProbe{client: services.Emby}, mapper: services.Mapper,
		guard: services.Guard, inventory: services.Inventory, authToken: []byte(services.AuthToken),
		authTokenScopes: authTokenScopes, adminAuth: services.AdminAuth, ui: services.UI,
	}
}

func (s *Server) Handler() http.Handler {
	return requestMiddleware(s.logger, http.HandlerFunc(s.serveHTTP))
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if (r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/assets/")) && s.ui != nil {
		s.ui.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/v1/auth/login" {
		s.handleLogin(w, r)
		return
	}
	if requiresAuthentication(r.URL.Path) {
		switch s.authorization(r) {
		case authorizationGranted:
			if isWriteOperation(r.URL.Path) && !s.bearerAuthorized(r) {
				if ok, code := s.validWriteSession(r); !ok {
					s.writeError(w, r, http.StatusForbidden, code, "a valid CSRF token and same-origin request are required")
					return
				}
			}
		case authorizationInsufficientScope:
			s.writeInsufficientScope(w, r, requiredAuthScope(r))
			return
		default:
			s.writeUnauthorized(w, r)
			return
		}
	}
	if route, operationID := subtitleOperationRoute(r.URL.Path); route != "" {
		s.handleSubtitleOperations(w, r, route, operationID)
		return
	}
	if d2Operation(r.URL.Path) != "" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			s.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		s.handleD2(w, r, d2Operation(r.URL.Path))
		return
	}
	if d3Operation(r.URL.Path) != "" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			s.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		s.handleD3(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	switch r.URL.Path {
	case "/livez":
		if !noQuery(r) {
			s.writeError(w, r, http.StatusBadRequest, "invalid_query", "query parameters are not allowed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case "/readyz":
		if !noQuery(r) {
			s.writeError(w, r, http.StatusBadRequest, "invalid_query", "query parameters are not allowed")
			return
		}
		if s.readiness != nil && s.readiness.check(r.Context()) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
	case "/v1/health":
		if !noQuery(r) {
			s.writeError(w, r, http.StatusBadRequest, "invalid_query", "query parameters are not allowed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"version":     s.ver,
			"features":    s.cfg.Features,
			"emby_status": s.readinessStatus(),
		})
	case "/v1/emby/libraries":
		if !noQuery(r) {
			s.writeError(w, r, http.StatusBadRequest, "invalid_query", "query parameters are not allowed")
			return
		}
		s.handleLibraries(w, r)
	case "/v1/emby/items":
		s.handleItems(w, r)
	default:
		if strings.HasPrefix(r.URL.Path, "/v1/media/") {
			s.handleMedia(w, r)
			return
		}
		s.writeError(w, r, http.StatusNotFound, "not_found", "not found")
	}
}

func d2Operation(path string) string {
	if !strings.HasPrefix(path, "/v1/media/") {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(path, "/v1/media/"), "/")
	if len(parts) != 3 || parts[1] != "subtitles" {
		return ""
	}
	switch parts[2] {
	case "search", "fetch", "preview", "upload":
		return parts[2]
	default:
		return ""
	}
}

func d3Operation(path string) string {
	if !strings.HasPrefix(path, "/v1/media/") {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(path, "/v1/media/"), "/")
	if len(parts) == 3 && parts[1] == "subtitles" && parts[2] == "add" {
		return "add"
	}
	if len(parts) == 4 && parts[1] == "subtitles" && (parts[3] == "replace" || parts[3] == "delete") {
		return parts[3]
	}
	return ""
}

func subtitleOperationRoute(path string) (string, string) {
	if path == "/v1/subtitle-operations" {
		return "list", ""
	}
	const prefix = "/v1/subtitle-operations/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) == 2 && parts[1] == "restore" && validID(parts[0]) {
		return "restore", parts[0]
	}
	return "", ""
}

func isWriteOperation(path string) bool {
	// Upload only creates a short-lived PreviewArtifact, but it accepts a
	// browser-supplied body. Treat it as CSRF sensitive for administrator
	// sessions just like media write operations.
	if d2Operation(path) == "upload" {
		return true
	}
	if d3Operation(path) != "" {
		return true
	}
	route, _ := subtitleOperationRoute(path)
	return route == "restore"
}

func requiresAuthentication(path string) bool {
	return path == "/v1" || strings.HasPrefix(path, "/v1/")
}

type authorizationResult uint8

const (
	authorizationDenied authorizationResult = iota
	authorizationGranted
	authorizationInsufficientScope
)

func (s *Server) authorization(r *http.Request) authorizationResult {
	if s == nil {
		return authorizationDenied
	}
	if _, exists := r.URL.Query()["token"]; exists {
		return authorizationDenied
	}
	insufficientScope := false
	if len(s.authToken) > 0 {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && subtle.ConstantTimeCompare([]byte(parts[1]), s.authToken) == 1 {
			scope := requiredAuthScope(r)
			if scope != "" {
				if _, granted := s.authTokenScopes[scope]; !granted {
					insufficientScope = true
				} else {
					return authorizationGranted
				}
			} else {
				return authorizationGranted
			}
		}
	}
	if s.adminAuth != nil {
		cookie, err := r.Cookie(adminSessionCookie)
		if err == nil && s.adminAuth.ValidSession(cookie.Value) {
			return authorizationGranted
		}
	}
	if insufficientScope {
		return authorizationInsufficientScope
	}
	return authorizationDenied
}

func (s *Server) bearerAuthorized(r *http.Request) bool {
	if s == nil || r == nil || len(s.authToken) == 0 {
		return false
	}
	if _, exists := r.URL.Query()["token"]; exists {
		return false
	}
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || subtle.ConstantTimeCompare([]byte(parts[1]), s.authToken) != 1 {
		return false
	}
	scope := requiredAuthScope(r)
	if scope == "" {
		return true
	}
	_, ok := s.authTokenScopes[scope]
	return ok
}

func (s *Server) validWriteSession(r *http.Request) (bool, string) {
	if s == nil || s.adminAuth == nil || r == nil {
		return false, "csrf_required"
	}
	cookie, err := r.Cookie(adminSessionCookie)
	if err != nil || !s.adminAuth.ValidSessionCSRF(cookie.Value, r.Header.Get("X-CSRF-Token")) {
		return false, "csrf_required"
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) || (u.Scheme != "" && r.URL.Scheme != "" && !strings.EqualFold(u.Scheme, r.URL.Scheme)) {
			return false, "csrf_origin_invalid"
		}
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site") {
		return false, "csrf_origin_invalid"
	}
	return true, ""
}

func requiredAuthScope(r *http.Request) string {
	if r == nil {
		return ""
	}
	switch d2Operation(r.URL.Path) {
	case "search":
		return config.APIAuthScopeSubtitleSearch
	case "fetch", "preview", "upload":
		return config.APIAuthScopeSubtitlePreview
	case "":
		if d3Operation(r.URL.Path) != "" || func() bool {
			route, _ := subtitleOperationRoute(r.URL.Path)
			return route == "list" || route == "restore"
		}() {
			return config.APIAuthScopeSubtitleWrite
		}
		return config.APIAuthScopeMediaRead
	default:
		return config.APIAuthScopeMediaRead
	}
}

type loginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	if !noQuery(r) {
		s.writeError(w, r, http.StatusBadRequest, "invalid_query", "query parameters are not allowed")
		return
	}
	if s.adminAuth == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, "admin_login_unavailable", "administrator login is not configured")
		return
	}
	var body loginBody
	if err := decodeJSONBody(r, &body, maxD2RequestBody); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid login request")
		return
	}
	if body.Username == "" || body.Password == "" {
		s.writeError(w, r, http.StatusBadRequest, "invalid_request", "username and password are required")
		return
	}
	token, csrfToken, err := s.adminAuth.LoginWithCSRF(remoteClientKey(r), body.Username, body.Password)
	if errors.Is(err, auth.ErrRateLimited) {
		w.Header().Set("Retry-After", "60")
		s.writeError(w, r, http.StatusTooManyRequests, "login_rate_limited", "too many login attempts")
		return
	}
	if errors.Is(err, auth.ErrInvalidCredentials) {
		s.writeError(w, r, http.StatusUnauthorized, "invalid_credentials", "invalid administrator credentials")
		return
	}
	if err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, "session_unavailable", "administrator session is unavailable")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: adminSessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: s.cfg.Security.SessionCookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(s.adminAuth.SessionTTL().Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "csrf_token": csrfToken})
}

func remoteClientKey(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	if len(r.RemoteAddr) <= 128 {
		return r.RemoteAddr
	}
	return "unknown"
}

func (s *Server) writeUnauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	s.writeError(w, r, http.StatusUnauthorized, "unauthorized", "authentication required")
}

func (s *Server) writeInsufficientScope(w http.ResponseWriter, r *http.Request, scope string) {
	if scope == "" {
		scope = config.APIAuthScopeMediaRead
	}
	w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", scope="`+scope+`"`)
	s.writeError(w, r, http.StatusForbidden, "insufficient_scope", "the bearer token lacks the required permission")
}

func (s *Server) handleLibraries(w http.ResponseWriter, r *http.Request) {
	if s.emby == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, "emby_unavailable", "Emby is unavailable")
		return
	}
	libraries, err := s.emby.ListLibraries(r.Context())
	if err != nil {
		s.writeEmbyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, libraries)
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	query, err := parseItemsQuery(r)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid_query", err.Error())
		return
	}
	if s.emby == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, "emby_unavailable", "Emby is unavailable")
		return
	}
	page, err := s.emby.ListItems(r.Context(), query.libraryID, query.startIndex, query.limit)
	if err != nil {
		s.writeEmbyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

type mediaRequest struct {
	itemID    string
	subtitles bool
	sourceID  string
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	req, err := parseMediaRequest(r)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if s.emby == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, "emby_unavailable", "Emby is unavailable")
		return
	}
	item, err := s.emby.GetItem(r.Context(), req.itemID)
	if err != nil {
		s.writeEmbyError(w, r, err)
		return
	}
	ctx, err := media.Build(item, media.BuildOptions{MediaSourceID: req.sourceID, Mapper: s.mapper, Guard: s.guard})
	if err != nil {
		if errors.Is(err, media.ErrMediaSourceSelectionRequired) {
			s.writeSourceSelectionRequired(w, r, item)
			return
		}
		if errors.Is(err, media.ErrMediaSourceNotFound) {
			s.writeError(w, r, http.StatusNotFound, "media_source_not_found", "media source not found")
			return
		}
		s.writeError(w, r, http.StatusBadGateway, "emby_invalid_response", "Emby media response was invalid")
		return
	}
	public := projectMedia(ctx)
	if !req.subtitles {
		writeJSON(w, http.StatusOK, public)
		return
	}
	if s.inventory == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, "inventory_unavailable", "subtitle inventory is unavailable")
		return
	}
	inventoryResult, err := s.inventory.Build(ctx)
	if err != nil {
		code := "inventory_error"
		message := "subtitle inventory failed"
		if errors.Is(err, inventory.ErrConflictingStream) || errors.Is(err, inventory.ErrInvalidStreamIndex) {
			code = "emby_invalid_response"
			message = "Emby media response was invalid"
		}
		s.writeError(w, r, http.StatusBadGateway, code, message)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"media": public, "inventory": inventoryResult})
}

func (s *Server) handleD2(w http.ResponseWriter, r *http.Request, operation string) {
	if r.URL.RawQuery != "" {
		s.writeError(w, r, http.StatusBadRequest, "invalid_request", "query parameters are not allowed")
		return
	}
	if s.d2 == nil || !s.d2.Enabled() {
		s.writeError(w, r, http.StatusForbidden, "remote_search_disabled", "remote subtitle search is disabled")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/media/"), "/")
	if len(parts) != 3 || !validID(parts[0]) || strings.ContainsAny(parts[0], `/\\`) {
		s.writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid item id")
		return
	}
	itemID := parts[0]
	switch operation {
	case "search":
		var body d2SearchBody
		if err := decodeD2JSON(r, &body); err != nil {
			s.writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request")
			return
		}
		response, err := s.d2.Search(r.Context(), itemID, d2.SearchRequest{MediaSourceID: body.MediaSourceID, Language: body.Language, Forced: body.Forced})
		if err != nil {
			s.writeD2Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case "fetch":
		var body d2FetchBody
		if err := decodeD2JSON(r, &body); err != nil || body.CandidateToken == "" {
			s.writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request")
			return
		}
		response, err := s.d2.Fetch(r.Context(), itemID, d2.FetchRequest{CandidateToken: body.CandidateToken})
		if err != nil {
			s.writeD2Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case "preview":
		var body d2PreviewBody
		if err := decodeD2JSON(r, &body); err != nil || body.ArtifactToken == "" {
			s.writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request")
			return
		}
		response, err := s.d2.Preview(r.Context(), itemID, d2.PreviewRequest{ArtifactToken: body.ArtifactToken, Offset: body.Offset, Limit: body.Limit})
		if err != nil {
			s.writeD2Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case "upload":
		body, err := decodeUpload(r)
		if err != nil {
			s.writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid subtitle upload")
			return
		}
		response, err := s.d2.Upload(r.Context(), itemID, d2.UploadRequest{MediaSourceID: body.MediaSourceID, Language: body.Language, Content: body.Content})
		if err != nil {
			s.writeD2Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	}
}

type d3AddBody struct {
	ArtifactToken string `json:"artifact_token"`
	MediaSourceID string `json:"media_source_id"`
	OperationID   string `json:"operation_id"`
}

type d3ReplaceBody struct {
	ArtifactToken string `json:"artifact_token"`
	MediaSourceID string `json:"media_source_id"`
	OperationID   string `json:"operation_id"`
}

type d3DeleteBody struct {
	MediaSourceID string `json:"media_source_id"`
	OperationID   string `json:"operation_id"`
}

func (s *Server) handleD3(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		s.writeError(w, r, http.StatusBadRequest, "invalid_request", "query parameters are not allowed")
		return
	}
	if s.d3 == nil || !s.d3.Enabled() {
		s.writeError(w, r, http.StatusForbidden, "write_disabled", "D3 Add is disabled")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/media/"), "/")
	operation := d3Operation(r.URL.Path)
	if (operation == "add" && len(parts) != 3) || ((operation == "replace" || operation == "delete") && len(parts) != 4) || !validID(parts[0]) || strings.ContainsAny(parts[0], `/\\`) {
		s.writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid item id")
		return
	}
	var response any
	var err error
	switch operation {
	case "add":
		var body d3AddBody
		if err = decodeJSONBody(r, &body, maxD2RequestBody); err == nil {
			response, err = s.d3.Add(r.Context(), parts[0], d3.AddRequest{ArtifactToken: body.ArtifactToken, MediaSourceID: body.MediaSourceID, OperationID: body.OperationID})
		}
	case "replace":
		var body d3ReplaceBody
		if err = decodeJSONBody(r, &body, maxD2RequestBody); err == nil {
			response, err = s.d3.Replace(r.Context(), parts[0], parts[2], d3.ReplaceRequest{ArtifactToken: body.ArtifactToken, MediaSourceID: body.MediaSourceID, OperationID: body.OperationID})
		}
	case "delete":
		var body d3DeleteBody
		if err = decodeJSONBody(r, &body, maxD2RequestBody); err == nil {
			response, err = s.d3.Delete(r.Context(), parts[0], parts[2], d3.DeleteRequest{MediaSourceID: body.MediaSourceID, OperationID: body.OperationID})
		}
	}
	if err != nil {
		var d3Err *d3.Error
		if errors.As(err, &d3Err) && d3Err != nil {
			s.writeError(w, r, d3Err.Status, d3Err.Code, d3Err.Message)
			return
		}
		s.writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type restoreBody struct {
	MediaSourceID string `json:"media_source_id"`
	OperationID   string `json:"operation_id"`
}

func subtitleOperationQuery(query url.Values) (string, int, bool) {
	itemValues, ok := query["item_id"]
	if !ok || len(itemValues) != 1 || !validID(itemValues[0]) {
		return "", 0, false
	}
	limit := defaultHistoryLimit
	for key, values := range query {
		switch key {
		case "item_id":
			if len(values) != 1 {
				return "", 0, false
			}
		case "limit":
			if len(values) != 1 {
				return "", 0, false
			}
			parsed, err := strconv.Atoi(values[0])
			if err != nil || parsed < 1 || parsed > maxHistoryLimit {
				return "", 0, false
			}
			limit = parsed
		default:
			return "", 0, false
		}
	}
	return itemValues[0], limit, true
}

func (s *Server) handleSubtitleOperations(w http.ResponseWriter, r *http.Request, route, sourceOperationID string) {
	if s.d3 == nil || !s.d3.Enabled() {
		s.writeError(w, r, http.StatusForbidden, "write_disabled", "subtitle operations are disabled")
		return
	}
	switch route {
	case "list":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			s.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		itemID, limit, valid := subtitleOperationQuery(r.URL.Query())
		if !valid {
			s.writeError(w, r, http.StatusBadRequest, "invalid_query", "item_id is required")
			return
		}
		response, err := s.d3.ListOperations(itemID, limit)
		if err != nil {
			s.writeD3Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"operations": response})
	case "restore":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			s.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		if r.URL.RawQuery != "" {
			s.writeError(w, r, http.StatusBadRequest, "invalid_request", "query parameters are not allowed")
			return
		}
		var body restoreBody
		if err := decodeJSONBody(r, &body, maxD2RequestBody); err != nil {
			s.writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request")
			return
		}
		response, err := s.d3.Restore(r.Context(), sourceOperationID, d3.RestoreRequest{MediaSourceID: body.MediaSourceID, OperationID: body.OperationID})
		if err != nil {
			s.writeD3Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	default:
		s.writeError(w, r, http.StatusNotFound, "not_found", "not found")
	}
}

func (s *Server) writeD3Error(w http.ResponseWriter, r *http.Request, err error) {
	var d3Err *d3.Error
	if errors.As(err, &d3Err) && d3Err != nil {
		s.writeError(w, r, d3Err.Status, d3Err.Code, d3Err.Message)
		return
	}
	s.writeError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
}

type d2SearchBody struct {
	MediaSourceID string `json:"media_source_id"`
	Language      string `json:"language"`
	Forced        bool   `json:"forced"`
}

type d2FetchBody struct {
	CandidateToken string `json:"candidate_token"`
}

type d2PreviewBody struct {
	ArtifactToken string `json:"artifact_token"`
	Offset        int    `json:"offset"`
	Limit         int    `json:"limit"`
}

type uploadBody struct {
	MediaSourceID string
	Language      string
	Content       []byte
}

// decodeUpload accepts exactly one subtitle body and the two declared text
// fields. It deliberately ignores the browser filename and MIME type, then
// lets the service validator determine the supported subtitle format.
func decodeUpload(r *http.Request) (uploadBody, error) {
	if r == nil {
		return uploadBody{}, errors.New("missing request")
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return uploadBody{}, errors.New("multipart content type is required")
	}
	r.Body = http.MaxBytesReader(nil, r.Body, maxUploadRequestBody)
	reader, err := r.MultipartReader()
	if err != nil {
		return uploadBody{}, err
	}
	result := uploadBody{}
	seen := make(map[string]bool, 3)
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return uploadBody{}, nextErr
		}
		name := part.FormName()
		if name != "file" && name != "media_source_id" && name != "language" || seen[name] {
			_ = part.Close()
			return uploadBody{}, errors.New("unknown or duplicate multipart field")
		}
		seen[name] = true
		limit := int64(maxUploadFieldBytes)
		if name == "file" {
			limit = 4 << 20
		}
		value, readErr := io.ReadAll(io.LimitReader(part, limit+1))
		closeErr := part.Close()
		if readErr != nil || closeErr != nil || int64(len(value)) > limit {
			return uploadBody{}, errors.New("multipart field is too large")
		}
		switch name {
		case "file":
			result.Content = value
		case "media_source_id":
			result.MediaSourceID = string(value)
		case "language":
			result.Language = string(value)
		}
	}
	if !seen["file"] || !seen["media_source_id"] || !seen["language"] || len(result.Content) == 0 {
		return uploadBody{}, errors.New("required multipart field is missing")
	}
	return result, nil
}

func decodeD2JSON(r *http.Request, target any) error {
	return decodeJSONBody(r, target, maxD2RequestBody)
}

func decodeJSONBody(r *http.Request, target any, maxBytes int64) error {
	contentType := strings.TrimSpace(strings.ToLower(r.Header.Get("Content-Type")))
	if contentType != "" && !strings.HasPrefix(contentType, "application/json") {
		return errors.New("JSON content type is required")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil || int64(len(body)) > maxBytes {
		return errors.New("request body too large")
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return errors.New("request must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values are not allowed")
	}
	return nil
}

func (s *Server) writeD2Error(w http.ResponseWriter, r *http.Request, err error) {
	var d2Err *d2.Error
	if !errors.As(err, &d2Err) || d2Err == nil {
		s.writeError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if d2Err.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(d2Err.RetryAfter))
	}
	s.writeError(w, r, d2Err.Status, d2Err.Code, d2Err.Message)
}

func parseMediaRequest(r *http.Request) (mediaRequest, error) {
	const prefix = "/v1/media/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return mediaRequest{}, errors.New("invalid media path")
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) != 1 && len(parts) != 2 {
		return mediaRequest{}, errors.New("invalid media path")
	}
	itemID := parts[0]
	if !validID(itemID) || strings.ContainsAny(itemID, `/\\`) {
		return mediaRequest{}, errors.New("invalid item id")
	}
	subtitles := len(parts) == 2 && parts[1] == "subtitles"
	if len(parts) == 2 && !subtitles {
		return mediaRequest{}, errors.New("invalid media path")
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return mediaRequest{}, errors.New("invalid query")
	}
	for key, values := range query {
		if key != "media_source_id" {
			return mediaRequest{}, errors.New("unknown query parameter")
		}
		if len(values) != 1 || !validID(values[0]) {
			return mediaRequest{}, errors.New("invalid media_source_id")
		}
	}
	return mediaRequest{itemID: itemID, subtitles: subtitles, sourceID: query.Get("media_source_id")}, nil
}

// MediaDTO is the only public projection of MediaContext. It intentionally
// contains neither the Emby-reported path nor stream paths.
type MediaDTO struct {
	ItemID            string              `json:"item_id"`
	MediaSourceID     string              `json:"media_source_id"`
	MediaSourceName   string              `json:"media_source_name,omitempty"`
	Type              string              `json:"type"`
	Title             string              `json:"title"`
	SeriesID          string              `json:"series_id,omitempty"`
	SeriesName        string              `json:"series_name,omitempty"`
	Season            *int                `json:"season,omitempty"`
	Episode           *int                `json:"episode,omitempty"`
	Year              *int                `json:"year,omitempty"`
	ProviderIDs       map[string]string   `json:"provider_ids,omitempty"`
	Container         string              `json:"container,omitempty"`
	IsSTRM            bool                `json:"is_strm"`
	MappingStatus     media.MappingStatus `json:"mapping_status"`
	Warnings          []string            `json:"warnings,omitempty"`
	InventoryComplete bool                `json:"inventory_complete"`
}

func projectMedia(ctx media.MediaContext) MediaDTO {
	providers := make(map[string]string, 3)
	for key, value := range ctx.ProviderIDs {
		switch strings.ToLower(strings.ReplaceAll(key, "-", "")) {
		case "imdb":
			providers["imdb"] = value
		case "tmdb":
			providers["tmdb"] = value
		case "tvdb":
			providers["tvdb"] = value
		}
	}
	if len(providers) == 0 {
		providers = nil
	}
	return MediaDTO{ItemID: ctx.ItemID, MediaSourceID: ctx.MediaSourceID, MediaSourceName: ctx.MediaSourceName, Type: ctx.Type, Title: ctx.Title,
		SeriesID: ctx.SeriesID, SeriesName: ctx.SeriesName, Season: ctx.ParentIndexNumber, Episode: ctx.IndexNumber,
		Year: ctx.ProductionYear, ProviderIDs: providers, Container: ctx.Container, IsSTRM: ctx.IsStrm,
		MappingStatus: ctx.MappingStatus, Warnings: append([]string(nil), ctx.Warnings...), InventoryComplete: ctx.InventoryComplete}
}

type sourceOptionDTO struct {
	ID        string `json:"media_source_id"`
	Name      string `json:"name,omitempty"`
	Container string `json:"container,omitempty"`
	Default   bool   `json:"is_default"`
}

func (s *Server) writeSourceSelectionRequired(w http.ResponseWriter, r *http.Request, item domain.EmbyItem) {
	options := make([]sourceOptionDTO, 0, len(item.MediaSources))
	for _, source := range item.MediaSources {
		options = append(options, sourceOptionDTO{ID: source.ID, Name: source.Name, Container: source.Container, Default: source.IsDefault != nil && *source.IsDefault})
	}
	requestID, _ := r.Context().Value(requestIDKey{}).(string)
	writeJSON(w, http.StatusConflict, struct {
		Error         errorBody         `json:"error"`
		RequestID     string            `json:"request_id"`
		SourceOptions []sourceOptionDTO `json:"media_sources"`
	}{Error: errorBody{Code: "media_source_required", Message: "media source selection is required"}, RequestID: requestID, SourceOptions: options})
}

type itemsQuery struct {
	libraryID  string
	startIndex int
	limit      int
}

func parseItemsQuery(r *http.Request) (itemsQuery, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return itemsQuery{}, errors.New("invalid query")
	}
	for key, values := range q {
		if key != "library_id" && key != "start_index" && key != "limit" {
			return itemsQuery{}, errors.New("unknown query parameter")
		}
		if len(values) != 1 || values[0] == "" {
			return itemsQuery{}, errors.New("duplicate or empty query parameter")
		}
	}
	libraryID := q.Get("library_id")
	if !validID(libraryID) {
		return itemsQuery{}, errors.New("invalid library_id")
	}
	startIndex := defaultStartIndex
	if raw := q.Get("start_index"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return itemsQuery{}, errors.New("invalid start_index")
		}
		startIndex = parsed
	}
	limit := defaultLimit
	if raw := q.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxHTTPItemsLimit {
			return itemsQuery{}, errors.New("invalid limit")
		}
		limit = parsed
	}
	return itemsQuery{libraryID: libraryID, startIndex: startIndex, limit: limit}, nil
}

func validID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func noQuery(r *http.Request) bool {
	return r.URL.RawQuery == ""
}

func (s *Server) readinessStatus() string {
	if s.readiness == nil {
		return "unknown"
	}
	return s.readiness.status()
}

func (s *Server) writeEmbyError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := mapEmbyError(err)
	s.writeError(w, r, status, code, message)
}

func mapEmbyError(err error) (int, string, string) {
	var clientErr *embyclient.Error
	if !errors.As(err, &clientErr) {
		return http.StatusBadGateway, "emby_error", "Emby request failed"
	}
	switch clientErr.Code() {
	case embyclient.ErrInvalidInput:
		return http.StatusBadRequest, "invalid_request", "invalid request"
	case embyclient.ErrNotFound:
		return http.StatusNotFound, "not_found", "item not found"
	case embyclient.ErrTimeout:
		return http.StatusGatewayTimeout, "emby_timeout", "Emby request timed out"
	case embyclient.ErrHTTP:
		switch {
		case clientErr.StatusCode() == http.StatusNotFound:
			return http.StatusNotFound, "emby_not_found", "Emby resource not found"
		case clientErr.StatusCode() >= 500:
			return http.StatusBadGateway, "emby_upstream_error", "Emby returned an upstream error"
		default:
			return http.StatusServiceUnavailable, "emby_unavailable", "Emby is unavailable"
		}
	case embyclient.ErrTransport:
		return http.StatusServiceUnavailable, "emby_unavailable", "Emby is unavailable"
	case embyclient.ErrCanceled:
		return http.StatusServiceUnavailable, "emby_unavailable", "Emby is unavailable"
	case embyclient.ErrMalformedJSON, embyclient.ErrInvalidResponse, embyclient.ErrResponseTooLarge, embyclient.ErrRedirect:
		return http.StatusBadGateway, "emby_invalid_response", "Emby response was invalid"
	default:
		return http.StatusBadGateway, "emby_error", "Emby request failed"
	}
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error     errorBody `json:"error"`
	RequestID string    `json:"request_id"`
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := r.Context().Value(requestIDKey{})
	id, _ := requestID.(string)
	if id == "" {
		id = w.Header().Get("X-Request-ID")
	}
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}, RequestID: id})
}

type requestIDKey struct{}

func requestMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID))
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logger.Info("http request", "request_id", requestID, "method", r.Method, "route", routeLabel(r.URL.Path), "status", recorder.status)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return "unavailable"
}

func routeLabel(path string) string {
	switch path {
	case "/", "/livez", "/readyz", "/v1/health", "/v1/auth/login", "/v1/emby/libraries", "/v1/emby/items", "/v1/subtitle-operations":
		return path
	}
	if strings.HasPrefix(path, "/assets/") {
		return "/assets/{asset}"
	}
	if strings.HasPrefix(path, "/v1/media/") {
		parts := strings.Split(strings.TrimPrefix(path, "/v1/media/"), "/")
		if len(parts) == 1 {
			return "/v1/media/{itemId}"
		}
		if len(parts) == 2 && parts[1] == "subtitles" {
			return "/v1/media/{itemId}/subtitles"
		}
		if len(parts) == 3 && parts[1] == "subtitles" {
			switch parts[2] {
			case "search", "fetch", "preview", "upload", "add":
				return "/v1/media/{itemId}/subtitles/" + parts[2]
			}
		}
		if len(parts) == 4 && parts[1] == "subtitles" && (parts[3] == "replace" || parts[3] == "delete") {
			return "/v1/media/{itemId}/subtitles/{subtitleId}/" + parts[3]
		}
	}
	if strings.HasPrefix(path, "/v1/subtitle-operations/") && strings.HasSuffix(path, "/restore") {
		parts := strings.Split(strings.TrimPrefix(path, "/v1/subtitle-operations/"), "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] == "restore" {
			return "/v1/subtitle-operations/{operationId}/restore"
		}
	}
	return "<unmatched>"
}

type readinessProbe struct {
	client     EmbyReader
	mu         sync.Mutex
	validUntil time.Time
	ready      bool
	inFlight   chan struct{}
}

func (p *readinessProbe) check(parent context.Context) bool {
	if p == nil || p.client == nil {
		return false
	}
	for {
		now := time.Now()
		p.mu.Lock()
		if now.Before(p.validUntil) {
			ready := p.ready
			p.mu.Unlock()
			return ready
		}
		if p.inFlight != nil {
			wait := p.inFlight
			p.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-parent.Done():
				return false
			}
		}
		p.inFlight = make(chan struct{})
		flight := p.inFlight
		p.mu.Unlock()

		ctx, cancel := context.WithTimeout(parent, readinessProbeTimeout)
		_, err := p.client.ListLibraries(ctx)
		cancel()
		ready := err == nil
		p.mu.Lock()
		p.ready = ready
		if ready {
			p.validUntil = time.Now().Add(readinessSuccessTTL)
		} else {
			p.validUntil = time.Now().Add(readinessFailureTTL)
		}
		close(flight)
		p.inFlight = nil
		p.mu.Unlock()
		return ready
	}
}

func (p *readinessProbe) status() string {
	if p == nil {
		return "unknown"
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Now().After(p.validUntil) {
		return "unknown"
	}
	if p.ready {
		return "ready"
	}
	return "unavailable"
}
