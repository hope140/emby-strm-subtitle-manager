// Package httpapi exposes the intentionally small, read-only D1 HTTP surface.
package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/hope140/emby-strm-subtitle-manager/internal/config"
	"github.com/hope140/emby-strm-subtitle-manager/internal/version"
)

type Server struct {
	cfg    config.Config
	ver    version.Info
	logger *slog.Logger
}

func NewServer(cfg config.Config, ver version.Info, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{cfg: cfg, ver: ver, logger: logger}
}

func (s *Server) Handler() http.Handler {
	return requestMiddleware(s.logger, http.HandlerFunc(s.serveHTTP))
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	allowed := r.Method == http.MethodGet
	if !allowed {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	switch r.URL.Path {
	case "/livez":
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case "/readyz":
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	case "/v1/health":
		writeJSON(w, http.StatusOK, map[string]any{
			"status":   "ok",
			"version":  s.ver,
			"features": s.cfg.Features,
		})
	case "/v1/version":
		writeJSON(w, http.StatusOK, s.ver)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func requestMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
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
	case "/livez", "/readyz", "/v1/health", "/v1/version":
		return path
	default:
		return "<unmatched>"
	}
}
