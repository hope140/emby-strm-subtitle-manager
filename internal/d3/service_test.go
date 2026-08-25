package d3

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hope140/subbridge/internal/config"
	"github.com/hope140/subbridge/internal/domain"
	"github.com/hope140/subbridge/internal/inventory"
	"github.com/hope140/subbridge/internal/pathmap"
	"github.com/hope140/subbridge/internal/preview"
	"github.com/hope140/subbridge/internal/subtitle"
)

type fakeItemReader struct {
	mu       sync.Mutex
	item     domain.EmbyItem
	visible  bool
	getCalls int
}

func (f *fakeItemReader) GetItem(context.Context, string) (domain.EmbyItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	item := f.item
	if f.visible {
		path := "/srv/media/movie.subbridge.zh-CN.srt"
		streams := []domain.MediaStream{{Type: "Subtitle", Path: path, IsExternal: boolPtr(true), IsTextSubtitleStream: boolPtr(true)}}
		item.MediaSources[0].MediaStreams = &streams
	}
	return item, nil
}

type fakeRefresher struct {
	reader *fakeItemReader
	mu     sync.Mutex
	calls  int
	err    error
}

func (f *fakeRefresher) RefreshItem(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err == nil {
		f.reader.visible = true
	}
	return f.err
}

func boolPtr(value bool) *bool { return &value }

func newD3TestService(t *testing.T, refreshErr error) (*Service, *fakeItemReader, *fakeRefresher, string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.strm"), []byte("https://media.example/movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	history := filepath.Join(t.TempDir(), "history")
	quarantine := filepath.Join(t.TempDir(), "quarantine")
	archive := filepath.Join(t.TempDir(), "archive")
	trash := filepath.Join(t.TempDir(), "trash")
	cache := filepath.Join(t.TempDir(), "cache")
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: "/srv/media", Local: root}})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := pathmap.NewPathGuard([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	inventoryService, err := inventory.New(inventory.Options{FileSystem: inventory.OSFileSystem{}, IdentityKey: []byte("d3-test-identity-key-012345678901234567890"), Mapper: mapper, Guard: guard})
	if err != nil {
		t.Fatal(err)
	}
	store, err := preview.NewArtifactStore(preview.ArtifactStoreOptions{Directory: cache, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	streams := []domain.MediaStream{}
	item := domain.EmbyItem{ItemSummary: domain.ItemSummary{ID: "movie-1", Name: "Movie", Type: "Movie"}, Path: "/srv/media/movie.strm", MediaSources: []domain.MediaSource{{ID: "source-1", Path: "https://media.example/movie.mkv?opaque=private", Protocol: "Http", MediaStreams: &streams}}}
	reader := &fakeItemReader{item: item}
	refresher := &fakeRefresher{reader: reader, err: refreshErr}
	allowlist := preview.NewAllowlist([]string{"movie-1"})
	service, err := New(Options{Config: config.D3Config{HistoryDir: history, QuarantineDir: quarantine, ArchiveDir: archive, TrashDir: trash}, WriteEnabled: true, Canary: allowlist, Emby: reader, Refresher: refresher, Mapper: mapper, Guard: guard, Inventory: inventoryService, Artifacts: store, AuthContext: "ctx"})
	if err != nil {
		t.Fatal(err)
	}
	generationAllowed, generation := allowlist.Allows("movie-1")
	if !generationAllowed {
		t.Fatal("allowlist did not contain fixture")
	}
	document, err := subtitle.ValidateAndParse([]byte("1\n00:00:01,000 --> 00:00:02,000\n你好\n"), "srt", 4<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(preview.Binding{ItemID: "movie-1", SourceID: "source-1", Language: "zh-CN", AuthContext: "ctx", AllowlistGeneration: generation}, document.Format, "zh-CN", document.Canonical, document.Cues); err != nil {
		t.Fatal(err)
	}
	return service, reader, refresher, cache, root
}

func TestAddWritesAtomicVersionAndIsIdempotent(t *testing.T) {
	service, reader, refresher, _, root := newD3TestService(t, nil)
	store := service.artifacts
	_, generation := service.canary.Allows("movie-1")
	document, err := subtitle.ValidateAndParse([]byte("1\n00:00:01,000 --> 00:00:02,000\n你好\n"), "srt", 4<<20)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.Create(preview.Binding{ItemID: "movie-1", SourceID: "source-1", Language: "zh-CN", AuthContext: "ctx", AllowlistGeneration: generation}, document.Format, "zh-CN", document.Canonical, document.Cues)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Add(context.Background(), "movie-1", AddRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "operation-123"})
	if err != nil {
		t.Fatal(err)
	}
	if result.FileName != "movie.subbridge.zh-CN.srt" || result.Refresh != "verified" {
		t.Fatalf("result = %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(root, result.FileName))
	if err != nil || string(content) != string(document.Canonical) {
		t.Fatalf("written content = %q err=%v", content, err)
	}
	replay, err := service.Add(context.Background(), "movie-1", AddRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "operation-123"})
	if err != nil || replay.FileName != result.FileName {
		t.Fatalf("replay = %#v err=%v", replay, err)
	}
	secondDocument, err := subtitle.ValidateAndParse([]byte("1\n00:00:01,000 --> 00:00:02,000\n不同内容\n"), "srt", 4<<20)
	if err != nil {
		t.Fatal(err)
	}
	secondArtifact, err := store.Create(preview.Binding{ItemID: "movie-1", SourceID: "source-1", Language: "zh-CN", AuthContext: "ctx", AllowlistGeneration: generation}, secondDocument.Format, "zh-CN", secondDocument.Canonical, secondDocument.Cues)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Add(context.Background(), "movie-1", AddRequest{ArtifactToken: secondArtifact.Token, MediaSourceID: "source-1", OperationID: "operation-123"}); err == nil || !hasD3Code(err, "operation_conflict") {
		t.Fatalf("same operation id with a different artifact = %v", err)
	}
	if refresher.calls != 1 || reader.getCalls < 2 {
		t.Fatalf("refresh/get calls = %d/%d", refresher.calls, reader.getCalls)
	}
	entries, readErr := os.ReadDir(service.settings.HistoryDir)
	if readErr != nil || len(entries) != 1 {
		t.Fatalf("history entries = %d err=%v", len(entries), readErr)
	}
}

func TestAddRefreshFailureQuarantinesNewFile(t *testing.T) {
	service, _, _, _, root := newD3TestService(t, errors.New("refresh failed"))
	store := service.artifacts
	_, generation := service.canary.Allows("movie-1")
	document, err := subtitle.ValidateAndParse([]byte("1\n00:00:01,000 --> 00:00:02,000\n失败\n"), "srt", 4<<20)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.Create(preview.Binding{ItemID: "movie-1", SourceID: "source-1", Language: "zh-CN", AuthContext: "ctx", AllowlistGeneration: generation}, document.Format, "zh-CN", document.Canonical, document.Cues)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Add(context.Background(), "movie-1", AddRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "operation-failure"}); err == nil {
		t.Fatal("refresh failure was accepted")
	}
	if entries, readErr := os.ReadDir(root); readErr != nil || len(entries) != 1 {
		t.Fatalf("media after quarantine = %d err=%v", len(entries), readErr)
	}
	if entries, readErr := os.ReadDir(service.settings.QuarantineDir); readErr != nil || len(entries) != 1 {
		t.Fatalf("quarantine = %d err=%v", len(entries), readErr)
	}
}
