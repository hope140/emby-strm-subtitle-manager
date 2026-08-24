package pathmap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrGuardRootUnavailable = errors.New("path guard root is unavailable")
	ErrGuardOutsideRoot     = errors.New("path is outside the allowed media root")
)

// PathGuard verifies that a local directory exists and that symlink
// resolution keeps it beneath one of the configured roots. It is intentionally
// a runtime check in addition to Mapper's lexical containment check.
type PathGuard struct {
	roots []string
}

// NewPathGuard resolves and validates existing directory roots. Roots are
// copied and never exposed to callers or included in returned errors.
func NewPathGuard(roots []string) (*PathGuard, error) {
	if len(roots) == 0 {
		return nil, ErrGuardRootUnavailable
	}
	resolved := make([]string, 0, len(roots))
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			return nil, ErrGuardRootUnavailable
		}
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			return nil, ErrGuardRootUnavailable
		}
		realRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, ErrGuardRootUnavailable
		}
		resolved = append(resolved, filepath.Clean(realRoot))
	}
	return &PathGuard{roots: resolved}, nil
}

// CheckDirectory requires path to exist as a directory and to remain within
// at least one configured root after symlink resolution.
func (g *PathGuard) CheckDirectory(path string) error {
	if g == nil || strings.TrimSpace(path) == "" {
		return ErrGuardOutsideRoot
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ErrGuardOutsideRoot
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ErrGuardOutsideRoot
	}
	realPath = filepath.Clean(realPath)
	for _, root := range g.roots {
		if within(root, realPath) {
			return nil
		}
	}
	return ErrGuardOutsideRoot
}

func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return false
	}
	return true
}
