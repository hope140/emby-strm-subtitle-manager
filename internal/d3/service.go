// Package d3 contains the bounded local subtitle write and recovery flows.
package d3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hope140/subbridge/internal/config"
	"github.com/hope140/subbridge/internal/domain"
	"github.com/hope140/subbridge/internal/inventory"
	"github.com/hope140/subbridge/internal/media"
	"github.com/hope140/subbridge/internal/pathmap"
	"github.com/hope140/subbridge/internal/pathsecurity"
	"github.com/hope140/subbridge/internal/preview"
	"github.com/hope140/subbridge/internal/subtitle"
)

const (
	getItemTimeout   = 3 * time.Second
	maxOperationID   = 128
	maxFilenameBytes = 180
	maxVersions      = 100
)

var (
	ErrDisabled          = errors.New("D3 writes are disabled")
	ErrInvalidRequest    = errors.New("invalid D3 Add request")
	ErrItemNotAllowed    = errors.New("item is not allowed for the D3 Canary")
	ErrMultiSource       = errors.New("multiple media sources are not supported by D3 Add")
	ErrUnsafeMediaPath   = errors.New("media path is unsafe or unavailable")
	ErrArtifact          = errors.New("preview artifact is unavailable")
	ErrUnsupportedFormat = errors.New("subtitle format is unsupported")
	ErrWrite             = errors.New("subtitle write failed")
	ErrRefresh           = errors.New("Emby refresh failed")
	ErrNotVisible        = errors.New("Emby did not expose the added subtitle")
	ErrHistory           = errors.New("D3 history write failed")
	ErrOperationConflict = errors.New("operation id is already bound to another Add")
)

// Error is safe to map to an HTTP envelope. It never contains a path, item
// identifier, candidate identifier, token or upstream response body.
type Error struct {
	Status  int
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type ItemReader interface {
	GetItem(context.Context, string) (domain.EmbyItem, error)
}

type Refresher interface {
	RefreshItem(context.Context, string) error
}

type Options struct {
	Config       config.D3Config
	WriteEnabled bool
	Gate         preview.ItemGate
	// Canary remains for source-compatible callers. New construction should
	// pass the common Gate shared with D2.
	Canary           *preview.Allowlist
	Emby             ItemReader
	Refresher        Refresher
	Mapper           *pathmap.Mapper
	Guard            *pathmap.PathGuard
	Inventory        *inventory.Service
	Artifacts        *preview.ArtifactStore
	AuthContext      string
	MaxSubtitleBytes int64
	Now              func() time.Time
}

type Service struct {
	settings config.D3Config
	enabled  bool
	gate     preview.ItemGate
	// Retained only for legacy in-package tests and callers that inspect the
	// original Canary object; all admission decisions go through gate.
	canary             *preview.Allowlist
	emby               ItemReader
	refresher          Refresher
	mapper             *pathmap.Mapper
	guard              *pathmap.PathGuard
	inventory          *inventory.Service
	artifacts          *preview.ArtifactStore
	authContext        string
	maxSubtitleBytes   int64
	now                func() time.Time
	global             chan struct{}
	mu                 sync.Mutex
	itemLocks          map[string]*sync.Mutex
	operations         map[[32]byte]AddResponse
	recoveryOperations map[[32]byte]operationMemory
}

type AddRequest struct {
	ArtifactToken string
	MediaSourceID string
	OperationID   string
}

type AddResponse struct {
	OperationID   string        `json:"operation_id"`
	Type          OperationType `json:"type"`
	ItemID        string        `json:"item_id"`
	MediaSourceID string        `json:"media_source_id"`
	FileName      string        `json:"file_name"`
	Language      string        `json:"language"`
	Format        string        `json:"format"`
	ByteLength    int           `json:"byte_length"`
	ContentHash   string        `json:"content_sha256"`
	Refresh       string        `json:"refresh"`
	Status        string        `json:"status"`
	CreatedAt     time.Time     `json:"created_at"`
}

type historyRecord struct {
	Version       int       `json:"version"`
	OperationHash string    `json:"operation_hash"`
	ItemID        string    `json:"item_id"`
	MediaSourceID string    `json:"media_source_id"`
	FileName      string    `json:"file_name"`
	Language      string    `json:"language"`
	Format        string    `json:"format"`
	ByteLength    int       `json:"byte_length"`
	ContentHash   string    `json:"content_sha256"`
	CreatedAt     time.Time `json:"created_at"`
}

func New(options Options) (*Service, error) {
	settings := options.Config.WithDefaults()
	if options.Now == nil {
		options.Now = time.Now
	}
	gate := options.Gate
	if gate == nil && options.Canary != nil {
		gate = options.Canary
	}
	if options.MaxSubtitleBytes <= 0 {
		options.MaxSubtitleBytes = subtitle.DefaultMaxBytes
	}
	service := &Service{settings: settings, enabled: options.WriteEnabled, gate: gate, canary: options.Canary,
		emby: options.Emby, refresher: options.Refresher, mapper: options.Mapper, guard: options.Guard,
		inventory: options.Inventory, artifacts: options.Artifacts, authContext: options.AuthContext, maxSubtitleBytes: options.MaxSubtitleBytes, now: options.Now,
		global: make(chan struct{}, 1), itemLocks: make(map[string]*sync.Mutex), operations: make(map[[32]byte]AddResponse), recoveryOperations: make(map[[32]byte]operationMemory)}
	if !service.enabled {
		return service, nil
	}
	if service.gate == nil {
		return nil, errors.New("D3 item gate is required when writes are enabled")
	}
	if service.authContext == "" {
		service.authContext = "shared"
	}
	if service.artifacts == nil || service.emby == nil || service.refresher == nil || service.mapper == nil || service.guard == nil || service.inventory == nil {
		return nil, errors.New("D3 dependencies are incomplete")
	}
	if settings.HistoryDir == "" || settings.QuarantineDir == "" || settings.ArchiveDir == "" || settings.TrashDir == "" || !filepath.IsAbs(settings.HistoryDir) || !filepath.IsAbs(settings.QuarantineDir) || !filepath.IsAbs(settings.ArchiveDir) || !filepath.IsAbs(settings.TrashDir) {
		return nil, errors.New("D3 private directories are required")
	}
	for _, directory := range []string{settings.HistoryDir, settings.QuarantineDir, settings.ArchiveDir, settings.TrashDir} {
		if pathsecurity.IsFilesystemRoot(directory) {
			return nil, errors.New("D3 private directory must not be a filesystem root")
		}
		if linked, err := pathsecurity.UsesSymlink(directory); err != nil || linked {
			return nil, errors.New("D3 private directory is unsafe")
		}
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, errors.New("D3 private directory is unavailable")
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return nil, errors.New("D3 private directory permissions are unavailable")
		}
	}
	return service, nil
}

func (s *Service) Enabled() bool {
	return s != nil && s.enabled && s.gate != nil
}

func (s *Service) Add(ctx context.Context, itemID string, request AddRequest) (AddResponse, error) {
	if s == nil || !s.Enabled() {
		return AddResponse{}, &Error{Status: 403, Code: "write_disabled", Message: "D3 Add is disabled", Cause: ErrDisabled}
	}
	if !validID(itemID) || !validID(request.MediaSourceID) || request.ArtifactToken == "" || !validOperationID(request.OperationID) {
		return AddResponse{}, &Error{Status: 400, Code: "invalid_request", Message: "invalid D3 Add request", Cause: ErrInvalidRequest}
	}
	if allowed, _ := s.gate.Allows(itemID); !allowed {
		return AddResponse{}, &Error{Status: 403, Code: "d3_item_not_allowed", Message: "item is not allowed for D3 Add", Cause: ErrItemNotAllowed}
	}
	opHash := sha256.Sum256([]byte(request.OperationID))
	s.mu.Lock()
	lock := s.itemLockLocked(itemID)
	s.mu.Unlock()
	s.global <- struct{}{}
	defer func() { <-s.global }()
	lock.Lock()
	defer lock.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	itemCtx, cancel := context.WithTimeout(ctx, getItemTimeout)
	item, err := s.emby.GetItem(itemCtx, itemID)
	cancel()
	if err != nil {
		return AddResponse{}, mapItemError(err)
	}
	if item.ID != itemID || (item.Type != "Movie" && item.Type != "Episode") {
		return AddResponse{}, &Error{Status: 502, Code: "emby_invalid_response", Message: "Emby response was invalid"}
	}
	generationAllowed, generation := s.gate.Allows(item.ID)
	if !generationAllowed {
		return AddResponse{}, &Error{Status: 403, Code: "d3_item_not_allowed", Message: "item is not allowed for D3 Add", Cause: ErrItemNotAllowed}
	}
	mediaCtx, err := media.Build(item, media.BuildOptions{MediaSourceID: request.MediaSourceID, Mapper: s.mapper, Guard: s.guard})
	if err != nil {
		return AddResponse{}, mapMediaError(err)
	}
	if mediaCtx.MappingStatus != media.MappingStatusMapped || mediaCtx.LocalDirectory == "" || mediaCtx.LocalPath == "" {
		return AddResponse{}, &Error{Status: 422, Code: "media_path_unsafe", Message: "media path is unavailable for D3 Add", Cause: ErrUnsafeMediaPath}
	}
	writeTarget, err := media.ResolveWriteTarget(item, request.MediaSourceID, s.mapper, s.guard)
	if err != nil {
		return AddResponse{}, mapMediaError(err)
	}
	artifact, content, err := s.artifacts.GetContent(request.ArtifactToken, preview.Binding{ItemID: item.ID, SourceID: mediaCtx.MediaSourceID, AuthContext: s.authContext, AllowlistGeneration: generation})
	if err != nil {
		return AddResponse{}, mapArtifactError(err)
	}
	if artifact.Format != "srt" && artifact.Format != "ass" && artifact.Format != "ssa" {
		return AddResponse{}, &Error{Status: 422, Code: "subtitle_format_unsupported", Message: "subtitle format is unsupported", Cause: ErrUnsupportedFormat}
	}
	if len(content) != artifact.ByteLength || hashBytes(content) != artifact.ContentHash {
		return AddResponse{}, &Error{Status: 502, Code: "artifact_invalid", Message: "preview artifact is unavailable", Cause: ErrArtifact}
	}
	// An operation ID is bound to the fully validated artifact content as well
	// as the item and source. Do this only after obtaining the item lock, so a
	// concurrent retry cannot create a second versioned sidecar.
	s.mu.Lock()
	if result, ok := s.operations[opHash]; ok {
		s.mu.Unlock()
		if result.ItemID != item.ID || result.MediaSourceID != mediaCtx.MediaSourceID || result.ContentHash != artifact.ContentHash {
			return AddResponse{}, operationConflict()
		}
		return result, nil
	}
	s.mu.Unlock()
	if result, found, conflict := s.loadHistory(opHash, request.OperationID, item.ID, mediaCtx.MediaSourceID, writeTarget.LocalDirectory, artifact.ContentHash); found {
		if conflict {
			return AddResponse{}, operationConflict()
		}
		s.mu.Lock()
		s.operations[opHash] = result
		s.mu.Unlock()
		return result, nil
	}
	fileName, err := s.writeNewFile(writeTarget.LocalDirectory, writeTarget.LocalPath, artifact, content, request.OperationID)
	if err != nil {
		return AddResponse{}, err
	}
	target := filepath.Join(writeTarget.LocalDirectory, fileName)
	if err := verifyFile(target, content); err != nil {
		s.quarantine(target, request.OperationID, fileName)
		return AddResponse{}, &Error{Status: 503, Code: "write_verification_failed", Message: "subtitle write could not be verified", Cause: err}
	}
	refreshCtx, refreshCancel := context.WithTimeout(ctx, time.Duration(s.settings.RefreshTimeoutSeconds)*time.Second)
	refreshErr := s.refresher.RefreshItem(refreshCtx, item.ID)
	refreshCancel()
	if refreshErr != nil {
		s.quarantine(target, request.OperationID, fileName)
		return AddResponse{}, &Error{Status: 502, Code: "emby_refresh_failed", Message: "Emby refresh failed; the new subtitle was quarantined", Cause: ErrRefresh}
	}
	visible := s.pollVisible(ctx, item.ID, mediaCtx.MediaSourceID, target)
	if !visible {
		s.quarantine(target, request.OperationID, fileName)
		return AddResponse{}, &Error{Status: 502, Code: "emby_subtitle_not_visible", Message: "Emby did not expose the new subtitle; it was quarantined", Cause: ErrNotVisible}
	}
	created := s.now().UTC()
	response := AddResponse{OperationID: request.OperationID, Type: OperationAdd, ItemID: item.ID, MediaSourceID: mediaCtx.MediaSourceID, FileName: fileName, Language: artifact.Language, Format: artifact.Format, ByteLength: artifact.ByteLength, ContentHash: artifact.ContentHash, Refresh: "verified", Status: "verified", CreatedAt: created}
	if err := s.writeHistory(response); err != nil {
		s.quarantine(target, request.OperationID, fileName)
		return AddResponse{}, &Error{Status: 503, Code: "d3_history_unavailable", Message: "D3 history could not be recorded; the new subtitle was quarantined", Cause: ErrHistory}
	}
	s.mu.Lock()
	s.operations[opHash] = response
	s.mu.Unlock()
	return response, nil
}

func (s *Service) itemLockLocked(itemID string) *sync.Mutex {
	if lock := s.itemLocks[itemID]; lock != nil {
		return lock
	}
	lock := &sync.Mutex{}
	s.itemLocks[itemID] = lock
	return lock
}

func (s *Service) writeNewFile(directory, mediaPath string, artifact preview.Artifact, content []byte, operationID string) (string, error) {
	if err := s.guard.CheckDirectory(directory); err != nil {
		return "", &Error{Status: 422, Code: "media_path_unsafe", Message: "media path is unsafe for D3 Add", Cause: ErrUnsafeMediaPath}
	}
	base := safeMediaBase(mediaPath)
	if base == "" {
		return "", &Error{Status: 422, Code: "media_path_unsafe", Message: "media filename is unsafe for D3 Add", Cause: ErrUnsafeMediaPath}
	}
	ext := strings.ToLower(artifact.Format)
	if ext != "srt" && ext != "ass" && ext != "ssa" {
		return "", &Error{Status: 422, Code: "subtitle_format_unsupported", Message: "subtitle format is unsupported", Cause: ErrUnsupportedFormat}
	}
	language := strings.TrimSpace(artifact.Language)
	if language == "" || strings.IndexFunc(language, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) || r == '/' || r == '\\' }) >= 0 {
		return "", &Error{Status: 422, Code: "invalid_request", Message: "invalid subtitle language", Cause: ErrInvalidRequest}
	}
	prefix := base + ".subbridge." + language
	if len([]byte(prefix)) > maxFilenameBytes {
		prefix = string([]byte(prefix)[:maxFilenameBytes])
	}
	operationHash := sha256.Sum256([]byte(operationID))
	temp := filepath.Join(directory, ".subbridge-"+hex.EncodeToString(operationHash[:8])+".tmp")
	file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", &Error{Status: 503, Code: "subtitle_write_failed", Message: "subtitle write failed", Cause: ErrWrite}
	}
	keepTemp := false
	defer func() {
		_ = file.Close()
		if !keepTemp {
			_ = os.Remove(temp)
		}
	}()
	if _, err := file.Write(content); err != nil {
		return "", &Error{Status: 503, Code: "subtitle_write_failed", Message: "subtitle write failed", Cause: ErrWrite}
	}
	if err := file.Sync(); err != nil {
		return "", &Error{Status: 503, Code: "subtitle_write_failed", Message: "subtitle write failed", Cause: ErrWrite}
	}
	if err := file.Close(); err != nil {
		return "", &Error{Status: 503, Code: "subtitle_write_failed", Message: "subtitle write failed", Cause: ErrWrite}
	}
	if err := os.Chmod(temp, 0o644); err != nil {
		return "", &Error{Status: 503, Code: "subtitle_write_failed", Message: "subtitle write failed", Cause: ErrWrite}
	}
	for version := 1; version <= maxVersions; version++ {
		name := prefix + "." + ext
		if version > 1 {
			name = fmt.Sprintf("%s.v%d.%s", prefix, version, ext)
		}
		if len([]byte(name)) > maxFilenameBytes {
			return "", &Error{Status: 422, Code: "invalid_request", Message: "subtitle filename is too long", Cause: ErrInvalidRequest}
		}
		target := filepath.Join(directory, name)
		if err := os.Link(temp, target); err == nil {
			keepTemp = false
			_ = os.Remove(temp)
			return name, nil
		} else if errors.Is(err, os.ErrExist) {
			continue
		} else {
			return "", &Error{Status: 503, Code: "subtitle_write_failed", Message: "subtitle write failed", Cause: ErrWrite}
		}
	}
	return "", &Error{Status: 409, Code: "subtitle_name_conflict", Message: "no safe subtitle filename is available", Cause: ErrWrite}
}

func (s *Service) pollVisible(parent context.Context, itemID, sourceID, target string) bool {
	deadline := time.Now().Add(time.Duration(s.settings.RefreshTimeoutSeconds) * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(parent, getItemTimeout)
		item, err := s.emby.GetItem(ctx, itemID)
		cancel()
		if err == nil && hasSubtitlePath(item, sourceID, target, s.mapper) {
			return true
		}
		select {
		case <-parent.Done():
			return false
		case <-time.After(250 * time.Millisecond):
		}
	}
	return false
}

func hasSubtitlePath(item domain.EmbyItem, sourceID, target string, mapper *pathmap.Mapper) bool {
	selected, err := media.SelectSource(item, sourceID)
	if err != nil || selected.MediaStreams == nil {
		return false
	}
	cleanTarget := filepath.Clean(target)
	for _, stream := range *selected.MediaStreams {
		if !strings.EqualFold(strings.TrimSpace(stream.Type), "subtitle") || stream.Path == "" {
			continue
		}
		if mapper != nil {
			if mapped, mapErr := mapper.Map(stream.Path); mapErr == nil && filepath.Clean(mapped) == cleanTarget {
				return true
			}
		}
	}
	return false
}

func verifyFile(filename string, expected []byte) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, int64(len(expected))+1))
	if err != nil || len(content) != len(expected) || string(content) != string(expected) {
		return ErrWrite
	}
	return nil
}

func (s *Service) quarantine(source, operationID, fileName string) {
	if source == "" || s == nil || s.settings.QuarantineDir == "" {
		return
	}
	hash := sha256.Sum256([]byte(operationID))
	destination := filepath.Join(s.settings.QuarantineDir, hex.EncodeToString(hash[:8])+"-"+fileName)
	if err := os.Rename(source, destination); err == nil {
		return
	}
	// A cross-filesystem quarantine is handled by copy-then-remove only after
	// the copy has been flushed and atomically renamed into its final name.
	tmp := destination + ".tmp"
	in, err := os.Open(source)
	if err != nil {
		return
	}
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = in.Close()
		return
	}
	_, copyErr := io.Copy(out, in)
	_ = in.Close()
	if copyErr == nil {
		copyErr = out.Sync()
	}
	_ = out.Close()
	if copyErr == nil {
		copyErr = os.Chmod(tmp, 0o600)
	}
	if copyErr == nil {
		copyErr = os.Rename(tmp, destination)
	}
	if copyErr == nil {
		_ = os.Remove(source)
	} else {
		_ = os.Remove(tmp)
	}
}

func (s *Service) writeHistory(result AddResponse) error {
	hash := sha256.Sum256([]byte(result.OperationID))
	record := recoveryRecord{Version: 2, OperationHash: hex.EncodeToString(hash[:]), OperationID: result.OperationID, Type: OperationAdd,
		Fingerprint: operationFingerprint(OperationAdd, result.ItemID, result.MediaSourceID, "", result.ContentHash), ItemID: result.ItemID, MediaSourceID: result.MediaSourceID,
		FileName: result.FileName, Language: result.Language, Format: result.Format, ByteLength: result.ByteLength, ContentHash: result.ContentHash, Status: "verified", CreatedAt: result.CreatedAt}
	return s.writeRecoveryHistory(record)
}

func (s *Service) loadHistory(operationHash [32]byte, operationID, itemID, sourceID, directory, expectedHash string) (AddResponse, bool, bool) {
	filename := filepath.Join(s.settings.HistoryDir, hex.EncodeToString(operationHash[:])+".json")
	data, err := os.ReadFile(filename)
	if err != nil {
		return AddResponse{}, false, false
	}
	var current recoveryRecord
	if json.Unmarshal(data, &current) == nil && current.Version == 2 {
		if !validOperationRecord(current) || current.Type != OperationAdd || current.OperationID != operationID || current.OperationHash != hex.EncodeToString(operationHash[:]) {
			return AddResponse{}, true, true
		}
		if current.ItemID != itemID || current.MediaSourceID != sourceID || current.ContentHash != expectedHash || current.Fingerprint != operationFingerprint(OperationAdd, itemID, sourceID, "", expectedHash) {
			return AddResponse{}, true, true
		}
		content, readErr := os.ReadFile(filepath.Join(directory, current.FileName))
		if readErr != nil || hashBytes(content) != current.ContentHash || len(content) != current.ByteLength {
			return AddResponse{}, false, false
		}
		return AddResponse{OperationID: operationID, Type: OperationAdd, ItemID: current.ItemID, MediaSourceID: current.MediaSourceID, FileName: current.FileName, Language: current.Language, Format: current.Format, ByteLength: current.ByteLength, ContentHash: current.ContentHash, Refresh: "verified", Status: "verified", CreatedAt: current.CreatedAt}, true, false
	}
	var record historyRecord
	if json.Unmarshal(data, &record) != nil || record.Version != 1 || record.OperationHash != hex.EncodeToString(operationHash[:]) {
		return AddResponse{}, false, false
	}
	if record.ItemID != itemID || record.MediaSourceID != sourceID || record.ContentHash != expectedHash {
		return AddResponse{}, true, true
	}
	if record.FileName == "" || record.FileName == "." || record.FileName == ".." || strings.ContainsAny(record.FileName, `/\\:`) || strings.IndexFunc(record.FileName, unicode.IsControl) >= 0 {
		return AddResponse{}, false, false
	}
	content, err := os.ReadFile(filepath.Join(directory, record.FileName))
	if err != nil || hashBytes(content) != record.ContentHash || len(content) != record.ByteLength {
		return AddResponse{}, false, false
	}
	return AddResponse{OperationID: operationID, Type: OperationAdd, ItemID: record.ItemID, MediaSourceID: record.MediaSourceID, FileName: record.FileName, Language: record.Language, Format: record.Format, ByteLength: record.ByteLength, ContentHash: record.ContentHash, Refresh: "verified", Status: "verified", CreatedAt: record.CreatedAt}, true, false
}

func safeMediaBase(value string) string {
	base := filepath.Base(filepath.Clean(value))
	ext := strings.ToLower(filepath.Ext(base))
	if ext != "" {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if base == "" || base == "." || base == ".." || strings.ContainsAny(base, `/\\:`) || strings.IndexFunc(base, unicode.IsControl) >= 0 {
		return ""
	}
	return base
}

func validID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsAny(value, `/\\`) && strings.IndexFunc(value, unicode.IsControl) < 0 && value != "." && value != ".."
}

func validOperationID(value string) bool {
	return len([]byte(value)) >= 8 && len([]byte(value)) <= maxOperationID && value == strings.TrimSpace(value) && strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) < 0 && !strings.ContainsAny(value, `/\\`)
}

func hashBytes(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

func mapItemError(err error) error {
	if err == nil {
		return nil
	}
	return &Error{Status: 502, Code: "emby_unavailable", Message: "Emby is unavailable", Cause: err}
}

func mapMediaError(err error) error {
	switch {
	case errors.Is(err, media.ErrMediaSourceSelectionRequired):
		return &Error{Status: 409, Code: "media_source_selection_required", Message: "media source selection is required", Cause: err}
	case errors.Is(err, media.ErrMediaSourceNotFound):
		return &Error{Status: 409, Code: "media_source_mismatch", Message: "media source does not match the item", Cause: err}
	case errors.Is(err, media.ErrMappedPathUnavailable), errors.Is(err, media.ErrMediaSourceUnavailable):
		return &Error{Status: 422, Code: "media_path_unsafe", Message: "media path is unavailable for D3 Add", Cause: err}
	default:
		return &Error{Status: 422, Code: "media_path_unsafe", Message: "media path is unsafe for D3 Add", Cause: err}
	}
}

func mapArtifactError(err error) error {
	status, code, message := 410, "artifact_expired", "preview artifact has expired"
	if errors.Is(err, preview.ErrArtifactInvalid) {
		status, code, message = 404, "artifact_invalid", "preview artifact is unavailable"
	} else if errors.Is(err, preview.ErrArtifactUnavailable) {
		status, code, message = 503, "preview_store_unavailable", "preview artifact is unavailable"
	}
	return &Error{Status: status, Code: code, Message: message, Cause: err}
}
