// Package media builds the read-only, source-specific context used by D1.
package media

import (
	"errors"
	"strings"
	"unicode"

	"github.com/hope140/subbridge/internal/domain"
	"github.com/hope140/subbridge/internal/pathmap"
)

var (
	ErrInvalidItem                  = errors.New("invalid media item")
	ErrMediaSourceUnavailable       = errors.New("media source is unavailable")
	ErrMediaSourceNotFound          = errors.New("media source was not found")
	ErrMediaSourceSelectionRequired = errors.New("media source selection is required")
	ErrDuplicateMediaSourceID       = errors.New("media source identifiers are invalid")
	ErrInvalidUpstreamResponse      = errors.New("Emby media source response is invalid")
	ErrMediaStreamsUnavailable      = errors.New("media streams are unavailable")
	ErrMappedPathUnavailable        = errors.New("mapped media path is unavailable")
)

// MappingStatus describes the safe state of the local path mapping. A
// degraded state is returned in MediaContext instead of being mistaken for a
// missing subtitle inventory.
type MappingStatus string

const (
	MappingStatusMapped      MappingStatus = "mapped"
	MappingStatusUnmapped    MappingStatus = "unmapped"
	MappingStatusUnsafe      MappingStatus = "unsafe"
	MappingStatusUnavailable MappingStatus = "unavailable"

	WarningSourcePathFallback        = "source_path_fallback"
	WarningSourceStreamsFallback     = "source_media_streams_fallback"
	WarningSourceStreamsUnavailable  = "source_media_streams_unavailable"
	WarningPathMappingNotFound       = "path_mapping_not_found"
	WarningMediaPathUnsafe           = "media_path_unsafe"
	WarningMediaDirectoryUnavailable = "media_directory_unavailable"

	// Compatibility aliases retain descriptive names for internal callers;
	// their serialized values remain the stable public contract above.
	WarningSingleSourcePathFallback    = WarningSourcePathFallback
	WarningSingleSourceStreamsFallback = WarningSourceStreamsFallback
	WarningPathMappingUnmapped         = WarningPathMappingNotFound
	WarningPathMappingUnsafe           = WarningMediaPathUnsafe
	WarningPathMappingUnavailable      = WarningMediaDirectoryUnavailable
	WarningPathGuardUnsafe             = WarningMediaPathUnsafe
)

// SourceSelector applies the explicit source-selection contract. It never
// guesses the first source when an item has multiple sources.
type SourceSelector struct{}

// NewSourceSelector returns a stateless source selector.
func NewSourceSelector() SourceSelector { return SourceSelector{} }

// Select chooses a source by exact ID, or auto-selects the sole source when
// sourceID is empty.
func (SourceSelector) Select(item domain.EmbyItem, sourceID string) (domain.MediaSource, error) {
	if len(item.MediaSources) == 0 {
		return domain.MediaSource{}, ErrMediaSourceUnavailable
	}
	if err := validateSources(item.MediaSources); err != nil {
		return domain.MediaSource{}, err
	}
	if sourceID != "" {
		var selected *domain.MediaSource
		for i := range item.MediaSources {
			if item.MediaSources[i].ID != sourceID {
				continue
			}
			if selected != nil {
				return domain.MediaSource{}, ErrDuplicateMediaSourceID
			}
			selected = &item.MediaSources[i]
		}
		if selected == nil {
			return domain.MediaSource{}, ErrMediaSourceNotFound
		}
		if selected.ID == "" {
			return domain.MediaSource{}, ErrMediaSourceUnavailable
		}
		return cloneSource(*selected), nil
	}
	if len(item.MediaSources) != 1 {
		return domain.MediaSource{}, ErrMediaSourceSelectionRequired
	}
	if item.MediaSources[0].ID == "" {
		return domain.MediaSource{}, ErrMediaSourceUnavailable
	}
	return cloneSource(item.MediaSources[0]), nil
}

// SelectSource is the function form for callers that do not need to retain a
// selector value.
func SelectSource(item domain.EmbyItem, sourceID string) (domain.MediaSource, error) {
	return (SourceSelector{}).Select(item, sourceID)
}

// MediaContext is the selected Movie/Episode view used by later read-only
// inventory code. EmbyPath and LocalPath are internal facts and must never be
// encoded directly in a public response.
type MediaContext struct {
	ItemID            string
	MediaSourceID     string
	Container         string
	Type              string
	Title             string
	ParentID          string
	SeriesID          string
	SeriesName        string
	ParentIndexNumber *int
	IndexNumber       *int
	ProductionYear    *int
	ProviderIDs       map[string]string
	EmbyPath          string
	LocalPath         string
	LocalDirectory    string
	IsStrm            bool
	MediaStreams      *[]domain.MediaStream
	MappingStatus     MappingStatus
	Warnings          []string
	InventoryComplete bool
}

// BuildOptions controls mapping and runtime containment checks. A nil mapper
// is allowed for pure source-selection tests; production D1 construction
// should provide one.
type BuildOptions struct {
	MediaSourceID string
	Mapper        *pathmap.Mapper
	Guard         *pathmap.PathGuard
}

// Build creates a source-specific context. STRM items always use Item.Path
// for local mapping, directory guarding and classification; non-STRM local
// items may use a selected local MediaSource.Path for inventory. A remote
// source path remains a playback locator and is never mapped. For a
// multi-source item, missing source-level streams are never filled from
// item-level streams.
func Build(item domain.EmbyItem, options BuildOptions) (MediaContext, error) {
	if item.ID == "" || (item.Type != "Movie" && item.Type != "Episode") {
		return MediaContext{}, ErrInvalidItem
	}
	selected, err := (SourceSelector{}).Select(item, options.MediaSourceID)
	if err != nil {
		return MediaContext{}, err
	}
	warnings := make([]string, 0, 3)
	isStrm := isSTRM(item.Path)
	embyPath := item.Path
	if !isStrm {
		switch {
		case isRemoteSource(selected):
			// A remote/non-file source is a playback locator, not a local
			// inventory path. Do not send it through PathMapper.
			embyPath = ""
		case selected.Path != "":
			embyPath = selected.Path
		case len(item.MediaSources) == 1:
			// A sole local source may use the item path when Emby omitted its
			// source path. This fallback is only for non-STRM local media.
			embyPath = item.Path
			warnings = append(warnings, WarningSourcePathFallback)
		default:
			embyPath = ""
		}
	}

	var localPath, localDirectory string
	mappingStatus := MappingStatusUnavailable
	canMap := true
	if embyPath == "" {
		warnings = append(warnings, WarningMediaDirectoryUnavailable)
		canMap = false
	} else if strings.IndexFunc(embyPath, unicode.IsControl) >= 0 {
		warnings = append(warnings, WarningMediaPathUnsafe)
		mappingStatus = MappingStatusUnsafe
		canMap = false
	}
	if canMap && options.Mapper != nil {
		localPath, err = options.Mapper.Map(embyPath)
		if err != nil {
			mappingStatus = classifyMappingError(err)
			warnings = append(warnings, mappingWarning(mappingStatus))
			localPath = ""
		} else {
			localDirectory, err = pathmap.Directory(localPath)
			if err != nil {
				mappingStatus = MappingStatusUnavailable
				warnings = append(warnings, WarningMediaDirectoryUnavailable)
				localPath = ""
				localDirectory = ""
			} else if options.Guard != nil {
				if err := options.Guard.CheckDirectory(localDirectory); err != nil {
					mappingStatus = MappingStatusUnsafe
					warnings = append(warnings, WarningMediaPathUnsafe)
					localPath = ""
					localDirectory = ""
				}
			}
			if localPath != "" {
				mappingStatus = MappingStatusMapped
			}
		}
	} else if canMap {
		warnings = append(warnings, WarningMediaDirectoryUnavailable)
	}

	streams := selected.MediaStreams
	streamsComplete := streams != nil
	if streams == nil {
		if len(item.MediaSources) == 1 {
			streams = cloneStreams(item.MediaStreams)
			streamsComplete = item.MediaStreams != nil
			warnings = append(warnings, WarningSourceStreamsFallback)
		} else {
			empty := []domain.MediaStream{}
			streams = &empty
			warnings = append(warnings, WarningSourceStreamsUnavailable)
		}
	}
	complete := mappingStatus == MappingStatusMapped && streamsComplete
	return MediaContext{
		ItemID: item.ID, MediaSourceID: selected.ID, Container: selected.Container, Type: item.Type, Title: item.Name,
		ParentID: item.ParentID, SeriesID: item.SeriesID, SeriesName: item.SeriesName,
		ParentIndexNumber: item.ParentIndexNumber, IndexNumber: item.IndexNumber, ProductionYear: item.ProductionYear,
		ProviderIDs: cloneStringMap(item.ProviderIDs), EmbyPath: embyPath, LocalPath: localPath,
		LocalDirectory: localDirectory, IsStrm: isStrm, MediaStreams: streams,
		MappingStatus: mappingStatus, Warnings: warnings, InventoryComplete: complete,
	}, nil
}

// NewMediaContext is an expressive alias for Build.
func NewMediaContext(item domain.EmbyItem, options BuildOptions) (MediaContext, error) {
	return Build(item, options)
}

func isSTRM(value string) bool {
	lastSeparator := strings.LastIndexAny(value, `/\\`)
	base := value[lastSeparator+1:]
	dot := strings.LastIndexByte(base, '.')
	return dot > 0 && strings.EqualFold(base[dot+1:], "strm")
}

func isRemoteSource(source domain.MediaSource) bool {
	if source.IsRemote != nil && *source.IsRemote {
		return true
	}
	if strings.Contains(source.Path, "://") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(source.Protocol)) {
	case "http", "https", "rtmp", "rtsp", "udp", "mms":
		return true
	default:
		return false
	}
}

func cloneSource(source domain.MediaSource) domain.MediaSource {
	source.MediaStreams = cloneStreams(source.MediaStreams)
	return source
}

func validateSources(sources []domain.MediaSource) error {
	seen := make(map[string]struct{}, len(sources))
	defaults := 0
	for _, source := range sources {
		if source.ID == "" {
			return ErrInvalidUpstreamResponse
		}
		if _, exists := seen[source.ID]; exists {
			return ErrDuplicateMediaSourceID
		}
		seen[source.ID] = struct{}{}
		if source.IsDefault != nil && *source.IsDefault {
			defaults++
		}
	}
	if defaults > 1 {
		return ErrInvalidUpstreamResponse
	}
	return nil
}

func classifyMappingError(err error) MappingStatus {
	if errors.Is(err, pathmap.ErrPathNotMapped) {
		return MappingStatusUnmapped
	}
	if errors.Is(err, pathmap.ErrInvalidPath) {
		return MappingStatusUnsafe
	}
	return MappingStatusUnavailable
}

func mappingWarning(status MappingStatus) string {
	switch status {
	case MappingStatusUnmapped:
		return WarningPathMappingNotFound
	case MappingStatusUnsafe:
		return WarningMediaPathUnsafe
	default:
		return WarningMediaDirectoryUnavailable
	}
}

func cloneStreams(streams *[]domain.MediaStream) *[]domain.MediaStream {
	if streams == nil {
		return nil
	}
	clone := append([]domain.MediaStream(nil), (*streams)...)
	return &clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
