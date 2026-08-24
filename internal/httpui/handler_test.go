package httpui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRootAndAssets(t *testing.T) {
	handler := NewHandler()

	root := request(handler, http.MethodGet, "/")
	if root.Code != http.StatusOK || root.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("root = %d %q", root.Code, root.Header().Get("Content-Type"))
	}
	if !strings.Contains(root.Body.String(), "D1.5") || !strings.Contains(root.Body.String(), "/assets/app.js") {
		t.Fatal("root did not serve the embedded index")
	}
	if got := root.Header().Get("Content-Security-Policy"); got != uiCSP {
		t.Fatalf("root CSP = %q", got)
	}

	for _, test := range []struct {
		path        string
		contentType string
	}{
		{path: "/assets/app.js", contentType: "javascript"},
		{path: "/assets/app.css", contentType: "text/css"},
		{path: "/assets/subtitle-steward.svg", contentType: "image/svg+xml"},
	} {
		rec := request(handler, http.MethodGet, test.path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), test.contentType) {
			t.Errorf("%s = %d %q", test.path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}

func TestUnknownAndTraversalPathsAreNotServed(t *testing.T) {
	handler := NewHandler()
	for _, target := range []string{
		"/missing",
		"/assets/",
		"/assets/missing.js",
		"/assets/../app.js",
		"/assets/%2e%2e/app.js",
		"/assets/..\\app.js",
		"/?token=not-a-token",
		"/assets/app.js?token=not-a-token",
	} {
		want := http.StatusNotFound
		if strings.Contains(target, "?") {
			want = http.StatusBadRequest
		}
		if rec := request(handler, http.MethodGet, target); rec.Code != want {
			t.Errorf("%s status = %d, want %d", target, rec.Code, want)
		}
	}
}

func TestUIHasNoExternalResourcesOrPersistentTokenStorage(t *testing.T) {
	for _, name := range []string{"assets/index.html", "assets/app.js", "assets/app.css"} {
		body, err := assets.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		content := string(body)
		for _, forbidden := range []string{"http://", "https://", "localStorage", "sessionStorage", "document.cookie", "innerHTML"} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s contains forbidden %q", name, forbidden)
			}
		}
	}
}

func request(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}
