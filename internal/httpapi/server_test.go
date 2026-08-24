package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hope140/emby-strm-subtitle-manager/internal/config"
	"github.com/hope140/emby-strm-subtitle-manager/internal/domain"
	"github.com/hope140/emby-strm-subtitle-manager/internal/embyclient"
	"github.com/hope140/emby-strm-subtitle-manager/internal/version"
)

type fakeEmby struct {
	libraries []domain.Library
	page      domain.ItemPage
	listErr   error
	itemErr   error
	listCalls atomic.Int32
	itemCalls atomic.Int32
	block     <-chan struct{}
	entered   chan<- struct{}
}

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

func testServer(t *testing.T, fake EmbyReader, logs *bytes.Buffer) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	cfg := config.Config{Features: config.FeatureConfig{WriteEnabled: false, RemoteSearchEnabled: false}}
	return NewServer(cfg, version.Info{Version: "test", Commit: "abc", BuildTime: "now"}, logger, fake).Handler()
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
	if rec := serve(handler, http.MethodGet, "/v1/media/secret-item"); rec.Code != 404 {
		t.Fatalf("unknown route status = %d", rec.Code)
	}
}

func serve(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
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
