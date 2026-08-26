package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hope140/subbridge/internal/auth"
	"github.com/hope140/subbridge/internal/config"
	"github.com/hope140/subbridge/internal/domain"
	"github.com/hope140/subbridge/internal/embyclient"
	"github.com/hope140/subbridge/internal/httpui"
	"github.com/hope140/subbridge/internal/inventory"
	"github.com/hope140/subbridge/internal/pathmap"
	"github.com/hope140/subbridge/internal/version"
)

type fakeEmby struct {
	libraries []domain.Library
	page      domain.ItemPage
	browse    domain.BrowsePage
	listErr   error
	itemErr   error
	item      domain.EmbyItem
	browseQ   domain.BrowseQuery
	listCalls atomic.Int32
	itemCalls atomic.Int32
	block     <-chan struct{}
	entered   chan<- struct{}
}

const testAuthToken = "test-api-auth-token-01234567890123456789"

func (f *fakeEmby) ListLibraries(ctx context.Context) ([]domain.Library, error) {
	f.listCalls.Add(1)
	if f.entered != nil {
		select {
		case f.entered <- struct{}{}:
		default:
		}
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, &embyclient.Error{Kind: embyclient.ErrTimeout}
		}
	}
	return f.libraries, f.listErr
}

func (f *fakeEmby) ListItems(context.Context, string, int, int) (domain.ItemPage, error) {
	f.itemCalls.Add(1)
	return f.page, f.itemErr
}

func (f *fakeEmby) ListBrowseNodes(_ context.Context, query domain.BrowseQuery) (domain.BrowsePage, error) {
	f.itemCalls.Add(1)
	f.browseQ = query
	return f.browse, f.itemErr
}

func (f *fakeEmby) GetItem(context.Context, string) (domain.EmbyItem, error) {
	return f.item, f.itemErr
}

func testServer(t *testing.T, fake EmbyReader, logs *bytes.Buffer) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	cfg := config.Config{Features: config.FeatureConfig{WriteEnabled: false, RemoteSearchEnabled: false}}
	return NewServerWithServices(cfg, version.Info{Version: "test", Commit: "abc", BuildTime: "now"}, logger, Services{Emby: fake, AuthToken: testAuthToken, UI: httpui.NewHandler()}).Handler()
}

func TestUIUsesSharedMiddlewareAndOnlyServesGET(t *testing.T) {
	var logs bytes.Buffer
	handler := testServer(t, &fakeEmby{}, &logs)
	rec := serve(handler, http.MethodGet, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("root status = %d", rec.Code)
	}
	if rec.Header().Get("X-Request-ID") == "" || rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("root did not use shared request middleware")
	}
	wantCSP := "default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; object-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'"
	if rec.Header().Get("Content-Security-Policy") != wantCSP {
		t.Fatalf("root CSP = %q", rec.Header().Get("Content-Security-Policy"))
	}
	if rec := serve(handler, http.MethodGet, "/assets/app.js"); rec.Code != http.StatusOK {
		t.Fatalf("asset status = %d", rec.Code)
	}
	if rec := serve(handler, http.MethodPost, "/"); rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST root = %d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
	if rec := serve(handler, http.MethodGet, "/unknown"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d", rec.Code)
	}
	if !strings.Contains(logs.String(), `"route":"/"`) || strings.Contains(logs.String(), "/assets/app.js") {
		t.Fatalf("UI logs did not use coarse route labels: %s", logs.String())
	}
}

func TestLiveHealthAndVersionRouteRemoval(t *testing.T) {
	fake := &fakeEmby{libraries: []domain.Library{{ID: "lib-1", Name: "Movies"}}}
	var logs bytes.Buffer
	handler := testServer(t, fake, &logs)
	for _, test := range []struct {
		path string
		code int
	}{{"/livez", 200}, {"/v1/health", 200}, {"/v1/version", 404}} {
		rec := serve(handler, http.MethodGet, test.path)
		if rec.Code != test.code {
			t.Fatalf("%s status = %d, want %d", test.path, rec.Code, test.code)
		}
		if rec.Header().Get("X-Request-ID") == "" || rec.Header().Get("Content-Security-Policy") != "default-src 'none'" {
			t.Fatalf("%s missing security/request headers", test.path)
		}
	}
	if fake.listCalls.Load() != 0 {
		t.Fatal("live/health unexpectedly made an Emby request")
	}
	var health map[string]any
	if err := json.Unmarshal(serve(handler, http.MethodGet, "/v1/health").Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health["emby_status"] != "unknown" {
		t.Fatalf("health emby_status = %#v, want unknown", health["emby_status"])
	}
	if strings.Contains(logs.String(), "lib-1") || strings.Contains(logs.String(), "secret") {
		t.Fatalf("logs contain sensitive values: %s", logs.String())
	}
}

func TestLibrariesAndItemsBrowsing(t *testing.T) {
	fake := &fakeEmby{libraries: []domain.Library{{ID: "lib-1", Name: "Movies", CollectionType: "movies"}}, page: domain.ItemPage{Items: []domain.ItemSummary{{ID: "movie-1", Name: "Movie", Type: "Movie"}}, TotalRecordCount: 1, StartIndex: 0, Limit: 50}}
	handler := testServer(t, fake, new(bytes.Buffer))
	rec := serve(handler, http.MethodGet, "/v1/emby/libraries")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"id":"lib-1"`) {
		t.Fatalf("libraries response = %d %s", rec.Code, rec.Body.String())
	}
	rec = serve(handler, http.MethodGet, "/v1/emby/items?library_id=lib-1&start_index=0&limit=50")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"movie-1"`) || fake.itemCalls.Load() != 1 {
		t.Fatalf("items response = %d %s, calls=%d", rec.Code, rec.Body.String(), fake.itemCalls.Load())
	}
}

func TestItemsQueryValidation(t *testing.T) {
	fake := &fakeEmby{}
	handler := testServer(t, fake, new(bytes.Buffer))
	for _, path := range []string{"/v1/emby/items", "/v1/emby/items?library_id=lib-1&library_id=lib-2", "/v1/emby/items?library_id=lib-1&unknown=x", "/v1/emby/items?library_id=lib-1&start_index=-1", "/v1/emby/items?library_id=lib-1&limit=0", "/v1/emby/items?library_id=lib-1&limit=201", "/v1/emby/items?library_id=%20lib-1", "/v1/emby/items?library_id=lib-1&limit=wat"} {
		rec := serve(handler, http.MethodGet, path)
		if rec.Code != 400 {
			t.Errorf("%s status = %d, want 400", path, rec.Code)
		}
		assertErrorEnvelope(t, rec, "invalid_query")
	}
	if rec := serve(handler, http.MethodGet, "/v1/emby/items?library_id=%zz"); rec.Code != 400 {
		t.Fatalf("malformed query status = %d", rec.Code)
	}
	if fake.itemCalls.Load() != 0 {
		t.Fatal("invalid query reached Emby")
	}
}

func TestBrowseAndMediaSourcesAreSafeReadOnlyProjections(t *testing.T) {
	defaultSource := true
	fake := &fakeEmby{
		browse: domain.BrowsePage{Items: []domain.BrowseNode{{ID: "series-1", Name: "Series", Type: "Series"}}, TotalRecordCount: 1, Limit: 50},
		item:   domain.EmbyItem{ItemSummary: domain.ItemSummary{ID: "episode-1", Name: "Episode", Type: "Episode"}, Path: `C:\\private\\episode.strm`, ProviderIDs: map[string]string{"Imdb": "private-provider-id"}, MediaSources: []domain.MediaSource{{ID: "source-1", Name: "Version", Container: "mkv", Path: "https://private.example.invalid/episode?token=secret", IsDefault: &defaultSource}}},
	}
	handler := testServer(t, fake, new(bytes.Buffer))
	rec := serve(handler, http.MethodGet, "/v1/emby/browse?library_id=lib-1&level=root&limit=50")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"series-1"`) {
		t.Fatalf("browse response = %d %s", rec.Code, rec.Body.String())
	}
	if fake.browseQ.LibraryID != "lib-1" || fake.browseQ.Level != domain.BrowseLevelRoot || fake.browseQ.ParentID != "" || fake.browseQ.Limit != 50 {
		t.Fatalf("browse query = %#v", fake.browseQ)
	}
	for _, path := range []string{
		"/v1/emby/browse?library_id=lib-1&level=root&parent_id=unexpected",
		"/v1/emby/browse?library_id=lib-1&level=series",
		"/v1/emby/browse?library_id=lib-1&level=season&parent_id=season-1&unknown=x",
		"/v1/emby/browse?library_id=lib-1&level=invalid",
	} {
		rec := serve(handler, http.MethodGet, path)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", path, rec.Code)
		}
	}
	rec = serve(handler, http.MethodGet, "/v1/media/episode-1/sources")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"media_source_id":"source-1"`) {
		t.Fatalf("sources response = %d %s", rec.Code, rec.Body.String())
	}
	for _, forbidden := range []string{"C:\\private", "private.example.invalid", "private-provider-id", "secret"} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Fatalf("sources response leaked %q: %s", forbidden, rec.Body.String())
		}
	}
	if rec := serve(handler, http.MethodGet, "/v1/media/episode-1/sources?media_source_id=source-1"); rec.Code != http.StatusBadRequest {
		t.Fatalf("sources query status = %d, want 400", rec.Code)
	}
}

func TestReadinessEmptySuccessCacheAndHealthStatus(t *testing.T) {
	fake := &fakeEmby{}
	handler := testServer(t, fake, new(bytes.Buffer))
	if rec := serve(handler, http.MethodGet, "/readyz"); rec.Code != 200 || rec.Body.String() != "{\"status\":\"ready\"}\n" {
		t.Fatalf("empty-library readiness = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(handler, http.MethodGet, "/readyz"); rec.Code != 200 {
		t.Fatalf("cached readiness = %d", rec.Code)
	}
	if fake.listCalls.Load() != 1 {
		t.Fatalf("readiness calls = %d, want one cached probe", fake.listCalls.Load())
	}
	var health map[string]any
	if err := json.Unmarshal(serve(handler, http.MethodGet, "/v1/health").Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health["emby_status"] != "ready" {
		t.Fatalf("health status = %#v, want ready", health["emby_status"])
	}
}

func TestReadinessConcurrentRequestsFoldIntoOneProbe(t *testing.T) {
	gate := make(chan struct{})
	entered := make(chan struct{}, 4)
	fake := &fakeEmby{block: gate, entered: entered}
	handler := testServer(t, fake, new(bytes.Buffer))
	const callers = 4
	results := make(chan int, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() { defer wg.Done(); results <- serve(handler, http.MethodGet, "/readyz").Code }()
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("readiness probe did not start")
	}
	close(gate)
	wg.Wait()
	close(results)
	for code := range results {
		if code != 200 {
			t.Fatalf("concurrent readiness status = %d", code)
		}
	}
	if fake.listCalls.Load() != 1 {
		t.Fatalf("concurrent probe calls = %d, want one", fake.listCalls.Load())
	}
}

func TestReadinessFailureAndHTTPErrorMapping(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		code int
	}{{"unauthorized", &embyclient.Error{Kind: embyclient.ErrHTTP, Status: 401}, 503}, {"timeout", &embyclient.Error{Kind: embyclient.ErrTimeout}, 504}, {"not found", &embyclient.Error{Kind: embyclient.ErrNotFound}, 404}} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeEmby{listErr: test.err, itemErr: test.err}
			handler := testServer(t, fake, new(bytes.Buffer))
			rec := serve(handler, http.MethodGet, "/readyz")
			if rec.Code != 503 || rec.Body.String() != "{\"status\":\"not_ready\"}\n" {
				t.Fatalf("ready response = %d %s", rec.Code, rec.Body.String())
			}
			if cached := serve(handler, http.MethodGet, "/readyz"); cached.Code != 503 || fake.listCalls.Load() != 1 {
				t.Fatalf("failed readiness was not cached: status=%d calls=%d", cached.Code, fake.listCalls.Load())
			}
			rec = serve(handler, http.MethodGet, "/v1/emby/items?library_id=lib-1")
			if rec.Code != test.code {
				t.Fatalf("mapped status = %d, want %d", rec.Code, test.code)
			}
			assertErrorEnvelope(t, rec, "")
		})
	}
}

func TestErrorsContainRequestIDAndDoNotLeakDetails(t *testing.T) {
	const secret = "token-secret-private-path"
	fake := &fakeEmby{itemErr: errors.New(secret)}
	var logs bytes.Buffer
	handler := testServer(t, fake, &logs)
	rec := serve(handler, http.MethodGet, "/v1/emby/items?library_id=lib-secret")
	if rec.Code != 502 {
		t.Fatalf("status = %d", rec.Code)
	}
	assertErrorEnvelope(t, rec, "emby_error")
	if !strings.Contains(rec.Body.String(), rec.Header().Get("X-Request-ID")) || strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), "lib-secret") {
		t.Fatalf("unsafe response = %s", rec.Body.String())
	}
	if strings.Contains(logs.String(), secret) || strings.Contains(logs.String(), "lib-secret") {
		t.Fatalf("unsafe logs = %s", logs.String())
	}
}

func TestMethodRestrictionUnknownRouteAndQueryRejection(t *testing.T) {
	handler := testServer(t, &fakeEmby{}, new(bytes.Buffer))
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := serve(handler, method, "/v1/health")
		if rec.Code != 405 || rec.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("%s status=%d allow=%q", method, rec.Code, rec.Header().Get("Allow"))
		}
		assertErrorEnvelope(t, rec, "method_not_allowed")
	}
	if rec := serve(handler, http.MethodGet, "/v1/health?x=1"); rec.Code != 400 {
		t.Fatalf("unexpected health query status = %d", rec.Code)
	}
	if rec := serve(handler, http.MethodGet, "/v1/other/secret-item"); rec.Code != 404 {
		t.Fatalf("unknown route status = %d", rec.Code)
	}
}

func TestBearerAuthenticationProtectsV1WithoutQueryFallback(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := NewServerWithServices(config.Config{}, version.Info{Version: "test"}, logger, Services{Emby: &fakeEmby{}, AuthToken: testAuthToken}).Handler()
	for _, test := range []struct {
		name          string
		path          string
		authorization string
		wantStatus    int
	}{
		{name: "missing", path: "/v1/health", wantStatus: http.StatusUnauthorized},
		{name: "wrong", path: "/v1/health", authorization: "Bearer wrong-token", wantStatus: http.StatusUnauthorized},
		{name: "query token", path: "/v1/health?token=" + testAuthToken, wantStatus: http.StatusUnauthorized},
		{name: "ready public", path: "/readyz", wantStatus: http.StatusOK},
		{name: "correct", path: "/v1/health", authorization: "Bearer " + testAuthToken, wantStatus: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := serveWithAuthorization(handler, http.MethodGet, test.path, test.authorization)
			if rec.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, test.wantStatus, rec.Body.String())
			}
			if test.wantStatus == http.StatusUnauthorized {
				if rec.Header().Get("WWW-Authenticate") != "Bearer" {
					t.Fatalf("WWW-Authenticate = %q", rec.Header().Get("WWW-Authenticate"))
				}
				assertErrorEnvelope(t, rec, "unauthorized")
				if strings.Contains(rec.Body.String(), testAuthToken) {
					t.Fatal("authentication token leaked in response")
				}
			}
		})
	}
	if rec := serveWithAuthorization(handler, http.MethodGet, "/livez", ""); rec.Code != http.StatusOK {
		t.Fatalf("livez status = %d, want 200", rec.Code)
	}
	if strings.Contains(logs.String(), testAuthToken) {
		t.Fatal("authentication token leaked in logs")
	}
}

func TestBearerScopesSeparateMediaReadFromSubtitleOperations(t *testing.T) {
	handler := NewServerWithServices(config.Config{}, version.Info{Version: "test"}, slog.Default(), Services{
		Emby: &fakeEmby{}, AuthToken: testAuthToken, AuthTokenScopes: []string{config.APIAuthScopeMediaRead},
	}).Handler()
	if rec := serveWithAuthorization(handler, http.MethodGet, "/v1/health", "Bearer "+testAuthToken); rec.Code != http.StatusOK {
		t.Fatalf("media read status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{
		"/v1/media/movie-1/subtitles/search",
		"/v1/media/movie-1/subtitles/fetch",
		"/v1/media/movie-1/subtitles/preview",
		"/v1/media/movie-1/subtitles/upload",
		"/v1/media/movie-1/subtitles/add",
		"/v1/media/movie-1/subtitles/sub_v1_scope-test/replace",
		"/v1/media/movie-1/subtitles/sub_v1_scope-test/delete",
		"/v1/subtitle-operations/operation-scope-test/restore",
	} {
		t.Run(path, func(t *testing.T) {
			rec := serveWithAuthorization(handler, http.MethodPost, path, "Bearer "+testAuthToken)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
			assertErrorEnvelope(t, rec, "insufficient_scope")
			if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "insufficient_scope") {
				t.Fatalf("WWW-Authenticate = %q", rec.Header().Get("WWW-Authenticate"))
			}
		})
	}
	if rec := serveWithAuthorization(handler, http.MethodGet, "/v1/subtitle-operations?item_id=movie-1", "Bearer "+testAuthToken); rec.Code != http.StatusForbidden {
		t.Fatalf("history scope status = %d body=%s", rec.Code, rec.Body.String())
	} else {
		assertErrorEnvelope(t, rec, "insufficient_scope")
	}
}

func TestAdminPasswordLoginIssuesHttpOnlySessionAndKeepsBearerAutomation(t *testing.T) {
	admin, err := auth.New("operator", "correct horse battery staple", auth.Options{SessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	handler := NewServerWithServices(config.Config{}, version.Info{Version: "test"}, slog.New(slog.NewJSONHandler(&logs, nil)), Services{Emby: &fakeEmby{}, AuthToken: testAuthToken, AdminAuth: admin}).Handler()

	wrong := loginRequest(handler, `{"username":"operator","password":"wrong password"}`)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong login status = %d body=%s", wrong.Code, wrong.Body.String())
	}
	assertErrorEnvelope(t, wrong, "invalid_credentials")
	if strings.Contains(wrong.Body.String(), "wrong password") || strings.Contains(logs.String(), "wrong password") {
		t.Fatal("administrator password leaked")
	}

	login := loginRequest(handler, `{"username":"operator","password":"correct horse battery staple"}`)
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), `"status":"ok"`) || !strings.Contains(login.Body.String(), `"csrf_token":"`) {
		t.Fatalf("login = %d %s", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != adminSessionCookie || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Secure || cookies[0].Value == "" {
		t.Fatalf("unexpected session cookie: %#v", cookies)
	}
	if strings.Contains(login.Body.String(), cookies[0].Value) || strings.Contains(logs.String(), cookies[0].Value) {
		t.Fatal("session token leaked")
	}

	withCookie := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	withCookie.AddCookie(cookies[0])
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, withCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("session health status = %d body=%s", response.Code, response.Body.String())
	}
	withCookieQuery := httptest.NewRequest(http.MethodGet, "/v1/health?token="+testAuthToken, nil)
	withCookieQuery.AddCookie(cookies[0])
	queryResponse := httptest.NewRecorder()
	handler.ServeHTTP(queryResponse, withCookieQuery)
	if queryResponse.Code != http.StatusUnauthorized {
		t.Fatalf("session plus query token status = %d, want 401", queryResponse.Code)
	}
	if rec := serve(handler, http.MethodGet, "/v1/health"); rec.Code != http.StatusOK {
		t.Fatalf("bearer automation status = %d", rec.Code)
	}
	if rec := serveWithAuthorization(handler, http.MethodGet, "/v1/health?token="+testAuthToken, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("query bearer status = %d", rec.Code)
	}
}

func TestAdminLoginRouteIsPublicOnlyForPOSTAndNoQuery(t *testing.T) {
	admin, err := auth.New("operator", "correct horse battery staple", auth.Options{})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServerWithServices(config.Config{}, version.Info{Version: "test"}, slog.Default(), Services{AuthToken: testAuthToken, AdminAuth: admin}).Handler()
	for _, test := range []struct {
		method string
		target string
		code   int
		error  string
	}{
		{method: http.MethodGet, target: "/v1/auth/login", code: http.StatusMethodNotAllowed, error: "method_not_allowed"},
		{method: http.MethodPost, target: "/v1/auth/login?next=/", code: http.StatusBadRequest, error: "invalid_query"},
		{method: http.MethodPost, target: "/v1/auth/login", code: http.StatusBadRequest, error: "invalid_request"},
	} {
		rec := requestWithBody(handler, test.method, test.target, "application/json", `{"username":"operator"}`)
		if rec.Code != test.code {
			t.Fatalf("%s %s status = %d, want %d", test.method, test.target, rec.Code, test.code)
		}
		assertErrorEnvelope(t, rec, test.error)
	}
}

func TestAdminLoginHonorsSecureCookieConfiguration(t *testing.T) {
	admin, err := auth.New("operator", "correct horse battery staple", auth.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Security: config.SecurityConfig{SessionCookieSecure: true}}
	handler := NewServerWithServices(cfg, version.Info{Version: "test"}, slog.Default(), Services{AdminAuth: admin}).Handler()
	rec := loginRequest(handler, `{"username":"operator","password":"correct horse battery staple"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("secure login status = %d body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("secure session cookie = %#v", cookies)
	}
}

func TestD3AddRequiresWriteScopeAndSessionCSRF(t *testing.T) {
	admin, err := auth.New("operator", "correct horse battery staple", auth.Options{SessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServerWithServices(config.Config{Features: config.FeatureConfig{WriteEnabled: true}}, version.Info{Version: "test"}, slog.Default(), Services{
		AuthToken: testAuthToken, AuthTokenScopes: []string{config.APIAuthScopeMediaRead, config.APIAuthScopeSubtitleWrite}, AdminAuth: admin,
	}).Handler()
	login := loginRequest(handler, `{"username":"operator","password":"correct horse battery staple"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &loginBody); err != nil || loginBody.CSRFToken == "" {
		t.Fatalf("login csrf = %q err=%v", loginBody.CSRFToken, err)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("login did not issue one session cookie")
	}
	withoutCSRF := httptest.NewRequest(http.MethodPost, "/v1/media/movie-1/subtitles/add", strings.NewReader(`{"artifact_token":"opaque","media_source_id":"source-1","operation_id":"operation-1"}`))
	withoutCSRF.Header.Set("Content-Type", "application/json")
	withoutCSRF.AddCookie(cookies[0])
	withoutCSRFResponse := httptest.NewRecorder()
	handler.ServeHTTP(withoutCSRFResponse, withoutCSRF)
	if withoutCSRFResponse.Code != http.StatusForbidden {
		t.Fatalf("without CSRF = %d %s", withoutCSRFResponse.Code, withoutCSRFResponse.Body.String())
	}
	assertErrorEnvelope(t, withoutCSRFResponse, "csrf_required")
	withCSRF := httptest.NewRequest(http.MethodPost, "/v1/media/movie-1/subtitles/add", strings.NewReader(`{"artifact_token":"opaque","media_source_id":"source-1","operation_id":"operation-1"}`))
	withCSRF.Header.Set("Content-Type", "application/json")
	withCSRF.Header.Set("X-CSRF-Token", loginBody.CSRFToken)
	withCSRF.AddCookie(cookies[0])
	withCSRFResponse := httptest.NewRecorder()
	handler.ServeHTTP(withCSRFResponse, withCSRF)
	if withCSRFResponse.Code != http.StatusForbidden {
		t.Fatalf("with CSRF should reach disabled D3 = %d %s", withCSRFResponse.Code, withCSRFResponse.Body.String())
	}
	assertErrorEnvelope(t, withCSRFResponse, "write_disabled")
	bearerRequest := httptest.NewRequest(http.MethodPost, "/v1/media/movie-1/subtitles/add", strings.NewReader(`{"artifact_token":"opaque","media_source_id":"source-1","operation_id":"operation-2"}`))
	bearerRequest.Header.Set("Content-Type", "application/json")
	bearerRequest.Header.Set("Authorization", "Bearer "+testAuthToken)
	bearerRecorder := httptest.NewRecorder()
	handler.ServeHTTP(bearerRecorder, bearerRequest)
	if bearerRecorder.Code != http.StatusForbidden {
		t.Fatalf("bearer D3 = %d %s", bearerRecorder.Code, bearerRecorder.Body.String())
	}
	assertErrorEnvelope(t, bearerRecorder, "write_disabled")
}

func TestUploadRequiresSessionCSRF(t *testing.T) {
	admin, err := auth.New("operator", "correct horse battery staple", auth.Options{SessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServerWithServices(config.Config{Features: config.FeatureConfig{RemoteSearchEnabled: true}}, version.Info{Version: "test"}, slog.Default(), Services{AdminAuth: admin}).Handler()
	login := loginRequest(handler, `{"username":"operator","password":"correct horse battery staple"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &loginBody); err != nil || loginBody.CSRFToken == "" {
		t.Fatalf("login csrf = %q err=%v", loginBody.CSRFToken, err)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("login did not issue one session cookie")
	}
	withoutCSRF := httptest.NewRequest(http.MethodPost, "/v1/media/movie-1/subtitles/upload", strings.NewReader("--invalid"))
	withoutCSRF.Header.Set("Content-Type", "multipart/form-data; boundary=invalid")
	withoutCSRF.AddCookie(cookies[0])
	withoutCSRFResponse := httptest.NewRecorder()
	handler.ServeHTTP(withoutCSRFResponse, withoutCSRF)
	if withoutCSRFResponse.Code != http.StatusForbidden {
		t.Fatalf("upload without CSRF = %d %s", withoutCSRFResponse.Code, withoutCSRFResponse.Body.String())
	}
	assertErrorEnvelope(t, withoutCSRFResponse, "csrf_required")

	withCSRF := httptest.NewRequest(http.MethodPost, "/v1/media/movie-1/subtitles/upload", strings.NewReader("--invalid"))
	withCSRF.Header.Set("Content-Type", "multipart/form-data; boundary=invalid")
	withCSRF.Header.Set("X-CSRF-Token", loginBody.CSRFToken)
	withCSRF.AddCookie(cookies[0])
	withCSRFResponse := httptest.NewRecorder()
	handler.ServeHTTP(withCSRFResponse, withCSRF)
	if withCSRFResponse.Code == http.StatusForbidden && strings.Contains(withCSRFResponse.Body.String(), `"csrf_required"`) {
		t.Fatalf("upload with CSRF was rejected at CSRF boundary: %d %s", withCSRFResponse.Code, withCSRFResponse.Body.String())
	}
}

func loginRequest(handler http.Handler, body string) *httptest.ResponseRecorder {
	return requestWithBody(handler, http.MethodPost, "/v1/auth/login", "application/json", body)
}

func requestWithBody(handler http.Handler, method, target, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestMediaMovieProjectionAndSubtitleInventory(t *testing.T) {
	root, err := os.MkdirTemp(".", "httpapi-media-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Movie.zh.srt"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: `C:\emby\media`, Local: root}})
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	inventoryService, err := inventory.New(inventory.Options{FileSystem: inventory.OSFileSystem{}, IdentityKey: key, Mapper: mapper})
	if err != nil {
		t.Fatal(err)
	}
	emptyStreams := []domain.MediaStream{}
	fake := &fakeEmby{item: domain.EmbyItem{
		ItemSummary: domain.ItemSummary{ID: "movie-1", Name: "Movie", Type: "Movie", ProductionYear: intPtr(2024)},
		Path:        `C:\emby\media\Movie.strm`, ProviderIDs: map[string]string{"Imdb": "tt123", "Tmdb": "456", "Tvdb": "789", "private": "do-not-show"},
		MediaSources: []domain.MediaSource{{ID: "source-1", Name: "Main", Container: "mkv", Protocol: "Http", Path: `https://media.example.invalid/Movie.mkv?opaque=private`, MediaStreams: &emptyStreams}},
	}}
	server := NewServerWithServices(config.Config{}, version.Info{Version: "test"}, slog.Default(), Services{Emby: fake, Mapper: mapper, Inventory: inventoryService, AuthToken: testAuthToken})
	handler := server.Handler()
	rec := serve(handler, http.MethodGet, "/v1/media/movie-1")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), `C:\emby\media`) || strings.Contains(rec.Body.String(), "private") {
		t.Fatalf("unsafe media response = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"is_strm":true`) || !strings.Contains(rec.Body.String(), `"imdb":"tt123"`) || strings.Contains(rec.Body.String(), `"private"`) {
		t.Fatalf("media projection = %s", rec.Body.String())
	}
	rec = serve(handler, http.MethodGet, "/v1/media/movie-1/subtitles")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"inventory"`) || !strings.Contains(rec.Body.String(), "Movie.zh.srt") {
		t.Fatalf("subtitle response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestMediaMultipleSourcesRequireExplicitSafeSelection(t *testing.T) {
	first, second := []domain.MediaStream{}, []domain.MediaStream{}
	fake := &fakeEmby{item: domain.EmbyItem{ItemSummary: domain.ItemSummary{ID: "episode-1", Name: "Episode", Type: "Episode"}, Path: `/media/episode.strm`, MediaSources: []domain.MediaSource{
		{ID: "source-a", Name: "A", Container: "mkv", Protocol: "Http", Path: "https://media.example.invalid/a.mkv?opaque=private", MediaStreams: &first},
		{ID: "source-b", Name: "B", Container: "mkv", Protocol: "Http", Path: "https://media.example.invalid/b.mkv?opaque=private", MediaStreams: &second},
	}}}
	handler := NewServerWithServices(config.Config{}, version.Info{Version: "test"}, slog.Default(), Services{Emby: fake, AuthToken: testAuthToken}).Handler()
	rec := serve(handler, http.MethodGet, "/v1/media/episode-1")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"media_sources"`) || !strings.Contains(rec.Body.String(), `"media_source_id":"source-a"`) || !strings.Contains(rec.Body.String(), `"media_source_id":"source-b"`) || strings.Contains(rec.Body.String(), `"source_options"`) || strings.Contains(rec.Body.String(), "/secret") {
		t.Fatalf("source selection response = %d %s", rec.Code, rec.Body.String())
	}
	rec = serve(handler, http.MethodGet, "/v1/media/episode-1?media_source_id=source-b")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"media_source_id":"source-b"`) || !strings.Contains(rec.Body.String(), `"is_strm":true`) || !strings.Contains(rec.Body.String(), `"reason_code":"strm_multisource_write_unsupported"`) || strings.Contains(rec.Body.String(), "/secret") || strings.Contains(rec.Body.String(), "media.example.invalid") {
		t.Fatalf("selected source response = %d %s", rec.Code, rec.Body.String())
	}
	rec = serve(handler, http.MethodGet, "/v1/media/episode-1?media_source_id=missing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing source status = %d", rec.Code)
	}
}

func TestMediaUnmappedDegradesSafelyAndRejectsBadQueriesMethods(t *testing.T) {
	empty := []domain.MediaStream{}
	fake := &fakeEmby{item: domain.EmbyItem{ItemSummary: domain.ItemSummary{ID: "movie-2", Name: "Movie", Type: "Movie"}, Path: "/unmapped/Movie.strm", MediaSources: []domain.MediaSource{{ID: "source-1", Path: "https://media.example.invalid/Movie.mkv?opaque=private", MediaStreams: &empty}}}}
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: "/emby/media", Local: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServerWithServices(config.Config{}, version.Info{Version: "test"}, slog.Default(), Services{Emby: fake, Mapper: mapper, AuthToken: testAuthToken}).Handler()
	rec := serve(handler, http.MethodGet, "/v1/media/movie-2")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"mapping_status":"unmapped"`) || strings.Contains(rec.Body.String(), "/unmapped") {
		t.Fatalf("unmapped response = %d %s", rec.Code, rec.Body.String())
	}
	for _, target := range []string{"/v1/media/movie-2?unknown=x", "/v1/media/movie-2?media_source_id=source-1&media_source_id=source-1", "/v1/media/movie-2?media_source_id="} {
		if rec := serve(handler, http.MethodGet, target); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad media query %s status = %d", target, rec.Code)
		}
	}
	if rec := serve(handler, http.MethodPost, "/v1/media/movie-2"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("media POST status = %d", rec.Code)
	}
}

func TestMediaRejectsContradictoryUpstreamSources(t *testing.T) {
	for name, sources := range map[string][]domain.MediaSource{
		"empty":     {},
		"empty-id":  {{ID: "", Path: "/private/movie.strm"}},
		"duplicate": {{ID: "same", Path: "/private/a.strm"}, {ID: "same", Path: "/private/b.strm"}},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeEmby{item: domain.EmbyItem{
				ItemSummary:  domain.ItemSummary{ID: "movie-invalid", Name: "Movie", Type: "Movie"},
				MediaSources: sources,
			}}
			handler := NewServerWithServices(config.Config{}, version.Info{Version: "test"}, slog.Default(), Services{Emby: fake, AuthToken: testAuthToken}).Handler()
			rec := serve(handler, http.MethodGet, "/v1/media/movie-invalid")
			if rec.Code != http.StatusBadGateway {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
			assertErrorEnvelope(t, rec, "emby_invalid_response")
			if strings.Contains(rec.Body.String(), "/private/") {
				t.Fatalf("response leaked a source path: %s", rec.Body.String())
			}
		})
	}
}

func TestSubtitleOperationQueryUsesBoundedLimit(t *testing.T) {
	for raw, wantID := range map[string]struct {
		id     string
		source string
		limit  int
		ok     bool
	}{
		"item_id=movie-1":                          {"movie-1", "", defaultHistoryLimit, true},
		"item_id=movie-1&media_source_id=source-a": {"movie-1", "source-a", defaultHistoryLimit, true},
		"item_id=movie-1&limit=3":                  {"movie-1", "", 3, true},
		"item_id=movie-1&limit=101":                {"", "", 0, false},
		"item_id=movie-1&limit=zero":               {"", "", 0, false},
		"item_id=movie-1&media_source_id=":         {"", "", 0, false},
		"item_id=movie-1&extra=x":                  {"", "", 0, false},
	} {
		query, err := url.ParseQuery(raw)
		if err != nil {
			t.Fatal(err)
		}
		gotID, gotSource, gotLimit, gotOK := subtitleOperationQuery(query)
		if gotID != wantID.id || gotSource != wantID.source || gotLimit != wantID.limit || gotOK != wantID.ok {
			t.Fatalf("query %q = (%q, %q, %d, %t), want (%q, %q, %d, %t)", raw, gotID, gotSource, gotLimit, gotOK, wantID.id, wantID.source, wantID.limit, wantID.ok)
		}
	}
}

func TestRouteLabelCoversCoreABOperations(t *testing.T) {
	for path, want := range map[string]string{
		"/v1/media/movie-1/subtitles/upload":                     "/v1/media/{itemId}/subtitles/upload",
		"/v1/media/movie-1/subtitles/sub_v1_example/replace":     "/v1/media/{itemId}/subtitles/{subtitleId}/replace",
		"/v1/media/movie-1/subtitles/sub_v1_example/delete":      "/v1/media/{itemId}/subtitles/{subtitleId}/delete",
		"/v1/subtitle-operations":                                "/v1/subtitle-operations",
		"/v1/subtitle-operations/operation-example-0001/restore": "/v1/subtitle-operations/{operationId}/restore",
	} {
		if got := routeLabel(path); got != want {
			t.Fatalf("routeLabel(%q) = %q, want %q", path, got, want)
		}
	}
}

func intPtr(value int) *int { return &value }

func serve(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", "Bearer "+testAuthToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func serveWithAuthorization(handler http.Handler, method, target, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func assertErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder, code string) {
	t.Helper()
	var envelope errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid error JSON: %v; body=%s", err, rec.Body.String())
	}
	if envelope.RequestID == "" || envelope.RequestID != rec.Header().Get("X-Request-ID") {
		t.Fatalf("request id mismatch: %#v header=%q", envelope, rec.Header().Get("X-Request-ID"))
	}
	if code != "" && envelope.Error.Code != code {
		t.Fatalf("error code = %q, want %q", envelope.Error.Code, code)
	}
}
