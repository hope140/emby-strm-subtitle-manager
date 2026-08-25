// Package embyclient implements the deliberately bounded Emby API facade
// used by D1, D2 and the gated D3 Add flow. Write access is limited to the
// item Refresh endpoint; subtitle bytes are written locally by D3.
package embyclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/hope140/subbridge/internal/domain"
)

const (
	defaultTimeout       = 20 * time.Second
	defaultMaxBodyBytes  = 8 << 20
	remoteSearchMaxBody  = 1 << 20
	remoteFetchMaxBody   = 4 << 20
	maxListLimit         = 200
	listIncludeItemTypes = "Movie,Episode"
)

// Config configures a read-only Emby client. APIKey is held in memory by the
// client and is only sent in the X-Emby-Token header.
type Config struct {
	BaseURL         string
	APIKey          string
	HTTPClient      *http.Client
	Timeout         time.Duration
	MaxResponseBody int64
}

// ClientErrorKind is a stable classification suitable for HTTP/API mapping.
type ClientErrorKind string

const (
	ErrInvalidInput     ClientErrorKind = "invalid_input"
	ErrTransport        ClientErrorKind = "transport"
	ErrTimeout          ClientErrorKind = "timeout"
	ErrCanceled         ClientErrorKind = "canceled"
	ErrHTTP             ClientErrorKind = "http"
	ErrRedirect         ClientErrorKind = "redirect"
	ErrResponseTooLarge ClientErrorKind = "response_too_large"
	ErrMalformedJSON    ClientErrorKind = "malformed_json"
	ErrInvalidResponse  ClientErrorKind = "invalid_response"
	ErrNotFound         ClientErrorKind = "not_found"
)

// Error is the only error type returned by Client methods. Its text never
// includes the response body, request URL/path, API key, or transport error
// text, so it is safe to send to normal application logs.
type Error struct {
	Kind   ClientErrorKind
	Status int
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	switch e.Kind {
	case ErrHTTP:
		return fmt.Sprintf("Emby request returned HTTP status %d", e.Status)
	case ErrNotFound:
		return "Emby item was not found"
	case ErrInvalidInput:
		return "invalid Emby client input"
	case ErrTimeout:
		return "Emby request timed out"
	case ErrCanceled:
		return "Emby request was canceled"
	case ErrRedirect:
		return "Emby request was redirected"
	case ErrResponseTooLarge:
		return "Emby response body is too large"
	case ErrMalformedJSON:
		return "Emby response contained invalid JSON"
	case ErrInvalidResponse:
		return "Emby response had an invalid shape"
	default:
		return "Emby request failed"
	}
}

// Code returns the stable error classification.
func (e *Error) Code() ClientErrorKind {
	if e == nil {
		return ""
	}
	return e.Kind
}

// StatusCode returns the upstream HTTP status, if the error came from an HTTP
// response. It intentionally does not expose response content.
func (e *Error) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.Status
}

var errRedirect = errors.New("redirect rejected")

// NewClient creates a bounded client. It validates the base URL and clones
// the supplied HTTP client so redirect policy cannot be weakened by callers.
func NewClient(baseURL, apiKey string, httpClient *http.Client) (*Client, error) {
	return New(Config{BaseURL: baseURL, APIKey: apiKey, HTTPClient: httpClient})
}

// New creates a bounded client from Config.
func New(cfg Config) (*Client, error) {
	base, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, &Error{Kind: ErrInvalidInput}
	}
	if cfg.APIKey == "" || strings.IndexFunc(cfg.APIKey, unicode.IsSpace) >= 0 {
		return nil, &Error{Kind: ErrInvalidInput}
	}
	if cfg.MaxResponseBody <= 0 {
		cfg.MaxResponseBody = defaultMaxBodyBytes
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	client := *cfg.HTTPClient
	client.Timeout = cfg.Timeout
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errRedirect
	}
	return &Client{baseURL: *base, apiKey: cfg.APIKey, httpClient: &client, maxBodyBytes: cfg.MaxResponseBody}, nil
}

// Client provides D1/D2 reads and the explicitly gated item Refresh call.
type Client struct {
	baseURL      url.URL
	apiKey       string
	httpClient   *http.Client
	maxBodyBytes int64
}

// RemoteSubtitleReader is the exact read-only Emby surface used by D2.
type RemoteSubtitleReader interface {
	SearchRemoteSubtitles(context.Context, string, string, string, bool) ([]domain.RemoteSubtitleInfo, error)
	FetchRemoteSubtitle(context.Context, string) ([]byte, error)
}

// ListLibraries returns the Emby library folders without exposing locations.
func (c *Client) ListLibraries(ctx context.Context) ([]domain.Library, error) {
	query := url.Values{}
	query.Set("IsHidden", "false")
	var raw libraryResultDTO
	if err := c.getJSON(ctx, "/Library/MediaFolders", query, &raw); err != nil {
		return nil, err
	}
	if raw.Items == nil || raw.TotalRecordCount == nil || *raw.TotalRecordCount < 0 {
		return nil, &Error{Kind: ErrInvalidResponse}
	}
	libraries := make([]domain.Library, 0, len(*raw.Items))
	for _, item := range *raw.Items {
		if !item.validLibraryShape() {
			return nil, &Error{Kind: ErrInvalidResponse}
		}
		libraries = append(libraries, item.toDomain())
	}
	return libraries, nil
}

// ListItems returns one deterministic page of Movie and Episode items in a
// library. startIndex is zero-based and limit is constrained to 1..200.
func (c *Client) ListItems(ctx context.Context, libraryID string, startIndex, limit int) (domain.ItemPage, error) {
	var err error
	libraryID, err = normalizeID(libraryID)
	if err != nil || startIndex < 0 || limit < 1 || limit > maxListLimit {
		return domain.ItemPage{}, &Error{Kind: ErrInvalidInput}
	}
	query := url.Values{}
	query.Set("EnableImages", "false")
	query.Set("EnableUserData", "false")
	query.Set("GroupItemsIntoCollections", "false")
	query.Set("IncludeItemTypes", listIncludeItemTypes)
	query.Set("Limit", strconv.Itoa(limit))
	query.Set("ParentId", libraryID)
	query.Set("Recursive", "true")
	query.Set("SortBy", "SortName")
	query.Set("SortOrder", "Ascending")
	query.Set("StartIndex", strconv.Itoa(startIndex))
	var raw itemsResponseDTO
	if err := c.getJSON(ctx, "/Items", query, &raw); err != nil {
		return domain.ItemPage{}, err
	}
	if raw.Items == nil || raw.TotalRecordCount == nil || *raw.TotalRecordCount < 0 {
		return domain.ItemPage{}, &Error{Kind: ErrInvalidResponse}
	}
	items := make([]domain.ItemSummary, 0, len(*raw.Items))
	for _, item := range *raw.Items {
		if !item.validItemShape() {
			return domain.ItemPage{}, &Error{Kind: ErrInvalidResponse}
		}
		items = append(items, item.toSummary())
	}
	return domain.ItemPage{
		Items: items, TotalRecordCount: *raw.TotalRecordCount,
		StartIndex: startIndex, Limit: limit,
		HasMore: len(items) == limit && startIndex+len(items) < *raw.TotalRecordCount,
	}, nil
}

// GetItem fetches one detailed item using the Items endpoint. It intentionally
// does not use /Items/{id}, PlaybackInfo, Refresh, or any search endpoint.
func (c *Client) GetItem(ctx context.Context, itemID string) (domain.EmbyItem, error) {
	var err error
	itemID, err = normalizeID(itemID)
	if err != nil {
		return domain.EmbyItem{}, &Error{Kind: ErrInvalidInput}
	}
	query := url.Values{}
	query.Set("EnableImages", "false")
	query.Set("EnableUserData", "false")
	// MediaSources and AlternateMediaSources are detail fields rather than
	// default item fields. Emby 4.9.x can return only the default version
	// unless AlternateMediaSources is explicitly requested.
	query.Set("Fields", "Path,ProviderIds,MediaStreams,MediaSources,AlternateMediaSources")
	query.Set("Ids", itemID)
	query.Set("Limit", "2")
	var raw itemsResponseDTO
	if err := c.getJSON(ctx, "/Items", query, &raw); err != nil {
		return domain.EmbyItem{}, err
	}
	if raw.Items == nil {
		return domain.EmbyItem{}, &Error{Kind: ErrInvalidResponse}
	}
	if len(*raw.Items) == 0 {
		return domain.EmbyItem{}, &Error{Kind: ErrNotFound}
	}
	if len(*raw.Items) != 1 {
		return domain.EmbyItem{}, &Error{Kind: ErrInvalidResponse}
	}
	if !(*raw.Items)[0].validDetailedItemShape() {
		return domain.EmbyItem{}, &Error{Kind: ErrInvalidResponse}
	}
	return (*raw.Items)[0].toDomain(), nil
}

// SearchRemoteSubtitles calls the one Emby Remote Subtitle Search endpoint
// allowed by D2. All source and matching flags are supplied by the server.
func (c *Client) SearchRemoteSubtitles(ctx context.Context, itemID, language, mediaSourceID string, forced bool) ([]domain.RemoteSubtitleInfo, error) {
	itemID, err := normalizePathSegment(itemID)
	if err != nil {
		return nil, &Error{Kind: ErrInvalidInput}
	}
	language, err = normalizePathSegment(language)
	if err != nil {
		return nil, &Error{Kind: ErrInvalidInput}
	}
	mediaSourceID, err = normalizePathSegment(mediaSourceID)
	if err != nil {
		return nil, &Error{Kind: ErrInvalidInput}
	}
	query := url.Values{}
	query.Set("MediaSourceId", mediaSourceID)
	query.Set("IsForced", strconv.FormatBool(forced))
	query.Set("IsPerfectMatch", "false")
	query.Set("IsHearingImpaired", "false")
	var raw []remoteSubtitleDTO
	if err := c.getJSONSegments(ctx, query, remoteSearchMaxBody, &raw, "Items", itemID, "RemoteSearch", "Subtitles", language); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, &Error{Kind: ErrInvalidResponse}
	}
	result := make([]domain.RemoteSubtitleInfo, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, candidate := range raw {
		if !nonEmpty(candidate.ID) {
			return nil, &Error{Kind: ErrInvalidResponse}
		}
		id := stringValue(candidate.ID)
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, candidate.toDomain())
	}
	return result, nil
}

// FetchRemoteSubtitle calls the one Emby Remote Subtitle Fetch endpoint allowed
// by D2. The caller must obtain subtitleID from a server-side Candidate map.
func (c *Client) FetchRemoteSubtitle(ctx context.Context, subtitleID string) ([]byte, error) {
	var err error
	subtitleID, err = normalizePathSegment(subtitleID)
	if err != nil {
		return nil, &Error{Kind: ErrInvalidInput}
	}
	return c.getBytesSegments(ctx, nil, remoteFetchMaxBody, "Providers", "Subtitles", "Subtitles", subtitleID)
}

// RefreshItem asks Emby to rescan one item after a D3 local Add. It does not
// accept arbitrary refresh query flags; the server supplies a bounded,
// non-recursive refresh request.
func (c *Client) RefreshItem(ctx context.Context, itemID string) error {
	itemID, err := normalizePathSegment(itemID)
	if err != nil {
		return &Error{Kind: ErrInvalidInput}
	}
	query := url.Values{"Recursive": []string{"false"}}
	return c.postSegments(ctx, query, "Items", itemID, "Refresh")
}

func normalizeID(value string) (string, error) {
	if value == "" || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", errors.New("invalid id")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("invalid id")
	}
	return value, nil
}

func normalizePathSegment(value string) (string, error) {
	value, err := normalizeID(value)
	if err != nil || value != strings.TrimSpace(value) || strings.ContainsAny(value, "/\\") || value == "." || value == ".." {
		return "", errors.New("invalid path segment")
	}
	return value, nil
}

func (c *Client) endpoint(path string, query url.Values) string {
	u := c.baseURL
	basePath := strings.TrimRight(u.Path, "/")
	u.Path = basePath + "/" + strings.TrimLeft(path, "/")
	u.RawQuery = query.Encode()
	return u.String()
}

func (c *Client) endpointSegments(query url.Values, segments ...string) string {
	u := c.baseURL
	basePath := strings.TrimRight(u.Path, "/")
	baseEscaped := strings.TrimRight(u.EscapedPath(), "/")
	pathParts := append([]string{basePath}, segments...)
	escapedParts := append([]string{baseEscaped}, make([]string, len(segments))...)
	for i, segment := range segments {
		escapedParts[i+1] = url.PathEscape(segment)
	}
	u.Path = strings.Join(pathParts, "/")
	u.RawPath = strings.Join(escapedParts, "/")
	if u.RawPath == u.Path {
		u.RawPath = ""
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, target any) error {
	return c.getJSONLimited(ctx, c.endpoint(path, query), c.maxBodyBytes, target)
}

func (c *Client) getJSONSegments(ctx context.Context, query url.Values, limit int64, target any, segments ...string) error {
	return c.getJSONLimited(ctx, c.endpointSegments(query, segments...), limit, target)
}

func (c *Client) getJSONLimited(ctx context.Context, endpoint string, limit int64, target any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return &Error{Kind: ErrTransport}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Emby-Token", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, errRedirect) {
			return &Error{Kind: ErrRedirect}
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return &Error{Kind: ErrTimeout}
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			return &Error{Kind: ErrCanceled}
		}
		return &Error{Kind: ErrTransport}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &Error{Kind: ErrHTTP, Status: resp.StatusCode}
	}
	body, err := readLimited(resp.Body, limit)
	if err != nil {
		if errors.Is(err, errBodyTooLarge) {
			return &Error{Kind: ErrResponseTooLarge}
		}
		return &Error{Kind: ErrTransport}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		return &Error{Kind: ErrMalformedJSON}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return &Error{Kind: ErrMalformedJSON}
	}
	return nil
}

func (c *Client) getBytesSegments(ctx context.Context, query url.Values, limit int64, segments ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpointSegments(query, segments...), nil)
	if err != nil {
		return nil, &Error{Kind: ErrTransport}
	}
	req.Header.Set("Accept", "text/plain, text/*, application/octet-stream")
	req.Header.Set("X-Emby-Token", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, errRedirect) {
			return nil, &Error{Kind: ErrRedirect}
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, &Error{Kind: ErrTimeout}
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			return nil, &Error{Kind: ErrCanceled}
		}
		return nil, &Error{Kind: ErrTransport}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &Error{Kind: ErrHTTP, Status: resp.StatusCode}
	}
	body, err := readLimited(resp.Body, limit)
	if err != nil {
		if errors.Is(err, errBodyTooLarge) {
			return nil, &Error{Kind: ErrResponseTooLarge}
		}
		return nil, &Error{Kind: ErrTransport}
	}
	return body, nil
}

func (c *Client) postSegments(ctx context.Context, query url.Values, segments ...string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpointSegments(query, segments...), nil)
	if err != nil {
		return &Error{Kind: ErrTransport}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Emby-Token", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, errRedirect) {
			return &Error{Kind: ErrRedirect}
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return &Error{Kind: ErrTimeout}
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			return &Error{Kind: ErrCanceled}
		}
		return &Error{Kind: ErrTransport}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &Error{Kind: ErrHTTP, Status: resp.StatusCode}
	}
	if _, err := readLimited(resp.Body, 64<<10); err != nil {
		if errors.Is(err, errBodyTooLarge) {
			return &Error{Kind: ErrResponseTooLarge}
		}
		return &Error{Kind: ErrTransport}
	}
	return nil
}

var errBodyTooLarge = errors.New("body too large")

func readLimited(body io.Reader, limit int64) ([]byte, error) {
	if limit < 1 {
		return nil, errBodyTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errBodyTooLarge
	}
	return data, nil
}
