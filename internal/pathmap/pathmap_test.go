package pathmap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMapperLongestPrefixAndBoundary(t *testing.T) {
	mapper, err := New([]Mapping{
		{Emby: "/srv/media", Local: "/media"},
		{Emby: "/srv/media/anime", Local: "/anime"},
	})
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{
		"/srv/media/movie.strm":        "/media/movie.strm",
		"/srv/media/anime/show.strm":   "/anime/show.strm",
		"/srv/medial/animation/a.strm": "",
	}
	for input, want := range checks {
		got, err := mapper.Map(input)
		if want == "" {
			if !errors.Is(err, ErrPathNotMapped) {
				t.Errorf("Map(%q) error = %v, want not mapped", input, err)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("Map(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestMapperWindowsCaseInsensitiveAndUNC(t *testing.T) {
	mapper, err := New([]Mapping{
		{Emby: `C:\Media`, Local: `/media`},
		{Emby: `\\NAS\Share`, Local: `D:\mapped`},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := mapper.Map(`c:/media/Film\Movie.strm`)
	if err != nil || got != `/media/Film/Movie.strm` {
		t.Fatalf("drive mapping = %q, %v", got, err)
	}
	got, err = mapper.Map(`//nas/share/sub/movie.strm`)
	if err != nil || got != `d:\mapped\sub\movie.strm` {
		t.Fatalf("UNC mapping = %q, %v", got, err)
	}
}

func TestMapperRejectsTraversalDeviceAndADS(t *testing.T) {
	mapper, err := New([]Mapping{{Emby: `/srv/media`, Local: `/media`}})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`/srv/media/../secret`,
		`/srv/media/./movie.strm`,
		"/srv/media/movie\x1f.strm",
		`C:\srv\media\movie`,
		`\\?\C:\srv\media\movie`,
		`\\.\PhysicalDrive0`,
	} {
		if _, err := mapper.Map(input); !errors.Is(err, ErrInvalidPath) && !errors.Is(err, ErrPathNotMapped) {
			t.Errorf("Map(%q) error = %v, want safe rejection", input, err)
		}
	}
	windowsMapper, err := New([]Mapping{{Emby: `C:\srv\media`, Local: `/media`}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := windowsMapper.Map(`C:\srv\media\file:stream`); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("ADS path error = %v", err)
	}
	if _, err := New([]Mapping{{Emby: `C:\media`, Local: `C:\local:stream`}}); !errors.Is(err, ErrAmbiguousMapping) {
		t.Errorf("ADS mapping error = %v, want ambiguous mapping", err)
	}
	for _, input := range []string{`C:relative\movie`, `\relative\movie`, `\\server`, `\\\share`} {
		if _, err := mapper.Map(input); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("non-absolute path %q error = %v", input, err)
		}
	}
}

func TestMapperMixedWindowsSeparatorsAndUNCRootValidation(t *testing.T) {
	mapper, err := New([]Mapping{{Emby: `C:\Media\Anime`, Local: `/anime`}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := mapper.Map(`c:/MEDIA\anime/Show\Episode.strm`)
	if err != nil || got != `/anime/Show/Episode.strm` {
		t.Fatalf("mixed separator mapping = %q, %v", got, err)
	}
	for _, mapping := range []Mapping{
		{Emby: `\\server`, Local: `/media`},
		{Emby: `\\\share`, Local: `/media`},
	} {
		if _, err := New([]Mapping{mapping}); err == nil {
			t.Errorf("mapping %q unexpectedly accepted", mapping.Emby)
		}
	}
	if _, err := New([]Mapping{{Emby: `\\server\share`, Local: `/media`}}); err != nil {
		t.Fatalf("valid UNC root rejected: %v", err)
	}
}

func TestMapperRejectsDuplicateSource(t *testing.T) {
	if _, err := New([]Mapping{
		{Emby: `C:\Media`, Local: `/one`},
		{Emby: `c:/media`, Local: `/two`},
	}); !errors.Is(err, ErrAmbiguousMapping) {
		t.Fatalf("duplicate source error = %v", err)
	}
}

func TestPathGuardContainmentAndSymlinkEscape(t *testing.T) {
	root := testWorkspaceTempDir(t)
	inside := filepath.Join(root, "inside")
	outside := testWorkspaceTempDir(t)
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	guard, err := NewPathGuard([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.CheckDirectory(inside); err != nil {
		t.Fatalf("inside check = %v", err)
	}
	if err := guard.CheckDirectory(outside); !errors.Is(err, ErrGuardOutsideRoot) {
		t.Fatalf("outside check = %v", err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := guard.CheckDirectory(link); !errors.Is(err, ErrGuardOutsideRoot) {
		t.Fatalf("symlink escape check = %v", err)
	}
}

func TestPathGuardRequiresExistingDirectory(t *testing.T) {
	root := testWorkspaceTempDir(t)
	guard, err := NewPathGuard([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.CheckDirectory(filepath.Join(root, "missing")); !errors.Is(err, ErrGuardOutsideRoot) {
		t.Fatalf("missing check = %v", err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := guard.CheckDirectory(file); !errors.Is(err, ErrGuardOutsideRoot) {
		t.Fatalf("file check = %v", err)
	}
}

func testWorkspaceTempDir(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp(".", "pathguard-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return directory
}
