package d3

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/hope140/subbridge/internal/config"
	"github.com/hope140/subbridge/internal/domain"
	"github.com/hope140/subbridge/internal/inventory"
	"github.com/hope140/subbridge/internal/media"
	"github.com/hope140/subbridge/internal/pathmap"
	"github.com/hope140/subbridge/internal/preview"
	"github.com/hope140/subbridge/internal/subtitle"
)

// fileBackedD3Reader models only the Emby facts D3 relies on. It derives
// external streams from the fixture directory, making every refresh/poll
// assertion exercise the actual on-disk transaction rather than a boolean
// test double.
type fileBackedD3Reader struct {
	mu          sync.Mutex
	root        string
	itemID      string
	itemPath    string
	sourcePaths map[string]string
	getCalls    int
}

func (f *fileBackedD3Reader) GetItem(_ context.Context, itemID string) (domain.EmbyItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if itemID != f.itemID {
		return domain.EmbyItem{}, errors.New("item not found")
	}
	f.getCalls++
	streams := f.subtitleStreams()
	sourceIDs := make([]string, 0, len(f.sourcePaths))
	for sourceID := range f.sourcePaths {
		sourceIDs = append(sourceIDs, sourceID)
	}
	sort.Strings(sourceIDs)
	sources := make([]domain.MediaSource, 0, len(sourceIDs))
	for index, sourceID := range sourceIDs {
		copyStreams := append([]domain.MediaStream(nil), streams...)
		sources = append(sources, domain.MediaSource{ID: sourceID, Name: sourceID, Path: f.sourcePaths[sourceID], IsDefault: boolPtr(index == 0), MediaStreams: &copyStreams})
	}
	return domain.EmbyItem{ItemSummary: domain.ItemSummary{ID: f.itemID, Name: "Fixture Movie", Type: "Movie"}, Path: f.itemPath, MediaSources: sources}, nil
}

func (f *fileBackedD3Reader) subtitleStreams() []domain.MediaStream {
	entries, err := os.ReadDir(f.root)
	if err != nil {
		return []domain.MediaStream{}
	}
	streams := make([]domain.MediaStream, 0)
	for _, entry := range entries {
		if entry.IsDir() || !entry.Type().IsRegular() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".srt" && ext != ".ass" && ext != ".ssa" {
			continue
		}
		index := len(streams)
		streams = append(streams, domain.MediaStream{Index: &index, Type: "Subtitle", Path: "/srv/media/" + entry.Name(), Codec: ext[1:], IsExternal: boolPtr(true), IsTextSubtitleStream: boolPtr(true)})
	}
	return streams
}

type fileBackedRefresher struct {
	mu      sync.Mutex
	calls   int
	failAt  int
	failErr error
}

func (f *fileBackedRefresher) RefreshItem(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.failAt == f.calls {
		if f.failErr != nil {
			return f.failErr
		}
		return errors.New("refresh failed")
	}
	return nil
}

func newRecoveryTestService(t *testing.T, sourcePaths map[string]string, failAt int) (*Service, *fileBackedD3Reader, *fileBackedRefresher, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.strm"), []byte("https://media.example/movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	if len(sourcePaths) == 0 {
		sourcePaths = map[string]string{"source-1": "/srv/media/movie.mkv"}
	}
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: "/srv/media", Local: root}})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := pathmap.NewPathGuard([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	inventoryService, err := inventory.New(inventory.Options{FileSystem: inventory.OSFileSystem{}, IdentityKey: []byte("d3-recovery-test-identity-key-012345678901234567890"), Mapper: mapper, Guard: guard})
	if err != nil {
		t.Fatal(err)
	}
	store, err := preview.NewArtifactStore(preview.ArtifactStoreOptions{Directory: filepath.Join(t.TempDir(), "cache"), TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	reader := &fileBackedD3Reader{root: root, itemID: "movie-1", itemPath: "/srv/media/movie.strm", sourcePaths: sourcePaths}
	refresher := &fileBackedRefresher{failAt: failAt}
	service, err := New(Options{
		Config: config.D3Config{
			HistoryDir:            filepath.Join(t.TempDir(), "history"),
			QuarantineDir:         filepath.Join(t.TempDir(), "quarantine"),
			ArchiveDir:            filepath.Join(t.TempDir(), "archive"),
			TrashDir:              filepath.Join(t.TempDir(), "trash"),
			RefreshTimeoutSeconds: 1,
		},
		WriteEnabled:     true,
		Gate:             preview.NewDailyGate(),
		Emby:             reader,
		Refresher:        refresher,
		Mapper:           mapper,
		Guard:            guard,
		Inventory:        inventoryService,
		Artifacts:        store,
		AuthContext:      "recovery-test",
		MaxSubtitleBytes: 4 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, reader, refresher, root
}

func recoveryArtifact(t *testing.T, service *Service, sourceID, text string) preview.Artifact {
	t.Helper()
	document, err := subtitle.ValidateAndParse([]byte("1\n00:00:01,000 --> 00:00:02,000\n"+text+"\n"), "srt", 4<<20)
	if err != nil {
		t.Fatal(err)
	}
	_, generation := service.gate.Allows("movie-1")
	artifact, err := service.artifacts.Create(preview.Binding{ItemID: "movie-1", SourceID: sourceID, Language: "zh-CN", AuthContext: "recovery-test", AllowlistGeneration: generation}, document.Format, "zh-CN", document.Canonical, document.Cues)
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func managedSubtitleID(t *testing.T, service *Service, reader *fileBackedD3Reader, sourceID, fileName string) string {
	t.Helper()
	item, err := reader.GetItem(context.Background(), "movie-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := media.Build(item, media.BuildOptions{MediaSourceID: sourceID, Mapper: service.mapper, Guard: service.guard})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.inventory.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range result.Subtitles {
		if current.FileName == fileName && current.Manageable {
			return current.ID
		}
	}
	t.Fatalf("manageable subtitle %q not found: %#v", fileName, result.Subtitles)
	return ""
}

func TestReplaceArchivesOldSubtitleAndRestoresIt(t *testing.T) {
	service, reader, _, root := newRecoveryTestService(t, nil, 0)
	oldName := "movie.zh-CN.srt"
	oldContent := []byte("1\n00:00:01,000 --> 00:00:02,000\n旧字幕\n")
	if err := os.WriteFile(filepath.Join(root, oldName), oldContent, 0o644); err != nil {
		t.Fatal(err)
	}
	subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
	artifact := recoveryArtifact(t, service, "source-1", "新字幕")
	replaced, err := service.Replace(context.Background(), "movie-1", subtitleID, ReplaceRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "replace-0001"})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Type != OperationReplace || replaced.Status != "verified" || replaced.FileName == oldName {
		t.Fatalf("replace response = %#v", replaced)
	}
	if _, err := os.Stat(filepath.Join(root, oldName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old subtitle remains after replace: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, replaced.FileName)); err != nil || string(got) == string(oldContent) {
		t.Fatalf("replacement content = %q err=%v", got, err)
	}
	if entries, err := os.ReadDir(service.settings.ArchiveDir); err != nil || len(entries) != 1 {
		t.Fatalf("archive entries = %d err=%v", len(entries), err)
	}
	replay, err := service.Replace(context.Background(), "movie-1", subtitleID, ReplaceRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "replace-0001"})
	if err != nil || replay.OperationID != replaced.OperationID {
		t.Fatalf("replace replay = %#v err=%v", replay, err)
	}
	restored, err := service.Restore(context.Background(), replaced.OperationID, RestoreRequest{MediaSourceID: "source-1", OperationID: "restore-0001"})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Type != OperationRestore || restored.Status != "verified" {
		t.Fatalf("restore response = %#v", restored)
	}
	if got, err := os.ReadFile(filepath.Join(root, oldName)); err != nil || string(got) != string(oldContent) {
		t.Fatalf("restored content = %q err=%v", got, err)
	}
	operations, err := service.ListOperations("movie-1")
	if err != nil || len(operations) != 2 {
		t.Fatalf("operations = %#v err=%v", operations, err)
	}
}

func TestDeleteMovesToTrashAndRestoreRejectsOverwrite(t *testing.T) {
	service, reader, _, root := newRecoveryTestService(t, nil, 0)
	oldName := "movie.zh-CN.srt"
	oldContent := []byte("1\n00:00:01,000 --> 00:00:02,000\n删除前\n")
	if err := os.WriteFile(filepath.Join(root, oldName), oldContent, 0o644); err != nil {
		t.Fatal(err)
	}
	subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
	deleted, err := service.Delete(context.Background(), "movie-1", subtitleID, DeleteRequest{MediaSourceID: "source-1", OperationID: "delete-0001"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, oldName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted subtitle remains: %v", err)
	}
	if entries, err := os.ReadDir(service.settings.TrashDir); err != nil || len(entries) != 1 {
		t.Fatalf("trash entries = %d err=%v", len(entries), err)
	}
	if err := os.WriteFile(filepath.Join(root, oldName), []byte("1\n00:00:01,000 --> 00:00:02,000\n冲突\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Restore(context.Background(), deleted.OperationID, RestoreRequest{MediaSourceID: "source-1", OperationID: "restore-conflict"}); err == nil || !hasD3Code(err, "restore_target_conflict") {
		t.Fatalf("restore overwrite error = %v", err)
	}
	if err := os.Remove(filepath.Join(root, oldName)); err != nil {
		t.Fatal(err)
	}
	restored, err := service.Restore(context.Background(), deleted.OperationID, RestoreRequest{MediaSourceID: "source-1", OperationID: "restore-0002"})
	if err != nil || restored.Type != OperationRestore {
		t.Fatalf("restore = %#v err=%v", restored, err)
	}
	if got, err := os.ReadFile(filepath.Join(root, oldName)); err != nil || string(got) != string(oldContent) {
		t.Fatalf("restored content = %q err=%v", got, err)
	}
	replay, err := service.Delete(context.Background(), "movie-1", subtitleID, DeleteRequest{MediaSourceID: "source-1", OperationID: "delete-0001"})
	if err != nil || replay.OperationID != deleted.OperationID {
		t.Fatalf("delete replay = %#v err=%v", replay, err)
	}
}

func TestRestoreRejectsPersistedHistoryWhenWritesAreDisabled(t *testing.T) {
	service, reader, _, root := newRecoveryTestService(t, nil, 0)
	oldName := "movie.zh-CN.srt"
	if err := os.WriteFile(filepath.Join(root, oldName), []byte("1\n00:00:01,000 --> 00:00:02,000\n禁用恢复\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
	deleted, err := service.Delete(context.Background(), "movie-1", subtitleID, DeleteRequest{MediaSourceID: "source-1", OperationID: "delete-disabled-restore"})
	if err != nil {
		t.Fatal(err)
	}
	service.enabled = false
	if _, err := service.Restore(context.Background(), deleted.OperationID, RestoreRequest{MediaSourceID: "source-1", OperationID: "restore-while-disabled"}); !hasD3Code(err, "write_disabled") {
		t.Fatalf("disabled restore error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, oldName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled restore recreated subtitle: %v", err)
	}
}

func TestReplaceRefreshFailureRestoresOldAndQuarantinesNew(t *testing.T) {
	service, reader, refresher, root := newRecoveryTestService(t, nil, 2)
	oldName := "movie.zh-CN.srt"
	oldContent := []byte("1\n00:00:01,000 --> 00:00:02,000\n仍应保留\n")
	if err := os.WriteFile(filepath.Join(root, oldName), oldContent, 0o644); err != nil {
		t.Fatal(err)
	}
	subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
	artifact := recoveryArtifact(t, service, "source-1", "失败替换")
	if _, err := service.Replace(context.Background(), "movie-1", subtitleID, ReplaceRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "replace-fail-1"}); err == nil || !hasD3Code(err, "emby_refresh_failed") {
		t.Fatalf("replace failure = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, oldName)); err != nil || string(got) != string(oldContent) {
		t.Fatalf("old content after rollback = %q err=%v", got, err)
	}
	if entries, err := os.ReadDir(service.settings.QuarantineDir); err != nil || len(entries) != 1 {
		t.Fatalf("quarantine entries = %d err=%v", len(entries), err)
	}
	if refresher.calls < 4 {
		t.Fatalf("expected rollback refreshes, got %d", refresher.calls)
	}
}

func TestAddUsesSelectedSourcePathForMultiSourceItem(t *testing.T) {
	service, _, _, root := newRecoveryTestService(t, map[string]string{
		"source-a": "/srv/media/version-A.mkv",
		"source-b": "/srv/media/version-B.mkv",
	}, 0)
	artifact := recoveryArtifact(t, service, "source-b", "多源目标")
	result, err := service.Add(context.Background(), "movie-1", AddRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-b", OperationID: "add-source-b"})
	if err != nil {
		t.Fatal(err)
	}
	if result.FileName != "version-B.subbridge.zh-CN.srt" {
		t.Fatalf("selected source filename = %q", result.FileName)
	}
	if _, err := os.Stat(filepath.Join(root, result.FileName)); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryLocationClassifiesSourceSpecificSTRMSidecarsWithoutPersistingPath(t *testing.T) {
	itemDirectory := filepath.Join(t.TempDir(), "item")
	sourceDirectory := filepath.Join(t.TempDir(), "source")
	if got, err := recoveryLocation(filepath.Join(sourceDirectory, "version-B.zh-CN.srt"), itemDirectory, sourceDirectory); err != nil || got != "source" {
		t.Fatalf("source recovery location = %q err=%v", got, err)
	}
	if got, err := recoveryLocation(filepath.Join(itemDirectory, "movie.zh-CN.srt"), itemDirectory, sourceDirectory); err != nil || got != "item" {
		t.Fatalf("item recovery location = %q err=%v", got, err)
	}
	if _, err := recoveryLocation(filepath.Join(t.TempDir(), "outside.zh-CN.srt"), itemDirectory, sourceDirectory); !errors.Is(err, ErrHistory) {
		t.Fatalf("outside recovery location error = %v", err)
	}
}

func TestRecoveryNameDoesNotDependOnOriginalSidecarLength(t *testing.T) {
	var operationHash [32]byte
	copy(operationHash[:], []byte("recovery-name-test"))
	name := recoveryName("archive", operationHash)
	if !safeRecoveryName(name) || len([]byte(name)) > maxFilenameBytes {
		t.Fatalf("recovery name = %q", name)
	}
}

func hasD3Code(err error, code string) bool {
	var d3Err *Error
	return errors.As(err, &d3Err) && d3Err != nil && d3Err.Code == code
}
