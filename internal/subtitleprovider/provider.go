// Package subtitleprovider adapts the Emby Remote Subtitle Bridge to D2's
// candidate-level and bounded retry model.
package subtitleprovider

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/hope140/emby-strm-subtitle-manager/internal/embyclient"
)

const (
	MaxProviderTextBytes = 512
	MaxProviderNameBytes = 64
	MaxFormatBytes       = 32
	MaxRawIDBytes        = 512
	MaxReasons           = 8
	MaxReasonBytes       = 128
)

type Candidate struct {
	RawID       string `json:"-"`
	Provider    string
	Name        string
	Language    string
	Format      string
	Comment     string
	IsHashMatch bool
	Score       float64
	Reasons     []string
}

type FetchResult struct {
	Content  []byte
	Attempts int
}

type Provider interface {
	Search(context.Context, string, string, string, bool) ([]Candidate, error)
	Fetch(context.Context, string) (FetchResult, error)
}

type EmbyRemoteSubtitleProvider struct {
	client embyclient.RemoteSubtitleReader
	sleep  func(context.Context, time.Duration) error
}

func NewEmbyRemoteSubtitleProvider(client embyclient.RemoteSubtitleReader) *EmbyRemoteSubtitleProvider {
	return &EmbyRemoteSubtitleProvider{client: client, sleep: sleepContext}
}

func (p *EmbyRemoteSubtitleProvider) Search(ctx context.Context, itemID, sourceID, language string, forced bool) ([]Candidate, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("provider is unavailable")
	}
	items, err := p.client.SearchRemoteSubtitles(ctx, itemID, language, sourceID, forced)
	if err != nil {
		return nil, err
	}
	result := make([]Candidate, 0, len(items))
	for _, item := range items {
		if !validRawID(item.ID) || len([]byte(item.ID)) > MaxRawIDBytes {
			return nil, errors.New("provider returned an invalid candidate")
		}
		result = append(result, Candidate{
			RawID: item.ID, Provider: limitPlain(item.Provider, MaxProviderNameBytes),
			Name: limitPlain(item.Name, MaxProviderTextBytes), Language: limitPlain(item.Language, MaxFormatBytes),
			Format: limitPlain(normalizeFormat(item.Format), MaxFormatBytes), Comment: limitPlain(item.Comment, MaxProviderTextBytes),
			IsHashMatch: item.IsHashMatch, Score: finiteScore(item.Score), Reasons: limitReasons(item.Reasons),
		})
	}
	return result, nil
}

func (p *EmbyRemoteSubtitleProvider) Fetch(ctx context.Context, rawID string) (FetchResult, error) {
	if p == nil || p.client == nil || strings.TrimSpace(rawID) == "" {
		return FetchResult{}, errors.New("provider is unavailable")
	}
	var last error
	for attempt := 1; attempt <= 2; attempt++ {
		content, err := p.client.FetchRemoteSubtitle(ctx, rawID)
		if err == nil {
			return FetchResult{Content: content, Attempts: attempt}, nil
		}
		last = err
		if !retryable(err) || attempt == 2 {
			return FetchResult{Attempts: attempt}, last
		}
		var clientErr *embyclient.Error
		if errors.As(err, &clientErr) && clientErr.StatusCode() == 429 {
			if err := p.sleep(ctx, time.Second); err != nil {
				return FetchResult{Attempts: attempt}, err
			}
		}
	}
	return FetchResult{Attempts: 2}, last
}

func retryable(err error) bool {
	var clientErr *embyclient.Error
	if !errors.As(err, &clientErr) {
		return false
	}
	switch clientErr.Code() {
	case embyclient.ErrTimeout, embyclient.ErrTransport:
		return true
	case embyclient.ErrHTTP:
		return clientErr.StatusCode() == 429
	default:
		return false
	}
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func limitPlain(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) && r != '\t' && r != '\n' {
			continue
		}
		encoded := string(r)
		if builder.Len()+len(encoded) > maxBytes {
			break
		}
		builder.WriteString(encoded)
	}
	return builder.String()
}

func limitReasons(values []string) []string {
	if len(values) > MaxReasons {
		values = values[:MaxReasons]
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, limitPlain(value, MaxReasonBytes))
	}
	return result
}

func normalizeFormat(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, ".")
	if value == "substation alpha" {
		return "ssa"
	}
	return value
}

func validRawID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && value != "." && value != ".." && !strings.ContainsAny(value, "/\\") && strings.IndexFunc(value, unicode.IsControl) < 0
}

func finiteScore(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}
