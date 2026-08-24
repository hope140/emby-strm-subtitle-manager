// Package httpapi exposes the intentionally small, read-only D1 HTTP surface.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hope140/emby-strm-subtitle-manager/internal/config"
	"github.com/hope140/emby-strm-subtitle-manager/internal/domain"
	"github.com/hope140/emby-strm-subtitle-manager/internal/embyclient"
	"github.com/hope140/emby-strm-subtitle-manager/internal/inventory"
	"github.com/hope140/emby-strm-subtitle-manager/internal/media"
	"github.com/hope140/emby-strm-subtitle-manager/internal/pathmap"
	"github.com/hope140/emby-strm-subtitle-manager/internal/version"
)

const (
	readinessProbeTimeout = 3 * time.Second
	readinessSuccessTTL   = 15 * time.Second
	readinessFailureTTL   = 5 * time.Second
	defaultStartIndex     = 0
	defaultLimit          = 50
	maxHTTPItemsLimit     = 200
)

// EmbyReader is the smallest interface needed by the HTTP layer. Keeping the
// interface here allows a later Media/Inventory handler to use the same
// client while keeping HTTP tests independent from a live Emby instance.
type EmbyReader interface {
	ListLibraries(context.Context) ([]domain.Library, error)
	ListItems(context.Context, string, int, int) (domain.ItemPage, error)
	GetItem(context.Context, string) (domain.EmbyItem, error)
}

// Services contains the dependencies needed by the D1 media slice. The
// fields are interfaces/pointers so HTTP tests can inject a fake Emby client
// without creating a filesystem or exposing server paths.
type Services struct {
	Emby      EmbyReader
	Mapper    *pathmap.Mapper
	Guard     *pathmap.PathGuard
	Inventory *inventory.Service
}

type Server struct {
	cfg       config.Config
	ver       version.Info
	logger    *slog.Logger
	emby      EmbyReader
	readiness *readinessProbe
	mapper    *pathmap.Mapper
	guard     *pathmap.PathGuard
	inventory *inventory.Service
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

// NewServerWithServices constructs the full D1 read-only server.
func NewServerWithServices(cfg config.Config, ver version.Info, logger *slog.Logger, services Services) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg: cfg, ver: ver, logger: logger, emby: services.Emby,
		readiness: &readinessProbe{client: services.Emby}, mapper: services.Mapper,
		guard: services.Guard, inventory: services.Inventory,
	}
}

func (s *Server) Handler() http.Handler {
	return requestMiddleware(s.logger, http.HandlerFunc(s.serveHTTP))
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
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
	return MediaDTO{ItemID: ctx.ItemID, MediaSourceID: ctx.MediaSourceID, Type: ctx.Type, Title: ctx.Title,
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
	case "/livez", "/readyz", "/v1/health", "/v1/emby/libraries", "/v1/emby/items":
		return path
	}
	if strings.HasPrefix(path, "/v1/media/") {
		parts := strings.Split(strings.TrimPrefix(path, "/v1/media/"), "/")
		if len(parts) == 1 {
			return "/v1/media/{itemId}"
		}
		if len(parts) == 2 && parts[1] == "subtitles" {
			return "/v1/media/{itemId}/subtitles"
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
