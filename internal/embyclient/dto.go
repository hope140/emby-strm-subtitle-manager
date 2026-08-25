package embyclient

import (
	"strings"

	"github.com/hope140/subbridge/internal/domain"
)

// Pointer fields intentionally preserve the distinction between an omitted
// Emby field and a field explicitly set to its zero value. Emby adds fields
// over time, so unknown fields are ignored while known optional fields remain
// lossless at the DTO boundary.
type libraryDTO struct {
	ID             *string   `json:"Id"`
	Name           *string   `json:"Name"`
	CollectionType *string   `json:"CollectionType"`
	Locations      *[]string `json:"Locations"`
}

type libraryResultDTO struct {
	Items            *[]libraryDTO `json:"Items"`
	TotalRecordCount *int          `json:"TotalRecordCount"`
}

type itemsResponseDTO struct {
	Items            *[]itemDTO `json:"Items"`
	TotalRecordCount *int       `json:"TotalRecordCount"`
}

type itemDTO struct {
	ID                    *string            `json:"Id"`
	Name                  *string            `json:"Name"`
	Type                  *string            `json:"Type"`
	ParentID              *string            `json:"ParentId"`
	SeriesID              *string            `json:"SeriesId"`
	SeriesName            *string            `json:"SeriesName"`
	ParentIndexNumber     *int               `json:"ParentIndexNumber"`
	IndexNumber           *int               `json:"IndexNumber"`
	ProductionYear        *int               `json:"ProductionYear"`
	Path                  *string            `json:"Path"`
	ProviderIDs           *map[string]string `json:"ProviderIds"`
	MediaSources          *[]mediaSourceDTO  `json:"MediaSources"`
	AlternateMediaSources *[]mediaSourceDTO  `json:"AlternateMediaSources"`
	MediaStreams          *[]mediaStreamDTO  `json:"MediaStreams"`
}

type remoteSubtitleDTO struct {
	ID          *string   `json:"Id"`
	Provider    *string   `json:"ProviderName"`
	Name        *string   `json:"Name"`
	Language    *string   `json:"Language"`
	Format      *string   `json:"Format"`
	Author      *string   `json:"Author"`
	Comment     *string   `json:"Comment"`
	IsHashMatch *bool     `json:"IsHashMatch"`
	Score       *float64  `json:"Score"`
	Reasons     *[]string `json:"Reasons"`
}

type mediaSourceDTO struct {
	ID                 *string           `json:"Id"`
	Name               *string           `json:"Name"`
	Path               *string           `json:"Path"`
	Type               *string           `json:"Type"`
	Protocol           *string           `json:"Protocol"`
	Container          *string           `json:"Container"`
	IsRemote           *bool             `json:"IsRemote"`
	IsDefault          *bool             `json:"Default"`
	SupportsDirectPlay *bool             `json:"SupportsDirectPlay"`
	MediaStreams       *[]mediaStreamDTO `json:"MediaStreams"`
}

type mediaStreamDTO struct {
	Index                *int    `json:"Index"`
	Type                 *string `json:"Type"`
	Codec                *string `json:"Codec"`
	Title                *string `json:"Title"`
	Language             *string `json:"Language"`
	DisplayLanguage      *string `json:"DisplayLanguage"`
	DisplayTitle         *string `json:"DisplayTitle"`
	Path                 *string `json:"Path"`
	IsExternal           *bool   `json:"IsExternal"`
	IsForced             *bool   `json:"IsForced"`
	IsDefault            *bool   `json:"IsDefault"`
	IsTextSubtitleStream *bool   `json:"IsTextSubtitleStream"`
	DeliveryMethod       *string `json:"DeliveryMethod"`
	Protocol             *string `json:"Protocol"`
}

func (d libraryDTO) validLibraryShape() bool {
	return nonEmpty(d.ID) && nonEmpty(d.Name)
}

func (d itemDTO) validItemShape() bool {
	if !nonEmpty(d.ID) || !nonEmpty(d.Name) || !nonEmpty(d.Type) {
		return false
	}
	return *d.Type == "Movie" || *d.Type == "Episode"
}

func (d itemDTO) validDetailedItemShape() bool {
	return nonEmpty(d.ID) && nonEmpty(d.Name) && nonEmpty(d.Type)
}

func nonEmpty(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

func (d libraryDTO) toDomain() domain.Library {
	return domain.Library{ID: stringValue(d.ID), Name: stringValue(d.Name), CollectionType: stringValue(d.CollectionType)}
}

func (d itemDTO) toSummary() domain.ItemSummary {
	return domain.ItemSummary{
		ID: stringValue(d.ID), Name: stringValue(d.Name), Type: stringValue(d.Type),
		ParentID: stringValue(d.ParentID), SeriesID: stringValue(d.SeriesID), SeriesName: stringValue(d.SeriesName),
		ParentIndexNumber: d.ParentIndexNumber, IndexNumber: d.IndexNumber, ProductionYear: d.ProductionYear,
	}
}

func (d itemDTO) toDomain() domain.EmbyItem {
	item := domain.EmbyItem{ItemSummary: d.toSummary(), Path: stringValue(d.Path)}
	if d.ProviderIDs != nil {
		item.ProviderIDs = cloneStringMap(*d.ProviderIDs)
	}
	if d.MediaSources != nil || d.AlternateMediaSources != nil {
		item.MediaSources = make([]domain.MediaSource, 0, mediaSourceCount(d.MediaSources, d.AlternateMediaSources))
		appendSources := func(sources *[]mediaSourceDTO, skipIDs map[string]struct{}) {
			if sources == nil {
				return
			}
			for _, source := range *sources {
				id := sourceString(source.ID)
				if id != "" && skipIDs != nil {
					if _, exists := skipIDs[id]; exists {
						continue
					}
				}
				mapped := domain.MediaSource{
					ID: id, Name: sourceString(source.Name), Path: sourceString(source.Path), Type: sourceString(source.Type),
					Protocol: sourceString(source.Protocol), Container: sourceString(source.Container),
					IsRemote: source.IsRemote, IsDefault: source.IsDefault, SupportsDirectPlay: source.SupportsDirectPlay,
				}
				if source.MediaStreams != nil {
					streams := make([]domain.MediaStream, 0, len(*source.MediaStreams))
					for _, stream := range *source.MediaStreams {
						streams = append(streams, mapMediaStream(stream))
					}
					mapped.MediaStreams = &streams
				}
				item.MediaSources = append(item.MediaSources, mapped)
			}
		}
		appendSources(d.MediaSources, nil)
		primaryIDs := make(map[string]struct{})
		if d.MediaSources != nil {
			for _, source := range *d.MediaSources {
				if id := sourceString(source.ID); id != "" {
					primaryIDs[id] = struct{}{}
				}
			}
		}
		appendSources(d.AlternateMediaSources, primaryIDs)
	}
	if d.MediaStreams != nil {
		streams := make([]domain.MediaStream, 0, len(*d.MediaStreams))
		for _, stream := range *d.MediaStreams {
			streams = append(streams, mapMediaStream(stream))
		}
		item.MediaStreams = &streams
	}
	return item
}

func mediaSourceCount(primary, alternate *[]mediaSourceDTO) int {
	count := 0
	if primary != nil {
		count += len(*primary)
	}
	if alternate != nil {
		count += len(*alternate)
	}
	return count
}

func (d remoteSubtitleDTO) toDomain() domain.RemoteSubtitleInfo {
	result := domain.RemoteSubtitleInfo{
		ID: stringValue(d.ID), Provider: stringValue(d.Provider), Name: stringValue(d.Name),
		Language: stringValue(d.Language), Format: stringValue(d.Format), Author: stringValue(d.Author),
		Comment: stringValue(d.Comment), IsHashMatch: boolValue(d.IsHashMatch), Score: floatValue(d.Score),
	}
	if d.Reasons != nil {
		result.Reasons = append([]string(nil), (*d.Reasons)...)
	}
	return result
}

func mapMediaStream(stream mediaStreamDTO) domain.MediaStream {
	return domain.MediaStream{
		Index: stream.Index, Type: sourceString(stream.Type), Codec: sourceString(stream.Codec), Title: sourceString(stream.Title),
		Language: sourceString(stream.Language), DisplayLanguage: sourceString(stream.DisplayLanguage), DisplayTitle: sourceString(stream.DisplayTitle),
		Path: sourceString(stream.Path), IsExternal: stream.IsExternal, IsForced: stream.IsForced, IsDefault: stream.IsDefault,
		IsTextSubtitleStream: stream.IsTextSubtitleStream, DeliveryMethod: sourceString(stream.DeliveryMethod), Protocol: sourceString(stream.Protocol),
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func floatValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func sourceString(value *string) string {
	return stringValue(value)
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	clone := make(map[string]string, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}
