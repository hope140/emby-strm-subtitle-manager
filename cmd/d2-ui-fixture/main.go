// d2-ui-fixture serves a local-only Fake Emby and SubBridge instance for the
// real-browser D2 UI acceptance script. It is not a production entry point.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hope140/emby-strm-subtitle-manager/internal/config"
	"github.com/hope140/emby-strm-subtitle-manager/internal/d2"
	"github.com/hope140/emby-strm-subtitle-manager/internal/domain"
	"github.com/hope140/emby-strm-subtitle-manager/internal/embyclient"
	"github.com/hope140/emby-strm-subtitle-manager/internal/httpapi"
	"github.com/hope140/emby-strm-subtitle-manager/internal/httpui"
	"github.com/hope140/emby-strm-subtitle-manager/internal/inventory"
	"github.com/hope140/emby-strm-subtitle-manager/internal/pathmap"
	"github.com/hope140/emby-strm-subtitle-manager/internal/preview"
	"github.com/hope140/emby-strm-subtitle-manager/internal/subtitleprovider"
	"github.com/hope140/emby-strm-subtitle-manager/internal/version"
)

const fixtureItemID = "movie-ui-fixture"

type fixtureEmby struct {
	item domain.EmbyItem
}

func (f fixtureEmby) ListLibraries(context.Context) ([]domain.Library, error) {
	return []domain.Library{{ID: "library-ui-fixture", Name: "D2 UI Fixture", CollectionType: "movies"}}, nil
}

func (f fixtureEmby) ListItems(context.Context, string, int, int) (domain.ItemPage, error) {
	return domain.ItemPage{
		Items:            []domain.ItemSummary{f.item.ItemSummary},
		TotalRecordCount: 1,
		StartIndex:       0,
		Limit:            50,
		HasMore:          false,
	}, nil
}

func (f fixtureEmby) GetItem(context.Context, string) (domain.EmbyItem, error) {
	return f.item, nil
}

type fakeUpstream struct {
	itemJSON      []byte
	failedID      string
	previewableID string
}

func (f fakeUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.URL.Path {
	case "/Items":
		writeJSON(w, map[string]any{"Items": []json.RawMessage{f.itemJSON}})
	case "/Items/" + fixtureItemID + "/RemoteSearch/Subtitles/zh-CN":
		writeJSON(w, []map[string]any{
			{"Id": f.failedID, "ProviderName": "FakeThunder", "Name": "失败候选", "Language": "zho", "Format": "srt", "Comment": "用于候选级失败验收", "Score": 0.31},
			{"Id": f.previewableID, "ProviderName": "FakeASSRT", "Name": "可预览候选", "Language": "zho", "Format": "srt", "Comment": "用于分页预览验收", "IsHashMatch": true, "Score": 0.92},
		})
	case "/Providers/Subtitles/Subtitles/" + f.failedID:
		http.Error(w, "candidate failed", http.StatusInternalServerError)
	case "/Providers/Subtitles/Subtitles/" + f.previewableID:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, fixtureSubtitle())
	default:
		http.NotFound(w, r)
	}
}

func main() {
	testToken := strings.TrimSpace(os.Getenv("D2_UI_TEST_TOKEN"))
	if testToken == "" {
		log.Fatal("D2_UI_TEST_TOKEN is required")
	}
	listenAddress := strings.TrimSpace(os.Getenv("D2_UI_LISTEN_ADDRESS"))
	if listenAddress == "" {
		listenAddress = "127.0.0.1:0"
	}
	remoteEnabled := strings.EqualFold(strings.TrimSpace(os.Getenv("D2_UI_REMOTE_SEARCH_ENABLED")), "true")
	artifactTTL := envInt("D2_UI_ARTIFACT_TTL_SECONDS", 600)
	if artifactTTL < 1 {
		artifactTTL = 1
	}

	root, err := os.MkdirTemp("", "d2-ui-fixture-media-")
	if err != nil {
		log.Fatal("create fixture media root")
	}
	defer os.RemoveAll(root)
	if err := os.WriteFile(filepath.Join(root, "Fixture.strm"), []byte{}, 0o600); err != nil {
		log.Fatal("create fixture media")
	}
	item := fixtureItem(root)
	upstream := httptest.NewServer(fakeUpstream{itemJSON: fixtureItemJSON(), failedID: opaqueValue(), previewableID: opaqueValue()})
	defer upstream.Close()

	client, err := embyclient.New(embyclient.Config{BaseURL: upstream.URL, APIKey: opaqueValue(), HTTPClient: upstream.Client()})
	if err != nil {
		log.Fatal("create fake Emby client")
	}
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: `C:\d2-ui\media`, Local: root}})
	if err != nil {
		log.Fatal("create fixture path mapper")
	}
	inventoryService, err := inventory.New(inventory.Options{
		FileSystem:  inventory.OSFileSystem{},
		IdentityKey: []byte("d2-ui-fixture-identity-key-012345678901"),
		Mapper:      mapper,
	})
	if err != nil {
		log.Fatal("create fixture inventory")
	}
	cacheDir, err := os.MkdirTemp("", "d2-ui-fixture-cache-")
	if err != nil {
		log.Fatal("create fixture cache")
	}
	defer os.RemoveAll(cacheDir)
	allowlist := preview.NewAllowlist([]string{fixtureItemID})
	d2Service, err := d2.New(d2.Options{
		Config:              config.D2Config{CacheDir: cacheDir, ArtifactTTLSeconds: artifactTTL},
		RemoteSearchEnabled: remoteEnabled,
		CanaryEnabled:       true,
		Allowlist:           allowlist,
		Emby:                client,
		Provider:            subtitleprovider.NewEmbyRemoteSubtitleProvider(client),
		AuthContext:         d2.AuthContextFromToken(testToken),
	})
	if err != nil {
		log.Fatal("create D2 service")
	}
	app := httpapi.NewServerWithServices(config.Config{Features: config.FeatureConfig{RemoteSearchEnabled: remoteEnabled}}, version.Info{Version: "d2-ui-fixture"}, slog.New(slog.NewJSONHandler(io.Discard, nil)), httpapi.Services{
		Emby: fixtureEmby{item: item}, D2: d2Service, Mapper: mapper, Inventory: inventoryService, AuthToken: testToken, UI: httpui.NewHandler(),
	}).Handler()

	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatal("listen fixture")
	}
	defer listener.Close()
	server := &http.Server{Handler: app, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	fmt.Printf("D2_UI_FIXTURE_URL=http://%s\n", listener.Addr().String())
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal("fixture stopped")
	}
}

func fixtureItem(root string) domain.EmbyItem {
	streams := []domain.MediaStream{}
	defaultSource := true
	return domain.EmbyItem{
		ItemSummary:  domain.ItemSummary{ID: fixtureItemID, Name: "D2 UI Fixture Movie", Type: "Movie", ProductionYear: intPointer(2026)},
		Path:         `C:\d2-ui\media\Fixture.strm`,
		MediaSources: []domain.MediaSource{{ID: "fixture-source", Name: "Fixture source", Path: filepath.Join(root, "Fixture.strm"), Container: "mkv", IsDefault: &defaultSource, MediaStreams: &streams}},
		MediaStreams: &streams,
	}
}

func fixtureItemJSON() []byte {
	return mustJSON(map[string]any{
		"Id":   fixtureItemID,
		"Name": "D2 UI Fixture Movie",
		"Type": "Movie",
		"Path": `C:\d2-ui\media\Fixture.strm`,
		"MediaSources": []map[string]any{{
			"Id": "fixture-source", "Name": "Fixture source", "Path": `C:\d2-ui\media\Fixture.strm`,
			"Container": "mkv", "Default": true, "MediaStreams": []any{},
		}},
		"MediaStreams": []any{},
	})
}

func fixtureSubtitle() string {
	var builder strings.Builder
	for index := 1; index <= 205; index++ {
		start := (index - 1) * 1000
		end := start + 800
		fmt.Fprintf(&builder, "%d\n%s --> %s\n第 %d 条预览文本\n\n", index, subtitleTime(start), subtitleTime(end), index)
	}
	return builder.String()
}

func subtitleTime(milliseconds int) string {
	hours := milliseconds / 3600000
	minutes := (milliseconds % 3600000) / 60000
	seconds := (milliseconds % 60000) / 1000
	millis := milliseconds % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

func intPointer(value int) *int { return &value }

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		log.Fatal("encode fixture item")
	}
	return data
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return value
}

func opaqueValue() string {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatal("generate fixture opaque value")
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}
