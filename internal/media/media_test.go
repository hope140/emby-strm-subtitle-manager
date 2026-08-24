package media

import (
	"errors"
	"strings"
	"testing"

	"github.com/hope140/emby-strm-subtitle-manager/internal/domain"
	"github.com/hope140/emby-strm-subtitle-manager/internal/pathmap"
)

func TestSourceSelectorRequiresExplicitIDForMultipleSources(t *testing.T) {
	item := domain.EmbyItem{MediaSources: []domain.MediaSource{
		{ID: "one", Path: "/media/one.strm"},
		{ID: "two", Path: "/media/two.strm"},
	}}
	if _, err := SelectSource(item, ""); !errors.Is(err, ErrMediaSourceSelectionRequired) {
		t.Fatalf("auto-selection error = %v", err)
	}
	selected, err := SelectSource(item, "two")
	if err != nil || selected.ID != "two" {
		t.Fatalf("explicit selection = %#v, %v", selected, err)
	}
	if _, err := SelectSource(item, "missing"); !errors.Is(err, ErrMediaSourceNotFound) {
		t.Fatalf("missing selection error = %v", err)
	}
}

func TestBuildSingleSourceFallbackAndNilEmptyStreams(t *testing.T) {
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: `/srv/media`, Local: `/media`}})
	if err != nil {
		t.Fatal(err)
	}
	empty := []domain.MediaStream{}
	item := domain.EmbyItem{
		ItemSummary: domain.ItemSummary{ID: "movie-1", Name: "Movie", Type: "Movie"},
		Path:        `/srv/media/movie.strm`,
		MediaSources: []domain.MediaSource{{
			ID: "source-1", Container: "mkv", Protocol: "Http", Path: "https://media.example.invalid/movie.mkv?opaque=private", MediaStreams: nil,
		}},
		MediaStreams: &empty,
	}
	ctx, err := Build(item, BuildOptions{Mapper: mapper})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.EmbyPath != `/srv/media/movie.strm` || ctx.LocalPath != `/media/movie.strm` || ctx.LocalDirectory != `/media` || !ctx.IsStrm {
		t.Fatalf("unexpected context paths: %#v", ctx)
	}
	if ctx.MediaStreams == nil {
		t.Fatal("empty item stream list became nil")
	}
	if len(*ctx.MediaStreams) != 0 {
		t.Fatalf("stream list = %#v", *ctx.MediaStreams)
	}
	if ctx.MappingStatus != MappingStatusMapped || !ctx.InventoryComplete || !hasWarnings(ctx.Warnings, WarningSingleSourceStreamsFallback) {
		t.Fatalf("fallback state = %#v", ctx)
	}
}

func TestBuildUsesItemPathForInventoryAndIgnoresSourceLocator(t *testing.T) {
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: `/media`, Local: `/local`}})
	if err != nil {
		t.Fatal(err)
	}
	streams := []domain.MediaStream{}
	item := domain.EmbyItem{
		ItemSummary: domain.ItemSummary{ID: "movie-1", Name: "Movie", Type: "Movie"},
		Path:        `/media/x.strm`,
		MediaSources: []domain.MediaSource{{
			ID: "source-1", Path: `https://media.example.invalid/x.mkv?opaque=private`,
			Container: "mkv", Protocol: "Http", MediaStreams: &streams,
		}},
	}
	ctx, err := Build(item, BuildOptions{Mapper: mapper})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.EmbyPath != `/media/x.strm` || ctx.LocalPath != `/local/x.strm` || ctx.LocalDirectory != `/local` || !ctx.IsStrm || ctx.Container != "mkv" {
		t.Fatalf("item path context = %#v", ctx)
	}
	if strings.Contains(ctx.EmbyPath, "media.example") || strings.Contains(ctx.LocalPath, "media.example") {
		t.Fatal("source locator leaked into inventory paths")
	}
}

func TestBuildNonSTRMUsesLocalSourcePathAndRemoteDegrades(t *testing.T) {
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: `/media`, Local: `/local`}})
	if err != nil {
		t.Fatal(err)
	}
	streams := []domain.MediaStream{}
	item := domain.EmbyItem{
		ItemSummary:  domain.ItemSummary{ID: "movie-2", Name: "Movie", Type: "Movie"},
		Path:         `/media/item.mkv`,
		MediaSources: []domain.MediaSource{{ID: "source-1", Path: `/media/source.mkv`, MediaStreams: &streams}},
	}
	ctx, err := Build(item, BuildOptions{Mapper: mapper})
	if err != nil || ctx.EmbyPath != `/media/source.mkv` || ctx.LocalPath != `/local/source.mkv` || ctx.IsStrm {
		t.Fatalf("local non-STRM source state = %#v, %v", ctx, err)
	}
	item.MediaSources[0].Path = `https://media.example.invalid/source.mkv?opaque=private`
	ctx, err = Build(item, BuildOptions{Mapper: mapper})
	if err != nil || ctx.EmbyPath != "" || ctx.MappingStatus != MappingStatusUnavailable || ctx.InventoryComplete || !hasWarnings(ctx.Warnings, WarningMediaDirectoryUnavailable) {
		t.Fatalf("remote non-STRM source state = %#v, %v", ctx, err)
	}
	item.MediaSources[0].Path = ""
	ctx, err = Build(item, BuildOptions{Mapper: mapper})
	if err != nil || ctx.EmbyPath != `/media/item.mkv` || !hasWarnings(ctx.Warnings, WarningSourcePathFallback) {
		t.Fatalf("single local source fallback state = %#v, %v", ctx, err)
	}
}

func TestBuildSourceStreamsAreAuthoritative(t *testing.T) {
	itemStreams := []domain.MediaStream{{Type: "Subtitle"}}
	sourceStreams := []domain.MediaStream{}
	item := domain.EmbyItem{
		ItemSummary:  domain.ItemSummary{ID: "episode-1", Name: "Episode", Type: "Episode"},
		MediaStreams: &itemStreams,
		MediaSources: []domain.MediaSource{{ID: "one", Path: `https://media.example.invalid/e.mkv`, MediaStreams: &sourceStreams}},
	}
	ctx, err := Build(item, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.MediaStreams == nil || len(*ctx.MediaStreams) != 0 {
		t.Fatalf("source empty list was not authoritative: %#v", ctx.MediaStreams)
	}

	item.MediaSources = []domain.MediaSource{{ID: "one", Path: `https://media.example.invalid/e.mkv`}, {ID: "two", Path: `https://media.example.invalid/e2.mkv`}}
	ctx, err = Build(item, BuildOptions{MediaSourceID: "one"})
	if err != nil || ctx.MediaStreams == nil || len(*ctx.MediaStreams) != 0 || ctx.InventoryComplete || !hasWarnings(ctx.Warnings, WarningSourceStreamsUnavailable) {
		t.Fatalf("multi-source missing streams state = %#v, %v", ctx, err)
	}
}

func TestBuildPreservesSelectedSourceAndDoesNotExposePathInErrors(t *testing.T) {
	item := domain.EmbyItem{
		ItemSummary: domain.ItemSummary{ID: "movie-1", Name: "Movie", Type: "Movie"},
		MediaSources: []domain.MediaSource{
			{ID: "one", Path: `https://media.example.invalid/movie.mkv`, MediaStreams: pointerStreams(nil)},
			{ID: "two", Path: `https://media.example.invalid/other.mkv`, MediaStreams: pointerStreams(nil)},
		},
	}
	if _, err := Build(item, BuildOptions{}); !errors.Is(err, ErrMediaSourceSelectionRequired) {
		t.Fatalf("selection error = %v", err)
	}
	ctx, err := Build(item, BuildOptions{MediaSourceID: "one"})
	if err != nil || ctx.MediaSourceID != "one" {
		t.Fatalf("selected source = %#v, %v", ctx, err)
	}
}

func TestSourceSelectorValidatesAllSourcesBeforeSelection(t *testing.T) {
	valid := domain.MediaSource{ID: "one"}
	cases := []struct {
		name    string
		sources []domain.MediaSource
		want    error
	}{
		{name: "empty nonselected id", sources: []domain.MediaSource{valid, {ID: ""}}, want: ErrInvalidUpstreamResponse},
		{name: "duplicate nonselected id", sources: []domain.MediaSource{valid, {ID: "two"}, {ID: "two"}}, want: ErrDuplicateMediaSourceID},
		{name: "multiple defaults", sources: []domain.MediaSource{{ID: "one", IsDefault: boolPointer(true)}, {ID: "two", IsDefault: boolPointer(true)}}, want: ErrInvalidUpstreamResponse},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			item := domain.EmbyItem{MediaSources: test.sources}
			if _, err := SelectSource(item, "one"); !errors.Is(err, test.want) {
				t.Fatalf("selection error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestBuildMappingFailuresDegradeWithoutMissingInventory(t *testing.T) {
	item := domain.EmbyItem{
		ItemSummary:  domain.ItemSummary{ID: "movie-1", Name: "Movie", Type: "Movie"},
		Path:         `/unmapped/movie.strm`,
		MediaSources: []domain.MediaSource{{ID: "one", Path: `https://media.example.invalid/movie.mkv`, MediaStreams: pointerStreams(nil)}},
	}
	mapper, err := pathmap.New([]pathmap.Mapping{{Emby: `/mapped/root`, Local: `/local`}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := Build(item, BuildOptions{Mapper: mapper})
	if err != nil || ctx.MappingStatus != MappingStatusUnmapped || ctx.InventoryComplete || !hasWarnings(ctx.Warnings, WarningPathMappingUnmapped) {
		t.Fatalf("unmapped state = %#v, %v", ctx, err)
	}
	unsafeMapper, err := pathmap.New([]pathmap.Mapping{{Emby: `/srv/media`, Local: `/local`}})
	if err != nil {
		t.Fatal(err)
	}
	item.Path = `/srv/media/../secret.strm`
	ctx, err = Build(item, BuildOptions{Mapper: unsafeMapper})
	if err != nil || ctx.MappingStatus != MappingStatusUnsafe || ctx.InventoryComplete || !hasWarnings(ctx.Warnings, WarningPathMappingUnsafe) {
		t.Fatalf("unsafe state = %#v, %v", ctx, err)
	}
	item.Path = `/srv/media/movie.strm`
	ctx, err = Build(item, BuildOptions{})
	if err != nil || ctx.MappingStatus != MappingStatusUnavailable || ctx.InventoryComplete || !hasWarnings(ctx.Warnings, WarningPathMappingUnavailable) {
		t.Fatalf("unavailable state = %#v, %v", ctx, err)
	}

	guard, err := pathmap.NewPathGuard([]string{"."})
	if err != nil {
		t.Fatal(err)
	}
	guardMapper, err := pathmap.New([]pathmap.Mapping{{Emby: `/srv/media`, Local: `/outside`}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = Build(item, BuildOptions{Mapper: guardMapper, Guard: guard})
	if err != nil || ctx.MappingStatus != MappingStatusUnsafe || ctx.InventoryComplete || !hasWarnings(ctx.Warnings, WarningPathGuardUnsafe) {
		t.Fatalf("guard state = %#v, %v", ctx, err)
	}
}

func TestBuildMultiSourcePathFailuresDegradeWithoutItemPathFallback(t *testing.T) {
	item := domain.EmbyItem{
		ItemSummary: domain.ItemSummary{ID: "movie-1", Name: "Movie", Type: "Movie"},
		Path:        "",
		MediaSources: []domain.MediaSource{
			{ID: "one", Path: `https://media.example.invalid/one.mkv`, MediaStreams: pointerStreams(nil)},
			{ID: "two", Path: `https://media.example.invalid/two.mkv`, MediaStreams: pointerStreams(nil)},
		},
	}
	ctx, err := Build(item, BuildOptions{MediaSourceID: "one"})
	if err != nil || ctx.EmbyPath != "" || ctx.MappingStatus != MappingStatusUnavailable || ctx.InventoryComplete || !hasWarnings(ctx.Warnings, WarningMediaDirectoryUnavailable) {
		t.Fatalf("empty multi-source path state = %#v, %v", ctx, err)
	}
	item.Path = "/srv/media/movie\x1f.strm"
	ctx, err = Build(item, BuildOptions{MediaSourceID: "one"})
	if err != nil || ctx.MappingStatus != MappingStatusUnsafe || ctx.InventoryComplete || !hasWarnings(ctx.Warnings, WarningMediaPathUnsafe) {
		t.Fatalf("unsafe multi-source path state = %#v, %v", ctx, err)
	}
}

func hasWarnings(warnings []string, wanted ...string) bool {
	seen := make(map[string]bool, len(warnings))
	for _, warning := range warnings {
		seen[warning] = true
	}
	for _, warning := range wanted {
		if !seen[warning] {
			return false
		}
	}
	return true
}

func boolPointer(value bool) *bool { return &value }

func pointerStreams(values []domain.MediaStream) *[]domain.MediaStream { return &values }
