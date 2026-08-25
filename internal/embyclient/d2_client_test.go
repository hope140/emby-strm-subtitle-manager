package embyclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func TestRemoteSubtitleEndpointsAreFixedReadOnlyGETs(t *testing.T) {
	var searchQuery url.Values
	var fetchCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("X-Emby-Token") != testToken {
			t.Fatalf("upstream request = %s token=%q", r.Method, r.Header.Get("X-Emby-Token"))
		}
		if r.URL.Query().Get("api_key") != "" {
			t.Fatal("API key appeared in query")
		}
		switch r.URL.Path {
		case "/Items/movie-1/RemoteSearch/Subtitles/zh-CN":
			searchQuery = r.URL.Query()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"Id":"server-id","ProviderName":"Thunder","Name":"name","Language":"zho","Format":"srt","Comment":"comment","IsHashMatch":true}]`))
		case "/Providers/Subtitles/Subtitles/server-id":
			fetchCalls++
			_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\n你好\n"))
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, APIKey: testToken, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	items, err := client.SearchRemoteSubtitles(context.Background(), "movie-1", "zh-CN", "source-1", true)
	if err != nil || len(items) != 1 || items[0].ID != "server-id" {
		t.Fatalf("search = %#v, %v", items, err)
	}
	want := url.Values{"MediaSourceId": {"source-1"}, "IsForced": {"true"}, "IsPerfectMatch": {"false"}, "IsHearingImpaired": {"false"}}
	if !reflect.DeepEqual(searchQuery, want) {
		t.Fatalf("search query = %#v, want %#v", searchQuery, want)
	}
	body, err := client.FetchRemoteSubtitle(context.Background(), "server-id")
	if err != nil || string(body) == "" || fetchCalls != 1 {
		t.Fatalf("fetch = %q, %v calls=%d", body, err, fetchCalls)
	}
}

func TestGetItemPreservesUnsupportedTypeForD2Gate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Items":[{"Id":"item","Name":"Series","Type":"Series","MediaSources":[]}]}`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, APIKey: testToken, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	item, err := client.GetItem(context.Background(), "item")
	if err != nil || item.Type != "Series" {
		t.Fatalf("GetItem = %#v, %v", item, err)
	}
}
