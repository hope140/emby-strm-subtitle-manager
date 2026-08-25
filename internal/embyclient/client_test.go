package embyclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testToken = "test-token-never-log"

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	client, err := New(Config{BaseURL: server.URL, APIKey: testToken, Timeout: time.Second, MaxResponseBody: 1024})
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	return client, server
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func TestReadEndpointsHeadersQueryAndMapping(t *testing.T) {
	var calls atomic.Int32
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("X-Emby-Token"); got != testToken {
			t.Errorf("token header = %q, want test header", got)
		}
		if r.URL.Query().Get("api_key") != "" || r.URL.Query().Get("ApiKey") != "" {
			t.Errorf("API key appeared in query: %v", r.URL.Query())
		}
		switch r.URL.Path {
		case "/Library/MediaFolders":
			if r.URL.Query().Get("IsHidden") != "false" || len(r.URL.Query()) != 1 {
				t.Errorf("library query = %v, want IsHidden=false", r.URL.Query())
			}
			writeJSON(t, w, map[string]any{"Items": []map[string]any{{"Id": "lib-1", "Name": "Movies", "CollectionType": "movies", "Locations": []string{"C:\\private"}}}, "TotalRecordCount": 1})
		case "/Items":
			if r.URL.Query().Get("Ids") == "" {
				q := r.URL.Query()
				want := url.Values{"EnableImages": {"false"}, "EnableUserData": {"false"}, "GroupItemsIntoCollections": {"false"}, "IncludeItemTypes": {"Movie,Episode"}, "Limit": {"25"}, "ParentId": {"lib-1"}, "Recursive": {"true"}, "SortBy": {"SortName"}, "SortOrder": {"Ascending"}, "StartIndex": {"50"}}
				if q.Encode() != want.Encode() {
					t.Errorf("list query = %s, want %s", q.Encode(), want.Encode())
				}
				writeJSON(t, w, map[string]any{"Items": []map[string]any{{"Id": "movie-1", "Name": "Movie", "Type": "Movie"}}, "TotalRecordCount": 1})
				return
			}
			q := r.URL.Query()
			if q.Get("Ids") != "movie-1" || q.Get("Fields") != "Path,ProviderIds,MediaStreams,MediaSources,AlternateMediaSources" || q.Get("Limit") != "2" || q.Get("EnableImages") != "false" || q.Get("EnableUserData") != "false" || len(q) != 5 {
				t.Errorf("get query = %v", q)
			}
			writeJSON(t, w, map[string]any{"Items": []map[string]any{{
				"Id": "movie-1", "Name": "Movie", "Type": "Movie", "Path": "C:\\private\\movie.strm",
				"ProviderIds":           map[string]string{"Imdb": "tt123"},
				"MediaSources":          []map[string]any{{"Id": "source-1", "Path": "C:\\private\\movie.strm", "Protocol": "file", "Container": "strm", "Default": false, "MediaStreams": []map[string]any{{"Index": 3, "Type": "Subtitle", "Title": "中文", "DisplayLanguage": "Chinese", "IsTextSubtitleStream": true, "DeliveryMethod": "External", "Protocol": "http"}}}},
				"AlternateMediaSources": []map[string]any{{"Id": "source-2", "Name": "Alternate", "Path": "C:\\private\\movie-alt.strm", "Protocol": "file", "Container": "strm", "Default": false}},
				"MediaStreams":          []map[string]any{{"Index": 2, "Type": "Subtitle", "IsExternal": false, "IsForced": false, "IsDefault": true}},
			}}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	libraries, err := client.ListLibraries(context.Background())
	if err != nil || len(libraries) != 1 || libraries[0].ID != "lib-1" || libraries[0].Name != "Movies" {
		t.Fatalf("libraries = %#v, err=%v", libraries, err)
	}
	items, err := client.ListItems(context.Background(), "lib-1", 50, 25)
	if err != nil || len(items.Items) != 1 || items.Items[0].ID != "movie-1" || items.TotalRecordCount != 1 || items.StartIndex != 50 || items.Limit != 25 || items.HasMore {
		t.Fatalf("items = %#v, err=%v", items, err)
	}
	item, err := client.GetItem(context.Background(), "movie-1")
	if err != nil || item.Path == "" || item.ProviderIDs["Imdb"] != "tt123" || len(item.MediaSources) != 2 || item.MediaSources[0].Protocol != "file" || item.MediaSources[0].MediaStreams == nil || len(*item.MediaSources[0].MediaStreams) != 1 || (*item.MediaSources[0].MediaStreams)[0].Title != "中文" || item.MediaSources[1].ID != "source-2" || item.MediaStreams == nil || len(*item.MediaStreams) != 1 {
		t.Fatalf("item = %#v, err=%v", item, err)
	}
	if calls.Load() != 3 {
		t.Fatalf("request count = %d, want 3", calls.Load())
	}
}

func TestListItemsValidation(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid input made a request")
	}))
	for _, test := range []struct {
		name       string
		libraryID  string
		startIndex int
		limit      int
	}{
		{"empty library", "", 0, 1},
		{"negative start", "lib", -1, 1},
		{"zero limit", "lib", 0, 0},
		{"large limit", "lib", 0, 201},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.ListItems(context.Background(), test.libraryID, test.startIndex, test.limit)
			assertKind(t, err, ErrInvalidInput)
		})
	}
}

func TestListAndGetItemCardinality(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				items := make([]map[string]any, count)
				for i := range items {
					items[i] = map[string]any{"Id": strconv.Itoa(i + 1), "Name": "item", "Type": "Movie"}
				}
				writeJSON(t, w, map[string]any{"Items": items, "TotalRecordCount": count})
			}))
			items, err := client.ListItems(context.Background(), "lib", 0, 1)
			if err != nil || len(items.Items) != count {
				t.Fatalf("ListItems = %#v, err=%v", items, err)
			}
			_, err = client.GetItem(context.Background(), "item")
			if count == 0 {
				assertKind(t, err, ErrNotFound)
			} else if count == 1 {
				if err != nil {
					t.Fatalf("GetItem error = %v", err)
				}
			} else {
				assertKind(t, err, ErrInvalidResponse)
			}
		})
	}
}

func TestHTTPStatusErrorsAreTypedAndDoNotLeakBody(t *testing.T) {
	const secretBody = "body-secret-path-and-token"
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, secretBody, status)
			}))
			_, err := client.ListLibraries(context.Background())
			assertKind(t, err, ErrHTTP)
			if !strings.Contains(err.Error(), strconv.Itoa(status)) || strings.Contains(err.Error(), secretBody) || strings.Contains(err.Error(), testToken) || strings.Contains(err.Error(), "/Library") {
				t.Fatalf("unsafe error = %q", err)
			}
		})
	}
}

func TestRedirectIsRejectedWithoutForwardingToken(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		if r.Header.Get("X-Emby-Token") != "" {
			t.Error("token was forwarded to redirect target")
		}
	}))
	defer target.Close()
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	_, err := client.ListLibraries(context.Background())
	assertKind(t, err, ErrRedirect)
	if redirected.Load() != 0 {
		t.Fatal("redirect target received a request")
	}
}

func TestTimeoutMalformedAndOversizedBody(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := client.ListLibraries(ctx)
	assertKind(t, err, ErrTimeout)

	badClient, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{")) }))
	_, err = badClient.ListLibraries(context.Background())
	assertKind(t, err, ErrMalformedJSON)

	largeClient, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", 1100))) }))
	_, err = largeClient.ListLibraries(context.Background())
	assertKind(t, err, ErrResponseTooLarge)
}

func TestDTOMissingFieldsRemainDistinguishable(t *testing.T) {
	var empty itemDTO
	var emptyStream mediaStreamDTO
	if empty.ParentIndexNumber != nil || emptyStream.IsExternal != nil {
		t.Fatal("missing pointer fields were not nil")
	}
	var decoded itemDTO
	if err := json.Unmarshal([]byte(`{"IndexNumber":0,"MediaStreams":[],"MediaSources":[]}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.IndexNumber == nil || *decoded.IndexNumber != 0 || decoded.MediaStreams == nil || decoded.MediaSources == nil {
		t.Fatalf("decoded pointers lost zero/empty distinction: %#v", decoded)
	}
}

func TestDTOCombinesAlternateMediaSourcesAndDeduplicatesIDs(t *testing.T) {
	var decoded itemDTO
	if err := json.Unmarshal([]byte(`{"Id":"item","Name":"Movie","Type":"Movie","MediaSources":[{"Id":"source-1","Name":"primary"},{"Id":"","Name":"unnamed"}],"AlternateMediaSources":[{"Id":"source-1","Name":"duplicate"},{"Id":"source-2","Name":"alternate"}]}`), &decoded); err != nil {
		t.Fatal(err)
	}
	item := decoded.toDomain()
	if len(item.MediaSources) != 3 {
		t.Fatalf("MediaSources = %#v, want primary, unnamed and alternate", item.MediaSources)
	}
	if item.MediaSources[0].ID != "source-1" || item.MediaSources[1].ID != "" || item.MediaSources[2].ID != "source-2" {
		t.Fatalf("MediaSources order/IDs = %#v", item.MediaSources)
	}
	if item.MediaSources[0].Name != "primary" {
		t.Fatalf("primary source was replaced by duplicate: %#v", item.MediaSources[0])
	}
}

func TestDTOPreservesDuplicateIDsWithinPrimarySources(t *testing.T) {
	var decoded itemDTO
	if err := json.Unmarshal([]byte(`{"Id":"item","Name":"Movie","Type":"Movie","MediaSources":[{"Id":"source-1"},{"Id":"source-1"}]}`), &decoded); err != nil {
		t.Fatal(err)
	}
	item := decoded.toDomain()
	if len(item.MediaSources) != 2 || item.MediaSources[0].ID != "source-1" || item.MediaSources[1].ID != "source-1" {
		t.Fatalf("duplicate primary sources were hidden: %#v", item.MediaSources)
	}
}

func TestDTOPreservesDuplicateIDsWithinAlternateSources(t *testing.T) {
	var decoded itemDTO
	if err := json.Unmarshal([]byte(`{"Id":"item","Name":"Movie","Type":"Movie","AlternateMediaSources":[{"Id":"source-2"},{"Id":"source-2"}]}`), &decoded); err != nil {
		t.Fatal(err)
	}
	item := decoded.toDomain()
	if len(item.MediaSources) != 2 || item.MediaSources[0].ID != "source-2" || item.MediaSources[1].ID != "source-2" {
		t.Fatalf("duplicate alternate sources were hidden: %#v", item.MediaSources)
	}
}

func TestStrictCredentialIDsAndResponseShapes(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	for _, key := range []string{" leading", "trailing ", "has space", "line\nfeed", "tab\tvalue"} {
		if _, err := New(Config{BaseURL: server.URL, APIKey: key}); err == nil {
			t.Fatalf("New accepted whitespace API key %q", key)
		}
	}
	var calls atomic.Int32
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(t, w, map[string]any{"Items": []map[string]any{{"Id": "item", "Name": "Name", "Type": "Series"}}, "TotalRecordCount": 1})
	}))
	if _, err := client.ListItems(context.Background(), "  lib  ", 0, 1); err == nil {
		t.Fatal("ListItems accepted a non-Movie/Episode type")
	}
	calls.Store(0)
	_, err := client.ListItems(context.Background(), "lib\x00", 0, 1)
	assertKind(t, err, ErrInvalidInput)
	if calls.Load() != 0 {
		t.Fatal("ListItems sent a request for a control-character ID")
	}
	_, err = client.GetItem(context.Background(), "item\n")
	assertKind(t, err, ErrInvalidInput)
	if calls.Load() != 0 {
		t.Fatal("GetItem sent a request for a control-character ID")
	}
}

func TestHasMoreRequiresFullPage(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"Items":            []map[string]any{{"Id": "movie", "Name": "Movie", "Type": "Movie"}},
			"TotalRecordCount": 10,
		})
	}))
	page, err := client.ListItems(context.Background(), "lib", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore {
		t.Fatal("HasMore=true for a short upstream page")
	}
}

func TestPagingTotalIsRequiredAndNonNegative(t *testing.T) {
	for _, response := range []string{`{"Items":[]}`, `{"Items":[],"TotalRecordCount":-1}`} {
		client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(response)) }))
		_, err := client.ListItems(context.Background(), "lib", 0, 1)
		assertKind(t, err, ErrInvalidResponse)
	}
}

func assertKind(t *testing.T, err error, want ClientErrorKind) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	var typed *Error
	if !errors.As(err, &typed) || typed.Kind != want {
		t.Fatalf("error = %#v, want kind %s", err, want)
	}
}
