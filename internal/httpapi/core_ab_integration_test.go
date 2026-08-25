package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hope140/subbridge/internal/config"
	"github.com/hope140/subbridge/internal/d2"
	"github.com/hope140/subbridge/internal/d3"
	"github.com/hope140/subbridge/internal/domain"
	"github.com/hope140/subbridge/internal/inventory"
	"github.com/hope140/subbridge/internal/pathmap"
	"github.com/hope140/subbridge/internal/preview"
	"github.com/hope140/subbridge/internal/subtitleprovider"
	"github.com/hope140/subbridge/internal/version"
)

// coreABFakeEmby is a stateful, file-backed Fake Emby: Refresh is observed
// through a subsequent GetItem that rebuilds external subtitle streams from
// the fixture directory. It supports ordinary multi-source media and the
// real-shaped single/multi-source STRM modes used by the HTTP tests.
type coreABFakeEmby struct {
	mu          sync.Mutex
	root        string
	strm        bool
	strmSources int
	refresh     int
	getItems    int
}

func (f *coreABFakeEmby) ListLibraries(context.Context) ([]domain.Library, error) {
	return []domain.Library{{ID: "library-1", Name: "Fixture"}}, nil
}

func (f *coreABFakeEmby) ListItems(context.Context, string, int, int) (domain.ItemPage, error) {
	return domain.ItemPage{Items: []domain.ItemSummary{{ID: "movie-1", Name: "Fixture Movie", Type: "Movie"}}, TotalRecordCount: 1, Limit: 50}, nil
}

func (f *coreABFakeEmby) GetItem(_ context.Context, itemID string) (domain.EmbyItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if itemID != "movie-1" {
		return domain.EmbyItem{}, errors.New("not found")
	}
	f.getItems++
	streams := f.streamsLocked()
	sourceAStreams := append([]domain.MediaStream(nil), streams...)
	sourceBStreams := append([]domain.MediaStream(nil), streams...)
	defaultSource := true
	itemPath := "/emby/media/movie.mkv"
	sourceAPath := "/emby/media/movie.mkv"
	sourceBPath := "/emby/media/version-B.mkv"
	if f.strm {
		itemPath = "/emby/media/movie.strm"
		sourceAPath = "https://media.example/version-A.mkv?opaque=a"
		sourceBPath = "https://media.example/version-B.mkv?opaque=b"
	}
	sources := []domain.MediaSource{{ID: "source-a", Name: "Version A", Path: sourceAPath, IsDefault: &defaultSource, MediaStreams: &sourceAStreams}}
	if !f.strm || f.strmSources != 1 {
		sources = append(sources, domain.MediaSource{ID: "source-b", Name: "Version B", Path: sourceBPath, MediaStreams: &sourceBStreams})
	}
	return domain.EmbyItem{
		ItemSummary:  domain.ItemSummary{ID: "movie-1", Name: "Fixture Movie", Type: "Movie"},
		Path:         itemPath,
		MediaSources: sources,
	}, nil
}

func (f *coreABFakeEmby) RefreshItem(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refresh++
	return nil
}

func (f *coreABFakeEmby) streamsLocked() []domain.MediaStream {
	entries, err := os.ReadDir(f.root)
	if err != nil {
		return []domain.MediaStream{}
	}
	streams := make([]domain.MediaStream, 0)
	for _, entry := range entries {
		if entry.IsDir() || !entry.Type().IsRegular() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".srt" && ext != ".ass" && ext != ".ssa" {
			continue
		}
		index := len(streams)
		external, text := true, true
		streams = append(streams, domain.MediaStream{Index: &index, Type: "Subtitle", Path: "/emby/media/" + entry.Name(), Codec: strings.TrimPrefix(ext, "."), IsExternal: &external, IsTextSubtitleStream: &text})
	}
	return streams
}

type coreABProvider struct{}

func (coreABProvider) Search(context.Context, string, string, string, bool) ([]subtitleprovider.Candidate, error) {
	return []subtitleprovider.Candidate{{RawID: "fixture-candidate", Provider: "Fixture", Name: "远程候选", Language: "zh-CN", Format: "srt"}}, nil
}

func (coreABProvider) Fetch(context.Context, string) (subtitleprovider.FetchResult, error) {
	return subtitleprovider.FetchResult{Content: []byte("1\n00:00:01,000 --> 00:00:02,000\n远程字幕\n"), Attempts: 1}, nil
}

func newCoreABHTTPServer(t *testing.T) (http.Handler, *coreABFakeEmby, string, *bytes.Buffer) {
	return newCoreABHTTPServerWithMode(t, false)
}

func newCoreABHTTPServerWithMode(t *testing.T, strm bool) (http.Handler, *coreABFakeEmby, string, *bytes.Buffer) {
	return newCoreABHTTPServerWithSTRMSources(t, strm, 0)
}

func newCoreABHTTPServerWithSTRMSources(t *testing.T, strm bool, strmSources int) (http.Handler, *coreABFakeEmby, string, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	mediaFiles := []string{"movie.mkv", "version-B.mkv"}
	if strm {
		mediaFiles = []string{"movie.strm"}
	}
	for _, name := range mediaFiles {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture media"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !strm {
		if err := os.WriteFile(filepath.Join(root, "movie.zh-CN.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\n旧字幕\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	} else if strmSources == 1 {
		if err := os.WriteFile(filepath.Join(root, "movie.zh-CN.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\nSTRM 旧字幕\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: "/emby/media", Local: root}})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := pathmap.NewPathGuard([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	inventoryService, err := inventory.New(inventory.Options{FileSystem: inventory.OSFileSystem{}, IdentityKey: []byte("core-ab-http-integration-identity-key-012345678901234567890"), Mapper: mapper, Guard: guard})
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(t.TempDir(), "preview-cache")
	artifacts, err := preview.NewArtifactStore(preview.ArtifactStoreOptions{Directory: cache, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	gate := preview.NewDailyGate()
	fake := &coreABFakeEmby{root: root, strm: strm, strmSources: strmSources}
	authContext := d2.AuthContextFromToken(testAuthToken)
	d2Service, err := d2.New(d2.Options{Config: config.D2Config{DefaultLanguage: "zh-CN"}, RemoteSearchEnabled: true, Gate: gate, Emby: fake, Provider: coreABProvider{}, ArtifactStore: artifacts, AuthContext: authContext})
	if err != nil {
		t.Fatal(err)
	}
	d3Service, err := d3.New(d3.Options{
		Config: config.D3Config{
			HistoryDir:            filepath.Join(t.TempDir(), "history"),
			QuarantineDir:         filepath.Join(t.TempDir(), "quarantine"),
			ArchiveDir:            filepath.Join(t.TempDir(), "archive"),
			TrashDir:              filepath.Join(t.TempDir(), "trash"),
			RefreshTimeoutSeconds: 1,
		},
		WriteEnabled:     true,
		Gate:             gate,
		Emby:             fake,
		Refresher:        fake,
		Mapper:           mapper,
		Guard:            guard,
		Inventory:        inventoryService,
		Artifacts:        artifacts,
		AuthContext:      authContext,
		MaxSubtitleBytes: 4 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	logs := new(bytes.Buffer)
	handler := NewServerWithServices(config.Config{Features: config.FeatureConfig{RemoteSearchEnabled: true, WriteEnabled: true}}, version.Info{Version: "core-ab-test"}, slog.New(slog.NewJSONHandler(logs, nil)), Services{
		Emby: fake, D2: d2Service, D3: d3Service, Mapper: mapper, Guard: guard, Inventory: inventoryService,
		AuthToken: testAuthToken, AuthTokenScopes: []string{config.APIAuthScopeMediaRead, config.APIAuthScopeSubtitleSearch, config.APIAuthScopeSubtitlePreview, config.APIAuthScopeSubtitleWrite},
	}).Handler()
	return handler, fake, root, logs
}

func coreABJSON(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testAuthToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func coreABGet(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+testAuthToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func coreABUpload(t *testing.T, handler http.Handler, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "sensitive-client-name.srt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("media_source_id", "source-a"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("language", "zh-CN"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/media/movie-1/subtitles/upload", &body)
	req.Header.Set("Authorization", "Bearer "+testAuthToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCoreABHTTPFakeEmbyDailyMultiSourceFlow(t *testing.T) {
	handler, fake, root, logs := newCoreABHTTPServer(t)
	mediaResponse := coreABGet(t, handler, "/v1/media/movie-1?media_source_id=source-a")
	if mediaResponse.Code != http.StatusOK || !strings.Contains(mediaResponse.Body.String(), `"write_capabilities":{"add":true,"replace":true,"delete":true,"restore":true}`) || strings.Contains(mediaResponse.Body.String(), "media.example") {
		t.Fatalf("ordinary local write capabilities = %d %s", mediaResponse.Code, mediaResponse.Body.String())
	}

	missing := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/search", `{"language":"zh-CN"}`)
	if missing.Code != http.StatusConflict || !strings.Contains(missing.Body.String(), `"media_source_selection_required"`) {
		t.Fatalf("missing source selection = %d %s", missing.Code, missing.Body.String())
	}
	search := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/search", `{"media_source_id":"source-a","language":"zh-CN"}`)
	if search.Code != http.StatusOK {
		t.Fatalf("search = %d %s", search.Code, search.Body.String())
	}
	var searchBody struct {
		Candidates []struct {
			Token string `json:"token"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &searchBody); err != nil || len(searchBody.Candidates) != 1 {
		t.Fatalf("search body = %s err=%v", search.Body.String(), err)
	}
	fetched := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/fetch", `{"candidate_token":"`+searchBody.Candidates[0].Token+`"}`)
	if fetched.Code != http.StatusOK {
		t.Fatalf("fetch = %d %s", fetched.Code, fetched.Body.String())
	}
	var remoteArtifact struct {
		ArtifactToken string `json:"artifact_token"`
	}
	if err := json.Unmarshal(fetched.Body.Bytes(), &remoteArtifact); err != nil || remoteArtifact.ArtifactToken == "" {
		t.Fatalf("fetch body = %s err=%v", fetched.Body.String(), err)
	}
	previewed := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/preview", `{"artifact_token":"`+remoteArtifact.ArtifactToken+`"}`)
	if previewed.Code != http.StatusOK || !strings.Contains(previewed.Body.String(), "远程字幕") {
		t.Fatalf("preview = %d %s", previewed.Code, previewed.Body.String())
	}
	added := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/add", `{"artifact_token":"`+remoteArtifact.ArtifactToken+`","media_source_id":"source-a","operation_id":"add-http-0001"}`)
	if added.Code != http.StatusOK || !strings.Contains(added.Body.String(), `"file_name":"movie.subbridge.zh-CN.srt"`) {
		t.Fatalf("add = %d %s", added.Code, added.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "movie.subbridge.zh-CN.srt")); err != nil {
		t.Fatal(err)
	}

	uploaded := coreABUpload(t, handler, []byte("1\n00:00:01,000 --> 00:00:02,000\n本地上传字幕\n"))
	if uploaded.Code != http.StatusOK || strings.Contains(uploaded.Body.String(), "sensitive-client-name") {
		t.Fatalf("upload = %d %s", uploaded.Code, uploaded.Body.String())
	}
	var uploadArtifact struct {
		ArtifactToken string `json:"artifact_token"`
	}
	if err := json.Unmarshal(uploaded.Body.Bytes(), &uploadArtifact); err != nil || uploadArtifact.ArtifactToken == "" {
		t.Fatalf("upload body = %s err=%v", uploaded.Body.String(), err)
	}
	uploadPreview := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/preview", `{"artifact_token":"`+uploadArtifact.ArtifactToken+`"}`)
	if uploadPreview.Code != http.StatusOK || !strings.Contains(uploadPreview.Body.String(), "本地上传字幕") {
		t.Fatalf("upload preview = %d %s", uploadPreview.Code, uploadPreview.Body.String())
	}

	oldSubtitleID := coreABSubtitleID(t, handler, "movie.zh-CN.srt")
	deleted := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/"+oldSubtitleID+"/delete", `{"media_source_id":"source-a","operation_id":"delete-http-0001"}`)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", deleted.Code, deleted.Body.String())
	}
	var deleteOperation struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(deleted.Body.Bytes(), &deleteOperation); err != nil || deleteOperation.OperationID == "" {
		t.Fatalf("delete body = %s err=%v", deleted.Body.String(), err)
	}
	restoredDelete := coreABJSON(t, handler, "/v1/subtitle-operations/"+deleteOperation.OperationID+"/restore", `{"media_source_id":"source-a","operation_id":"restore-http-0001"}`)
	if restoredDelete.Code != http.StatusOK {
		t.Fatalf("restore delete = %d %s", restoredDelete.Code, restoredDelete.Body.String())
	}

	oldSubtitleID = coreABSubtitleID(t, handler, "movie.zh-CN.srt")
	replaced := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/"+oldSubtitleID+"/replace", `{"artifact_token":"`+uploadArtifact.ArtifactToken+`","media_source_id":"source-a","operation_id":"replace-http-0001"}`)
	if replaced.Code != http.StatusOK {
		t.Fatalf("replace = %d %s", replaced.Code, replaced.Body.String())
	}
	var replaceOperation struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(replaced.Body.Bytes(), &replaceOperation); err != nil || replaceOperation.OperationID == "" {
		t.Fatalf("replace body = %s err=%v", replaced.Body.String(), err)
	}
	if _, err := os.Stat(filepath.Join(root, "movie.zh-CN.srt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old subtitle remains after replace: %v", err)
	}
	restoredReplace := coreABJSON(t, handler, "/v1/subtitle-operations/"+replaceOperation.OperationID+"/restore", `{"media_source_id":"source-a","operation_id":"restore-http-0002"}`)
	if restoredReplace.Code != http.StatusOK {
		t.Fatalf("restore replace = %d %s", restoredReplace.Code, restoredReplace.Body.String())
	}

	history := coreABGet(t, handler, "/v1/subtitle-operations?item_id=movie-1")
	if history.Code != http.StatusOK || strings.Contains(history.Body.String(), root) || strings.Contains(history.Body.String(), "/emby/media") {
		t.Fatalf("history = %d %s", history.Code, history.Body.String())
	}
	var historyBody struct {
		Operations []struct {
			Type string `json:"type"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(history.Body.Bytes(), &historyBody); err != nil || len(historyBody.Operations) < 5 {
		t.Fatalf("history body = %s err=%v", history.Body.String(), err)
	}
	fake.mu.Lock()
	refreshes := fake.refresh
	fake.mu.Unlock()
	if refreshes < 6 {
		t.Fatalf("expected refresh verification for Core B operations, got %d", refreshes)
	}
	if strings.Contains(logs.String(), testAuthToken) || strings.Contains(logs.String(), searchBody.Candidates[0].Token) || strings.Contains(logs.String(), uploadArtifact.ArtifactToken) || strings.Contains(logs.String(), "sensitive-client-name") || strings.Contains(logs.String(), root) {
		t.Fatalf("sensitive value leaked into logs: %s", logs.String())
	}
}

func TestCoreABHTTPSingleSourceSTRMFullFlow(t *testing.T) {
	handler, fake, root, logs := newCoreABHTTPServerWithSTRMSources(t, true, 1)
	search := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/search", `{"media_source_id":"source-a","language":"zh-CN"}`)
	if search.Code != http.StatusOK {
		t.Fatalf("STRM search = %d %s", search.Code, search.Body.String())
	}
	var searchBody struct {
		Candidates []struct {
			Token string `json:"token"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &searchBody); err != nil || len(searchBody.Candidates) != 1 {
		t.Fatalf("STRM search body = %s err=%v", search.Body.String(), err)
	}
	fetched := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/fetch", `{"candidate_token":"`+searchBody.Candidates[0].Token+`"}`)
	if fetched.Code != http.StatusOK {
		t.Fatalf("STRM fetch = %d %s", fetched.Code, fetched.Body.String())
	}
	var remoteArtifact struct {
		ArtifactToken string `json:"artifact_token"`
	}
	if err := json.Unmarshal(fetched.Body.Bytes(), &remoteArtifact); err != nil || remoteArtifact.ArtifactToken == "" {
		t.Fatalf("STRM fetch body = %s err=%v", fetched.Body.String(), err)
	}
	previewed := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/preview", `{"artifact_token":"`+remoteArtifact.ArtifactToken+`"}`)
	if previewed.Code != http.StatusOK || !strings.Contains(previewed.Body.String(), "远程字幕") {
		t.Fatalf("STRM preview = %d %s", previewed.Code, previewed.Body.String())
	}
	added := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/add", `{"artifact_token":"`+remoteArtifact.ArtifactToken+`","media_source_id":"source-a","operation_id":"add-strm-http-0001"}`)
	if added.Code != http.StatusOK || !strings.Contains(added.Body.String(), `"file_name":"movie.subbridge.zh-CN.srt"`) {
		t.Fatalf("STRM add = %d %s", added.Code, added.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "movie.subbridge.zh-CN.srt")); err != nil {
		t.Fatal(err)
	}

	uploaded := coreABUpload(t, handler, []byte("1\n00:00:01,000 --> 00:00:02,000\nSTRM 本地上传字幕\n"))
	if uploaded.Code != http.StatusOK || strings.Contains(uploaded.Body.String(), "sensitive-client-name") {
		t.Fatalf("STRM upload = %d %s", uploaded.Code, uploaded.Body.String())
	}
	var uploadArtifact struct {
		ArtifactToken string `json:"artifact_token"`
	}
	if err := json.Unmarshal(uploaded.Body.Bytes(), &uploadArtifact); err != nil || uploadArtifact.ArtifactToken == "" {
		t.Fatalf("STRM upload body = %s err=%v", uploaded.Body.String(), err)
	}
	uploadPreview := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/preview", `{"artifact_token":"`+uploadArtifact.ArtifactToken+`"}`)
	if uploadPreview.Code != http.StatusOK || !strings.Contains(uploadPreview.Body.String(), "STRM 本地上传字幕") {
		t.Fatalf("STRM upload preview = %d %s", uploadPreview.Code, uploadPreview.Body.String())
	}

	oldSubtitleID := coreABSubtitleID(t, handler, "movie.zh-CN.srt")
	deleted := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/"+oldSubtitleID+"/delete", `{"media_source_id":"source-a","operation_id":"delete-strm-http-0001"}`)
	if deleted.Code != http.StatusOK {
		t.Fatalf("STRM delete = %d %s", deleted.Code, deleted.Body.String())
	}
	var deleteOperation struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(deleted.Body.Bytes(), &deleteOperation); err != nil || deleteOperation.OperationID == "" {
		t.Fatalf("STRM delete body = %s err=%v", deleted.Body.String(), err)
	}
	restoredDelete := coreABJSON(t, handler, "/v1/subtitle-operations/"+deleteOperation.OperationID+"/restore", `{"media_source_id":"source-a","operation_id":"restore-strm-http-0001"}`)
	if restoredDelete.Code != http.StatusOK {
		t.Fatalf("STRM restore after delete = %d %s", restoredDelete.Code, restoredDelete.Body.String())
	}

	oldSubtitleID = coreABSubtitleID(t, handler, "movie.zh-CN.srt")
	replaced := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/"+oldSubtitleID+"/replace", `{"artifact_token":"`+uploadArtifact.ArtifactToken+`","media_source_id":"source-a","operation_id":"replace-strm-http-0001"}`)
	if replaced.Code != http.StatusOK {
		t.Fatalf("STRM replace = %d %s", replaced.Code, replaced.Body.String())
	}
	var replaceOperation struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(replaced.Body.Bytes(), &replaceOperation); err != nil || replaceOperation.OperationID == "" {
		t.Fatalf("STRM replace body = %s err=%v", replaced.Body.String(), err)
	}
	restoredReplace := coreABJSON(t, handler, "/v1/subtitle-operations/"+replaceOperation.OperationID+"/restore", `{"media_source_id":"source-a","operation_id":"restore-strm-http-0002"}`)
	if restoredReplace.Code != http.StatusOK {
		t.Fatalf("STRM restore after replace = %d %s", restoredReplace.Code, restoredReplace.Body.String())
	}

	history := coreABGet(t, handler, "/v1/subtitle-operations?item_id=movie-1&media_source_id=source-a")
	if history.Code != http.StatusOK || strings.Contains(history.Body.String(), root) || strings.Contains(history.Body.String(), "/emby/media") {
		t.Fatalf("STRM history = %d %s", history.Code, history.Body.String())
	}
	if !strings.Contains(history.Body.String(), `"restore_supported":true`) {
		t.Fatalf("STRM history did not expose current restore capability: %s", history.Body.String())
	}
	fake.mu.Lock()
	refreshes := fake.refresh
	fake.mu.Unlock()
	if refreshes < 6 {
		t.Fatalf("expected STRM refresh verification for Core B operations, got %d", refreshes)
	}
	if strings.Contains(logs.String(), testAuthToken) || strings.Contains(logs.String(), searchBody.Candidates[0].Token) || strings.Contains(logs.String(), uploadArtifact.ArtifactToken) || strings.Contains(logs.String(), "sensitive-client-name") || strings.Contains(logs.String(), root) || strings.Contains(logs.String(), "https://media.example") {
		t.Fatalf("sensitive value leaked into STRM logs: %s", logs.String())
	}
}

func TestCoreABHTTPRejectsMultiSourceSTRMWrite(t *testing.T) {
	handler, _, root, _ := newCoreABHTTPServerWithMode(t, true)
	search := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/search", `{"media_source_id":"source-a","language":"zh-CN"}`)
	if search.Code != http.StatusOK {
		t.Fatalf("STRM search = %d %s", search.Code, search.Body.String())
	}
	var searchBody struct {
		Candidates []struct {
			Token string `json:"token"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &searchBody); err != nil || len(searchBody.Candidates) != 1 {
		t.Fatalf("STRM search body = %s err=%v", search.Body.String(), err)
	}
	fetched := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/fetch", `{"candidate_token":"`+searchBody.Candidates[0].Token+`"}`)
	if fetched.Code != http.StatusOK {
		t.Fatalf("STRM fetch = %d %s", fetched.Code, fetched.Body.String())
	}
	var fetchedBody struct {
		ArtifactToken string `json:"artifact_token"`
	}
	if err := json.Unmarshal(fetched.Body.Bytes(), &fetchedBody); err != nil || fetchedBody.ArtifactToken == "" {
		t.Fatalf("STRM fetch body = %s err=%v", fetched.Body.String(), err)
	}
	added := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/add", `{"artifact_token":"`+fetchedBody.ArtifactToken+`","media_source_id":"source-a","operation_id":"add-strm-multi-0001"}`)
	if added.Code != http.StatusConflict || !strings.Contains(added.Body.String(), `"strm_multisource_write_unsupported"`) {
		t.Fatalf("STRM multi-source add = %d %s", added.Code, added.Body.String())
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "movie.strm" {
		t.Fatalf("STRM multi-source media mutation = %#v err=%v", entries, err)
	}
}

func TestCoreABHTTPRejectsAllMultiSourceSTRMWrites(t *testing.T) {
	handler, fake, root, _ := newCoreABHTTPServerWithSTRMSources(t, true, 1)
	search := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/search", `{"media_source_id":"source-a","language":"zh-CN"}`)
	if search.Code != http.StatusOK {
		t.Fatalf("single STRM setup search = %d %s", search.Code, search.Body.String())
	}
	var searchBody struct {
		Candidates []struct {
			Token string `json:"token"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &searchBody); err != nil || len(searchBody.Candidates) != 1 {
		t.Fatalf("single STRM setup search body = %s err=%v", search.Body.String(), err)
	}
	fetched := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/fetch", `{"candidate_token":"`+searchBody.Candidates[0].Token+`"}`)
	if fetched.Code != http.StatusOK {
		t.Fatalf("single STRM setup fetch = %d %s", fetched.Code, fetched.Body.String())
	}
	var artifact struct {
		Token string `json:"artifact_token"`
	}
	if err := json.Unmarshal(fetched.Body.Bytes(), &artifact); err != nil || artifact.Token == "" {
		t.Fatalf("single STRM setup artifact = %s err=%v", fetched.Body.String(), err)
	}
	oldSubtitleID := coreABSubtitleID(t, handler, "movie.zh-CN.srt")
	seedDelete := coreABJSON(t, handler, "/v1/media/movie-1/subtitles/"+oldSubtitleID+"/delete", `{"media_source_id":"source-a","operation_id":"seed-multi-source-history"}`)
	if seedDelete.Code != http.StatusOK {
		t.Fatalf("single STRM setup delete = %d %s", seedDelete.Code, seedDelete.Body.String())
	}
	var seedOperation struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(seedDelete.Body.Bytes(), &seedOperation); err != nil || seedOperation.OperationID == "" {
		t.Fatalf("single STRM setup operation = %s err=%v", seedDelete.Body.String(), err)
	}
	fake.mu.Lock()
	fake.strmSources = 2
	fake.mu.Unlock()

	for name, path := range map[string]string{
		"add":     "/v1/media/movie-1/subtitles/add",
		"replace": "/v1/media/movie-1/subtitles/sub_v1_multisource/replace",
		"delete":  "/v1/media/movie-1/subtitles/sub_v1_multisource/delete",
	} {
		var body string
		switch name {
		case "add", "replace":
			body = `{"artifact_token":"` + artifact.Token + `","media_source_id":"source-a","operation_id":"multi-` + name + `-0001"}`
		default:
			body = `{"media_source_id":"source-a","operation_id":"multi-delete-0001"}`
		}
		response := coreABJSON(t, handler, path, body)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"strm_multisource_write_unsupported"`) {
			t.Fatalf("multi-source STRM %s = %d %s", name, response.Code, response.Body.String())
		}
	}
	restored := coreABJSON(t, handler, "/v1/subtitle-operations/"+seedOperation.OperationID+"/restore", `{"media_source_id":"source-a","operation_id":"multi-restore-0001"}`)
	if restored.Code != http.StatusConflict || !strings.Contains(restored.Body.String(), `"strm_multisource_write_unsupported"`) {
		t.Fatalf("multi-source STRM restore = %d %s", restored.Code, restored.Body.String())
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "movie.strm" {
		t.Fatalf("multi-source STRM route media mutation = %#v err=%v", entries, err)
	}
}

func coreABSubtitleID(t *testing.T, handler http.Handler, fileName string) string {
	t.Helper()
	response := coreABGet(t, handler, "/v1/media/movie-1/subtitles?media_source_id=source-a")
	if response.Code != http.StatusOK {
		t.Fatalf("subtitle inventory = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Inventory struct {
			Complete  bool `json:"inventory_complete"`
			Subtitles []struct {
				ID         string `json:"id"`
				FileName   string `json:"file_name"`
				Manageable bool   `json:"manageable"`
			} `json:"subtitles"`
		} `json:"inventory"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || !body.Inventory.Complete {
		t.Fatalf("inventory decode = %s err=%v", response.Body.String(), err)
	}
	for _, subtitle := range body.Inventory.Subtitles {
		if subtitle.FileName == fileName && subtitle.Manageable {
			return subtitle.ID
		}
	}
	t.Fatalf("manageable subtitle %q not found: %s", fileName, response.Body.String())
	return ""
}

var _ subtitleprovider.Provider = coreABProvider{}
