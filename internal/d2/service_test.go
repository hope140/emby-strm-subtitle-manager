package d2

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hope140/subbridge/internal/config"
	"github.com/hope140/subbridge/internal/domain"
	"github.com/hope140/subbridge/internal/preview"
	"github.com/hope140/subbridge/internal/subtitleprovider"
)

type serviceFakeEmby struct {
	item  domain.EmbyItem
	calls atomic.Int32
}

func (f *serviceFakeEmby) GetItem(context.Context, string) (domain.EmbyItem, error) {
	f.calls.Add(1)
	return f.item, nil
}

type serviceFakeProvider struct {
	searchItems []subtitleprovider.Candidate
	fetch       map[string]subtitleprovider.FetchResult
	fetchErr    map[string]error
	searchCalls atomic.Int32
	fetchCalls  atomic.Int32
}

func (f *serviceFakeProvider) Search(context.Context, string, string, string, bool) ([]subtitleprovider.Candidate, error) {
	f.searchCalls.Add(1)
	return append([]subtitleprovider.Candidate(nil), f.searchItems...), nil
}

func (f *serviceFakeProvider) Fetch(_ context.Context, rawID string) (subtitleprovider.FetchResult, error) {
	f.fetchCalls.Add(1)
	if err := f.fetchErr[rawID]; err != nil {
		return subtitleprovider.FetchResult{Attempts: 1}, err
	}
	return f.fetch[rawID], nil
}

func enabledService(t *testing.T, fake *serviceFakeEmby, provider *serviceFakeProvider) (*Service, *preview.Allowlist) {
	t.Helper()
	allowlist := preview.NewAllowlist([]string{"movie-1"})
	artifactStore, err := preview.NewArtifactStore(preview.ArtifactStoreOptions{Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(Options{
		Config: config.D2Config{DefaultLanguage: "zh-CN"}, RemoteSearchEnabled: true, CanaryEnabled: true,
		Allowlist: allowlist, Emby: fake, Provider: provider, ArtifactStore: artifactStore, AuthContext: "test-auth",
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, allowlist
}

func TestD2EnabledWithoutInjectedArtifactStoreRequiresCacheDirectory(t *testing.T) {
	_, err := New(Options{
		Config: config.D2Config{}, RemoteSearchEnabled: true, CanaryEnabled: true,
		Allowlist: preview.NewAllowlist([]string{"movie-1"}),
	})
	if err == nil || !strings.Contains(err.Error(), "d2.cache_dir") {
		t.Fatalf("New() = %v, want explicit cache_dir error", err)
	}
}

func singleMovie() domain.EmbyItem {
	return domain.EmbyItem{ItemSummary: domain.ItemSummary{ID: "movie-1", Name: "Movie", Type: "Movie"}, MediaSources: []domain.MediaSource{{ID: "source-1"}}}
}

func TestD2DisabledFailsBeforeEmbyOrProvider(t *testing.T) {
	fake := &serviceFakeEmby{item: singleMovie()}
	provider := &serviceFakeProvider{}
	service, err := New(Options{RemoteSearchEnabled: false, CanaryEnabled: false, Emby: fake, Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Search(context.Background(), "movie-1", SearchRequest{}); !hasD2Code(err, "remote_search_disabled") {
		t.Fatalf("Search error = %v", err)
	}
	if _, err := service.Fetch(context.Background(), "movie-1", FetchRequest{CandidateToken: "x"}); !hasD2Code(err, "remote_search_disabled") {
		t.Fatalf("Fetch error = %v", err)
	}
	if _, err := service.Preview(context.Background(), "movie-1", PreviewRequest{ArtifactToken: "x"}); !hasD2Code(err, "remote_search_disabled") {
		t.Fatalf("Preview error = %v", err)
	}
	if fake.calls.Load() != 0 || provider.searchCalls.Load() != 0 || provider.fetchCalls.Load() != 0 {
		t.Fatalf("disabled path made calls: item=%d search=%d fetch=%d", fake.calls.Load(), provider.searchCalls.Load(), provider.fetchCalls.Load())
	}
}

func TestD2MultisourceFailsClosedForAllOperations(t *testing.T) {
	item := singleMovie()
	item.MediaSources = []domain.MediaSource{{ID: "source-a"}, {ID: "source-b"}}
	fake := &serviceFakeEmby{item: item}
	provider := &serviceFakeProvider{}
	service, _ := enabledService(t, fake, provider)
	if _, err := service.Search(context.Background(), "movie-1", SearchRequest{MediaSourceID: "source-b"}); !hasD2Code(err, "d2_multisource_unsupported") {
		t.Fatalf("Search error = %v", err)
	}
	if _, err := service.Fetch(context.Background(), "movie-1", FetchRequest{CandidateToken: "opaque"}); !hasD2Code(err, "d2_multisource_unsupported") {
		t.Fatalf("Fetch error = %v", err)
	}
	if _, err := service.Preview(context.Background(), "movie-1", PreviewRequest{ArtifactToken: "opaque"}); !hasD2Code(err, "d2_multisource_unsupported") {
		t.Fatalf("Preview error = %v", err)
	}
	if provider.searchCalls.Load() != 0 || provider.fetchCalls.Load() != 0 {
		t.Fatal("multi-source path reached provider")
	}
}

func TestD2CanaryAllowlistRejectsItemBeforeProvider(t *testing.T) {
	fake := &serviceFakeEmby{item: singleMovie()}
	provider := &serviceFakeProvider{searchItems: []subtitleprovider.Candidate{{RawID: "candidate"}}}
	artifactStore, err := preview.NewArtifactStore(preview.ArtifactStoreOptions{Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(Options{
		Config: config.D2Config{}, RemoteSearchEnabled: true, CanaryEnabled: true,
		Allowlist: preview.NewAllowlist([]string{"other-item"}), Emby: fake, Provider: provider,
		ArtifactStore: artifactStore, AuthContext: "test-auth",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Search(context.Background(), "movie-1", SearchRequest{}); !hasD2Code(err, "canary_item_not_allowed") {
		t.Fatalf("allowlist error = %v", err)
	}
	if provider.searchCalls.Load() != 0 {
		t.Fatal("disallowed item reached Provider")
	}
}

func TestD2CandidateFailureIsolationFetchIdempotencyAndPreviewBinding(t *testing.T) {
	valid := []byte("1\n00:00:01,000 --> 00:00:02,000\n成功\n")
	fake := &serviceFakeEmby{item: singleMovie()}
	provider := &serviceFakeProvider{
		searchItems: []subtitleprovider.Candidate{{RawID: "candidate-a", Provider: "Thunder", Language: "zho", Format: "srt"}, {RawID: "candidate-b", Provider: "ASSRT", Language: "zho", Format: "srt"}},
		fetchErr:    map[string]error{"candidate-a": errors.New("candidate failed")},
		fetch:       map[string]subtitleprovider.FetchResult{"candidate-b": {Content: valid, Attempts: 1}},
	}
	service, allowlist := enabledService(t, fake, provider)
	search, err := service.Search(context.Background(), "movie-1", SearchRequest{})
	if err != nil || len(search.Candidates) != 2 {
		t.Fatalf("Search = %#v, %v", search, err)
	}
	if search.Language != "zh-CN" || search.Candidates[1].Language != "zho" {
		t.Fatalf("language projection = response=%q candidate=%q", search.Language, search.Candidates[1].Language)
	}
	first, err := service.Fetch(context.Background(), "movie-1", FetchRequest{CandidateToken: search.Candidates[0].Token})
	if !hasD2Code(err, "candidate_fetch_failed") || first.ArtifactToken != "" {
		t.Fatalf("failed candidate = %#v, %v", first, err)
	}
	second, err := service.Fetch(context.Background(), "movie-1", FetchRequest{CandidateToken: search.Candidates[1].Token})
	if err != nil || second.ArtifactToken == "" || second.Language != "zh-CN" {
		t.Fatalf("successful candidate = %#v, %v", second, err)
	}
	_, generation := allowlist.Allows("movie-1")
	artifact, err := service.artifacts.Get(second.ArtifactToken, preview.Binding{ItemID: "movie-1", SourceID: "source-1", Language: "zh-CN", AuthContext: "test-auth", AllowlistGeneration: generation})
	if err != nil || artifact.Language != "zh-CN" || artifact.Binding.Language != "zh-CN" {
		t.Fatalf("artifact language binding = %#v, %v", artifact, err)
	}
	fetchCalls := provider.fetchCalls.Load()
	replay, err := service.Fetch(context.Background(), "movie-1", FetchRequest{CandidateToken: search.Candidates[1].Token})
	if err != nil || replay.ArtifactToken != second.ArtifactToken || provider.fetchCalls.Load() != fetchCalls {
		t.Fatalf("idempotent replay = %#v, %v calls=%d/%d", replay, err, provider.fetchCalls.Load(), fetchCalls)
	}
	previewResponse, err := service.Preview(context.Background(), "movie-1", PreviewRequest{ArtifactToken: second.ArtifactToken, Offset: 0, Limit: 200})
	if err != nil || previewResponse.Language != "zh-CN" || len(previewResponse.Cues) != 1 || previewResponse.Cues[0].Text != "成功" {
		t.Fatalf("Preview = %#v, %v", previewResponse, err)
	}
	oldGeneration := allowlist.Replace([]string{"movie-1"})
	if oldGeneration == 0 {
		t.Fatal("allowlist generation was empty")
	}
	if _, err := service.Fetch(context.Background(), "movie-1", FetchRequest{CandidateToken: search.Candidates[1].Token}); !hasD2Code(err, "candidate_invalid") {
		t.Fatalf("candidate generation change error = %v", err)
	}
	if _, err := service.Preview(context.Background(), "movie-1", PreviewRequest{ArtifactToken: second.ArtifactToken}); !hasD2Code(err, "artifact_invalid") {
		t.Fatalf("artifact generation change error = %v", err)
	}
}

func hasD2Code(err error, code string) bool {
	var typed *Error
	return errors.As(err, &typed) && typed.Code == code
}
