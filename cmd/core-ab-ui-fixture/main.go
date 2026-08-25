// core-ab-ui-fixture serves a local-only, stateful Fake Emby and SubBridge
// instance for the real-browser Core A/B acceptance script. It is not a
// production entry point.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hope140/subbridge/internal/auth"
	"github.com/hope140/subbridge/internal/config"
	"github.com/hope140/subbridge/internal/d2"
	"github.com/hope140/subbridge/internal/d3"
	"github.com/hope140/subbridge/internal/domain"
	"github.com/hope140/subbridge/internal/httpapi"
	"github.com/hope140/subbridge/internal/httpui"
	"github.com/hope140/subbridge/internal/inventory"
	"github.com/hope140/subbridge/internal/pathmap"
	"github.com/hope140/subbridge/internal/preview"
	"github.com/hope140/subbridge/internal/subtitleprovider"
	"github.com/hope140/subbridge/internal/version"
)

const fixtureItemID = "core-ab-ui-fixture"

type fixtureEmby struct {
	mu   sync.Mutex
	root string
}

func (f *fixtureEmby) ListLibraries(context.Context) ([]domain.Library, error) {
	return []domain.Library{{ID: "core-ab-ui-library", Name: "Core A/B UI Fixture", CollectionType: "movies"}}, nil
}

func (f *fixtureEmby) ListItems(context.Context, string, int, int) (domain.ItemPage, error) {
	return domain.ItemPage{Items: []domain.ItemSummary{{ID: fixtureItemID, Name: "Core A/B UI Fixture Movie", Type: "Movie"}}, TotalRecordCount: 1, StartIndex: 0, Limit: 50}, nil
}

func (f *fixtureEmby) GetItem(_ context.Context, itemID string) (domain.EmbyItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if itemID != fixtureItemID {
		return domain.EmbyItem{}, errors.New("fixture item not found")
	}
	defaultSource := true
	sourceA := f.subtitleStreams("source-a")
	sourceB := f.subtitleStreams("source-b")
	return domain.EmbyItem{
		ItemSummary: domain.ItemSummary{ID: fixtureItemID, Name: "Core A/B UI Fixture Movie", Type: "Movie", ProductionYear: intPointer(2026)},
		Path:        "/fixture/media/Fixture.strm",
		MediaSources: []domain.MediaSource{
			{ID: "source-a", Name: "Version A", Path: "/fixture/media/Version-A.mkv", Container: "mkv", IsDefault: &defaultSource, MediaStreams: &sourceA},
			{ID: "source-b", Name: "Version B", Path: "/fixture/media/Version-B.mkv", Container: "mkv", MediaStreams: &sourceB},
		},
	}, nil
}

func (f *fixtureEmby) RefreshItem(context.Context, string) error { return nil }

func (f *fixtureEmby) subtitleStreams(sourceID string) []domain.MediaStream {
	entries, err := os.ReadDir(f.root)
	if err != nil {
		return []domain.MediaStream{}
	}
	streams := make([]domain.MediaStream, 0)
	for _, entry := range entries {
		if entry.IsDir() || !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".srt" && ext != ".ass" && ext != ".ssa" {
			continue
		}
		if strings.HasPrefix(name, "Version-A.") && sourceID != "source-a" {
			continue
		}
		if strings.HasPrefix(name, "Version-B.") && sourceID != "source-b" {
			continue
		}
		index := len(streams)
		external, text := true, true
		streams = append(streams, domain.MediaStream{Index: &index, Type: "Subtitle", Path: "/fixture/media/" + name, Codec: strings.TrimPrefix(ext, "."), IsExternal: &external, IsTextSubtitleStream: &text})
	}
	return streams
}

type fixtureProvider struct{}

func (fixtureProvider) Search(context.Context, string, string, string, bool) ([]subtitleprovider.Candidate, error) {
	return []subtitleprovider.Candidate{{RawID: "fixture-candidate", Provider: "Fixture", Name: "远程候选", Language: "zh-CN", Format: "srt", Comment: "本地 Fake Emby 候选"}}, nil
}

func (fixtureProvider) Fetch(context.Context, string) (subtitleprovider.FetchResult, error) {
	return subtitleprovider.FetchResult{Content: []byte("1\n00:00:01,000 --> 00:00:02,000\n远程预览字幕\n"), Attempts: 1}, nil
}

func main() {
	token := requiredEnv("CORE_AB_UI_TEST_TOKEN")
	username := optionalEnv("CORE_AB_UI_TEST_USERNAME", "fixture-admin")
	password := requiredEnv("CORE_AB_UI_TEST_PASSWORD")
	workRoot := requiredEnv("CORE_AB_UI_WORK_ROOT")
	listenAddress := optionalEnv("CORE_AB_UI_LISTEN_ADDRESS", "127.0.0.1:0")
	if err := os.MkdirAll(workRoot, 0o700); err != nil {
		log.Fatal("create fixture work root")
	}
	mediaRoot := filepath.Join(workRoot, "media")
	for _, directory := range []string{mediaRoot, filepath.Join(workRoot, "cache"), filepath.Join(workRoot, "history"), filepath.Join(workRoot, "quarantine"), filepath.Join(workRoot, "archive"), filepath.Join(workRoot, "trash")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			log.Fatal("create fixture directory")
		}
	}
	if err := os.WriteFile(filepath.Join(mediaRoot, "Fixture.strm"), []byte("https://fixture.example/movie"), 0o600); err != nil {
		log.Fatal("create fixture media")
	}
	if err := os.WriteFile(filepath.Join(mediaRoot, "Fixture.zh-CN.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\n已有字幕\n"), 0o644); err != nil {
		log.Fatal("create fixture subtitle")
	}

	adminAuth, err := auth.New(username, password, auth.Options{})
	if err != nil {
		log.Fatal("create fixture authentication")
	}
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: "/fixture/media", Local: mediaRoot}})
	if err != nil {
		log.Fatal("create fixture mapper")
	}
	guard, err := pathmap.NewPathGuard([]string{mediaRoot})
	if err != nil {
		log.Fatal("create fixture path guard")
	}
	inventoryService, err := inventory.New(inventory.Options{FileSystem: inventory.OSFileSystem{}, IdentityKey: []byte("core-ab-ui-fixture-identity-key-012345678901234567890"), Mapper: mapper, Guard: guard})
	if err != nil {
		log.Fatal("create fixture inventory")
	}
	artifactStore, err := preview.NewArtifactStore(preview.ArtifactStoreOptions{Directory: filepath.Join(workRoot, "cache"), TTL: 10 * time.Minute})
	if err != nil {
		log.Fatal("create fixture artifact store")
	}
	gate := preview.NewDailyGate()
	emby := &fixtureEmby{root: mediaRoot}
	authContext := d2.AuthContextFromToken(token)
	d2Service, err := d2.New(d2.Options{Config: config.D2Config{DefaultLanguage: "zh-CN"}, RemoteSearchEnabled: true, Gate: gate, Emby: emby, Provider: fixtureProvider{}, ArtifactStore: artifactStore, AuthContext: authContext})
	if err != nil {
		log.Fatal("create fixture D2")
	}
	d3Service, err := d3.New(d3.Options{
		Config:       config.D3Config{HistoryDir: filepath.Join(workRoot, "history"), QuarantineDir: filepath.Join(workRoot, "quarantine"), ArchiveDir: filepath.Join(workRoot, "archive"), TrashDir: filepath.Join(workRoot, "trash"), RefreshTimeoutSeconds: 1},
		WriteEnabled: true, Gate: gate, Emby: emby, Refresher: emby, Mapper: mapper, Guard: guard, Inventory: inventoryService, Artifacts: artifactStore, AuthContext: authContext, MaxSubtitleBytes: 4 << 20,
	})
	if err != nil {
		log.Fatal("create fixture D3")
	}
	app := httpapi.NewServerWithServices(config.Config{Features: config.FeatureConfig{RemoteSearchEnabled: true, WriteEnabled: true}}, version.Info{Version: "core-ab-ui-fixture"}, slog.New(slog.NewJSONHandler(io.Discard, nil)), httpapi.Services{
		Emby: emby, D2: d2Service, D3: d3Service, Mapper: mapper, Guard: guard, Inventory: inventoryService,
		AuthToken: token, AuthTokenScopes: []string{config.APIAuthScopeMediaRead, config.APIAuthScopeSubtitleSearch, config.APIAuthScopeSubtitlePreview, config.APIAuthScopeSubtitleWrite}, AdminAuth: adminAuth, UI: httpui.NewHandler(),
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
	fmt.Printf("CORE_AB_UI_FIXTURE_URL=http://%s\n", listener.Addr().String())
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal("fixture stopped")
	}
}

func requiredEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func optionalEnv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func intPointer(value int) *int { return &value }

var _ subtitleprovider.Provider = fixtureProvider{}
