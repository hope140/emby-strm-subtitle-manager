// Package pathmap translates paths reported by Emby into a deliberately
// bounded local namespace. The parser is platform independent: a Linux
// process can validate a Windows Emby path and vice versa.
package pathmap

import (
	"errors"
	"strings"
	"unicode"
)

var (
	ErrInvalidPath      = errors.New("invalid media path")
	ErrPathNotMapped    = errors.New("media path is not mapped")
	ErrAmbiguousMapping = errors.New("path mapping configuration is ambiguous")
)

// Mapping is one Emby-server root to local-container-root mapping.
type Mapping struct {
	Emby  string
	Local string
}

type pathStyle uint8

const (
	stylePOSIX pathStyle = iota + 1
	styleWindowsDrive
	styleWindowsUNC
)

type parsedPath struct {
	style pathStyle
	// root is "c:" for a drive, "//server/share" for UNC and "/" for
	// POSIX. Components never contain a separator or dot component.
	root  string
	parts []string
}

type rule struct {
	source parsedPath
	target parsedPath
}

// Mapper applies normalized, longest-prefix path mappings.
type Mapper struct {
	rules []rule
}

// PathMapper is the descriptive name used by the D1 contract.
type PathMapper = Mapper

// New validates mappings and returns an immutable mapper. Duplicate source
// roots are rejected even when their targets happen to be equal, because a
// duplicate rule is operationally ambiguous and usually a configuration bug.
func New(mappings []Mapping) (*Mapper, error) {
	rules := make([]rule, 0, len(mappings))
	for _, mapping := range mappings {
		source, err := parse(mapping.Emby)
		if err != nil {
			return nil, ErrAmbiguousMapping
		}
		target, err := parse(mapping.Local)
		if err != nil {
			return nil, ErrAmbiguousMapping
		}
		for _, existing := range rules {
			if samePath(existing.source, source) {
				return nil, ErrAmbiguousMapping
			}
		}
		rules = append(rules, rule{source: source, target: target})
	}
	return &Mapper{rules: rules}, nil
}

// NewPathMapper is an alias for New.
func NewPathMapper(mappings []Mapping) (*PathMapper, error) { return New(mappings) }

// Map converts an Emby-reported absolute path to its mapped local path.
// Matching uses component boundaries and the longest matching source root.
func (m *Mapper) Map(embyPath string) (string, error) {
	if m == nil {
		return "", ErrPathNotMapped
	}
	path, err := parse(embyPath)
	if err != nil {
		return "", ErrInvalidPath
	}
	best := -1
	bestLength := -1
	for i, candidate := range m.rules {
		if candidate.source.style != path.style || !hasPrefix(candidate.source, path) {
			continue
		}
		if len(candidate.source.parts) > bestLength {
			best = i
			bestLength = len(candidate.source.parts)
		}
	}
	if best < 0 {
		return "", ErrPathNotMapped
	}
	selected := m.rules[best]
	suffix := path.parts[len(selected.source.parts):]
	return format(selected.target, suffix), nil
}

// Directory returns the lexical parent of an absolute path using the same
// platform-independent rules as Map. It does not touch the filesystem.
func Directory(value string) (string, error) {
	parsed, err := parse(value)
	if err != nil || len(parsed.parts) == 0 {
		return "", ErrInvalidPath
	}
	parsed.parts = parsed.parts[:len(parsed.parts)-1]
	return format(parsed, nil), nil
}

func parse(value string) (parsedPath, error) {
	if value == "" || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return parsedPath{}, ErrInvalidPath
	}
	if isDevicePath(value) {
		return parsedPath{}, ErrInvalidPath
	}
	if isDriveAbsolute(value) {
		return parseWindowsDrive(value)
	}
	if isUNC(value) {
		return parseUNC(value)
	}
	if strings.HasPrefix(value, "/") {
		return parsePOSIX(value)
	}
	return parsedPath{}, ErrInvalidPath
}

func parsePOSIX(value string) (parsedPath, error) {
	parts, err := components(value, "/", false)
	if err != nil {
		return parsedPath{}, err
	}
	return parsedPath{style: stylePOSIX, root: "/", parts: parts}, nil
}

func parseWindowsDrive(value string) (parsedPath, error) {
	normalized := strings.ReplaceAll(value, "/", "\\")
	parts, err := components(normalized[3:], "\\", true)
	if err != nil {
		return parsedPath{}, err
	}
	return parsedPath{style: styleWindowsDrive, root: strings.ToLower(normalized[:2]), parts: parts}, nil
}

func parseUNC(value string) (parsedPath, error) {
	normalized := strings.ReplaceAll(value, "/", "\\")
	parts, err := components(normalized[2:], "\\", true)
	if err != nil || len(parts) < 2 {
		return parsedPath{}, ErrInvalidPath
	}
	// Keep server and share in the root. This prevents a mapping for
	// \\server\share from matching another share on the same server.
	root := `\\` + strings.ToLower(parts[0]) + `\` + strings.ToLower(parts[1])
	return parsedPath{style: styleWindowsUNC, root: root, parts: parts}, nil
}

func components(value, separator string, windows bool) ([]string, error) {
	if windows {
		value = strings.ReplaceAll(value, "/", separator)
	}
	raw := strings.Split(value, separator)
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		if part == "" {
			continue
		}
		if part == "." || part == ".." || strings.IndexFunc(part, unicode.IsControl) >= 0 {
			return nil, ErrInvalidPath
		}
		if windows && strings.Contains(part, ":") {
			// A colon in a component is an NTFS alternate data stream.
			return nil, ErrInvalidPath
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func isDriveAbsolute(value string) bool {
	return len(value) >= 3 && isASCIIAlpha(value[0]) && value[1] == ':' && (value[2] == '/' || value[2] == '\\')
}

func isUNC(value string) bool {
	return len(value) >= 2 && ((value[0] == '\\' && value[1] == '\\') || (value[0] == '/' && value[1] == '/'))
}

func isDevicePath(value string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(value, "/", "\\"))
	return strings.HasPrefix(normalized, `\\?\`) || strings.HasPrefix(normalized, `\\.\`) ||
		strings.HasPrefix(normalized, `\??\`) || strings.HasPrefix(normalized, `\device\`)
}

func isASCIIAlpha(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}

func samePath(a, b parsedPath) bool {
	if a.style != b.style || a.root != b.root || len(a.parts) != len(b.parts) {
		return false
	}
	for i := range a.parts {
		if !equalComponent(a.style, a.parts[i], b.parts[i]) {
			return false
		}
	}
	return true
}

func hasPrefix(root, value parsedPath) bool {
	if root.style != value.style || len(root.parts) > len(value.parts) || root.root != value.root {
		return false
	}
	for i := range root.parts {
		if !equalComponent(root.style, root.parts[i], value.parts[i]) {
			return false
		}
	}
	return true
}

func equalComponent(style pathStyle, left, right string) bool {
	if style == stylePOSIX {
		return left == right
	}
	return strings.EqualFold(left, right)
}

func format(target parsedPath, suffix []string) string {
	parts := append(append([]string(nil), target.parts...), suffix...)
	if target.style == stylePOSIX {
		if len(parts) == 0 {
			return "/"
		}
		return "/" + strings.Join(parts, "/")
	}
	separator := `\`
	if target.style == styleWindowsDrive {
		if len(parts) == 0 {
			return target.root + separator
		}
		return target.root + separator + strings.Join(parts, separator)
	}
	if len(parts) <= 2 {
		return target.root
	}
	return target.root + separator + strings.Join(parts[2:], separator)
}
