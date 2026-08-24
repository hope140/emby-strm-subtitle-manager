// Package version contains build-time version information.
package version

// These variables may be overridden with -ldflags during a release build.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Info is the public, non-sensitive version payload.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// Current returns the values embedded in this binary.
func Current() Info {
	return Info{Version: Version, Commit: Commit, BuildTime: BuildTime}
}
