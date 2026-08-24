package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/hope140/emby-strm-subtitle-manager/internal/config"
	"github.com/hope140/emby-strm-subtitle-manager/internal/embyclient"
	"github.com/hope140/emby-strm-subtitle-manager/internal/inventory"
	"github.com/hope140/emby-strm-subtitle-manager/internal/pathmap"
	"github.com/hope140/emby-strm-subtitle-manager/internal/version"
)

const integrationAPIKey = "d1-integration-token-never-in-response"
const integrationAuthToken = "d1-http-auth-token-01234567890123456789"

type integrationRequest struct {
	Method string
	Path   string
	Query  url.Values
	Token  string
}

type integrationFakeEmby struct {
	testing.TB
	mu       sync.Mutex
	requests []integrationRequest
}

func (f *integrationFakeEmby) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, integrationRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.Query(),
		Token:  r.Header.Get("X-Emby-Token"),
	})
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/Library/MediaFolders":
		writeIntegrationJSON(w, `{"Items":[{"Id":"library-1","Name":"Movies","CollectionType":"movies","Locations":["C:\\private\\media"]}],"TotalRecordCount":1}`)
	case "/Items":
		if r.URL.Query().Get("Ids") == "movie-1" {
			writeIntegrationJSON(w, integrationItemJSON())
			return
		}
		writeIntegrationJSON(w, `{"Items":[{"Id":"movie-1","Name":"Movie","Type":"Movie","ProductionYear":2024}],"TotalRecordCount":1}`)
	default:
		f.Errorf("unexpected Fake Emby endpoint: %s", r.URL.Path)
		http.NotFound(w, r)
	}
}

func writeIntegrationJSON(w http.ResponseWriter, body string) {
	_, _ = io.WriteString(w, body)
}

func integrationItemJSON() string {
	return `{"Items":[{"Id":"movie-1","Name":"Movie","Type":"Movie","Path":"C:\\emby\\media\\Movie.strm","ProviderIds":{"Imdb":"tt-d1","Tmdb":"tmdb-d1","private":"must-not-appear"},"MediaSources":[{"Id":"source-1","Name":"Main","Path":"C:\\emby\\media\\Movie.strm","Container":"strm","Default":true,"MediaStreams":[{"Index":0,"Type":"Subtitle","Codec":"ass","Language":"eng","Title":"Signs","IsExternal":false,"IsDefault":true,"IsTextSubtitleStream":true},{"Index":1,"Type":"Subtitle","Codec":"srt","Language":"zho","Title":"Chinese sidecar","Path":"C:\\emby\\media\\Movie.zh.srt","IsExternal":true,"IsTextSubtitleStream":true}]}]}],"TotalRecordCount":1}`
}

func (f *integrationFakeEmby) snapshot() []integrationRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]integrationRequest, len(f.requests))
	copy(result, f.requests)
	return result
}

func TestD1FullReadonlyCanaryWithFakeEmby(t *testing.T) {
	root, err := os.MkdirTemp(".", "d1-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	const sentinelURL = "https://strm-content-sentinel.invalid/private-media.m3u8?opaque=must-not-be-read"
	if err := os.WriteFile(filepath.Join(root, "Movie.strm"), []byte(sentinelURL+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Movie.zh.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\n你好\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fake := &integrationFakeEmby{TB: t}
	upstream := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(upstream.Close)
	emby, err := embyclient.New(embyclient.Config{BaseURL: upstream.URL, APIKey: integrationAPIKey, HTTPClient: upstream.Client()})
	if err != nil {
		t.Fatal(err)
	}
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: `C:\emby\media`, Local: root}})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := pathmap.NewPathGuard([]string{root})
	if err != nil {
		// The managed Windows test sandbox denies EvalSymlinks on absolute
		// workspace paths. Keep the real guard in this test everywhere it is
		// available; the elevated run and CI exercise the full path instead.
		if runtime.GOOS == "windows" && errors.Is(err, pathmap.ErrGuardRootUnavailable) {
			t.Skip("PathGuard requires EvalSymlinks permission unavailable in this Windows sandbox")
		}
		t.Fatal(err)
	}
	identityKey := []byte("01234567890123456789012345678901")
	inventoryService, err := inventory.New(inventory.Options{
		FileSystem:  inventory.OSFileSystem{},
		IdentityKey: identityKey,
		Mapper:      mapper,
		Guard:       guard,
	})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := NewServerWithServices(config.Config{Features: config.FeatureConfig{WriteEnabled: false, RemoteSearchEnabled: false}}, version.Info{Version: "d1-test"}, logger, Services{
		Emby:      emby,
		Mapper:    mapper,
		Guard:     guard,
		Inventory: inventoryService,
		AuthToken: integrationAuthToken,
	}).Handler()
	app := httptest.NewServer(handler)
	t.Cleanup(app.Close)

	ready := integrationGET(t, app.Client(), app.URL+"/readyz")
	if ready.StatusCode != http.StatusOK || ready.Body != "{\"status\":\"ready\"}\n" {
		t.Fatalf("readyz = %d %s", ready.StatusCode, ready.Body)
	}
	libraries := integrationGET(t, app.Client(), app.URL+"/v1/emby/libraries")
	if libraries.StatusCode != http.StatusOK || strings.Contains(libraries.Body, "Locations") || strings.Contains(libraries.Body, `C:\\private\\media`) {
		t.Fatalf("libraries response leaked upstream locations: %d %s", libraries.StatusCode, libraries.Body)
	}
	items := integrationGET(t, app.Client(), app.URL+"/v1/emby/items?library_id=library-1&start_index=0&limit=50")
	if items.StatusCode != http.StatusOK || !strings.Contains(items.Body, `"movie-1"`) {
		t.Fatalf("items = %d %s", items.StatusCode, items.Body)
	}
	media := integrationGET(t, app.Client(), app.URL+"/v1/media/movie-1")
	if media.StatusCode != http.StatusOK || !strings.Contains(media.Body, `"is_strm":true`) || !strings.Contains(media.Body, `"mapping_status":"mapped"`) || !strings.Contains(media.Body, `"imdb":"tt-d1"`) {
		t.Fatalf("media = %d %s", media.StatusCode, media.Body)
	}
	if strings.Contains(media.Body, "private") {
		t.Fatalf("media response leaked non-public provider data: %s", media.Body)
	}
	subtitles := integrationGET(t, app.Client(), app.URL+"/v1/media/movie-1/subtitles")
	if subtitles.StatusCode != http.StatusOK {
		t.Fatalf("subtitles = %d %s", subtitles.StatusCode, subtitles.Body)
	}
	var payload struct {
		Media     MediaDTO            `json:"media"`
		Inventory inventory.Inventory `json:"inventory"`
	}
	if err := json.Unmarshal([]byte(subtitles.Body), &payload); err != nil {
		t.Fatalf("decode subtitles: %v", err)
	}
	if !payload.Media.IsSTRM || payload.Media.MappingStatus != "mapped" || !payload.Media.InventoryComplete {
		t.Fatalf("media projection = %#v", payload.Media)
	}
	if payload.Inventory.Presence != inventory.PresencePresent || !payload.Inventory.Complete || len(payload.Inventory.Subtitles) != 2 {
		t.Fatalf("inventory = %#v", payload.Inventory)
	}
	var embedded, sidecar *inventory.Subtitle
	for i := range payload.Inventory.Subtitles {
		subtitle := &payload.Inventory.Subtitles[i]
		switch subtitle.Kind {
		case inventory.KindEmbedded:
			embedded = subtitle
		case inventory.KindSidecar:
			sidecar = subtitle
		}
	}
	if embedded == nil || sidecar == nil || !containsDiscovery(embedded.DiscoveredBy, inventory.DiscoveryEmby) || !containsDiscovery(sidecar.DiscoveredBy, inventory.DiscoveryEmby) || !containsDiscovery(sidecar.DiscoveredBy, inventory.DiscoveryFilesystem) || sidecar.FileName != "Movie.zh.srt" || len(sidecar.Indexes) != 1 || sidecar.Indexes[0] != 1 || !sidecar.Manageable {
		t.Fatalf("embedded/sidecar merge = embedded=%#v sidecar=%#v", embedded, sidecar)
	}

	responses := ready.Body + libraries.Body + items.Body + media.Body + subtitles.Body
	for _, value := range []string{responses, logs.String()} {
		for _, secret := range []string{integrationAPIKey, `C:\emby\media`, root, sentinelURL} {
			if strings.Contains(value, secret) {
				t.Fatalf("response/log leaked sensitive or path value %q", secret)
			}
		}
	}
	assertIntegrationUpstreamRequests(t, fake.snapshot())
}

func containsDiscovery(values []inventory.Discovery, wanted inventory.Discovery) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func integrationGET(t *testing.T, client *http.Client, target string) struct {
	StatusCode int
	Body       string
} {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+integrationAuthToken)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return struct {
		StatusCode int
		Body       string
	}{response.StatusCode, string(body)}
}

func assertIntegrationUpstreamRequests(t *testing.T, requests []integrationRequest) {
	t.Helper()
	if len(requests) != 5 {
		t.Fatalf("upstream request count = %d, want 5", len(requests))
	}
	expected := []struct {
		path  string
		query url.Values
	}{
		{"/Library/MediaFolders", url.Values{"IsHidden": {"false"}}},
		{"/Library/MediaFolders", url.Values{"IsHidden": {"false"}}},
		{"/Items", url.Values{"EnableImages": {"false"}, "EnableUserData": {"false"}, "GroupItemsIntoCollections": {"false"}, "IncludeItemTypes": {"Movie,Episode"}, "Limit": {"50"}, "ParentId": {"library-1"}, "Recursive": {"true"}, "SortBy": {"SortName"}, "SortOrder": {"Ascending"}, "StartIndex": {"0"}}},
		{"/Items", url.Values{"EnableImages": {"false"}, "EnableUserData": {"false"}, "Fields": {"Path,ProviderIds,MediaStreams,MediaSources"}, "Ids": {"movie-1"}, "Limit": {"2"}}},
		{"/Items", url.Values{"EnableImages": {"false"}, "EnableUserData": {"false"}, "Fields": {"Path,ProviderIds,MediaStreams,MediaSources"}, "Ids": {"movie-1"}, "Limit": {"2"}}},
	}
	for i, request := range requests {
		if request.Method != http.MethodGet {
			t.Fatalf("upstream request %d method = %s, want GET", i, request.Method)
		}
		if request.Path != expected[i].path || !reflect.DeepEqual(request.Query, expected[i].query) {
			t.Fatalf("upstream request %d = %s %#v, want %s %#v", i, request.Path, request.Query, expected[i].path, expected[i].query)
		}
		if request.Token != integrationAPIKey {
			t.Fatalf("upstream request %d did not receive X-Emby-Token", i)
		}
		if _, exists := request.Query["api_key"]; exists {
			t.Fatalf("upstream request %d included api_key query", i)
		}
		if strings.Contains(request.Path, "PlaybackInfo") || strings.Contains(request.Path, "Refresh") || strings.Contains(request.Path, "Videos") || strings.Contains(request.Path, "Subtitles") {
			t.Fatalf("upstream request %d reached forbidden endpoint %s", i, request.Path)
		}
	}
}
