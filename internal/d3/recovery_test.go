package d3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
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
	mu       sync.Mutex
	calls    int
	failAt   int
	failErr  error
	failures map[int]error
}

func (f *fileBackedRefresher) RefreshItem(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if err := f.failures[f.calls]; err != nil {
		return err
	}
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
		sourcePaths = map[string]string{"source-1": "https://media.example/movie.mkv?opaque=private"}
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
	if replaced.Type != OperationReplace || replaced.Status != "verified" || replaced.FileName != "movie.subbridge.zh-CN.srt" {
		t.Fatalf("replace response = %#v", replaced)
	}
	replaceHistory, found := service.loadRecoveryRecord(replaced.OperationID)
	if !found || replaceHistory.OriginalLocation != string(media.WriteTargetLocationItem) {
		t.Fatalf("STRM replace history location = %#v found=%v", replaceHistory, found)
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
	operations, err := service.ListOperations("movie-1", 0)
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
	deleteHistory, found := service.loadRecoveryRecord(deleted.OperationID)
	if !found || deleteHistory.OriginalLocation != string(media.WriteTargetLocationItem) {
		t.Fatalf("STRM delete history location = %#v found=%v", deleteHistory, found)
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

func TestReplaceRetryAfterSuccessfulRollbackReusesRecoveryFiles(t *testing.T) {
	service, reader, _, root := newRecoveryTestService(t, nil, 2)
	oldName := "movie.zh-CN.srt"
	oldContent := []byte("1\n00:00:01,000 --> 00:00:02,000\n重试前旧字幕\n")
	if err := os.WriteFile(filepath.Join(root, oldName), oldContent, 0o644); err != nil {
		t.Fatal(err)
	}
	subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
	artifact := recoveryArtifact(t, service, "source-1", "重试后新字幕")
	request := ReplaceRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "replace-retry-rollback"}
	if _, err := service.Replace(context.Background(), "movie-1", subtitleID, request); !hasD3Code(err, "emby_refresh_failed") {
		t.Fatalf("initial replace error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, oldName)); err != nil || string(got) != string(oldContent) {
		t.Fatalf("old content after rollback = %q err=%v", got, err)
	}
	retry, err := service.Replace(context.Background(), "movie-1", subtitleID, request)
	if err != nil || retry.Status != "verified" {
		t.Fatalf("retry replace = %#v err=%v", retry, err)
	}
	if _, err := os.Stat(filepath.Join(root, oldName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old subtitle remains after retry: %v", err)
	}
}

func TestDeleteRetryAfterSuccessfulRollbackReusesRecoveryFiles(t *testing.T) {
	service, reader, _, root := newRecoveryTestService(t, nil, 1)
	oldName := "movie.zh-CN.srt"
	if err := os.WriteFile(filepath.Join(root, oldName), []byte("1\n00:00:01,000 --> 00:00:02,000\n删除重试\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
	request := DeleteRequest{MediaSourceID: "source-1", OperationID: "delete-retry-rollback"}
	if _, err := service.Delete(context.Background(), "movie-1", subtitleID, request); !hasD3Code(err, "emby_refresh_failed") {
		t.Fatalf("initial delete error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, oldName)); err != nil {
		t.Fatalf("old subtitle was not restored: %v", err)
	}
	retry, err := service.Delete(context.Background(), "movie-1", subtitleID, request)
	if err != nil || retry.Status != "verified" {
		t.Fatalf("retry delete = %#v err=%v", retry, err)
	}
	if _, err := os.Stat(filepath.Join(root, oldName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old subtitle remains after retry: %v", err)
	}
}

func TestMoveToRecoveryReusesOnlyMatchingRecoveryHash(t *testing.T) {
	service, _, _, root := newRecoveryTestService(t, nil, 0)
	source := filepath.Join(root, "movie.zh-CN.srt")
	destination := filepath.Join(service.settings.ArchiveDir, "existing-archive.subbridge")
	matching := []byte("1\n00:00:01,000 --> 00:00:02,000\n匹配\n")
	if err := os.WriteFile(source, matching, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, matching, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.moveToRecovery(source, destination, hashBytes(matching)); err != nil {
		t.Fatalf("matching recovery reuse error = %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("matching source remains: %v", err)
	}
	mismatched := []byte("1\n00:00:01,000 --> 00:00:02,000\n不匹配\n")
	if err := os.WriteFile(source, mismatched, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := service.moveToRecovery(source, destination, hashBytes(mismatched)); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("mismatched recovery error = %v", err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != string(mismatched) {
		t.Fatalf("mismatched source changed = %q err=%v", got, err)
	}
}

func TestReplaceRollbackFailureRequiresManualRecovery(t *testing.T) {
	service, reader, _, root := newRecoveryTestService(t, nil, 2)
	oldName := "movie.zh-CN.srt"
	oldContent := []byte("1\n00:00:01,000 --> 00:00:02,000\n回滚恢复失败\n")
	if err := os.WriteFile(filepath.Join(root, oldName), oldContent, 0o644); err != nil {
		t.Fatal(err)
	}
	subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
	artifact := recoveryArtifact(t, service, "source-1", "新版本")
	service.rollbackHooks.restore = func(string, string, string) error { return errors.New("restore denied") }
	if _, err := service.Replace(context.Background(), "movie-1", subtitleID, ReplaceRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "replace-rollback-restore"}); !hasD3Code(err, "subtitle_rollback_failed") {
		t.Fatalf("rollback restore error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(service.settings.ArchiveDir, recoveryName("archive", sha256Operation("replace-rollback-restore")))); err != nil {
		t.Fatalf("archive must remain for manual recovery: %v", err)
	}
}

func TestRollbackFailureInjectionForQuarantineAndRefresh(t *testing.T) {
	t.Run("quarantine", func(t *testing.T) {
		service, reader, _, root := newRecoveryTestService(t, nil, 1)
		oldName := "movie.zh-CN.srt"
		if err := os.WriteFile(filepath.Join(root, oldName), []byte("1\n00:00:01,000 --> 00:00:02,000\n旧字幕\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
		artifact := recoveryArtifact(t, service, "source-1", "新字幕")
		service.rollbackHooks.quarantine = func(string, string, string) error { return errors.New("quarantine denied") }
		if _, err := service.Replace(context.Background(), "movie-1", subtitleID, ReplaceRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "replace-rollback-quarantine"}); !hasD3Code(err, "subtitle_rollback_failed") {
			t.Fatalf("rollback quarantine error = %v", err)
		}
	})
	t.Run("refresh", func(t *testing.T) {
		service, reader, refresher, root := newRecoveryTestService(t, nil, 0)
		oldName := "movie.zh-CN.srt"
		if err := os.WriteFile(filepath.Join(root, oldName), []byte("1\n00:00:01,000 --> 00:00:02,000\n旧字幕\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
		artifact := recoveryArtifact(t, service, "source-1", "新字幕")
		refresher.failures = map[int]error{2: errors.New("replace refresh failed"), 3: errors.New("rollback refresh failed")}
		if _, err := service.Replace(context.Background(), "movie-1", subtitleID, ReplaceRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "replace-rollback-refresh"}); !hasD3Code(err, "subtitle_rollback_failed") {
			t.Fatalf("rollback refresh error = %v", err)
		}
	})
}

func TestRestoreRollbackFailureFromRemoveRequiresManualRecovery(t *testing.T) {
	service, reader, refresher, root := newRecoveryTestService(t, nil, 0)
	oldName := "movie.zh-CN.srt"
	if err := os.WriteFile(filepath.Join(root, oldName), []byte("1\n00:00:01,000 --> 00:00:02,000\n待恢复\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
	deleted, err := service.Delete(context.Background(), "movie-1", subtitleID, DeleteRequest{MediaSourceID: "source-1", OperationID: "delete-rollback-remove"})
	if err != nil {
		t.Fatal(err)
	}
	refresher.failures = map[int]error{2: errors.New("restore refresh failed")}
	service.rollbackHooks.remove = func(string) error { return errors.New("remove denied") }
	if _, err := service.Restore(context.Background(), deleted.OperationID, RestoreRequest{MediaSourceID: "source-1", OperationID: "restore-rollback-remove"}); !hasD3Code(err, "subtitle_rollback_failed") {
		t.Fatalf("restore rollback remove error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(service.settings.TrashDir, recoveryName("trash", sha256Operation("delete-rollback-remove")))); err != nil {
		t.Fatalf("trash must remain for manual recovery: %v", err)
	}
}

func TestHistoryFailureRollsBackVerifiedReplace(t *testing.T) {
	service, reader, _, root := newRecoveryTestService(t, nil, 0)
	oldName := "movie.zh-CN.srt"
	oldContent := []byte("1\n00:00:01,000 --> 00:00:02,000\n历史回滚\n")
	if err := os.WriteFile(filepath.Join(root, oldName), oldContent, 0o644); err != nil {
		t.Fatal(err)
	}
	subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
	artifact := recoveryArtifact(t, service, "source-1", "新字幕")
	service.rollbackHooks.writeHistory = func(recoveryRecord) error { return ErrHistory }
	if _, err := service.Replace(context.Background(), "movie-1", subtitleID, ReplaceRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "replace-history-failure"}); !hasD3Code(err, "d3_history_unavailable") {
		t.Fatalf("history error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, oldName)); err != nil || string(got) != string(oldContent) {
		t.Fatalf("old content after history rollback = %q err=%v", got, err)
	}
	entries, err := os.ReadDir(service.settings.QuarantineDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("quarantine entries = %d err=%v", len(entries), err)
	}
}

func sha256Operation(value string) [32]byte {
	return sha256.Sum256([]byte(value))
}

func TestMultiSourceSTRMWritesUseSelectedSource(t *testing.T) {
	service, _, _, root := newRecoveryTestService(t, map[string]string{
		"source-a": "https://media.example/version-A.mkv?opaque=a",
		"source-b": "https://media.example/version-B.mkv?opaque=b",
	}, 0)
	artifact := recoveryArtifact(t, service, "source-b", "多源目标")
	response, err := service.Add(context.Background(), "movie-1", AddRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-b", OperationID: "add-source-b"})
	if err != nil || response.MediaSourceID != "source-b" {
		t.Fatalf("multi-source STRM Add = %#v err=%v", response, err)
	}
	if _, err := os.Stat(filepath.Join(root, response.FileName)); err != nil {
		t.Fatalf("multi-source STRM sidecar missing: %v", err)
	}
}

func TestRestoreRejectsLegacySourceLocationForCurrentSTRM(t *testing.T) {
	service, _, _, _ := newRecoveryTestService(t, nil, 0)
	operationID := "legacy-source-history"
	operationHash := sha256Operation(operationID)
	if err := service.writeRecoveryHistory(recoveryRecord{
		Version: 2, OperationHash: hex.EncodeToString(operationHash[:]), OperationID: operationID, Type: OperationDelete,
		Fingerprint: "legacy-source-location", ItemID: "movie-1", MediaSourceID: "source-1", SubtitleID: "sub_v1_legacy-source",
		FileName: "movie.zh-CN.srt", Format: "srt", Status: "verified", CreatedAt: time.Now().UTC(),
		RecoveryKind: "trash", RecoveryFile: "legacy-trash.subbridge", OriginalFileName: "movie.zh-CN.srt",
		OriginalLocation: string(media.WriteTargetLocationSource), OriginalHash: hashBytes([]byte("legacy")), OriginalFormat: "srt",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Restore(context.Background(), operationID, RestoreRequest{MediaSourceID: "source-1", OperationID: "restore-legacy-source"}); !hasD3Code(err, "strm_history_location_unsupported") {
		t.Fatalf("legacy STRM history error = %v", err)
	}
}

func TestHistoryListMarksLegacySourceRestoreUnsupportedForCurrentSTRM(t *testing.T) {
	service, _, _, _ := newRecoveryTestService(t, nil, 0)
	operationID := "legacy-source-list"
	writeLegacySourceHistory(t, service, operationID)
	operations, err := service.ListOperationsForSource(context.Background(), "movie-1", "source-1", 0)
	if err != nil || len(operations) != 1 {
		t.Fatalf("source history = %#v err=%v", operations, err)
	}
	if operations[0].RestoreSupported == nil || *operations[0].RestoreSupported || operations[0].RestoreErrorCode != "strm_history_location_unsupported" {
		t.Fatalf("legacy restore capability = %#v", operations[0])
	}
}

func TestRestoreLegacySourceHistoryRejectsBadSTRMAnchorsBeforeWriteResolution(t *testing.T) {
	cases := []struct {
		name     string
		itemPath string
		prepare  func(t *testing.T, root string)
	}{
		{name: "unmapped", itemPath: "/outside/movie.strm"},
		{name: "missing", itemPath: "/srv/media/missing.strm"},
		{name: "directory", itemPath: "/srv/media/movie.strm", prepare: func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "movie.strm")); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "movie.strm"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlink", itemPath: "/srv/media/movie.strm", prepare: func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "movie.strm")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "real.strm"), []byte("stub"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "real.strm"), filepath.Join(root, "movie.strm")); err != nil {
				t.Skipf("symlink creation unavailable: %v", err)
			}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			service, reader, _, root := newRecoveryTestService(t, nil, 0)
			if test.prepare != nil {
				test.prepare(t, root)
			}
			reader.mu.Lock()
			reader.itemPath = test.itemPath
			reader.mu.Unlock()
			operationID := "legacy-source-" + test.name
			writeLegacySourceHistory(t, service, operationID)
			if _, err := service.Restore(context.Background(), operationID, RestoreRequest{MediaSourceID: "source-1", OperationID: "restore-bad-" + test.name}); !hasD3Code(err, "strm_history_location_unsupported") {
				t.Fatalf("bad STRM anchor error = %v", err)
			}
		})
	}
}

func TestRecoveryHistoryUsesExplicitWriteTargetLocationForOrdinaryLocalMedia(t *testing.T) {
	for _, operation := range []OperationType{OperationReplace, OperationDelete} {
		t.Run(string(operation), func(t *testing.T) {
			service, reader, _, root := newRecoveryTestService(t, nil, 0)
			if err := os.WriteFile(filepath.Join(root, "movie.mkv"), []byte("fixture media"), 0o600); err != nil {
				t.Fatal(err)
			}
			reader.mu.Lock()
			reader.itemPath = "/srv/media/movie.mkv"
			reader.sourcePaths = map[string]string{"source-1": "/srv/media/movie.mkv"}
			reader.mu.Unlock()
			oldName := "movie.zh-CN.srt"
			oldContent := []byte("1\n00:00:01,000 --> 00:00:02,000\n普通本地媒体\n")
			if err := os.WriteFile(filepath.Join(root, oldName), oldContent, 0o644); err != nil {
				t.Fatal(err)
			}
			subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
			var response OperationResponse
			var err error
			if operation == OperationReplace {
				artifact := recoveryArtifact(t, service, "source-1", "普通本地替换")
				response, err = service.Replace(context.Background(), "movie-1", subtitleID, ReplaceRequest{ArtifactToken: artifact.Token, MediaSourceID: "source-1", OperationID: "ordinary-replace-0001"})
			} else {
				response, err = service.Delete(context.Background(), "movie-1", subtitleID, DeleteRequest{MediaSourceID: "source-1", OperationID: "ordinary-delete-0001"})
			}
			if err != nil {
				t.Fatal(err)
			}
			record, found := service.loadRecoveryRecord(response.OperationID)
			if !found || record.OriginalLocation != string(media.WriteTargetLocationSource) {
				t.Fatalf("ordinary local history location = %#v found=%v", record, found)
			}
			if _, err := service.Restore(context.Background(), response.OperationID, RestoreRequest{MediaSourceID: "source-1", OperationID: "ordinary-restore-0001"}); err != nil {
				t.Fatalf("ordinary local restore = %v", err)
			}
		})
	}
}

func TestCoreABWritesRequireExplicitMediaSource(t *testing.T) {
	service, reader, _, root := newRecoveryTestService(t, nil, 0)
	oldName := "movie.zh-CN.srt"
	if err := os.WriteFile(filepath.Join(root, oldName), []byte("1\n00:00:01,000 --> 00:00:02,000\n显式 source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	subtitleID := managedSubtitleID(t, service, reader, "source-1", oldName)
	if _, err := service.Delete(context.Background(), "movie-1", subtitleID, DeleteRequest{OperationID: "delete-no-source"}); !hasD3Code(err, "invalid_request") {
		t.Fatalf("missing source delete error = %v", err)
	}
}

func TestHistoryListUsesDefaultAndMaximumLimit(t *testing.T) {
	service, _, _, _ := newRecoveryTestService(t, nil, 0)
	for index := 1; index <= 3; index++ {
		operationID := fmt.Sprintf("history-limit-%04d", index)
		hash := sha256Operation(operationID)
		record := recoveryRecord{Version: 2, OperationHash: hex.EncodeToString(hash[:]), OperationID: operationID, Type: OperationAdd,
			Fingerprint: "history-limit", ItemID: "movie-1", MediaSourceID: "source-1", FileName: fmt.Sprintf("movie.%d.srt", index), Format: "srt", Status: "verified", CreatedAt: time.Now().Add(time.Duration(index) * time.Second)}
		if err := service.writeRecoveryHistory(record); err != nil {
			t.Fatal(err)
		}
	}
	if operations, err := service.ListOperations("movie-1", 2); err != nil || len(operations) != 2 {
		t.Fatalf("limited history = %#v err=%v", operations, err)
	}
	if operations, err := service.ListOperations("movie-1", 0); err != nil || len(operations) != 3 {
		t.Fatalf("default history = %#v err=%v", operations, err)
	}
	if _, err := service.ListOperations("movie-1", maxHistoryLimit+1); !hasD3Code(err, "invalid_request") {
		t.Fatalf("over-limit history error = %v", err)
	}
}

func TestHistoryListForSourceFiltersBeforeLimit(t *testing.T) {
	service, _, _, _ := newRecoveryTestService(t, map[string]string{
		"source-a": "https://media.example/movie-a.mkv?opaque=a",
	}, 0)
	base := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	write := func(operationID, sourceID string, createdAt time.Time) {
		t.Helper()
		opHash := sha256Operation(operationID)
		if err := service.writeRecoveryHistory(recoveryRecord{
			Version: 2, OperationHash: hex.EncodeToString(opHash[:]), OperationID: operationID, Type: OperationDelete,
			Fingerprint: "history-source-filter", ItemID: "movie-1", MediaSourceID: sourceID, SubtitleID: "sub_v1_history-filter",
			FileName: "movie.zh-CN.srt", Format: "srt", Status: "verified", CreatedAt: createdAt,
			RecoveryKind: "trash", RecoveryFile: recoveryName("trash", opHash), OriginalFileName: "movie.zh-CN.srt",
			OriginalLocation: string(media.WriteTargetLocationItem), OriginalHash: hashBytes([]byte(operationID)), OriginalFormat: "srt",
		}); err != nil {
			t.Fatal(err)
		}
	}
	write("history-source-a-old-0001", "source-a", base.Add(1*time.Minute))
	write("history-source-a-new-0001", "source-a", base.Add(2*time.Minute))
	write("history-source-b-new-0001", "source-b", base.Add(5*time.Minute))
	write("history-source-b-mid-0001", "source-b", base.Add(4*time.Minute))

	global, err := service.ListOperations("movie-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(global) != 2 || global[0].MediaSourceID != "source-b" || global[1].MediaSourceID != "source-b" {
		t.Fatalf("global history limit = %#v", global)
	}
	operations, err := service.ListOperationsForSource(context.Background(), "movie-1", "source-a", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 2 || operations[0].OperationID != "history-source-a-new-0001" || operations[1].OperationID != "history-source-a-old-0001" {
		t.Fatalf("source-filtered history = %#v", operations)
	}
	for _, operation := range operations {
		if operation.MediaSourceID != "source-a" || operation.RestoreSupported == nil || !*operation.RestoreSupported {
			t.Fatalf("source-filtered restore capability = %#v", operation)
		}
	}
}

func TestRecoveryLocationUsesExplicitWriteTargetClass(t *testing.T) {
	if got, err := recoveryLocation(media.WriteTarget{Location: media.WriteTargetLocationItem}); err != nil || got != "item" {
		t.Fatalf("item recovery location = %q err=%v", got, err)
	}
	if got, err := recoveryLocation(media.WriteTarget{Location: media.WriteTargetLocationSource}); err != nil || got != "source" {
		t.Fatalf("source recovery location = %q err=%v", got, err)
	}
	if _, err := recoveryLocation(media.WriteTarget{}); !errors.Is(err, ErrHistory) {
		t.Fatalf("unknown recovery location error = %v", err)
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

func writeLegacySourceHistory(t *testing.T, service *Service, operationID string) {
	t.Helper()
	operationHash := sha256Operation(operationID)
	oldHash := hashBytes([]byte("legacy"))
	if err := service.writeRecoveryHistory(recoveryRecord{
		Version: 2, OperationHash: hex.EncodeToString(operationHash[:]), OperationID: operationID, Type: OperationDelete,
		Fingerprint: "legacy-source-location", ItemID: "movie-1", MediaSourceID: "source-1", SubtitleID: "sub_v1_legacy-source",
		FileName: "movie.zh-CN.srt", Format: "srt", Status: "verified", CreatedAt: time.Now().UTC(),
		RecoveryKind: "trash", RecoveryFile: "legacy-trash.subbridge", OriginalFileName: "movie.zh-CN.srt",
		OriginalLocation: string(media.WriteTargetLocationSource), OriginalHash: oldHash, OriginalFormat: "srt",
	}); err != nil {
		t.Fatal(err)
	}
}

func hasD3Code(err error, code string) bool {
	var d3Err *Error
	return errors.As(err, &d3Err) && d3Err != nil && d3Err.Code == code
}
