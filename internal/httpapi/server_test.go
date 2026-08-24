package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hope140/emby-strm-subtitle-manager/internal/config"
	"github.com/hope140/emby-strm-subtitle-manager/internal/domain"
	"github.com/hope140/emby-strm-subtitle-manager/internal/embyclient"
	"github.com/hope140/emby-strm-subtitle-manager/internal/inventory"
	"github.com/hope140/emby-strm-subtitle-manager/internal/pathmap"
	"github.com/hope140/emby-strm-subtitle-manager/internal/version"
)

type fakeEmby struct {
	libraries []domain.Library
	page      domain.ItemPage
	listErr   error
	itemErr   error
	item      domain.EmbyItem
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

func (f *fakeEmby) GetItem(context.Context, string) (domain.EmbyItem, error) {
	return f.item, f.itemErr
}

func testServer(t *testing.T, fake EmbyReader, logs *bytes.Buffer) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	cfg := config.Config{Features: config.FeatureConfig{WriteEnabled: false, RemoteSearchEnabled: false}}
	return NewServerWithServices(cfg, version.Info{Version: "test", Commit: "abc", BuildTime: "now"}, logger, Services{Emby: fake, AuthToken: testAuthToken}).Handler()
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
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"media_source_id":"source-b"`) || !strings.Contains(rec.Body.String(), `"is_strm":true`) {
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
