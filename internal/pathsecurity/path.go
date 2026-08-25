// Package pathsecurity contains conservative filesystem path checks used by
// private, non-media application storage.
package pathsecurity

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// IsFilesystemRoot reports whether path names a filesystem or volume root.
// A private cache must never be allowed to operate on such a path.
func IsFilesystemRoot(path string) bool {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || !filepath.IsAbs(clean) {
		return false
	}
	if clean == "/" || clean == `\` || clean == string(filepath.Separator) {
		return true
	}
	if volume := filepath.VolumeName(clean); volume != "" {
		volumeRoot := filepath.Clean(volume + string(filepath.Separator))
		if samePath(clean, volumeRoot) {
			return true
		}
	}
	return samePath(filepath.Dir(clean), clean)
}

// UsesSymlink reports whether path itself or an existing parent resolves via a
// symlink/reparse point. Missing final components are allowed so a stable
// cache directory can be created on first startup; existing parents are still
// inspected. Inspection errors are returned so callers can fail closed.
func UsesSymlink(path string) (bool, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || !filepath.IsAbs(clean) {
		return false, nil
	}
	probe := clean
	for {
		info, err := os.Lstat(probe)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return true, nil
			}
			resolved, evalErr := filepath.EvalSymlinks(probe)
			if evalErr != nil {
				if errors.Is(evalErr, fs.ErrNotExist) {
					return false, nil
				}
				return false, evalErr
			}
			return !samePath(cleanPath(probe), cleanPath(resolved)), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return false, nil
		}
		probe = parent
	}
}

// Overlaps reports whether either path contains the other, including paths
// whose existing components resolve through a symlink/reparse point.
func Overlaps(left, right string) bool {
	leftVariants := pathVariants(left)
	rightVariants := pathVariants(right)
	for _, leftVariant := range leftVariants {
		for _, rightVariant := range rightVariants {
			if within(leftVariant, rightVariant) || within(rightVariant, leftVariant) {
				return true
			}
		}
	}
	return false
}

func pathVariants(path string) []string {
	clean := cleanPath(path)
	if clean == "" {
		return nil
	}
	variants := []string{clean}
	if resolved, ok := resolveForComparison(clean); ok && !samePath(clean, resolved) {
		variants = append(variants, resolved)
	}
	return variants
}

func resolveForComparison(path string) (string, bool) {
	clean := cleanPath(path)
	if clean == "" || !filepath.IsAbs(clean) {
		return "", false
	}
	probe := clean
	suffix := make([]string, 0, 2)
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			for _, component := range suffix {
				resolved = filepath.Join(resolved, component)
			}
			return cleanPath(resolved), true
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", false
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", false
		}
		suffix = append([]string{filepath.Base(probe)}, suffix...)
		probe = parent
	}
}

func within(candidate, root string) bool {
	candidate = cleanPath(candidate)
	root = cleanPath(root)
	if candidate == "" || root == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		candidate = strings.ToLower(candidate)
		root = strings.ToLower(root)
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	if relative == "." {
		return true
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || strings.HasPrefix(relative, "../") || strings.HasPrefix(relative, `..\`) {
		return false
	}
	return !filepath.IsAbs(relative)
}

func cleanPath(path string) string {
	return filepath.Clean(strings.TrimSpace(path))
}

func samePath(left, right string) bool {
	left = cleanPath(left)
	right = cleanPath(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
