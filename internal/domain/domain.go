// Package domain contains the small, transport-independent facts used by the
// read-only Emby slice. It deliberately contains no credentials or direct
// media/stream URLs.
package domain

// Library is the safe public representation of an Emby library. Locations
// are intentionally not part of this type: a library listing must not expose
// server filesystem paths.
type Library struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	CollectionType string `json:"collection_type,omitempty"`
}

// ItemSummary is the representation used when browsing a library. Detailed
// paths, media sources and streams are only available from EmbyItem.
type ItemSummary struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Type              string `json:"type"`
	ParentID          string `json:"parent_id,omitempty"`
	SeriesID          string `json:"series_id,omitempty"`
	SeriesName        string `json:"series_name,omitempty"`
	ParentIndexNumber *int   `json:"parent_index_number,omitempty"`
	IndexNumber       *int   `json:"index_number,omitempty"`
	ProductionYear    *int   `json:"production_year,omitempty"`
}

// ItemPage is one bounded page returned by ListItems. The paging facts are
// retained so the HTTP layer can provide a stable continuation indicator.
type ItemPage struct {
	Items            []ItemSummary `json:"items"`
	TotalRecordCount int           `json:"total_record_count"`
	StartIndex       int           `json:"start_index"`
	Limit            int           `json:"limit"`
	HasMore          bool          `json:"has_more"`
}

// MediaSource is an internal Emby fact retained for MediaContext selection.
// It has no URL field; Path is an Emby-reported filesystem fact and must be
// mapped and filtered before it can be used by another layer.
type MediaSource struct {
	ID                 string         `json:"-"`
	Name               string         `json:"-"`
	Path               string         `json:"-"`
	Type               string         `json:"-"`
	Protocol           string         `json:"-"`
	Container          string         `json:"-"`
	IsRemote           *bool          `json:"-"`
	IsDefault          *bool          `json:"-"`
	SupportsDirectPlay *bool          `json:"-"`
	MediaStreams       *[]MediaStream `json:"-"`
}

// MediaStream is an internal Emby fact used by the later subtitle inventory
// stage. It is kept out of the safe browsing JSON representation.
type MediaStream struct {
	Index                *int   `json:"-"`
	Type                 string `json:"-"`
	Codec                string `json:"-"`
	Title                string `json:"-"`
	Language             string `json:"-"`
	DisplayLanguage      string `json:"-"`
	DisplayTitle         string `json:"-"`
	Path                 string `json:"-"`
	IsExternal           *bool  `json:"-"`
	IsForced             *bool  `json:"-"`
	IsDefault            *bool  `json:"-"`
	IsTextSubtitleStream *bool  `json:"-"`
	DeliveryMethod       string `json:"-"`
	Protocol             string `json:"-"`
}

// EmbyItem is the detailed, internal representation returned by GetItem.
// Callers exposing it over HTTP must project it to a safe response rather
// than encoding this value directly.
type EmbyItem struct {
	ItemSummary
	Path         string            `json:"-"`
	ProviderIDs  map[string]string `json:"-"`
	MediaSources []MediaSource     `json:"-"`
	// MediaStreams is a pointer so callers can distinguish an omitted field
	// (nil) from an explicitly returned empty stream list.
	MediaStreams *[]MediaStream `json:"-"`
}

// RemoteSubtitleInfo is the server-side projection of an Emby Bridge search
// result. ID is deliberately excluded from JSON serialization: it may contain
// provider-specific data and is usable only through a short-lived Candidate.
type RemoteSubtitleInfo struct {
	ID          string   `json:"-"`
	Provider    string   `json:"provider,omitempty"`
	Name        string   `json:"name,omitempty"`
	Language    string   `json:"language,omitempty"`
	Format      string   `json:"format,omitempty"`
	Author      string   `json:"author,omitempty"`
	Comment     string   `json:"comment,omitempty"`
	IsHashMatch bool     `json:"is_hash_match"`
	Score       float64  `json:"score"`
	Reasons     []string `json:"reasons,omitempty"`
}
