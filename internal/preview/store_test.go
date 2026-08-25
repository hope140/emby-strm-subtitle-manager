package preview

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hope140/emby-strm-subtitle-manager/internal/subtitle"
)

func TestCandidateStoreRandomBindingTTLAndOneShot410(t *testing.T) {
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	store := NewCandidateStore(CandidateStoreOptions{TTL: time.Minute, Now: func() time.Time { return now }})
	binding := Binding{ItemID: "movie-1", SourceID: "source-1", Language: "zh-CN", AuthContext: "auth", AllowlistGeneration: 7}
	items, err := store.IssueMany(binding, []CandidateInput{{RawID: "server-only", Provider: "Thunder", Format: "srt"}})
	if err != nil || len(items) != 1 {
		t.Fatalf("IssueMany = %#v, %v", items, err)
	}
	if len(items[0].Token) < 40 || items[0].RawID != "server-only" {
		t.Fatalf("candidate = %#v", items[0])
	}
	if _, err := store.Resolve(items[0].Token, Binding{ItemID: "other", SourceID: "source-1", Language: "zh-CN", AuthContext: "auth", AllowlistGeneration: 7}); !errors.Is(err, ErrCandidateInvalid) {
		t.Fatalf("wrong binding error = %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := store.Resolve(items[0].Token, binding); !errors.Is(err, ErrCandidateExpired) {
		t.Fatalf("first expired lookup = %v", err)
	}
	if _, err := store.Resolve(items[0].Token, binding); !errors.Is(err, ErrCandidateInvalid) {
		t.Fatalf("second expired lookup = %v", err)
	}
}

func TestArtifactStorePrivateFileIdempotentReadExpiryAndGeneration(t *testing.T) {
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	store, err := NewArtifactStore(ArtifactStoreOptions{Directory: filepath.Join(dir, "cache"), TTL: time.Minute, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	binding := Binding{ItemID: "movie-1", SourceID: "source-1", Language: "zh-CN", AuthContext: "auth", AllowlistGeneration: 3}
	document, err := subtitle.ValidateAndParse([]byte("1\n00:00:01,000 --> 00:00:02,000\npreview\n"), "srt", subtitle.DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.Create(binding, document.Format, "zho", document.Canonical, document.Cues)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Language != binding.Language || artifact.Binding.Language != binding.Language {
		t.Fatalf("artifact language = %q binding=%q, want canonical binding language", artifact.Language, artifact.Binding.Language)
	}
	files, err := os.ReadDir(filepath.Join(dir, "cache"))
	if err != nil || len(files) != 1 || filepath.Ext(files[0].Name()) != ".subtitle" {
		t.Fatalf("cache files = %#v, %v", files, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := files[0].Info(); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("artifact mode = %v, %v", info.Mode().Perm(), err)
		}
		if info, err := os.Stat(filepath.Join(dir, "cache")); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("cache directory mode = %v, %v", info.Mode().Perm(), err)
		}
	}
	got, err := store.Get(artifact.Token, Binding{ItemID: binding.ItemID, SourceID: binding.SourceID, AuthContext: binding.AuthContext, AllowlistGeneration: binding.AllowlistGeneration})
	if err != nil || got.ContentHash != artifact.ContentHash || got.Language != binding.Language || got.Binding.Language != binding.Language || len(got.Cues) != 1 {
		t.Fatalf("Get = %#v, %v", got, err)
	}
	if _, err := store.Get(artifact.Token, Binding{ItemID: "other", SourceID: binding.SourceID, AuthContext: binding.AuthContext, AllowlistGeneration: binding.AllowlistGeneration}); !errors.Is(err, ErrArtifactInvalid) {
		t.Fatalf("wrong artifact binding = %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := store.Get(artifact.Token, binding); !errors.Is(err, ErrArtifactExpired) {
		t.Fatalf("first artifact expiry = %v", err)
	}
	if _, err := store.Get(artifact.Token, binding); !errors.Is(err, ErrArtifactInvalid) {
		t.Fatalf("second artifact expiry = %v", err)
	}
}

func TestArtifactStoreRestartReclaimsOldArtifacts(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "stable-cache")
	binding := Binding{ItemID: "movie-1", SourceID: "source-1", Language: "zh-CN", AuthContext: "auth", AllowlistGeneration: 1}
	store, err := NewArtifactStore(ArtifactStoreOptions{Directory: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.Create(binding, "srt", binding.Language, []byte("1\n00:00:01,000 --> 00:00:02,000\nold\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if files, err := os.ReadDir(cacheDir); err != nil || len(files) != 1 {
		t.Fatalf("initial cache files = %#v, %v", files, err)
	}

	restarted, err := NewArtifactStore(ArtifactStoreOptions{Directory: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("restart left artifact files = %#v", files)
	}
	if _, err := restarted.Get(artifact.Token, binding); !errors.Is(err, ErrArtifactInvalid) {
		t.Fatalf("restarted Get() = %v, want no tombstone invalid result", err)
	}
}

func TestArtifactStoreRejectsRootAndRelativeDirectories(t *testing.T) {
	root := string(filepath.Separator)
	if volume := filepath.VolumeName(t.TempDir()); volume != "" {
		root = volume + string(filepath.Separator)
	}
	if _, err := NewArtifactStore(ArtifactStoreOptions{Directory: root}); !errors.Is(err, ErrArtifactUnavailable) {
		t.Fatalf("root store error = %v, want unavailable", err)
	}
	if _, err := NewArtifactStore(ArtifactStoreOptions{Directory: "relative-cache"}); !errors.Is(err, ErrArtifactUnavailable) {
		t.Fatalf("relative store error = %v, want unavailable", err)
	}
}

func TestArtifactStoreFailsClosedWhenPrivateDirectoryPermissionCannotBeConfirmed(t *testing.T) {
	called := false
	store, err := newArtifactStore(ArtifactStoreOptions{Directory: filepath.Join(t.TempDir(), "cache")}, func(_ string, mode fs.FileMode) error {
		called = true
		if mode != 0o700 {
			t.Fatalf("chmod mode = %v, want 0700", mode)
		}
		return fs.ErrPermission
	})
	if !called {
		t.Fatal("private directory permission gate was not checked")
	}
	if store != nil || !errors.Is(err, ErrArtifactUnavailable) {
		t.Fatalf("permission failure = store=%#v err=%v, want unavailable", store, err)
	}
}

func TestAllowlistGenerationChangesOnReplace(t *testing.T) {
	allowlist := NewAllowlist([]string{"movie-1"})
	ok, first := allowlist.Allows("movie-1")
	if !ok || first == 0 {
		t.Fatalf("initial allowlist = %v, %d", ok, first)
	}
	second := allowlist.Replace([]string{"movie-2"})
	if second == first {
		t.Fatal("allowlist generation did not change")
	}
	if ok, generation := allowlist.Allows("movie-1"); ok || generation != second {
		t.Fatalf("replaced allowlist = %v, %d", ok, generation)
	}
}
