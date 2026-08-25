// Package d2 contains the Search, Fetch and Preview orchestration boundary.
// It is deliberately independent from HTTP so every security gate can be
// exercised by unit tests and Fake Emby integration tests.
package d2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/hope140/emby-strm-subtitle-manager/internal/config"
	"github.com/hope140/emby-strm-subtitle-manager/internal/domain"
	"github.com/hope140/emby-strm-subtitle-manager/internal/embyclient"
	"github.com/hope140/emby-strm-subtitle-manager/internal/preview"
	"github.com/hope140/emby-strm-subtitle-manager/internal/subtitle"
	"github.com/hope140/emby-strm-subtitle-manager/internal/subtitleprovider"
)

const maxPreviewResponseBytes = 1 << 20

type ItemReader interface {
	GetItem(context.Context, string) (domain.EmbyItem, error)
}

type Options struct {
	Config              config.D2Config
	RemoteSearchEnabled bool
	CanaryEnabled       bool
	Allowlist           *preview.Allowlist
	Emby                ItemReader
	Provider            subtitleprovider.Provider
	CandidateStore      *preview.CandidateStore
	ArtifactStore       *preview.ArtifactStore
	AuthContext         string
	Now                 func() time.Time
}

type Service struct {
	settings      config.D2Config
	enabled       bool
	canaryEnabled bool
	allowlist     *preview.Allowlist
	emby          ItemReader
	provider      subtitleprovider.Provider
	candidates    *preview.CandidateStore
	artifacts     *preview.ArtifactStore
	authContext   string
	now           func() time.Time
	limiter       *operationLimiter
}

// AuthContextFromToken creates an opaque process binding without retaining or
// exposing the application Bearer token itself.
func AuthContextFromToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

type SearchRequest struct {
	MediaSourceID string
	Language      string
	Forced        bool
}

type CandidateView struct {
	Token       string    `json:"token"`
	Provider    string    `json:"provider"`
	Name        string    `json:"name"`
	Language    string    `json:"language"`
	Format      string    `json:"format"`
	Comment     string    `json:"comment"`
	IsHashMatch bool      `json:"is_hash_match"`
	Score       float64   `json:"score"`
	State       string    `json:"state"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type SearchResponse struct {
	Language     string          `json:"language"`
	Candidates   []CandidateView `json:"candidates"`
	Truncated    bool            `json:"truncated"`
	Capabilities Capabilities    `json:"capabilities"`
}

type Capabilities struct {
	SupportsProviderSelection bool `json:"supports_provider_selection"`
	SupportsCustomQuery       bool `json:"supports_custom_query"`
	SupportsHashMatch         bool `json:"supports_hash_match"`
}

type FetchRequest struct {
	CandidateToken string
}

type FetchResponse struct {
	ArtifactToken string    `json:"artifact_token"`
	Provider      string    `json:"provider"`
	Language      string    `json:"language"`
	Format        string    `json:"format"`
	ByteLength    int       `json:"byte_length"`
	CueCount      int       `json:"cue_count"`
	ContentHash   string    `json:"content_sha256"`
	PreviewReady  bool      `json:"preview_ready"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type PreviewRequest struct {
	ArtifactToken string
	Offset        int
	Limit         int
}

type PreviewResponse struct {
	Format      string         `json:"format"`
	Language    string         `json:"language"`
	ByteLength  int            `json:"byte_length"`
	CueCount    int            `json:"cue_count"`
	ContentHash string         `json:"content_sha256"`
	Offset      int            `json:"offset"`
	Limit       int            `json:"limit"`
	Truncated   bool           `json:"truncated"`
	Cues        []subtitle.Cue `json:"cues"`
}

func New(options Options) (*Service, error) {
	settings := options.Config.WithDefaults()
	if options.Now == nil {
		options.Now = time.Now
	}
	service := &Service{
		settings: settings, enabled: options.RemoteSearchEnabled, canaryEnabled: options.CanaryEnabled,
		allowlist: options.Allowlist, emby: options.Emby, provider: options.Provider,
		authContext: options.AuthContext, now: options.Now,
	}
	if !service.enabled || !service.canaryEnabled || service.allowlist == nil || service.allowlist.Len() == 0 {
		return service, nil
	}
	if service.authContext == "" {
		service.authContext = "shared"
	}
	if options.CandidateStore != nil {
		service.candidates = options.CandidateStore
	} else {
		service.candidates = preview.NewCandidateStore(preview.CandidateStoreOptions{TTL: time.Duration(settings.CandidateTTLSeconds) * time.Second, Now: options.Now})
	}
	if options.ArtifactStore != nil {
		service.artifacts = options.ArtifactStore
	} else {
		if strings.TrimSpace(settings.CacheDir) == "" {
			return nil, errors.New("d2.cache_dir is required when D2 is enabled")
		}
		if !filepath.IsAbs(settings.CacheDir) {
			return nil, errors.New("d2.cache_dir must be an absolute path")
		}
		store, err := preview.NewArtifactStore(preview.ArtifactStoreOptions{
			Directory: settings.CacheDir, TTL: time.Duration(settings.ArtifactTTLSeconds) * time.Second,
			MaxBytes: settings.MaxSubtitleBytes, Now: options.Now,
		})
		if err != nil {
			return nil, errors.New("unable to initialize D2 preview cache")
		}
		service.artifacts = store
	}
	service.limiter = newOperationLimiter(settings.MaxConcurrent, options.Now)
	return service, nil
}

func (s *Service) Enabled() bool {
	return s != nil && s.enabled && s.canaryEnabled && s.allowlist != nil && s.allowlist.Len() > 0
}

func (s *Service) Search(ctx context.Context, itemID string, request SearchRequest) (SearchResponse, error) {
	if err := s.checkEnabled(); err != nil {
		return SearchResponse{}, err
	}
	ctx, cancel := withBudget(ctx, time.Duration(s.settings.SearchTimeoutSeconds)*time.Second)
	defer cancel()
	language, err := normalizeLanguage(request.Language, s.settings.DefaultLanguage)
	if err != nil {
		return SearchResponse{}, err
	}
	if !validItemID(itemID) {
		return SearchResponse{}, failure(400, "invalid_request", "invalid item id")
	}
	if request.MediaSourceID != "" && !validItemID(request.MediaSourceID) {
		return SearchResponse{}, failure(400, "invalid_request", "invalid media source id")
	}
	release, ok := s.limiter.acquire("search", itemID, s.authContext)
	if !ok {
		return SearchResponse{}, rateLimitError()
	}
	defer release()
	item, source, generation, err := s.loadSingleSource(ctx, itemID, request.MediaSourceID)
	if err != nil {
		return SearchResponse{}, err
	}
	if s.provider == nil {
		return SearchResponse{}, failure(502, "provider_search_failed", "subtitle search failed")
	}
	providerCtx, providerCancel := context.WithTimeout(ctx, 15*time.Second)
	defer providerCancel()
	items, err := s.provider.Search(providerCtx, item.ID, source.ID, language, request.Forced)
	if err != nil {
		return SearchResponse{}, mapProviderError(err, false)
	}
	truncated := false
	if len(items) > s.settings.MaxCandidateCount {
		items = items[:s.settings.MaxCandidateCount]
		truncated = true
	}
	inputs := make([]preview.CandidateInput, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.RawID) == "" {
			return SearchResponse{}, failure(502, "emby_invalid_response", "Emby response was invalid")
		}
		candidateLanguage := strings.TrimSpace(item.Language)
		if candidateLanguage == "" {
			candidateLanguage = language
		}
		inputs = append(inputs, preview.CandidateInput{RawID: item.RawID, Provider: item.Provider, Name: item.Name, Language: candidateLanguage, Format: item.Format, Comment: item.Comment, IsHashMatch: item.IsHashMatch, Score: finiteScore(item.Score), Reasons: item.Reasons})
	}
	binding := preview.Binding{ItemID: item.ID, SourceID: source.ID, Language: language, AuthContext: s.authContext, AllowlistGeneration: generation}
	issued, err := s.candidates.IssueMany(binding, inputs)
	if err != nil {
		return SearchResponse{}, failure(503, "preview_store_unavailable", "preview store is unavailable")
	}
	response := SearchResponse{Language: language, Truncated: truncated, Capabilities: Capabilities{SupportsHashMatch: true}, Candidates: make([]CandidateView, 0, len(issued))}
	for _, candidate := range issued {
		response.Candidates = append(response.Candidates, CandidateView{Token: candidate.Token, Provider: candidate.Provider, Name: candidate.Name, Language: candidate.Language, Format: candidate.Format, Comment: candidate.Comment, IsHashMatch: candidate.IsHashMatch, Score: finiteScore(candidate.Score), State: candidate.State, ExpiresAt: candidate.ExpiresAt})
	}
	return response, nil
}

func (s *Service) Fetch(ctx context.Context, itemID string, request FetchRequest) (FetchResponse, error) {
	if err := s.checkEnabled(); err != nil {
		return FetchResponse{}, err
	}
	if request.CandidateToken == "" {
		return FetchResponse{}, failure(400, "invalid_request", "candidate token is required")
	}
	if !validItemID(itemID) {
		return FetchResponse{}, failure(400, "invalid_request", "invalid item id")
	}
	release, ok := s.limiter.acquire("fetch", itemID, s.authContext)
	if !ok {
		return FetchResponse{}, rateLimitError()
	}
	defer release()
	ctx, cancel := withBudget(ctx, time.Duration(s.settings.FetchTimeoutSeconds)*time.Second)
	defer cancel()
	item, source, generation, err := s.loadSingleSource(ctx, itemID, "")
	if err != nil {
		return FetchResponse{}, err
	}
	binding := preview.Binding{ItemID: item.ID, SourceID: source.ID, AuthContext: s.authContext, AllowlistGeneration: generation}
	candidate, err := s.candidates.Resolve(request.CandidateToken, bindingWithoutLanguage(binding))
	if err != nil {
		return FetchResponse{}, mapCandidateError(err)
	}
	binding.Language = candidate.Binding.Language
	if candidate.State == "fetched" && candidate.ArtifactToken != "" {
		artifact, artifactErr := s.artifacts.Get(candidate.ArtifactToken, binding)
		if artifactErr != nil {
			return FetchResponse{}, mapArtifactError(artifactErr)
		}
		return fetchResponse(candidate, artifact), nil
	}
	if candidate.State == "failed" {
		return FetchResponse{}, failure(502, "candidate_fetch_failed", "this subtitle candidate failed; choose another candidate")
	}
	if s.provider == nil {
		s.candidates.RecordFailure(request.CandidateToken, "provider_unavailable", 1)
		return FetchResponse{}, failure(502, "candidate_fetch_failed", "this subtitle candidate failed; choose another candidate")
	}
	providerCtx, providerCancel := context.WithTimeout(ctx, 20*time.Second)
	defer providerCancel()
	result, err := s.provider.Fetch(providerCtx, candidate.RawID)
	attempts := result.Attempts
	if attempts < 1 {
		attempts = 1
	}
	if err != nil {
		s.candidates.RecordFailure(request.CandidateToken, providerFailureCode(err), attempts)
		return FetchResponse{}, mapFetchProviderError(err)
	}
	document, err := subtitle.ValidateAndParse(result.Content, candidate.Format, s.settings.MaxSubtitleBytes)
	if err != nil {
		s.candidates.RecordFailure(request.CandidateToken, subtitleFailureCode(err), attempts)
		return FetchResponse{}, mapSubtitleError(err)
	}
	artifact, err := s.artifacts.Create(binding, document.Format, binding.Language, document.Canonical, document.Cues)
	if err != nil {
		return FetchResponse{}, mapArtifactError(err)
	}
	s.candidates.RecordSuccess(request.CandidateToken, artifact.Token, attempts)
	return fetchResponse(candidate, artifact), nil
}

func (s *Service) Preview(ctx context.Context, itemID string, request PreviewRequest) (PreviewResponse, error) {
	if err := s.checkEnabled(); err != nil {
		return PreviewResponse{}, err
	}
	if request.ArtifactToken == "" {
		return PreviewResponse{}, failure(400, "invalid_request", "artifact token is required")
	}
	if !validItemID(itemID) {
		return PreviewResponse{}, failure(400, "invalid_request", "invalid item id")
	}
	if request.Offset < 0 || request.Limit < 0 || request.Limit > 500 {
		return PreviewResponse{}, failure(400, "invalid_request", "invalid preview pagination")
	}
	if request.Limit == 0 {
		request.Limit = 200
	}
	release, ok := s.limiter.acquire("preview", itemID, s.authContext)
	if !ok {
		return PreviewResponse{}, rateLimitError()
	}
	defer release()
	ctx, cancel := withBudget(ctx, time.Duration(s.settings.PreviewTimeoutSeconds)*time.Second)
	defer cancel()
	item, source, generation, err := s.loadSingleSource(ctx, itemID, "")
	if err != nil {
		return PreviewResponse{}, err
	}
	artifact, err := s.artifacts.Get(request.ArtifactToken, preview.Binding{ItemID: item.ID, SourceID: source.ID, Language: "", AuthContext: s.authContext, AllowlistGeneration: generation})
	if err != nil {
		return PreviewResponse{}, mapArtifactError(err)
	}
	return projectPreview(artifact, request.Offset, request.Limit), nil
}

// RunCleanup removes expired in-memory mappings and private artifact files at
// a bounded cadence. It is safe to run for the lifetime of the single service
// instance and stops when ctx is canceled.
func (s *Service) RunCleanup(ctx context.Context) {
	if s == nil || s.candidates == nil || s.artifacts == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.candidates.RemoveExpired()
			s.artifacts.RemoveExpired()
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) checkEnabled() error {
	if !s.Enabled() {
		return failure(403, "remote_search_disabled", "remote subtitle search is disabled")
	}
	return nil
}

func (s *Service) loadSingleSource(ctx context.Context, itemID, requestedSource string) (domain.EmbyItem, domain.MediaSource, uint64, error) {
	if s.emby == nil {
		return domain.EmbyItem{}, domain.MediaSource{}, 0, failure(502, "emby_unavailable", "Emby is unavailable")
	}
	if !validItemID(itemID) {
		return domain.EmbyItem{}, domain.MediaSource{}, 0, failure(400, "invalid_request", "invalid item id")
	}
	itemCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	item, err := s.emby.GetItem(itemCtx, itemID)
	if err != nil {
		return domain.EmbyItem{}, domain.MediaSource{}, 0, mapGetItemError(err)
	}
	if item.ID == "" || item.ID != itemID {
		return domain.EmbyItem{}, domain.MediaSource{}, 0, failure(502, "emby_invalid_response", "Emby response was invalid")
	}
	if item.Type != "Movie" && item.Type != "Episode" {
		return domain.EmbyItem{}, domain.MediaSource{}, 0, failure(422, "unsupported_media_type", "only Movie and Episode items are supported")
	}
	if len(item.MediaSources) == 0 {
		return domain.EmbyItem{}, domain.MediaSource{}, 0, failure(502, "media_source_unavailable", "Emby did not return a media source")
	}
	seen := make(map[string]struct{}, len(item.MediaSources))
	for _, source := range item.MediaSources {
		if !validItemID(source.ID) || source.ID == "." || source.ID == ".." {
			return domain.EmbyItem{}, domain.MediaSource{}, 0, failure(502, "emby_invalid_response", "Emby response was invalid")
		}
		if _, exists := seen[source.ID]; exists {
			return domain.EmbyItem{}, domain.MediaSource{}, 0, failure(502, "emby_invalid_response", "Emby response was invalid")
		}
		seen[source.ID] = struct{}{}
	}
	if len(item.MediaSources) > 1 {
		return domain.EmbyItem{}, domain.MediaSource{}, 0, failure(409, "d2_multisource_unsupported", "multiple media sources are not supported by D2")
	}
	source := item.MediaSources[0]
	if requestedSource != "" && requestedSource != source.ID {
		return domain.EmbyItem{}, domain.MediaSource{}, 0, failure(409, "media_source_mismatch", "media source does not match the current item")
	}
	allowed, generation := s.allowlist.Allows(item.ID)
	if !allowed {
		return domain.EmbyItem{}, domain.MediaSource{}, 0, failure(403, "canary_item_not_allowed", "item is not allowed for the D2 Canary")
	}
	return item, source, generation, nil
}

func bindingWithoutLanguage(binding preview.Binding) preview.Binding {
	binding.Language = ""
	return binding
}

func fetchResponse(candidate preview.Candidate, artifact preview.Artifact) FetchResponse {
	return FetchResponse{ArtifactToken: artifact.Token, Provider: candidate.Provider, Language: artifact.Binding.Language, Format: artifact.Format, ByteLength: artifact.ByteLength, CueCount: artifact.CueCount, ContentHash: artifact.ContentHash, PreviewReady: true, ExpiresAt: artifact.ExpiresAt}
}

func projectPreview(artifact preview.Artifact, offset, limit int) PreviewResponse {
	response := PreviewResponse{Format: artifact.Format, Language: artifact.Binding.Language, ByteLength: artifact.ByteLength, CueCount: artifact.CueCount, ContentHash: artifact.ContentHash, Offset: offset, Limit: limit, Cues: make([]subtitle.Cue, 0)}
	if offset < len(artifact.Cues) {
		end := offset + limit
		if end > len(artifact.Cues) {
			end = len(artifact.Cues)
		}
		response.Cues = append([]subtitle.Cue(nil), artifact.Cues[offset:end]...)
	}
	response.Truncated = offset+len(response.Cues) < len(artifact.Cues)
	for len(response.Cues) > 0 {
		encoded, err := json.Marshal(response)
		if err == nil && len(encoded) < maxPreviewResponseBytes {
			break
		}
		response.Cues = response.Cues[:len(response.Cues)-1]
		response.Truncated = true
	}
	return response
}

func normalizeLanguage(value, defaultLanguage string) (string, error) {
	if strings.TrimSpace(value) == "" {
		value = defaultLanguage
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "zh-cn", "zh", "zho", "chi":
		return "zh-CN", nil
	default:
		return "", failure(400, "invalid_request", "unsupported subtitle language")
	}
}

func validItemID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "/\\") && strings.IndexFunc(value, unicode.IsControl) < 0
}

func withBudget(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, timeout)
}

func finiteScore(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func mapGetItemError(err error) *Error {
	var clientErr *embyclient.Error
	if !errors.As(err, &clientErr) {
		return failure(502, "emby_unavailable", "Emby is unavailable")
	}
	switch clientErr.Code() {
	case embyclient.ErrNotFound:
		return failure(404, "media_not_found", "media item was not found")
	case embyclient.ErrHTTP:
		if clientErr.StatusCode() == 404 {
			return failure(404, "media_not_found", "media item was not found")
		}
		return failure(502, "emby_unavailable", "Emby is unavailable")
	case embyclient.ErrTimeout:
		return failure(504, "emby_timeout", "Emby request timed out")
	case embyclient.ErrMalformedJSON, embyclient.ErrInvalidResponse, embyclient.ErrResponseTooLarge, embyclient.ErrRedirect:
		return failure(502, "emby_invalid_response", "Emby response was invalid")
	default:
		return failure(502, "emby_unavailable", "Emby is unavailable")
	}
}

func mapProviderError(err error, fetch bool) *Error {
	var clientErr *embyclient.Error
	if !errors.As(err, &clientErr) {
		if fetch {
			return failure(502, "candidate_fetch_failed", "this subtitle candidate failed; choose another candidate")
		}
		return failure(502, "provider_search_failed", "subtitle search failed")
	}
	if clientErr.Code() == embyclient.ErrTimeout {
		if fetch {
			return failure(504, "candidate_fetch_timeout", "subtitle candidate fetch timed out")
		}
		return failure(504, "emby_timeout", "Emby search timed out")
	}
	if !fetch {
		switch clientErr.Code() {
		case embyclient.ErrMalformedJSON, embyclient.ErrInvalidResponse, embyclient.ErrResponseTooLarge, embyclient.ErrRedirect:
			return failure(502, "emby_invalid_response", "Emby response was invalid")
		}
	}
	if fetch {
		return failure(502, "candidate_fetch_failed", "this subtitle candidate failed; choose another candidate")
	}
	return failure(502, "provider_search_failed", "subtitle search failed")
}

func mapFetchProviderError(err error) *Error {
	var clientErr *embyclient.Error
	if errors.As(err, &clientErr) {
		switch clientErr.Code() {
		case embyclient.ErrTimeout:
			return failure(504, "candidate_fetch_timeout", "subtitle candidate fetch timed out")
		case embyclient.ErrResponseTooLarge:
			return failure(413, "subtitle_too_large", "subtitle exceeds the size limit")
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return failure(504, "candidate_fetch_timeout", "subtitle candidate fetch timed out")
	}
	return failure(502, "candidate_fetch_failed", "this subtitle candidate failed; choose another candidate")
}

func isEmbyTimeout(err error) bool {
	var clientErr *embyclient.Error
	return errors.As(err, &clientErr) && clientErr.Code() == embyclient.ErrTimeout
}

func providerFailureCode(err error) string {
	var clientErr *embyclient.Error
	if errors.As(err, &clientErr) && clientErr.Code() == embyclient.ErrResponseTooLarge {
		return "subtitle_too_large"
	}
	if isEmbyTimeout(err) || errors.Is(err, context.DeadlineExceeded) {
		return "candidate_fetch_timeout"
	}
	return "candidate_fetch_failed"
}

func subtitleFailureCode(err error) string {
	switch {
	case errors.Is(err, subtitle.ErrTooLarge):
		return "subtitle_too_large"
	case errors.Is(err, subtitle.ErrUnsupportedFormat):
		return "subtitle_format_unsupported"
	default:
		return "subtitle_invalid"
	}
}

func mapSubtitleError(err error) *Error {
	switch {
	case errors.Is(err, subtitle.ErrTooLarge):
		return failure(413, "subtitle_too_large", "subtitle exceeds the size limit")
	case errors.Is(err, subtitle.ErrUnsupportedFormat):
		return failure(422, "subtitle_format_unsupported", "subtitle format is unsupported")
	default:
		return failure(422, "subtitle_invalid", "subtitle content is invalid")
	}
}

func mapCandidateError(err error) *Error {
	switch {
	case errors.Is(err, preview.ErrCandidateExpired):
		return failure(410, "candidate_expired", "candidate token expired; search again")
	default:
		return failure(404, "candidate_invalid", "candidate token is invalid; search again")
	}
}

func mapArtifactError(err error) *Error {
	switch {
	case errors.Is(err, preview.ErrArtifactExpired):
		return failure(410, "artifact_expired", "preview artifact expired; fetch again")
	case errors.Is(err, preview.ErrArtifactInvalid):
		return failure(404, "artifact_invalid", "preview artifact is invalid; fetch again")
	default:
		return failure(503, "preview_store_unavailable", "preview store is unavailable")
	}
}
