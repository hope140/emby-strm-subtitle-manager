package d3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/hope140/subbridge/internal/domain"
	"github.com/hope140/subbridge/internal/inventory"
	"github.com/hope140/subbridge/internal/media"
	"github.com/hope140/subbridge/internal/preview"
	"github.com/hope140/subbridge/internal/subtitle"
)

type OperationType string

const (
	OperationAdd     OperationType = "add"
	OperationReplace OperationType = "replace"
	OperationDelete  OperationType = "delete"
	OperationRestore OperationType = "restore"
)

const (
	defaultHistoryLimit = 50
	maxHistoryLimit     = 100
)

type ReplaceRequest struct {
	ArtifactToken string
	MediaSourceID string
	OperationID   string
}

type DeleteRequest struct {
	MediaSourceID string
	OperationID   string
}

type RestoreRequest struct {
	MediaSourceID string
	OperationID   string
}

// OperationResponse is the safe public result shared by the recovery APIs.
// It intentionally contains no private recovery path or uploaded file name.
type OperationResponse struct {
	OperationID      string        `json:"operation_id"`
	Type             OperationType `json:"type"`
	ItemID           string        `json:"item_id"`
	MediaSourceID    string        `json:"media_source_id"`
	SubtitleID       string        `json:"subtitle_id,omitempty"`
	FileName         string        `json:"file_name,omitempty"`
	Language         string        `json:"language,omitempty"`
	Format           string        `json:"format,omitempty"`
	ByteLength       int           `json:"byte_length,omitempty"`
	ContentHash      string        `json:"content_sha256,omitempty"`
	Status           string        `json:"status"`
	CreatedAt        time.Time     `json:"created_at"`
	RestoreSupported *bool         `json:"restore_supported,omitempty"`
	RestoreErrorCode string        `json:"restore_error_code,omitempty"`
}

type OperationSummary = OperationResponse

type operationMemory struct {
	fingerprint string
	response    OperationResponse
}

// rollbackHooks are intentionally package-private test seams. Production
// behavior always uses the checked filesystem and history operations below.
type rollbackHooks struct {
	restore      func(destination, source, expectedHash string) error
	remove       func(path string) error
	quarantine   func(source, operationID, expectedHash string) error
	writeHistory func(record recoveryRecord) error
}

type rollbackPlan struct {
	itemID, sourceID string

	restorePath, recoveryPath, restoreHash string
	verifyPath, verifyHash, verifyFormat   string
	verifyVisible                          bool

	quarantinePath, quarantineHash, operationID string
	removePath                                  string
}

// recoveryRecord is private state in history_dir. Its recovery locator is a
// basename only; the configured archive/trash directory is never serialized
// into an HTTP response or log line.
type recoveryRecord struct {
	Version          int           `json:"version"`
	OperationHash    string        `json:"operation_hash"`
	OperationID      string        `json:"operation_id"`
	Type             OperationType `json:"type"`
	Fingerprint      string        `json:"fingerprint"`
	ItemID           string        `json:"item_id"`
	MediaSourceID    string        `json:"media_source_id"`
	SubtitleID       string        `json:"subtitle_id,omitempty"`
	FileName         string        `json:"file_name,omitempty"`
	Language         string        `json:"language,omitempty"`
	Format           string        `json:"format,omitempty"`
	ByteLength       int           `json:"byte_length,omitempty"`
	ContentHash      string        `json:"content_sha256,omitempty"`
	Status           string        `json:"status"`
	CreatedAt        time.Time     `json:"created_at"`
	RecoveryKind     string        `json:"recovery_kind,omitempty"`
	RecoveryFile     string        `json:"recovery_file,omitempty"`
	OriginalFileName string        `json:"original_file_name,omitempty"`
	// OriginalLocation is deliberately a location class rather than a path.
	// Restore uses it only to select the previously verified item/source
	// category. Legacy source history on a current STRM is rejected rather
	// than reinterpreted as an Item sidecar, without exposing or persisting a media path.
	OriginalLocation  string `json:"original_location,omitempty"`
	OriginalHash      string `json:"original_hash,omitempty"`
	OriginalFormat    string `json:"original_format,omitempty"`
	RestoresOperation string `json:"restores_operation,omitempty"`
}

func (r recoveryRecord) summary() OperationSummary {
	return OperationSummary{OperationID: r.OperationID, Type: r.Type, ItemID: r.ItemID, MediaSourceID: r.MediaSourceID,
		SubtitleID: r.SubtitleID, FileName: r.FileName, Language: r.Language, Format: r.Format,
		ByteLength: r.ByteLength, ContentHash: r.ContentHash, Status: r.Status, CreatedAt: r.CreatedAt}
}

func (s *Service) Replace(ctx context.Context, itemID, subtitleID string, request ReplaceRequest) (OperationResponse, error) {
	if err := s.validateWriteRequest(itemID, request.MediaSourceID, request.OperationID); err != nil || !validSubtitleID(subtitleID) || request.ArtifactToken == "" {
		return OperationResponse{}, invalidD3Request("invalid D3 Replace request")
	}
	unlock := s.lockItem(itemID)
	defer unlock()

	item, mediaCtx, target, generation, err := s.loadWritableItem(ctx, itemID, request.MediaSourceID)
	if err != nil {
		return OperationResponse{}, err
	}
	artifact, content, err := s.artifacts.GetContent(request.ArtifactToken, preview.Binding{ItemID: item.ID, SourceID: mediaCtx.MediaSourceID, AuthContext: s.authContext, AllowlistGeneration: generation})
	if err != nil {
		return OperationResponse{}, mapArtifactError(err)
	}
	if err := s.validateArtifact(artifact, content); err != nil {
		return OperationResponse{}, err
	}
	fingerprint := operationFingerprint(OperationReplace, item.ID, mediaCtx.MediaSourceID, subtitleID, artifact.ContentHash)
	if replay, found, conflict := s.replayRecovery(request.OperationID, fingerprint); found {
		if conflict {
			return OperationResponse{}, operationConflict()
		}
		return replay, nil
	}
	resolved, _, oldHash, err := s.resolveManagedSubtitle(mediaCtx, subtitleID)
	if err != nil {
		return OperationResponse{}, err
	}
	originalLocation, err := recoveryLocation(target)
	if err != nil {
		return OperationResponse{}, &Error{Status: 409, Code: "subtitle_inventory_incomplete", Message: "subtitle inventory is incomplete", Cause: err}
	}

	newName, err := s.writeNewFile(target.LocalDirectory, target.LocalPath, artifact, content, request.OperationID)
	if err != nil {
		return OperationResponse{}, err
	}
	newPath := filepath.Join(target.LocalDirectory, newName)
	newRollback := rollbackPlan{itemID: item.ID, sourceID: mediaCtx.MediaSourceID, verifyPath: resolved.Path(), verifyHash: oldHash, verifyFormat: resolved.Subtitle.Format, verifyVisible: true,
		quarantinePath: newPath, quarantineHash: artifact.ContentHash, operationID: request.OperationID}
	if err := verifyFile(newPath, content); err != nil {
		return OperationResponse{}, s.rollbackOrOriginal(newRollback, writeVerificationError())
	}
	if err := s.refreshAndRequire(ctx, item.ID, mediaCtx.MediaSourceID, newPath, true); err != nil {
		return OperationResponse{}, s.rollbackOrOriginal(newRollback, err)
	}

	opHash := sha256.Sum256([]byte(request.OperationID))
	recoveryName := recoveryName("archive", opHash)
	recoveryPath := filepath.Join(s.settings.ArchiveDir, recoveryName)
	if err := s.moveToRecovery(resolved.Path(), recoveryPath, oldHash); err != nil {
		return OperationResponse{}, s.rollbackOrOriginal(newRollback, recoveryError("subtitle_archive_failed", "old subtitle could not be archived"))
	}
	fullRollback := newRollback
	fullRollback.restorePath, fullRollback.recoveryPath, fullRollback.restoreHash = resolved.Path(), recoveryPath, oldHash
	if err := s.refreshAndRequire(ctx, item.ID, mediaCtx.MediaSourceID, newPath, true); err != nil || !s.pollAbsent(ctx, item.ID, mediaCtx.MediaSourceID, resolved.Path()) {
		if err != nil {
			return OperationResponse{}, s.rollbackOrOriginal(fullRollback, err)
		}
		return OperationResponse{}, s.rollbackOrOriginal(fullRollback, &Error{Status: 502, Code: "emby_subtitle_not_visible", Message: "Emby did not verify the replacement subtitle", Cause: ErrNotVisible})
	}

	response := OperationResponse{OperationID: request.OperationID, Type: OperationReplace, ItemID: item.ID, MediaSourceID: mediaCtx.MediaSourceID,
		SubtitleID: subtitleID, FileName: newName, Language: artifact.Language, Format: artifact.Format, ByteLength: artifact.ByteLength,
		ContentHash: artifact.ContentHash, Status: "verified", CreatedAt: s.now().UTC()}
	record := recoveryRecord{Version: 2, OperationHash: hex.EncodeToString(opHash[:]), OperationID: request.OperationID, Type: OperationReplace,
		Fingerprint: fingerprint, ItemID: item.ID, MediaSourceID: mediaCtx.MediaSourceID, SubtitleID: subtitleID, FileName: newName,
		Language: artifact.Language, Format: artifact.Format, ByteLength: artifact.ByteLength, ContentHash: artifact.ContentHash, Status: "verified", CreatedAt: response.CreatedAt,
		RecoveryKind: "archive", RecoveryFile: recoveryName, OriginalFileName: resolved.Subtitle.FileName, OriginalLocation: originalLocation, OriginalHash: oldHash, OriginalFormat: resolved.Subtitle.Format}
	if err := s.persistRecoveryHistory(record); err != nil {
		return OperationResponse{}, s.rollbackOrOriginal(fullRollback, &Error{Status: 503, Code: "d3_history_unavailable", Message: "subtitle operation history could not be recorded", Cause: ErrHistory})
	}
	s.rememberRecovery(opHash, fingerprint, response)
	return response, nil
}

func (s *Service) Delete(ctx context.Context, itemID, subtitleID string, request DeleteRequest) (OperationResponse, error) {
	if err := s.validateWriteRequest(itemID, request.MediaSourceID, request.OperationID); err != nil || !validSubtitleID(subtitleID) {
		return OperationResponse{}, invalidD3Request("invalid D3 Delete request")
	}
	unlock := s.lockItem(itemID)
	defer unlock()
	item, mediaCtx, target, _, err := s.loadWritableItem(ctx, itemID, request.MediaSourceID)
	if err != nil {
		return OperationResponse{}, err
	}
	if replay, found, conflict := s.replayDelete(request.OperationID, item.ID, mediaCtx.MediaSourceID, subtitleID); found {
		if conflict {
			return OperationResponse{}, operationConflict()
		}
		return replay, nil
	}
	resolved, _, oldHash, err := s.resolveManagedSubtitle(mediaCtx, subtitleID)
	if err != nil {
		return OperationResponse{}, err
	}
	originalLocation, err := recoveryLocation(target)
	if err != nil {
		return OperationResponse{}, &Error{Status: 409, Code: "subtitle_inventory_incomplete", Message: "subtitle inventory is incomplete", Cause: err}
	}
	fingerprint := operationFingerprint(OperationDelete, item.ID, mediaCtx.MediaSourceID, subtitleID, oldHash)
	if replay, found, conflict := s.replayRecovery(request.OperationID, fingerprint); found {
		if conflict {
			return OperationResponse{}, operationConflict()
		}
		return replay, nil
	}
	opHash := sha256.Sum256([]byte(request.OperationID))
	recoveryName := recoveryName("trash", opHash)
	recoveryPath := filepath.Join(s.settings.TrashDir, recoveryName)
	if err := s.moveToRecovery(resolved.Path(), recoveryPath, oldHash); err != nil {
		return OperationResponse{}, recoveryError("subtitle_trash_failed", "subtitle could not be moved to trash")
	}
	rollback := rollbackPlan{itemID: item.ID, sourceID: mediaCtx.MediaSourceID, restorePath: resolved.Path(), recoveryPath: recoveryPath, restoreHash: oldHash,
		verifyPath: resolved.Path(), verifyHash: oldHash, verifyFormat: resolved.Subtitle.Format, verifyVisible: true}
	if err := s.refreshAndRequire(ctx, item.ID, mediaCtx.MediaSourceID, resolved.Path(), false); err != nil {
		return OperationResponse{}, s.rollbackOrOriginal(rollback, err)
	}
	response := OperationResponse{OperationID: request.OperationID, Type: OperationDelete, ItemID: item.ID, MediaSourceID: mediaCtx.MediaSourceID,
		SubtitleID: subtitleID, FileName: resolved.Subtitle.FileName, Format: resolved.Subtitle.Format, ContentHash: oldHash, Status: "verified", CreatedAt: s.now().UTC()}
	record := recoveryRecord{Version: 2, OperationHash: hex.EncodeToString(opHash[:]), OperationID: request.OperationID, Type: OperationDelete,
		Fingerprint: fingerprint, ItemID: item.ID, MediaSourceID: mediaCtx.MediaSourceID, SubtitleID: subtitleID, FileName: resolved.Subtitle.FileName,
		Format: resolved.Subtitle.Format, ContentHash: oldHash, Status: "verified", CreatedAt: response.CreatedAt,
		RecoveryKind: "trash", RecoveryFile: recoveryName, OriginalFileName: resolved.Subtitle.FileName, OriginalLocation: originalLocation, OriginalHash: oldHash, OriginalFormat: resolved.Subtitle.Format}
	if err := s.persistRecoveryHistory(record); err != nil {
		return OperationResponse{}, s.rollbackOrOriginal(rollback, &Error{Status: 503, Code: "d3_history_unavailable", Message: "subtitle operation history could not be recorded", Cause: ErrHistory})
	}
	s.rememberRecovery(opHash, fingerprint, response)
	return response, nil
}

func (s *Service) Restore(ctx context.Context, sourceOperationID string, request RestoreRequest) (OperationResponse, error) {
	if !validOperationID(sourceOperationID) || !validOperationID(request.OperationID) || !validID(request.MediaSourceID) {
		return OperationResponse{}, invalidD3Request("invalid D3 Restore request")
	}
	sourceRecord, err := s.readRecoveryHistory(sourceOperationID)
	if err != nil {
		return OperationResponse{}, err
	}
	if sourceRecord.Type != OperationReplace && sourceRecord.Type != OperationDelete || sourceRecord.RecoveryKind == "" || sourceRecord.RecoveryFile == "" || sourceRecord.OriginalFileName == "" || sourceRecord.OriginalLocation == "" || sourceRecord.OriginalHash == "" {
		return OperationResponse{}, &Error{Status: 409, Code: "restore_unavailable", Message: "the selected operation cannot be restored", Cause: ErrInvalidRequest}
	}
	if sourceRecord.MediaSourceID != request.MediaSourceID {
		return OperationResponse{}, &Error{Status: 409, Code: "media_source_mismatch", Message: "media source does not match the selected operation", Cause: ErrInvalidRequest}
	}
	if err := s.validateWriteRequest(sourceRecord.ItemID, request.MediaSourceID, request.OperationID); err != nil {
		return OperationResponse{}, err
	}
	unlock := s.lockItem(sourceRecord.ItemID)
	defer unlock()
	currentItem, err := s.loadRestoreItemSource(ctx, sourceRecord.ItemID, request.MediaSourceID)
	if err != nil {
		return OperationResponse{}, err
	}
	if media.IsSTRMPath(currentItem.Path) && sourceRecord.OriginalLocation == string(media.WriteTargetLocationSource) {
		return OperationResponse{}, &Error{Status: 409, Code: "strm_history_location_unsupported", Message: "the STRM recovery history cannot be safely restored", Cause: ErrHistory}
	}
	item, mediaCtx, target, _, err := s.loadWritableItem(ctx, sourceRecord.ItemID, request.MediaSourceID)
	if err != nil {
		return OperationResponse{}, err
	}
	fingerprint := operationFingerprint(OperationRestore, item.ID, mediaCtx.MediaSourceID, sourceRecord.OperationID, sourceRecord.OriginalHash)
	if replay, found, conflict := s.replayRecovery(request.OperationID, fingerprint); found {
		if conflict {
			return OperationResponse{}, operationConflict()
		}
		return replay, nil
	}
	if !safeRecoveryName(sourceRecord.RecoveryFile) || !safeFileName(sourceRecord.OriginalFileName) {
		return OperationResponse{}, &Error{Status: 503, Code: "restore_unavailable", Message: "the selected operation cannot be restored", Cause: ErrHistory}
	}
	recoveryDirectory := s.settings.ArchiveDir
	if sourceRecord.RecoveryKind == "trash" {
		recoveryDirectory = s.settings.TrashDir
	} else if sourceRecord.RecoveryKind != "archive" {
		return OperationResponse{}, &Error{Status: 503, Code: "restore_unavailable", Message: "the selected operation cannot be restored", Cause: ErrHistory}
	}
	recoveryPath := filepath.Join(recoveryDirectory, sourceRecord.RecoveryFile)
	if _, _, err := s.readManagedFile(recoveryPath, sourceRecord.OriginalFormat, sourceRecord.OriginalHash); err != nil {
		return OperationResponse{}, &Error{Status: 409, Code: "restore_hash_mismatch", Message: "the recovery subtitle cannot be verified", Cause: ErrArtifact}
	}
	restoreDirectory := target.LocalDirectory
	switch sourceRecord.OriginalLocation {
	case "item":
		restoreDirectory = mediaCtx.LocalDirectory
	case "source":
		// The selected source is still re-resolved above and target was built
		// from that exact source, so this never trusts a historical path.
	default:
		return OperationResponse{}, &Error{Status: 503, Code: "restore_unavailable", Message: "the selected operation cannot be restored", Cause: ErrHistory}
	}
	if err := s.guard.CheckDirectory(restoreDirectory); err != nil {
		return OperationResponse{}, &Error{Status: 503, Code: "restore_unavailable", Message: "the selected operation cannot be restored", Cause: err}
	}
	targetPath := filepath.Join(restoreDirectory, sourceRecord.OriginalFileName)
	if err := s.restoreRecovery(targetPath, recoveryPath, sourceRecord.OriginalHash); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return OperationResponse{}, &Error{Status: 409, Code: "restore_target_conflict", Message: "a subtitle with the restore target name already exists", Cause: err}
		}
		return OperationResponse{}, recoveryError("restore_failed", "subtitle could not be restored")
	}
	rollback := rollbackPlan{itemID: item.ID, sourceID: mediaCtx.MediaSourceID, verifyPath: targetPath, verifyHash: sourceRecord.OriginalHash, verifyFormat: sourceRecord.OriginalFormat, removePath: targetPath}
	if err := s.refreshAndRequire(ctx, item.ID, mediaCtx.MediaSourceID, targetPath, true); err != nil {
		return OperationResponse{}, s.rollbackOrOriginal(rollback, err)
	}
	opHash := sha256.Sum256([]byte(request.OperationID))
	response := OperationResponse{OperationID: request.OperationID, Type: OperationRestore, ItemID: item.ID, MediaSourceID: mediaCtx.MediaSourceID,
		SubtitleID: sourceRecord.SubtitleID, FileName: sourceRecord.OriginalFileName, Format: sourceRecord.OriginalFormat, ContentHash: sourceRecord.OriginalHash, Status: "verified", CreatedAt: s.now().UTC()}
	record := recoveryRecord{Version: 2, OperationHash: hex.EncodeToString(opHash[:]), OperationID: request.OperationID, Type: OperationRestore,
		Fingerprint: fingerprint, ItemID: item.ID, MediaSourceID: mediaCtx.MediaSourceID, SubtitleID: sourceRecord.SubtitleID, FileName: sourceRecord.OriginalFileName,
		Format: sourceRecord.OriginalFormat, ContentHash: sourceRecord.OriginalHash, Status: "verified", CreatedAt: response.CreatedAt, RestoresOperation: sourceRecord.OperationID}
	if err := s.persistRecoveryHistory(record); err != nil {
		return OperationResponse{}, s.rollbackOrOriginal(rollback, &Error{Status: 503, Code: "d3_history_unavailable", Message: "subtitle operation history could not be recorded", Cause: ErrHistory})
	}
	s.rememberRecovery(opHash, fingerprint, response)
	return response, nil
}

func (s *Service) ListOperations(itemID string, limit int) ([]OperationSummary, error) {
	if s == nil || !s.Enabled() {
		return nil, &Error{Status: 403, Code: "write_disabled", Message: "subtitle operations are disabled", Cause: ErrDisabled}
	}
	if !validID(itemID) {
		return nil, invalidD3Request("invalid subtitle operation query")
	}
	if limit == 0 {
		limit = defaultHistoryLimit
	}
	if limit < 1 || limit > maxHistoryLimit {
		return nil, invalidD3Request("invalid subtitle operation limit")
	}
	if allowed, _ := s.gate.Allows(itemID); !allowed {
		return nil, &Error{Status: 403, Code: "d3_item_not_allowed", Message: "item is not allowed for subtitle operations", Cause: ErrItemNotAllowed}
	}
	return s.listOperations(itemID, "", limit)
}

func (s *Service) listOperations(itemID, sourceID string, limit int) ([]OperationSummary, error) {
	if limit == 0 {
		limit = defaultHistoryLimit
	}
	if limit < 1 || limit > maxHistoryLimit {
		return nil, invalidD3Request("invalid subtitle operation limit")
	}
	entries, err := os.ReadDir(s.settings.HistoryDir)
	if err != nil {
		return nil, &Error{Status: 503, Code: "d3_history_unavailable", Message: "subtitle operation history is unavailable", Cause: ErrHistory}
	}
	result := make([]OperationSummary, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || !safeRecoveryName(entry.Name()) {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(s.settings.HistoryDir, entry.Name()))
		if readErr != nil || len(data) > 64<<10 {
			continue
		}
		var record recoveryRecord
		if json.Unmarshal(data, &record) != nil || record.Version != 2 || record.ItemID != itemID || (sourceID != "" && record.MediaSourceID != sourceID) || !validOperationRecord(record) {
			continue
		}
		result = append(result, record.summary())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// ListOperationsForSource returns only the selected source's history and
// annotates replace/delete records with the current, safe restore capability.
// The capability is public metadata only; no path or recovery filename is
// exposed. Restore decisions are still rechecked by Restore itself.
func (s *Service) ListOperationsForSource(ctx context.Context, itemID, sourceID string, limit int) ([]OperationSummary, error) {
	if s == nil || !s.Enabled() {
		return nil, &Error{Status: 403, Code: "write_disabled", Message: "subtitle operations are disabled", Cause: ErrDisabled}
	}
	if !validID(itemID) || !validID(sourceID) {
		return nil, invalidD3Request("invalid subtitle operation query")
	}
	if allowed, _ := s.gate.Allows(itemID); !allowed {
		return nil, &Error{Status: 403, Code: "d3_item_not_allowed", Message: "item is not allowed for subtitle operations", Cause: ErrItemNotAllowed}
	}
	item, err := s.loadRestoreItemSource(ctx, itemID, sourceID)
	if err != nil {
		return nil, err
	}
	operations, err := s.listOperations(itemID, sourceID, limit)
	if err != nil {
		return nil, err
	}
	filtered := make([]OperationSummary, 0, len(operations))
	for _, operation := range operations {
		if operation.MediaSourceID != sourceID {
			continue
		}
		if operation.Type == OperationReplace || operation.Type == OperationDelete {
			supported := true
			reason := ""
			if record, found := s.loadRecoveryRecord(operation.OperationID); !found {
				supported = false
				reason = "restore_unavailable"
			} else if media.IsSTRMPath(item.Path) && record.OriginalLocation == string(media.WriteTargetLocationSource) {
				supported = false
				reason = "strm_history_location_unsupported"
			} else if _, resolveErr := media.ResolveWriteTarget(item, sourceID, s.mapper, s.guard); resolveErr != nil {
				supported = false
				reason = writeCapabilityErrorCode(resolveErr)
			}
			operation.RestoreSupported = boolPointer(supported)
			if !supported {
				operation.RestoreErrorCode = reason
			}
		}
		filtered = append(filtered, operation)
	}
	return filtered, nil
}

func writeCapabilityErrorCode(err error) string {
	if errors.Is(err, media.ErrMediaSourceNotFound) {
		return "media_source_mismatch"
	}
	return "media_path_unsafe"
}

// rollbackOrOriginal retains every recovery copy and reports a stable error
// when compensation itself cannot be proven. A canceled client context must
// not prevent the server from attempting its recovery transaction.
func (s *Service) rollbackOrOriginal(plan rollbackPlan, original error) error {
	if err := s.rollback(context.Background(), plan); err != nil {
		return &Error{Status: 503, Code: "subtitle_rollback_failed", Message: "subtitle rollback could not be verified; manual recovery is required", Cause: ErrWrite}
	}
	return original
}

func (s *Service) rollback(ctx context.Context, plan rollbackPlan) error {
	if plan.restorePath != "" {
		if err := s.restoreForRollback(plan.restorePath, plan.recoveryPath, plan.restoreHash); err != nil {
			return err
		}
	}
	if plan.verifyPath != "" {
		if _, _, err := s.readManagedFile(plan.verifyPath, plan.verifyFormat, plan.verifyHash); err != nil {
			return err
		}
		if plan.verifyVisible {
			if err := s.refreshAndRequire(ctx, plan.itemID, plan.sourceID, plan.verifyPath, true); err != nil {
				return err
			}
		}
	}
	if plan.quarantinePath != "" {
		if err := s.quarantineForRollback(plan.quarantinePath, plan.operationID, plan.quarantineHash); err != nil {
			return err
		}
		if err := s.refreshAndRequire(ctx, plan.itemID, plan.sourceID, plan.quarantinePath, false); err != nil {
			return err
		}
	}
	if plan.removePath != "" {
		if err := s.removeForRollback(plan.removePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := s.refreshAndRequire(ctx, plan.itemID, plan.sourceID, plan.removePath, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) restoreForRollback(destination, source, expectedHash string) error {
	if s.rollbackHooks.restore != nil {
		return s.rollbackHooks.restore(destination, source, expectedHash)
	}
	return s.restoreRecovery(destination, source, expectedHash)
}

func (s *Service) removeForRollback(path string) error {
	if s.rollbackHooks.remove != nil {
		return s.rollbackHooks.remove(path)
	}
	return os.Remove(path)
}

func (s *Service) quarantineForRollback(source, operationID, expectedHash string) error {
	if s.rollbackHooks.quarantine != nil {
		return s.rollbackHooks.quarantine(source, operationID, expectedHash)
	}
	return s.quarantine(source, operationID, expectedHash)
}

func (s *Service) persistRecoveryHistory(record recoveryRecord) error {
	if s.rollbackHooks.writeHistory != nil {
		return s.rollbackHooks.writeHistory(record)
	}
	return s.writeRecoveryHistory(record)
}

func (s *Service) validateWriteRequest(itemID, sourceID, operationID string) error {
	if s == nil || !s.Enabled() {
		return &Error{Status: 403, Code: "write_disabled", Message: "subtitle writes are disabled", Cause: ErrDisabled}
	}
	if !validID(itemID) || !validID(sourceID) || !validOperationID(operationID) {
		return invalidD3Request("invalid subtitle write request")
	}
	allowed, _ := s.gate.Allows(itemID)
	if !allowed {
		return &Error{Status: 403, Code: "d3_item_not_allowed", Message: "item is not allowed for subtitle writes", Cause: ErrItemNotAllowed}
	}
	return nil
}

func (s *Service) lockItem(itemID string) func() {
	s.mu.Lock()
	lock := s.itemLockLocked(itemID)
	s.mu.Unlock()
	s.global <- struct{}{}
	lock.Lock()
	return func() {
		lock.Unlock()
		<-s.global
	}
}

// loadRestoreItemSource re-reads only the safe Item/source facts needed to
// decide whether a persisted recovery location is still meaningful. It does
// not resolve a local write target, so an old STRM source history can return
// its stable incompatibility error even when Item.Path is unmapped, missing,
// a directory, or a symlink. The full writable target is reloaded after this
// preflight for every restore that remains eligible.
func (s *Service) loadRestoreItemSource(parent context.Context, itemID, sourceID string) (domain.EmbyItem, error) {
	if parent == nil {
		parent = context.Background()
	}
	itemCtx, cancel := context.WithTimeout(parent, getItemTimeout)
	item, err := s.emby.GetItem(itemCtx, itemID)
	cancel()
	if err != nil {
		return domain.EmbyItem{}, mapItemError(err)
	}
	if item.ID != itemID || (item.Type != "Movie" && item.Type != "Episode") {
		return domain.EmbyItem{}, &Error{Status: 502, Code: "emby_invalid_response", Message: "Emby response was invalid", Cause: ErrInvalidRequest}
	}
	if _, err := media.SelectSource(item, sourceID); err != nil {
		return domain.EmbyItem{}, mapMediaError(err)
	}
	return item, nil
}

func (s *Service) loadWritableItem(parent context.Context, itemID, sourceID string) (domain.EmbyItem, media.MediaContext, media.WriteTarget, uint64, error) {
	if parent == nil {
		parent = context.Background()
	}
	allowed, generation := s.gate.Allows(itemID)
	if !allowed {
		return domain.EmbyItem{}, media.MediaContext{}, media.WriteTarget{}, 0, &Error{Status: 403, Code: "d3_item_not_allowed", Message: "item is not allowed for subtitle writes", Cause: ErrItemNotAllowed}
	}
	itemCtx, cancel := context.WithTimeout(parent, getItemTimeout)
	item, err := s.emby.GetItem(itemCtx, itemID)
	cancel()
	if err != nil {
		return domain.EmbyItem{}, media.MediaContext{}, media.WriteTarget{}, 0, mapItemError(err)
	}
	if item.ID != itemID || (item.Type != "Movie" && item.Type != "Episode") {
		return domain.EmbyItem{}, media.MediaContext{}, media.WriteTarget{}, 0, &Error{Status: 502, Code: "emby_invalid_response", Message: "Emby response was invalid", Cause: ErrInvalidRequest}
	}
	mediaCtx, err := media.Build(item, media.BuildOptions{MediaSourceID: sourceID, Mapper: s.mapper, Guard: s.guard})
	if err != nil {
		return domain.EmbyItem{}, media.MediaContext{}, media.WriteTarget{}, 0, mapMediaError(err)
	}
	if mediaCtx.MappingStatus != media.MappingStatusMapped || mediaCtx.LocalDirectory == "" || mediaCtx.LocalPath == "" {
		return domain.EmbyItem{}, media.MediaContext{}, media.WriteTarget{}, 0, &Error{Status: 422, Code: "media_path_unsafe", Message: "media path is unavailable for subtitle writes", Cause: ErrUnsafeMediaPath}
	}
	target, err := media.ResolveWriteTarget(item, sourceID, s.mapper, s.guard)
	if err != nil {
		return domain.EmbyItem{}, media.MediaContext{}, media.WriteTarget{}, 0, mapMediaError(err)
	}
	return item, mediaCtx, target, generation, nil
}

func (s *Service) resolveManagedSubtitle(ctx media.MediaContext, subtitleID string) (inventory.ResolvedSubtitle, []byte, string, error) {
	resolved, err := s.inventory.Resolve(ctx, subtitleID)
	if err != nil {
		switch {
		case errors.Is(err, inventory.ErrSubtitleNotFound):
			return inventory.ResolvedSubtitle{}, nil, "", &Error{Status: 404, Code: "subtitle_not_found", Message: "subtitle was not found", Cause: err}
		case errors.Is(err, inventory.ErrSubtitleUnmanageable):
			return inventory.ResolvedSubtitle{}, nil, "", &Error{Status: 409, Code: "subtitle_unmanageable", Message: "subtitle is not safely manageable", Cause: err}
		default:
			return inventory.ResolvedSubtitle{}, nil, "", &Error{Status: 409, Code: "subtitle_inventory_incomplete", Message: "subtitle inventory is incomplete", Cause: err}
		}
	}
	content, hash, err := s.readManagedFile(resolved.Path(), resolved.Subtitle.Format, "")
	if err != nil {
		return inventory.ResolvedSubtitle{}, nil, "", err
	}
	return resolved, content, hash, nil
}

func (s *Service) validateArtifact(artifact preview.Artifact, content []byte) error {
	if artifact.Format != "srt" && artifact.Format != "ass" && artifact.Format != "ssa" {
		return &Error{Status: 422, Code: "subtitle_format_unsupported", Message: "subtitle format is unsupported", Cause: ErrUnsupportedFormat}
	}
	if len(content) != artifact.ByteLength || hashBytes(content) != artifact.ContentHash {
		return &Error{Status: 502, Code: "artifact_invalid", Message: "preview artifact is unavailable", Cause: ErrArtifact}
	}
	return nil
}

func (s *Service) readManagedFile(filename, format, expectedHash string) ([]byte, string, error) {
	content, hash, err := s.readBoundedFile(filename)
	if err != nil {
		return nil, "", &Error{Status: 422, Code: "subtitle_invalid", Message: "subtitle content is unavailable", Cause: err}
	}
	if expectedHash != "" && hash != expectedHash {
		return nil, "", &Error{Status: 409, Code: "restore_hash_mismatch", Message: "the recovery subtitle cannot be verified", Cause: ErrArtifact}
	}
	if _, err := subtitle.ValidateAndParse(content, format, s.maxSubtitleBytes); err != nil {
		return nil, "", &Error{Status: 422, Code: "subtitle_invalid", Message: "subtitle content is not safely manageable", Cause: err}
	}
	return content, hash, nil
}

func (s *Service) readBoundedFile(filename string) ([]byte, string, error) {
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > s.maxSubtitleBytes {
		return nil, "", ErrWrite
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, "", err
	}
	content, readErr := io.ReadAll(io.LimitReader(file, s.maxSubtitleBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || int64(len(content)) > s.maxSubtitleBytes {
		return nil, "", ErrWrite
	}
	return content, hashBytes(content), nil
}

func (s *Service) refreshAndRequire(parent context.Context, itemID, sourceID, target string, shouldExist bool) error {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(s.settings.RefreshTimeoutSeconds)*time.Second)
	err := s.refresher.RefreshItem(ctx, itemID)
	cancel()
	if err != nil {
		return &Error{Status: 502, Code: "emby_refresh_failed", Message: "Emby refresh failed", Cause: ErrRefresh}
	}
	visible := s.pollVisible(parent, itemID, sourceID, target)
	if !shouldExist {
		visible = s.pollAbsent(parent, itemID, sourceID, target)
	}
	if !visible {
		return &Error{Status: 502, Code: "emby_subtitle_not_visible", Message: "Emby did not expose the expected subtitle state", Cause: ErrNotVisible}
	}
	return nil
}

func (s *Service) pollAbsent(parent context.Context, itemID, sourceID, target string) bool {
	deadline := time.Now().Add(time.Duration(s.settings.RefreshTimeoutSeconds) * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(parent, getItemTimeout)
		item, err := s.emby.GetItem(ctx, itemID)
		cancel()
		if err == nil && !hasSubtitlePath(item, sourceID, target, s.mapper) {
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

// moveToRecovery copies to a private same-directory temporary file, flushes
// and hashes it, commits with a non-overwriting link, and only then removes
// the media copy. It works across filesystems without relying on os.Rename.
func (s *Service) moveToRecovery(source, destination, expectedHash string) error {
	if !safeRecoveryName(filepath.Base(destination)) {
		return ErrWrite
	}
	content, hash, err := s.readBoundedFile(source)
	if err != nil || (expectedHash != "" && hash != expectedHash) {
		return ErrWrite
	}
	if reused, err := s.reuseRecoveryFile(source, destination, hash); reused || err != nil {
		return err
	}
	temporary := destination + ".tmp"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return ErrWrite
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(temporary)
		}
	}()
	if _, err := file.Write(content); err != nil || file.Sync() != nil || file.Close() != nil || os.Chmod(temporary, 0o600) != nil {
		return ErrWrite
	}
	verified, verifiedHash, err := s.readBoundedFile(temporary)
	if err != nil || len(verified) != len(content) || verifiedHash != hash {
		return ErrWrite
	}
	if err := os.Link(temporary, destination); err != nil {
		if errors.Is(err, fs.ErrExist) {
			reused, reuseErr := s.reuseRecoveryFile(source, destination, hash)
			if !reused && reuseErr == nil {
				return ErrWrite
			}
			return reuseErr
		}
		return ErrWrite
	}
	committed = true
	_ = os.Remove(temporary)
	if err := os.Remove(source); err != nil {
		_ = os.Remove(destination)
		return ErrWrite
	}
	return nil
}

// reuseRecoveryFile makes a retry with the same operation ID safe without
// treating a mismatched or unsafe private recovery file as interchangeable.
// A matching recovery hash proves the prior rollback material is the exact
// same file; only then may the current media copy be removed.
func (s *Service) reuseRecoveryFile(source, destination, expectedHash string) (bool, error) {
	info, err := os.Lstat(destination)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return true, ErrWrite
	}
	_, existingHash, err := s.readBoundedFile(destination)
	if err != nil {
		return true, ErrWrite
	}
	if existingHash != expectedHash {
		return true, fs.ErrExist
	}
	if err := os.Remove(source); err != nil {
		return true, ErrWrite
	}
	return true, nil
}

// restoreRecovery is a no-overwrite copy from archive/trash into the selected
// media directory. The private recovery copy remains until the caller has
// completed its full transaction and history record.
func (s *Service) restoreRecovery(destination, source, expectedHash string) error {
	if !safeFileName(filepath.Base(destination)) {
		return ErrWrite
	}
	content, hash, err := s.readBoundedFile(source)
	if err != nil || (expectedHash != "" && hash != expectedHash) {
		return ErrWrite
	}
	if _, err := os.Lstat(destination); err == nil {
		return fs.ErrExist
	} else if !errors.Is(err, fs.ErrNotExist) {
		return ErrWrite
	}
	temporaryHash := sha256.Sum256([]byte(destination + hash))
	temporary := filepath.Join(filepath.Dir(destination), ".subbridge-restore-"+hex.EncodeToString(temporaryHash[:8])+".tmp")
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return ErrWrite
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(temporary)
		}
	}()
	if _, err := file.Write(content); err != nil || file.Sync() != nil || file.Close() != nil || os.Chmod(temporary, 0o644) != nil {
		return ErrWrite
	}
	if err := os.Link(temporary, destination); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fs.ErrExist
		}
		return ErrWrite
	}
	committed = true
	_ = os.Remove(temporary)
	if err := verifyFile(destination, content); err != nil {
		_ = os.Remove(destination)
		return ErrWrite
	}
	return nil
}

func recoveryName(kind string, operationHash [32]byte) string {
	// Recovery files are addressed by the operation hash. Keeping the media
	// basename out of this private filename avoids filesystem NAME_MAX failures
	// for otherwise valid long sidecar names; the original basename remains a
	// separately validated history field used only after re-resolving Item/source.
	return hex.EncodeToString(operationHash[:8]) + "-" + kind + ".subbridge"
}

func safeFileName(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsAny(value, `/\\:`) && strings.IndexFunc(value, unicode.IsControl) < 0 && len([]byte(value)) <= maxFilenameBytes
}

func safeRecoveryName(value string) bool {
	return safeFileName(value) && len([]byte(value)) <= maxFilenameBytes+48
}

// recoveryLocation persists the explicit location class selected by the
// write-target resolver. Directory equality is intentionally not used: for a
// normal local media item the Item and source directories may be identical,
// but the history must still record that the source path was the write anchor.
func recoveryLocation(target media.WriteTarget) (string, error) {
	switch target.Location {
	case media.WriteTargetLocationItem, media.WriteTargetLocationSource:
		return string(target.Location), nil
	default:
		return "", ErrHistory
	}
}

func validSubtitleID(value string) bool {
	return strings.HasPrefix(value, "sub_v1_") && len(value) <= 256 && value == strings.TrimSpace(value) && !strings.ContainsAny(value, `/\\`) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func boolPointer(value bool) *bool {
	return &value
}

func operationFingerprint(kind OperationType, itemID, sourceID, target, contentHash string) string {
	payload := strings.Join([]string{string(kind), itemID, sourceID, target, contentHash}, "\x00")
	hash := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(hash[:])
}

func operationConflict() *Error {
	return &Error{Status: 409, Code: "operation_conflict", Message: "operation id is already bound to another subtitle operation", Cause: ErrOperationConflict}
}

func invalidD3Request(message string) *Error {
	return &Error{Status: 400, Code: "invalid_request", Message: message, Cause: ErrInvalidRequest}
}

func writeVerificationError() *Error {
	return &Error{Status: 503, Code: "write_verification_failed", Message: "subtitle write could not be verified", Cause: ErrWrite}
}

func recoveryError(code, message string) *Error {
	return &Error{Status: 503, Code: code, Message: message, Cause: ErrWrite}
}

func (s *Service) replayRecovery(operationID, fingerprint string) (OperationResponse, bool, bool) {
	hash := sha256.Sum256([]byte(operationID))
	s.mu.Lock()
	if _, exists := s.operations[hash]; exists {
		s.mu.Unlock()
		return OperationResponse{}, true, true
	}
	if memory, exists := s.recoveryOperations[hash]; exists {
		s.mu.Unlock()
		return memory.response, true, memory.fingerprint != fingerprint
	}
	s.mu.Unlock()
	record, found := s.loadRecoveryRecord(operationID)
	if !found {
		return OperationResponse{}, false, false
	}
	if record.Fingerprint != fingerprint {
		return OperationResponse{}, true, true
	}
	response := record.summary()
	s.rememberRecovery(hash, fingerprint, response)
	return response, true, false
}

// replayDelete handles the one valid retry shape where the target subtitle
// has already been removed, so its current hash cannot be recomputed. The
// persisted record still binds the operation to its original item, source and
// opaque subtitle identifier. Any other use of the operation ID conflicts.
func (s *Service) replayDelete(operationID, itemID, sourceID, subtitleID string) (OperationResponse, bool, bool) {
	hash := sha256.Sum256([]byte(operationID))
	s.mu.Lock()
	if _, exists := s.operations[hash]; exists {
		s.mu.Unlock()
		return OperationResponse{}, true, true
	}
	if memory, exists := s.recoveryOperations[hash]; exists {
		s.mu.Unlock()
		response := memory.response
		if response.Type != OperationDelete || response.ItemID != itemID || response.MediaSourceID != sourceID || response.SubtitleID != subtitleID {
			return OperationResponse{}, true, true
		}
		return response, true, false
	}
	s.mu.Unlock()
	record, found := s.loadRecoveryRecord(operationID)
	if !found {
		return OperationResponse{}, false, false
	}
	if record.Type != OperationDelete || record.ItemID != itemID || record.MediaSourceID != sourceID || record.SubtitleID != subtitleID {
		return OperationResponse{}, true, true
	}
	response := record.summary()
	s.rememberRecovery(hash, record.Fingerprint, response)
	return response, true, false
}

func (s *Service) rememberRecovery(hash [32]byte, fingerprint string, response OperationResponse) {
	s.mu.Lock()
	s.recoveryOperations[hash] = operationMemory{fingerprint: fingerprint, response: response}
	s.mu.Unlock()
}

func (s *Service) writeRecoveryHistory(record recoveryRecord) error {
	if !validOperationRecord(record) {
		return ErrHistory
	}
	data, err := json.Marshal(record)
	if err != nil || len(data) > 64<<10 {
		return ErrHistory
	}
	filename := filepath.Join(s.settings.HistoryDir, record.OperationHash+".json")
	temporary := filename + ".tmp"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return ErrHistory
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(temporary)
		}
	}()
	if _, err := file.Write(data); err != nil || file.Sync() != nil || file.Close() != nil {
		return ErrHistory
	}
	if err := os.Link(temporary, filename); err != nil {
		return ErrHistory
	}
	committed = true
	_ = os.Remove(temporary)
	return nil
}

func (s *Service) readRecoveryHistory(operationID string) (recoveryRecord, error) {
	record, found := s.loadRecoveryRecord(operationID)
	if !found {
		return recoveryRecord{}, &Error{Status: 404, Code: "operation_not_found", Message: "subtitle operation was not found", Cause: ErrHistory}
	}
	return record, nil
}

func (s *Service) loadRecoveryRecord(operationID string) (recoveryRecord, bool) {
	hash := sha256.Sum256([]byte(operationID))
	filename := filepath.Join(s.settings.HistoryDir, hex.EncodeToString(hash[:])+".json")
	data, err := os.ReadFile(filename)
	if err != nil || len(data) > 64<<10 {
		return recoveryRecord{}, false
	}
	var record recoveryRecord
	if json.Unmarshal(data, &record) != nil || record.Version != 2 || record.OperationID != operationID || record.OperationHash != hex.EncodeToString(hash[:]) || !validOperationRecord(record) {
		return recoveryRecord{}, false
	}
	return record, true
}

func validOperationRecord(record recoveryRecord) bool {
	if record.Version != 2 || !validOperationID(record.OperationID) || !validID(record.ItemID) || !validID(record.MediaSourceID) || record.OperationHash == "" || record.Fingerprint == "" || (record.Status != "verified" && record.Status != "validated") || record.CreatedAt.IsZero() {
		return false
	}
	switch record.Type {
	case OperationReplace, OperationDelete, OperationRestore, OperationAdd:
	default:
		return false
	}
	if record.FileName != "" && !safeFileName(record.FileName) {
		return false
	}
	if record.RecoveryFile != "" && !safeRecoveryName(record.RecoveryFile) {
		return false
	}
	if record.OriginalFileName != "" && !safeFileName(record.OriginalFileName) {
		return false
	}
	if record.OriginalLocation != "" && record.OriginalLocation != "item" && record.OriginalLocation != "source" {
		return false
	}
	if (record.Type == OperationReplace || record.Type == OperationDelete) && (record.RecoveryKind == "" || record.RecoveryFile == "" || record.OriginalFileName == "" || record.OriginalLocation == "" || record.OriginalHash == "" || (record.OriginalFormat != "srt" && record.OriginalFormat != "ass" && record.OriginalFormat != "ssa")) {
		return false
	}
	return true
}
