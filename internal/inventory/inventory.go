// Package inventory builds the read-only subtitle inventory for one selected
// media source. It deliberately never opens a media or subtitle file.
package inventory

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/hope140/subbridge/internal/domain"
	"github.com/hope140/subbridge/internal/media"
	"github.com/hope140/subbridge/internal/pathmap"
)

// FileSystem is intentionally smaller than fs.FS. Inventory may enumerate a
// directory and inspect metadata, but it has no operation with which to read
// file contents.
type FileSystem interface {
	ReadDir(name string) ([]fs.DirEntry, error)
	Lstat(name string) (fs.FileInfo, error)
	EvalSymlinks(path string) (string, error)
}

// OSFileSystem is the production implementation of FileSystem.
type OSFileSystem struct{}

func (OSFileSystem) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }
func (OSFileSystem) Lstat(name string) (fs.FileInfo, error)     { return os.Lstat(name) }
func (OSFileSystem) EvalSymlinks(name string) (string, error)   { return filepath.EvalSymlinks(name) }

// Kind identifies the source represented by a subtitle record.
type Kind string

const (
	KindEmbedded Kind = "embedded"
	KindExternal Kind = "external"
	KindSidecar  Kind = "sidecar"
)

type Discovery string

const (
	DiscoveryEmby       Discovery = "emby"
	DiscoveryFilesystem Discovery = "filesystem"
)

type Presence string

const (
	PresencePresent Presence = "present"
	PresenceMissing Presence = "missing"
	PresenceUnknown Presence = "unknown"
)

type IssueCode string

const (
	IssueUnmanaged IssueCode = "unmanaged"
	IssueDuplicate IssueCode = "duplicate"
)

// Subtitle is the safe public projection. It intentionally contains no path;
// FileName is only a basename and can be shown in a UI without disclosing the
// media server's filesystem layout.
type Subtitle struct {
	ID           string      `json:"id"`
	Kind         Kind        `json:"kind"`
	DiscoveredBy []Discovery `json:"discovered_by"`
	FileName     string      `json:"file_name,omitempty"`
	Language     string      `json:"language,omitempty"`
	Format       string      `json:"format,omitempty"`
	IsDefault    bool        `json:"is_default"`
	IsForced     bool        `json:"is_forced"`
	IsText       bool        `json:"is_text"`
	Manageable   bool        `json:"manageable"`
	Reason       string      `json:"unmanageable_reason,omitempty"`
	Indexes      []int       `json:"emby_stream_indexes,omitempty"`
}

type Issue struct {
	Code   IssueCode `json:"code"`
	Reason string    `json:"reason,omitempty"`
}

type Inventory struct {
	Subtitles []Subtitle `json:"subtitles"`
	Presence  Presence   `json:"presence"`
	Complete  bool       `json:"inventory_complete"`
	Issues    []Issue    `json:"issues,omitempty"`
	Warnings  []string   `json:"warnings,omitempty"`
}

// Options configures one inventory build. IdentityKey is required and must
// be independent from any Emby credential or provider identifier.
type Options struct {
	FileSystem  FileSystem
	IdentityKey []byte
	Mapper      *pathmap.Mapper
	Guard       *pathmap.PathGuard
}

var (
	ErrInvalidOptions     = errors.New("invalid inventory options")
	ErrConflictingStream  = errors.New("conflicting subtitle stream")
	ErrInvalidStreamIndex = errors.New("invalid subtitle stream index")
)

// Build constructs a bounded inventory for one MediaContext. A degraded
// filesystem or mapping state is represented as unknown presence; it is not
// silently reported as missing.
func Build(ctx media.MediaContext, options Options) (Inventory, error) {
	if options.FileSystem == nil || len(options.IdentityKey) < 32 {
		return Inventory{}, ErrInvalidOptions
	}
	result := Inventory{Subtitles: []Subtitle{}, Issues: []Issue{}, Warnings: []string{}}
	if ctx.MediaStreams == nil {
		result.Warnings = append(result.Warnings, "media_streams_unavailable")
	}
	if ctx.MappingStatus != media.MappingStatusMapped || ctx.LocalDirectory == "" {
		result.Warnings = append(result.Warnings, "media_directory_unavailable")
	}

	files, scanComplete := scanSidecars(options.FileSystem, ctx.LocalDirectory, baseName(ctx.LocalPath), ctx.ItemID, ctx.MediaSourceID, options.IdentityKey, &result)
	if !scanComplete {
		result.Complete = false
	}

	streamRecords, streamComplete, err := collectStreams(ctx, options, files, result.Issues, options.IdentityKey)
	if err != nil {
		return Inventory{}, err
	}
	result.Issues = streamRecords.issues
	result.Subtitles = streamRecords.subtitles
	for _, file := range files {
		if !file.merged {
			result.Subtitles = append(result.Subtitles, file.subtitle)
		}
	}
	sort.SliceStable(result.Subtitles, func(i, j int) bool {
		if result.Subtitles[i].Kind != result.Subtitles[j].Kind {
			return result.Subtitles[i].Kind < result.Subtitles[j].Kind
		}
		return result.Subtitles[i].ID < result.Subtitles[j].ID
	})
	result.Complete = scanComplete && streamComplete && ctx.MappingStatus == media.MappingStatusMapped && ctx.InventoryComplete
	if len(result.Subtitles) > 0 {
		result.Presence = PresencePresent
	} else if !result.Complete {
		result.Presence = PresenceUnknown
	} else {
		result.Presence = PresenceMissing
	}
	return result, nil
}

// Service is a reusable inventory builder with immutable options.
type Service struct{ options Options }

func New(options Options) (*Service, error) {
	if options.FileSystem == nil || len(options.IdentityKey) < 32 {
		return nil, ErrInvalidOptions
	}
	key := append([]byte(nil), options.IdentityKey...)
	options.IdentityKey = key
	return &Service{options: options}, nil
}

func (s *Service) Build(ctx media.MediaContext) (Inventory, error) {
	if s == nil {
		return Inventory{}, ErrInvalidOptions
	}
	return Build(ctx, s.options)
}

type sidecar struct {
	subtitle  Subtitle
	path      string
	canonical string
	eligible  bool
	merged    bool
}

type streamResult struct {
	subtitles []Subtitle
	issues    []Issue
}

func scanSidecars(fsys FileSystem, dir, mediaBase, itemID, sourceID string, key []byte, result *Inventory) ([]sidecar, bool) {
	if dir == "" || mediaBase == "" {
		return nil, false
	}
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		result.Warnings = append(result.Warnings, "sidecar_scan_unavailable")
		return nil, false
	}
	files := make([]sidecar, 0)
	for _, entry := range entries {
		name := entry.Name()
		ext := extension(name)
		if !supportedExtension(ext) || !validStem(name, ext, mediaBase) {
			continue
		}
		full := filepath.Join(dir, name)
		info, err := fsys.Lstat(full)
		if err != nil || info == nil || !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
			reason := "sidecar_not_regular"
			if err != nil || info == nil {
				reason = "sidecar_metadata_unavailable"
			}
			result.Issues = append(result.Issues, Issue{Code: IssueUnmanaged, Reason: reason})
			files = append(files, sidecar{path: full, subtitle: Subtitle{
				ID: subtitleID(key, itemID, sourceID, string(KindSidecar), name), Kind: KindSidecar,
				DiscoveredBy: []Discovery{DiscoveryFilesystem}, FileName: name,
				Format: strings.ToLower(ext), IsText: true, Manageable: false, Reason: reason,
			}})
			continue
		}
		canonical, err := fsys.EvalSymlinks(full)
		if err != nil || canonical == "" {
			result.Issues = append(result.Issues, Issue{Code: IssueUnmanaged, Reason: "sidecar_canonicalization_unavailable"})
			files = append(files, sidecar{path: full, subtitle: Subtitle{
				ID: subtitleID(key, itemID, sourceID, string(KindSidecar), name), Kind: KindSidecar,
				DiscoveredBy: []Discovery{DiscoveryFilesystem}, FileName: name,
				Format: strings.ToLower(ext), IsText: true, Manageable: false,
				Reason: "sidecar_canonicalization_unavailable",
			}})
			continue
		}
		canonical = canonicalPath(canonical)
		files = append(files, sidecar{path: full, canonical: canonical, subtitle: Subtitle{
			ID: subtitleID(key, itemID, sourceID, string(KindSidecar), name), Kind: KindSidecar,
			DiscoveredBy: []Discovery{DiscoveryFilesystem}, FileName: name,
			Format: strings.ToLower(ext), IsText: true, Manageable: true,
		}, eligible: true})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].subtitle.FileName < files[j].subtitle.FileName })
	return files, true
}

func collectStreams(ctx media.MediaContext, options Options, files []sidecar, issues []Issue, key []byte) (streamResult, bool, error) {
	result := streamResult{subtitles: []Subtitle{}, issues: append([]Issue(nil), issues...)}
	if ctx.MediaStreams == nil {
		return result, false, nil
	}
	seen := make(map[int]streamFingerprint)
	complete := true
	for ordinal, stream := range *ctx.MediaStreams {
		if !strings.EqualFold(strings.TrimSpace(stream.Type), "subtitle") {
			continue
		}
		kind := kindForStream(stream)
		fingerprint := streamFingerprintOf(kind, stream)
		idx := ordinal
		if stream.Index != nil {
			if *stream.Index < 0 {
				return streamResult{}, false, ErrInvalidStreamIndex
			}
			idx = *stream.Index
			if prior, exists := seen[idx]; exists {
				if prior != fingerprint {
					return streamResult{}, false, ErrConflictingStream
				}
				result.issues = append(result.issues, Issue{Code: IssueDuplicate, Reason: "duplicate_stream_index"})
				continue
			}
		}
		indexes := []int(nil)
		if stream.Index != nil {
			indexes = []int{idx}
		}
		sub := Subtitle{ID: subtitleID(key, ctx.ItemID, ctx.MediaSourceID, string(kind), safeBaseName(stream.Path), fmt.Sprintf("%d", idx)), Kind: kind,
			DiscoveredBy: []Discovery{DiscoveryEmby}, FileName: safeBaseName(stream.Path),
			Language: subtitleLanguage(stream), Format: streamFormat(stream),
			IsDefault: boolValue(stream.IsDefault), IsForced: boolValue(stream.IsForced),
			IsText: streamText(stream), Manageable: false, Indexes: indexes,
		}
		if kind == KindEmbedded {
			result.subtitles = append(result.subtitles, sub)
			if stream.IsTextSubtitleStream != nil && !*stream.IsTextSubtitleStream {
				result.issues = append(result.issues, Issue{Code: IssueUnmanaged, Reason: "embedded_nontext"})
			}
			if stream.Index != nil {
				seen[idx] = fingerprint
			}
			continue
		}
		matched := mergeExternal(&sub, stream, ctx, options, files)
		if matched != nil {
			sub.Manageable = true
			sub.ID = subtitleID(key, ctx.ItemID, ctx.MediaSourceID, string(KindSidecar), matched.file.subtitle.FileName)
			sub.Kind = KindSidecar
			sub.DiscoveredBy = []Discovery{DiscoveryEmby, DiscoveryFilesystem}
			if matched.file.merged {
				for i := range result.subtitles {
					if result.subtitles[i].ID == sub.ID {
						if stream.Index != nil {
							result.subtitles[i].Indexes = appendUniqueIndex(result.subtitles[i].Indexes, idx)
						}
						result.subtitles[i].IsDefault = result.subtitles[i].IsDefault || sub.IsDefault
						result.subtitles[i].IsForced = result.subtitles[i].IsForced || sub.IsForced
						break
					}
				}
				if stream.Index != nil {
					seen[idx] = fingerprint
				}
				continue
			}
			matched.file.merged = true
			matched.file.subtitle.Language = firstNonEmpty(sub.Language, matched.file.subtitle.Language)
			matched.file.subtitle.IsDefault = sub.IsDefault
			matched.file.subtitle.IsForced = sub.IsForced
			matched.file.subtitle.ID = sub.ID
			matched.file.subtitle.Kind = KindSidecar
			matched.file.subtitle.DiscoveredBy = append([]Discovery(nil), sub.DiscoveredBy...)
			matched.file.subtitle.Indexes = append([]int(nil), indexes...)
			result.subtitles = append(result.subtitles, matched.file.subtitle)
		} else {
			sub.Reason = "external_path_unavailable"
			result.issues = append(result.issues, Issue{Code: IssueUnmanaged, Reason: sub.Reason})
			result.subtitles = append(result.subtitles, sub)
			complete = false
		}
		if stream.Index != nil {
			seen[idx] = fingerprint
		}
	}
	return result, complete, nil
}

type matchedSidecar struct {
	file *sidecar
}

type streamFingerprint struct {
	Kind                        Kind
	Path, Language, Format      string
	IsText, IsDefault, IsForced bool
}

func streamFingerprintOf(kind Kind, stream domain.MediaStream) streamFingerprint {
	return streamFingerprint{Kind: kind, Path: stream.Path, Language: subtitleLanguage(stream), Format: streamFormat(stream), IsText: streamText(stream), IsDefault: boolValue(stream.IsDefault), IsForced: boolValue(stream.IsForced)}
}

func mergeExternal(sub *Subtitle, stream domain.MediaStream, ctx media.MediaContext, options Options, files []sidecar) *matchedSidecar {
	if stream.Path != "" && options.Mapper != nil {
		if mapped, err := options.Mapper.Map(stream.Path); err == nil {
			if options.Guard != nil && guardDirectory(options.Guard, mapped) == nil {
				if canonical, err := options.FileSystem.EvalSymlinks(mapped); err == nil {
					canonical = canonicalPath(canonical)
					for i := range files {
						if files[i].eligible && files[i].canonical == canonical {
							return &matchedSidecar{file: &files[i]}
						}
					}
				}
			}
		}
	}
	base := safeBaseName(stream.Path)
	if !safeBaseFallback(stream.Path, base) {
		return nil
	}
	var found *sidecar
	for i := range files {
		if !files[i].eligible || files[i].subtitle.FileName != base {
			continue
		}
		if found != nil {
			return nil
		}
		found = &files[i]
	}
	if found == nil {
		return nil
	}
	return &matchedSidecar{file: found}
}

func guardDirectory(guard *pathmap.PathGuard, value string) error {
	dir, err := pathmap.Directory(value)
	if err != nil {
		return err
	}
	return guard.CheckDirectory(dir)
}

func extension(name string) string {
	dot := strings.LastIndexByte(name, '.')
	if dot < 1 {
		return ""
	}
	return name[dot+1:]
}
func supportedExtension(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == "srt" || ext == "ass" || ext == "ssa"
}
func validStem(name, ext, base string) bool {
	if strings.IndexFunc(name, unicode.IsControl) >= 0 || strings.ContainsAny(name, `/\`) {
		return false
	}
	stem := name[:len(name)-len(ext)-1]
	return stem == base || strings.HasPrefix(stem, base+".")
}
func baseName(value string) string {
	value = strings.TrimRight(value, `/\`)
	value = value[strings.LastIndexAny(value, `/\`)+1:]
	dot := strings.LastIndexByte(value, '.')
	if dot > 0 {
		return value[:dot]
	}
	return value
}
func safeBaseName(value string) string {
	if value == "" || strings.ContainsAny(value, "?#") || strings.IndexFunc(value, unicode.IsControl) >= 0 || looksLikeURI(value) {
		return ""
	}
	value = strings.TrimRight(value, `/\`)
	value = value[strings.LastIndexAny(value, `/\`)+1:]
	if value == "." || value == ".." || strings.ContainsAny(value, `/\\`) || strings.Contains(value, ":") {
		return ""
	}
	return value
}
func safeBaseFallback(value, base string) bool {
	if value == "" || base == "" || strings.IndexFunc(value, unicode.IsControl) >= 0 || strings.ContainsAny(value, "?#") || looksLikeURI(value) {
		return false
	}
	if value == "." || value == ".." || strings.ContainsAny(value, `/\`) || strings.Contains(value, ":") || base != value {
		return false
	}
	return true
}
func looksLikeURI(value string) bool {
	separator := strings.Index(value, "://")
	if separator <= 0 {
		return false
	}
	for _, r := range value[:separator] {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '+' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}
func canonicalPath(value string) string {
	value = filepath.Clean(value)
	if value == "." {
		return ""
	}
	return filepath.ToSlash(value)
}
func subtitleID(key []byte, parts ...string) string {
	payload := "v1"
	for _, part := range parts {
		payload += fmt.Sprintf("|%d:%s", len(part), part)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return "sub_v1_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func subtitleLanguage(stream domain.MediaStream) string {
	return firstNonEmpty(stream.Language, stream.DisplayLanguage, stream.DisplayTitle)
}
func streamFormat(stream domain.MediaStream) string {
	if ext := extension(safeBaseName(stream.Path)); supportedExtension(ext) {
		return strings.ToLower(ext)
	}
	if stream.Codec != "" {
		return strings.ToLower(stream.Codec)
	}
	return ""
}
func streamText(stream domain.MediaStream) bool {
	return stream.IsTextSubtitleStream == nil || *stream.IsTextSubtitleStream
}
func isExternal(stream domain.MediaStream) bool {
	return boolValue(stream.IsExternal) || (stream.IsExternal == nil && stream.Path != "")
}
func kindForStream(stream domain.MediaStream) Kind {
	if isExternal(stream) {
		return KindExternal
	}
	return KindEmbedded
}
func boolValue(value *bool) bool { return value != nil && *value }
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func appendUniqueIndex(values []int, value int) []int {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	values = append(values, value)
	sort.Ints(values)
	return values
}
