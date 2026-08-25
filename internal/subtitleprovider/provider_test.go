package subtitleprovider

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/hope140/emby-strm-subtitle-manager/internal/domain"
	"github.com/hope140/emby-strm-subtitle-manager/internal/embyclient"
)

type fakeRemoteClient struct {
	search      []domain.RemoteSubtitleInfo
	fetchBodies [][]byte
	fetchErrors []error
	fetchCalls  int
}

func (f *fakeRemoteClient) SearchRemoteSubtitles(context.Context, string, string, string, bool) ([]domain.RemoteSubtitleInfo, error) {
	return f.search, nil
}

func (f *fakeRemoteClient) FetchRemoteSubtitle(context.Context, string) ([]byte, error) {
	index := f.fetchCalls
	f.fetchCalls++
	if index < len(f.fetchErrors) && f.fetchErrors[index] != nil {
		return nil, f.fetchErrors[index]
	}
	if index < len(f.fetchBodies) {
		return f.fetchBodies[index], nil
	}
	return nil, errors.New("missing fixture")
}

func TestProviderProjectsFieldsAndRetriesOnlyTemporaryFetches(t *testing.T) {
	client := &fakeRemoteClient{search: []domain.RemoteSubtitleInfo{{
		ID: "server-only", Provider: string(make([]byte, 80)), Name: string(make([]byte, 600)), Language: "zho", Format: ".srt", Comment: "comment", Score: math.NaN(), Reasons: []string{"reason"},
	}}}
	provider := NewEmbyRemoteSubtitleProvider(client)
	items, err := provider.Search(context.Background(), "movie", "source", "zh-CN", false)
	if err != nil || len(items) != 1 {
		t.Fatalf("Search = %#v, %v", items, err)
	}
	if len(items[0].Provider) > MaxProviderNameBytes || len(items[0].Name) > MaxProviderTextBytes || items[0].Language != "zho" || items[0].Format != "srt" || items[0].Score != 0 {
		t.Fatalf("projected candidate = %#v", items[0])
	}
	client.fetchErrors = []error{&embyclient.Error{Kind: embyclient.ErrHTTP, Status: 429}}
	client.fetchBodies = [][]byte{nil, []byte("valid")}
	provider.sleep = func(context.Context, time.Duration) error { return nil }
	result, err := provider.Fetch(context.Background(), "server-only")
	if err != nil || result.Attempts != 2 || client.fetchCalls != 2 {
		t.Fatalf("retry result = %#v err=%v calls=%d", result, err, client.fetchCalls)
	}
}

func TestProviderDoesNotRetryCandidate500(t *testing.T) {
	client := &fakeRemoteClient{fetchErrors: []error{&embyclient.Error{Kind: embyclient.ErrHTTP, Status: 500}}}
	provider := NewEmbyRemoteSubtitleProvider(client)
	result, err := provider.Fetch(context.Background(), "server-only")
	if err == nil || result.Attempts != 1 || client.fetchCalls != 1 {
		t.Fatalf("500 retry result = %#v err=%v calls=%d", result, err, client.fetchCalls)
	}
}
