package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hope140/emby-strm-subtitle-manager/internal/config"
	"github.com/hope140/emby-strm-subtitle-manager/internal/d2"
	"github.com/hope140/emby-strm-subtitle-manager/internal/embyclient"
	"github.com/hope140/emby-strm-subtitle-manager/internal/preview"
	"github.com/hope140/emby-strm-subtitle-manager/internal/subtitleprovider"
	"github.com/hope140/emby-strm-subtitle-manager/internal/version"
)

type d2UpstreamRequest struct {
	Method string
	Path   string
	Query  url.Values
	Token  string
}

type d2FakeEmby struct {
	testing.TB
	mu          sync.Mutex
	requests    []d2UpstreamRequest
	multiSource bool
	mediaPath   string
}

func (f *d2FakeEmby) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, d2UpstreamRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Token: r.Header.Get("X-Emby-Token")})
	f.mu.Unlock()
	if strings.Contains(strings.ToLower(r.URL.Path), "refresh") || strings.Contains(strings.ToLower(r.URL.Path), "save") || r.Method != http.MethodGet {
		f.Errorf("forbidden D2 upstream endpoint: %s %s", r.Method, r.URL.Path)
		http.Error(w, "forbidden", http.StatusNotFound)
		return
	}
	switch r.URL.Path {
	case "/Items":
		f.writeItem(w)
	case "/Items/movie-1/RemoteSearch/Subtitles/zh-CN":
		if got := r.URL.Query(); got.Get("MediaSourceId") != "source-1" || got.Get("IsForced") != "true" || got.Get("IsPerfectMatch") != "false" || got.Get("IsHearingImpaired") != "false" {
			f.Errorf("unsafe Search query = %#v", got)
		}
		candidates := make([]map[string]any, 0, 21)
		candidates = append(candidates,
			map[string]any{"Id": "candidate-a", "ProviderName": "Thunder", "Name": "A", "Language": "zho", "Format": "srt"},
			map[string]any{"Id": "candidate-b", "ProviderName": "ASSRT", "Name": "B", "Language": "zho", "Format": "srt", "IsHashMatch": true},
		)
		for i := 0; i < 19; i++ {
			candidates = append(candidates, map[string]any{"Id": "candidate-extra-" + string(rune('a'+i)), "ProviderName": "Provider", "Name": "Extra", "Language": "zho", "Format": "srt"})
		}
		writeD2JSON(w, candidates)
	case "/Providers/Subtitles/Subtitles/candidate-a":
		http.Error(w, "upstream candidate failure", http.StatusInternalServerError)
	case "/Providers/Subtitles/Subtitles/candidate-b":
		_, _ = io.WriteString(w, "1\n00:00:01,000 --> 00:00:02,000\n预览内容\n")
	default:
		http.NotFound(w, r)
	}
}

func (f *d2FakeEmby) writeItem(w http.ResponseWriter) {
	sources := []map[string]any{{"Id": "source-1", "Name": "Main", "Container": "mkv"}}
	if f.multiSource {
		sources = append(sources, map[string]any{"Id": "source-2", "Name": "Other", "Container": "mkv"})
	}
	item := map[string]any{"Id": "movie-1", "Name": "Movie", "Type": "Movie", "MediaSources": sources}
	if f.mediaPath != "" {
		item["Path"] = f.mediaPath
		sources[0]["Path"] = f.mediaPath
	}
	writeD2JSON(w, map[string]any{"Items": []map[string]any{item}})
}

func (f *d2FakeEmby) snapshot() []d2UpstreamRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]d2UpstreamRequest, len(f.requests))
	copy(result, f.requests)
	return result
}

func writeD2JSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func newD2HTTPTestServer(t *testing.T, fake *d2FakeEmby, enabled bool) (http.Handler, *httptest.Server, *bytes.Buffer) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(fake.handler))
	client, err := embyclient.New(embyclient.Config{BaseURL: upstream.URL, APIKey: "fake-emby-api-key", HTTPClient: upstream.Client()})
	if err != nil {
		upstream.Close()
		t.Fatal(err)
	}
	var d2Service *d2.Service
	if enabled {
		cache := t.TempDir()
		var err error
		d2Service, err = d2.New(d2.Options{Config: config.D2Config{}, RemoteSearchEnabled: true, CanaryEnabled: true, Allowlist: preview.NewAllowlist([]string{"movie-1"}), Emby: client, Provider: subtitleprovider.NewEmbyRemoteSubtitleProvider(client), ArtifactStore: mustArtifactStore(t, cache), AuthContext: d2.AuthContextFromToken(testAuthToken)})
		if err != nil {
			upstream.Close()
			t.Fatal(err)
		}
	}
	logs := new(bytes.Buffer)
	app := NewServerWithServices(config.Config{Features: config.FeatureConfig{RemoteSearchEnabled: enabled}}, version.Info{Version: "d2-test"}, slog.New(slog.NewJSONHandler(logs, nil)), Services{Emby: client, D2: d2Service, AuthToken: testAuthToken}).Handler()
	return app, upstream, logs
}

func mustArtifactStore(t *testing.T, directory string) *preview.ArtifactStore {
	t.Helper()
	store, err := preview.NewArtifactStore(preview.ArtifactStoreOptions{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func d2POST(t *testing.T, handler http.Handler, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testAuthToken)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestD2HTTPFakeEmbyReadOnlyFlowAndDisabledZeroUpstream(t *testing.T) {
	fakeDisabled := &d2FakeEmby{TB: t}
	disabled, upstreamDisabled, _ := newD2HTTPTestServer(t, fakeDisabled, false)
	defer upstreamDisabled.Close()
	for _, test := range []struct {
		path string
		body string
	}{
		{"/v1/media/movie-1/subtitles/search", `{"language":"zh-CN"}`},
		{"/v1/media/movie-1/subtitles/fetch", `{"candidate_token":"opaque"}`},
		{"/v1/media/movie-1/subtitles/preview", `{"artifact_token":"opaque"}`},
	} {
		response := d2POST(t, disabled, test.path, test.body)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"remote_search_disabled"`) {
			t.Fatalf("disabled %s = %d %s", test.path, response.Code, response.Body.String())
		}
	}
	if requests := fakeDisabled.snapshot(); len(requests) != 0 {
		t.Fatalf("disabled D2 made upstream requests: %#v", requests)
	}

	mediaDir := t.TempDir()
	fake := &d2FakeEmby{TB: t, mediaPath: filepath.Join(mediaDir, "movie.strm")}
	handler, upstream, logs := newD2HTTPTestServer(t, fake, true)
	defer upstream.Close()
	search := d2POST(t, handler, "/v1/media/movie-1/subtitles/search", `{"language":"zho","forced":true}`)
	if search.Code != http.StatusOK {
		t.Fatalf("Search = %d %s", search.Code, search.Body.String())
	}
	var searchBody struct {
		Language   string `json:"language"`
		Candidates []struct {
			Token    string `json:"token"`
			Language string `json:"language"`
		} `json:"candidates"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &searchBody); err != nil {
		t.Fatal(err)
	}
	if len(searchBody.Candidates) != 20 || !searchBody.Truncated {
		t.Fatalf("candidate cap = %d truncated=%v", len(searchBody.Candidates), searchBody.Truncated)
	}
	if searchBody.Language != "zh-CN" || searchBody.Candidates[0].Language != "zho" {
		t.Fatalf("provider language projection = %q", searchBody.Candidates[0].Language)
	}
	if strings.Contains(search.Body.String(), "candidate-a") || strings.Contains(search.Body.String(), "candidate-b") || strings.Contains(search.Body.String(), "fake-emby-api-key") {
		t.Fatalf("Search leaked upstream values: %s", search.Body.String())
	}
	first := searchBody.Candidates[0].Token
	second := searchBody.Candidates[1].Token
	failed := d2POST(t, handler, "/v1/media/movie-1/subtitles/fetch", `{"candidate_token":"`+first+`"}`)
	if failed.Code != http.StatusBadGateway || !strings.Contains(failed.Body.String(), `"candidate_fetch_failed"`) {
		t.Fatalf("candidate A = %d %s", failed.Code, failed.Body.String())
	}
	fetched := d2POST(t, handler, "/v1/media/movie-1/subtitles/fetch", `{"candidate_token":"`+second+`"}`)
	if fetched.Code != http.StatusOK || strings.Contains(fetched.Body.String(), "candidate-b") || !strings.Contains(fetched.Body.String(), `"preview_ready":true`) {
		t.Fatalf("candidate B = %d %s", fetched.Code, fetched.Body.String())
	}
	var fetchedBody struct {
		ArtifactToken string `json:"artifact_token"`
		Language      string `json:"language"`
	}
	if err := json.Unmarshal(fetched.Body.Bytes(), &fetchedBody); err != nil || fetchedBody.ArtifactToken == "" || fetchedBody.Language != "zh-CN" {
		t.Fatalf("artifact response = %s", fetched.Body.String())
	}
	requestsBeforeReplay := fake.snapshot()
	replay := d2POST(t, handler, "/v1/media/movie-1/subtitles/fetch", `{"candidate_token":"`+second+`"}`)
	if replay.Code != http.StatusOK || replay.Body.String() != fetched.Body.String() {
		t.Fatalf("idempotent fetch = %d %s vs %s", replay.Code, replay.Body.String(), fetched.Body.String())
	}
	requestsAfterReplay := fake.snapshot()
	if countD2Path(requestsAfterReplay, "/Providers/Subtitles/Subtitles/candidate-b") != 1 {
		t.Fatalf("provider fetch was repeated: %#v", requestsAfterReplay)
	}
	previewResponse := d2POST(t, handler, "/v1/media/movie-1/subtitles/preview", `{"artifact_token":"`+fetchedBody.ArtifactToken+`","offset":0,"limit":200}`)
	if previewResponse.Code != http.StatusOK || !strings.Contains(previewResponse.Body.String(), "预览内容") {
		t.Fatalf("Preview = %d %s", previewResponse.Code, previewResponse.Body.String())
	}
	var previewBody struct {
		Language string `json:"language"`
	}
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &previewBody); err != nil || previewBody.Language != "zh-CN" {
		t.Fatalf("Preview language = %q err=%v body=%s", previewBody.Language, err, previewResponse.Body.String())
	}
	if countD2Path(fake.snapshot(), "/Providers/Subtitles/Subtitles/candidate-b") != 1 || countD2Path(fake.snapshot(), "/Items/movie-1/RemoteSearch/Subtitles/zh-CN") != 1 {
		t.Fatalf("Preview touched provider/search: %#v", fake.snapshot())
	}
	if len(requestsAfterReplay) <= len(requestsBeforeReplay) || countD2Path(fake.snapshot(), "/Items") != 5 {
		t.Fatalf("unexpected GetItem count after replay/preview: %#v", fake.snapshot())
	}
	for _, request := range fake.snapshot() {
		if request.Method != http.MethodGet || request.Token != "fake-emby-api-key" {
			t.Fatalf("unsafe upstream request = %#v", request)
		}
	}
	if strings.Contains(logs.String(), first) || strings.Contains(logs.String(), second) || strings.Contains(logs.String(), "candidate-a") || strings.Contains(logs.String(), "fake-emby-api-key") {
		t.Fatalf("sensitive D2 value appeared in logs: %s", logs.String())
	}
	if entries, err := os.ReadDir(mediaDir); err != nil || len(entries) != 0 {
		t.Fatalf("D2 wrote to media directory: entries=%v err=%v", entries, err)
	}
}

func TestD2HTTPMultisourceAndRequestBodyBoundaries(t *testing.T) {
	fake := &d2FakeEmby{TB: t, multiSource: true}
	handler, upstream, _ := newD2HTTPTestServer(t, fake, true)
	defer upstream.Close()
	for _, test := range []struct {
		path string
		body string
	}{
		{"/v1/media/movie-1/subtitles/search", `{"media_source_id":"source-2"}`},
		{"/v1/media/movie-1/subtitles/fetch", `{"candidate_token":"opaque"}`},
		{"/v1/media/movie-1/subtitles/preview", `{"artifact_token":"opaque"}`},
	} {
		response := d2POST(t, handler, test.path, test.body)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"d2_multisource_unsupported"`) {
			t.Fatalf("multi-source %s = %d %s", test.path, response.Code, response.Body.String())
		}
	}
	if countD2Path(fake.snapshot(), "/Items/movie-1/RemoteSearch/Subtitles/zh-CN") != 0 || countD2Path(fake.snapshot(), "/Providers/Subtitles/Subtitles/opaque") != 0 {
		t.Fatal("multi-source reached provider")
	}
	tooLarge := d2POST(t, handler, "/v1/media/movie-1/subtitles/search", `{"language":"`+strings.Repeat("x", 9000)+`"}`)
	if tooLarge.Code != http.StatusBadRequest || !strings.Contains(tooLarge.Body.String(), `"invalid_request"`) {
		t.Fatalf("oversized request = %d %s", tooLarge.Code, tooLarge.Body.String())
	}
	unknown := d2POST(t, handler, "/v1/media/movie-1/subtitles/search", `{"unknown":true}`)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown field = %d %s", unknown.Code, unknown.Body.String())
	}
}

func countD2Path(requests []d2UpstreamRequest, path string) int {
	count := 0
	for _, request := range requests {
		if request.Path == path {
			count++
		}
	}
	return count
}
