package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hope140/emby-strm-subtitle-manager/internal/config"
	"github.com/hope140/emby-strm-subtitle-manager/internal/version"
)

func testServer(t *testing.T, logs *bytes.Buffer) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	cfg := config.Config{Features: config.FeatureConfig{WriteEnabled: false, RemoteSearchEnabled: false}}
	return NewServer(cfg, version.Info{Version: "test", Commit: "abc", BuildTime: "now"}, logger).Handler()
}

func TestHealthEndpointsAndHeaders(t *testing.T) {
	var logs bytes.Buffer
	handler := testServer(t, &logs)
	for _, path := range []string{"/livez", "/readyz", "/v1/health", "/v1/version"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		if rec.Header().Get("X-Request-ID") == "" || rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Content-Security-Policy") != "default-src 'none'" {
			t.Fatalf("%s missing security/request headers: %v", path, rec.Header())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("%s invalid JSON: %v", path, err)
		}
	}
	if strings.Contains(logs.String(), "api") || strings.Contains(logs.String(), "secret") {
		t.Fatalf("logs contain unexpected credential-like values: %s", logs.String())
	}
}

func TestMethodRestrictionAndUnknownRoute(t *testing.T) {
	handler := testServer(t, new(bytes.Buffer))
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(method, "/v1/health", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("%s status=%d allow=%q", method, rec.Code, rec.Header().Get("Allow"))
		}
	}
	const sensitiveItemID = "secret-item-id"
	req := httptest.NewRequest(http.MethodGet, "/v1/media/"+sensitiveItemID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d", rec.Code)
	}
	if logs := new(bytes.Buffer); func() bool {
		handler := testServer(t, logs)
		req := httptest.NewRequest(http.MethodGet, "/v1/media/"+sensitiveItemID, nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)
		return strings.Contains(logs.String(), sensitiveItemID) || strings.Contains(logs.String(), "/v1/media") || !strings.Contains(logs.String(), "<unmatched>")
	}() {
		t.Fatalf("unknown route log leaked path: %s", logs.String())
	}
}

func TestStatusRecorderKeepsFirstStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}
	w.WriteHeader(http.StatusNotFound)
	w.WriteHeader(http.StatusInternalServerError)
	if w.status != http.StatusNotFound || rec.Code != http.StatusNotFound {
		t.Fatalf("status recorder = %d/HTTP %d, want 404/404", w.status, rec.Code)
	}
}
