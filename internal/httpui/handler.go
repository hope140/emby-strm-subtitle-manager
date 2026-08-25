// Package httpui serves the embedded Core A/B browser UI and its gated subtitle controls.
package httpui

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

const uiCSP = "default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; object-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'"

// NewHandler returns the embedded UI handler. It deliberately has no API or
// filesystem dependencies; all data comes from same-origin relative requests.
func NewHandler() http.Handler {
	return http.HandlerFunc(serve)
}

func serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", uiCSP)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.RawQuery != "" {
		http.Error(w, "query parameters are not allowed", http.StatusBadRequest)
		return
	}

	var (
		name string
		body []byte
	)
	switch {
	case r.URL.Path == "/":
		name = "assets/index.html"
	case strings.HasPrefix(r.URL.Path, "/assets/"):
		assetName := strings.TrimPrefix(r.URL.Path, "/assets/")
		if !safeAssetPath(assetName) {
			http.NotFound(w, r)
			return
		}
		name = "assets/" + assetName
	default:
		http.NotFound(w, r)
		return
	}

	var err error
	body, err = fs.ReadFile(assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", contentType(name))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func safeAssetPath(value string) bool {
	if value == "" || strings.Contains(value, `\`) || !fs.ValidPath(value) {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != "." && !strings.HasPrefix(clean, "../")
}

func contentType(name string) string {
	typeByExtension := mime.TypeByExtension(path.Ext(name))
	if typeByExtension != "" {
		if strings.Contains(typeByExtension, "charset=") {
			return typeByExtension
		}
		if strings.HasPrefix(typeByExtension, "text/") || strings.HasSuffix(typeByExtension, "+javascript") {
			return typeByExtension + "; charset=utf-8"
		}
		return typeByExtension
	}
	return "application/octet-stream"
}
