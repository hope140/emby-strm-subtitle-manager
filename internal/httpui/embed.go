package httpui

import "embed"

// assets contains the complete UI. Keeping the files in the binary means the
// UI and the read-only API always share the same origin and release.
//
//go:embed assets/index.html assets/app.js assets/app.css assets/subtitle-steward.svg
var assets embed.FS
