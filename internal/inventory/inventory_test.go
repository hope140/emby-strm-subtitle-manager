package inventory

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hope140/emby-strm-subtitle-manager/internal/domain"
	"github.com/hope140/emby-strm-subtitle-manager/internal/media"
)

type recordingFS struct {
	entries   []fs.DirEntry
	info      map[string]fs.FileInfo
	canonical map[string]string
	readDirs  []string
	stats     []string
	evals     []string
	err       error
}

func (f *recordingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	f.readDirs = append(f.readDirs, name)
	if f.err != nil {
		return nil, f.err
	}
	return append([]fs.DirEntry(nil), f.entries...), nil
}
func (f *recordingFS) Lstat(name string) (fs.FileInfo, error) {
	f.stats = append(f.stats, name)
	if value, ok := f.info[name]; ok {
		return value, nil
	}
	return nil, fs.ErrNotExist
}
func (f *recordingFS) EvalSymlinks(name string) (string, error) {
	f.evals = append(f.evals, name)
	if value, ok := f.canonical[name]; ok {
		return value, nil
	}
	return name, nil
}

type fakeInfo struct {
	name string
	mode fs.FileMode
}

func (i fakeInfo) Name() string           { return i.name }
func (i fakeInfo) Size() int64            { return 1 }
func (i fakeInfo) Mode() fs.FileMode      { return i.mode }
func (i fakeInfo) ModTime() (t time.Time) { return }
func (i fakeInfo) IsDir() bool            { return i.mode.IsDir() }
func (i fakeInfo) Sys() any               { return nil }

type fakeEntry struct {
	name string
	typ  fs.FileMode
}

func (e fakeEntry) Name() string               { return e.name }
func (e fakeEntry) IsDir() bool                { return e.typ.IsDir() }
func (e fakeEntry) Type() fs.FileMode          { return e.typ }
func (e fakeEntry) Info() (fs.FileInfo, error) { return fakeInfo{name: e.name, mode: e.typ}, nil }

func TestBuildScansOneDirectoryWithoutReadingBodies(t *testing.T) {
	dir := "/media"
	files := []string{"Movie.srt", "Movie.zh.ass", "Moviefoo.srt", "Movie..ssa", "Movie.txt"}
	f := &recordingFS{info: map[string]fs.FileInfo{}, canonical: map[string]string{}}
	for _, name := range files {
		f.entries = append(f.entries, fakeEntry{name: name})
		full := filepath.Join(dir, name)
		f.info[full] = fakeInfo{name: name, mode: 0}
		f.canonical[full] = full
	}
	ctx := completeContext()
	result, err := Build(ctx, Options{FileSystem: f, IdentityKey: testKey()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Presence != PresencePresent || !result.Complete || len(result.Subtitles) != 3 {
		t.Fatalf("inventory = %#v", result)
	}
	if len(f.readDirs) != 1 {
		t.Fatalf("ReadDir calls = %#v", f.readDirs)
	}
	for _, sub := range result.Subtitles {
		if strings.Contains(sub.ID, "/") || strings.Contains(sub.FileName, "/") {
			t.Fatalf("unsafe public value = %#v", sub)
		}
		if sub.Manageable == false {
			t.Fatalf("sidecar should be manageable = %#v", sub)
		}
	}
}

func TestBuildSeparatesEmbeddedAndMergesExternalBySafeBasename(t *testing.T) {
	dir := "/media"
	name := "Movie.zh.srt"
	full := filepath.Join(dir, name)
	f := &recordingFS{
		entries:   []fs.DirEntry{fakeEntry{name: name}},
		info:      map[string]fs.FileInfo{full: fakeInfo{name: name, mode: 0}},
		canonical: map[string]string{full: full},
	}
	embeddedIndex, externalIndex := 3, 7
	ctx := completeContext()
	ctx.MediaStreams = &[]domain.MediaStream{
		{Index: &embeddedIndex, Type: "Subtitle", Language: "jpn", IsExternal: boolPtr(false), IsTextSubtitleStream: boolPtr(true)},
		{Index: &externalIndex, Type: "Subtitle", Language: "zh-CN", Path: `Movie.zh.srt`, IsExternal: &externalIndexBool, IsTextSubtitleStream: boolPtr(true)},
	}
	result, err := Build(ctx, Options{FileSystem: f, IdentityKey: testKey()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Subtitles) != 2 {
		t.Fatalf("subtitles = %#v", result.Subtitles)
	}
	var embedded, merged Subtitle
	for _, sub := range result.Subtitles {
		if sub.Kind == KindEmbedded {
			embedded = sub
		}
		if sub.Kind == KindSidecar {
			merged = sub
		}
	}
	if embedded.Manageable || merged.ID == "" || merged.Kind != KindSidecar || !merged.Manageable || len(merged.DiscoveredBy) != 2 {
		t.Fatalf("embedded/merged = %#v %#v", embedded, merged)
	}
	if merged.ID != sidecarID(result, "Movie.zh.srt") {
		t.Fatalf("merged id differs from sidecar id")
	}
}

func TestBuildMergesMultipleExternalIndexesIntoOneSidecar(t *testing.T) {
	name := "Movie.zh.srt"
	full := filepath.Join("/media", name)
	f := &recordingFS{entries: []fs.DirEntry{fakeEntry{name: name}}, info: map[string]fs.FileInfo{full: fakeInfo{name: name}}, canonical: map[string]string{full: full}}
	first, second := 4, 9
	ctx := completeContext()
	ctx.MediaStreams = &[]domain.MediaStream{
		{Index: &first, Type: "Subtitle", Language: "zh", Path: `Movie.zh.srt`, IsExternal: boolPtr(true)},
		{Index: &second, Type: "Subtitle", Language: "zh", Path: `Movie.zh.srt`, IsExternal: boolPtr(true)},
	}
	result, err := Build(ctx, Options{FileSystem: f, IdentityKey: testKey()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Subtitles) != 1 || len(result.Subtitles[0].Indexes) != 2 || result.Subtitles[0].Indexes[0] != first || result.Subtitles[0].Indexes[1] != second {
		t.Fatalf("merged subtitles = %#v", result.Subtitles)
	}
}

func TestBuildExternalNilIndexDoesNotExposeOrdinal(t *testing.T) {
	name := "Movie.zh.srt"
	full := filepath.Join("/media", name)
	f := &recordingFS{entries: []fs.DirEntry{fakeEntry{name: name}}, info: map[string]fs.FileInfo{full: fakeInfo{name: name}}, canonical: map[string]string{full: full}}
	ctx := completeContext()
	ctx.MediaStreams = &[]domain.MediaStream{
		{Type: "Subtitle", Language: "zh", Path: name, IsExternal: boolPtr(true)},
		{Type: "Subtitle", Language: "zh-Hans", Path: name, IsExternal: boolPtr(true)},
	}
	result, err := Build(ctx, Options{FileSystem: f, IdentityKey: testKey()})
	if err != nil || len(result.Subtitles) != 1 || result.Subtitles[0].Kind != KindSidecar || len(result.Subtitles[0].Indexes) != 0 {
		t.Fatalf("nil index inventory = %#v, %v", result, err)
	}
}

func TestBuildDuplicateExternalIndexIsReportedButNotConflicting(t *testing.T) {
	idx := 5
	ctx := completeContext()
	ctx.MediaStreams = &[]domain.MediaStream{
		{Index: &idx, Type: "Subtitle", Language: "zh", Path: "missing.srt", IsExternal: boolPtr(true)},
		{Index: &idx, Type: "Subtitle", Language: "zh", Path: "missing.srt", IsExternal: boolPtr(true)},
	}
	result, err := Build(ctx, Options{FileSystem: &recordingFS{}, IdentityKey: testKey()})
	if err != nil || len(result.Subtitles) != 1 || !hasIssue(result.Issues, IssueDuplicate) {
		t.Fatalf("duplicate external = %#v, %v", result, err)
	}
}

func hasIssue(issues []Issue, code IssueCode) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestSafeBasenameFallbackRejectsDotNames(t *testing.T) {
	if safeBaseFallback(".", ".") || safeBaseFallback("..", "..") {
		t.Fatal("dot names must not be accepted for basename fallback")
	}
}

func TestBuildDoesNotUsePathBasenameFallbackForRelativeSubpaths(t *testing.T) {
	name := "Movie.zh.srt"
	full := filepath.Join("/media", name)
	f := &recordingFS{entries: []fs.DirEntry{fakeEntry{name: name}}, info: map[string]fs.FileInfo{full: fakeInfo{name: name}}, canonical: map[string]string{full: full}}
	idx := 1
	ctx := completeContext()
	ctx.MediaStreams = &[]domain.MediaStream{{Index: &idx, Type: "Subtitle", Language: "zh", Path: `subdir/Movie.zh.srt`, IsExternal: boolPtr(true)}}
	result, err := Build(ctx, Options{FileSystem: f, IdentityKey: testKey()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Subtitles) != 2 {
		t.Fatalf("subtitles = %#v", result.Subtitles)
	}
	var unmanagedExternal, manageableSidecar bool
	for _, sub := range result.Subtitles {
		if sub.Kind == KindExternal && !sub.Manageable {
			unmanagedExternal = true
		}
		if sub.Kind == KindSidecar && sub.Manageable {
			manageableSidecar = true
		}
	}
	if !unmanagedExternal || !manageableSidecar {
		t.Fatalf("subtitles = %#v", result.Subtitles)
	}
}

func TestBuildLeavesNilEmbyIndexEmpty(t *testing.T) {
	ctx := completeContext()
	ctx.MediaStreams = &[]domain.MediaStream{{Type: "Subtitle", Language: "zh"}}
	result, err := Build(ctx, Options{FileSystem: &recordingFS{}, IdentityKey: testKey()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Subtitles) != 1 || len(result.Subtitles[0].Indexes) != 0 {
		t.Fatalf("indexes = %#v", result.Subtitles)
	}
	if !strings.HasPrefix(result.Subtitles[0].ID, "sub_v1_") || strings.Contains(result.Subtitles[0].ID, "/") {
		t.Fatalf("id = %q", result.Subtitles[0].ID)
	}
}

func TestBuildDoesNotManageSymlinkOrNonRegularSidecars(t *testing.T) {
	link, directory := "Movie.link.srt", "Movie.dir.srt"
	f := &recordingFS{
		entries: []fs.DirEntry{fakeEntry{name: link, typ: fs.ModeSymlink}, fakeEntry{name: directory, typ: fs.ModeDir}},
		info: map[string]fs.FileInfo{
			filepath.Join("/media", link):      fakeInfo{name: link, mode: fs.ModeSymlink},
			filepath.Join("/media", directory): fakeInfo{name: directory, mode: fs.ModeDir},
		},
	}
	result, err := Build(completeContext(), Options{FileSystem: f, IdentityKey: testKey()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Presence != PresencePresent || len(result.Subtitles) != 2 || len(result.Issues) != 2 {
		t.Fatalf("unmanaged sidecars = %#v", result)
	}
}

var externalIndexBool = true

func TestBuildUnknownWhenScanOrStreamsAreIncomplete(t *testing.T) {
	ctx := completeContext()
	ctx.MediaStreams = nil
	f := &recordingFS{err: fs.ErrPermission}
	result, err := Build(ctx, Options{FileSystem: f, IdentityKey: testKey()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Presence != PresenceUnknown || result.Complete {
		t.Fatalf("state = %#v", result)
	}
	if len(result.Subtitles) != 0 {
		t.Fatalf("subtitles = %#v", result.Subtitles)
	}
}

func TestBuildRejectsConflictingSameSourceIndex(t *testing.T) {
	idx := 2
	ctx := completeContext()
	ctx.MediaStreams = &[]domain.MediaStream{
		{Index: &idx, Type: "Subtitle", Language: "zh"},
		{Index: &idx, Type: "Subtitle", Language: "en"},
	}
	result, err := Build(ctx, Options{FileSystem: &recordingFS{}, IdentityKey: testKey()})
	if !errors.Is(err, ErrConflictingStream) {
		t.Fatalf("error = %v, result = %#v", err, result)
	}
}

func TestBuildRejectsEmptyIdentityKey(t *testing.T) {
	if _, err := Build(completeContext(), Options{FileSystem: &recordingFS{}}); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildReportsPresentEvenWhenInventoryIsIncomplete(t *testing.T) {
	idx := 2
	ctx := completeContext()
	ctx.InventoryComplete = false
	ctx.MediaStreams = &[]domain.MediaStream{{Index: &idx, Type: "Subtitle", Language: "zh"}}
	result, err := Build(ctx, Options{FileSystem: &recordingFS{}, IdentityKey: testKey()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Presence != PresencePresent || result.Complete {
		t.Fatalf("state = %#v", result)
	}
}

func completeContext() media.MediaContext {
	empty := []domain.MediaStream{}
	return media.MediaContext{ItemID: "item", MediaSourceID: "source", LocalPath: "/media/Movie.strm", LocalDirectory: "/media", MappingStatus: media.MappingStatusMapped, MediaStreams: &empty, InventoryComplete: true}
}

func boolPtr(value bool) *bool { return &value }

func testKey() []byte { return []byte("01234567890123456789012345678901") }

func sidecarID(result Inventory, name string) string {
	for _, sub := range result.Subtitles {
		if sub.FileName == name {
			return sub.ID
		}
	}
	return ""
}
