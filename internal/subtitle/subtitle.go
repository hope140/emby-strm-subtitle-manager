// Package subtitle validates and parses the read-only subtitle formats used by
// D2. It never reads a filesystem path and has no provider or HTTP concerns.
package subtitle

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	DefaultMaxBytes = 4 << 20
	MaxCueCount     = 10000
	MaxLineBytes    = 8 << 10
)

var (
	ErrTooLarge          = errors.New("subtitle is too large")
	ErrInvalidEncoding   = errors.New("subtitle encoding is invalid")
	ErrEmpty             = errors.New("subtitle is empty")
	ErrUnsafeContent     = errors.New("subtitle content is unsafe")
	ErrUnsupportedFormat = errors.New("subtitle format is unsupported")
	ErrInvalidStructure  = errors.New("subtitle structure is invalid")
)

// Cue is the only subtitle content shape exposed to the preview layer.
type Cue struct {
	Index   int    `json:"index"`
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

// Document is the canonical, validated subtitle artifact.
type Document struct {
	Format      string
	Canonical   []byte
	Cues        []Cue
	ByteLength  int
	CueCount    int
	ContentHash string
}

var srtTimestamp = regexp.MustCompile(`^(\d{1,6}):(\d{2}):(\d{2})[,.](\d{3})\s+-->\s+(\d{1,6}):(\d{2}):(\d{2})[,.](\d{3})(?:\s+.*)?$`)

// ValidateAndParse bounds, decodes, validates and parses one provider body.
// expectedFormat may be empty to infer SRT versus ASS/SSA from the content.
func ValidateAndParse(raw []byte, expectedFormat string, maxBytes int64) (Document, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if int64(len(raw)) > maxBytes {
		return Document{}, ErrTooLarge
	}
	text, err := decode(raw)
	if err != nil {
		return Document{}, err
	}
	canonical := normalizeLineEndings(text)
	if int64(len(canonical)) > maxBytes {
		return Document{}, ErrTooLarge
	}
	if strings.TrimSpace(canonical) == "" {
		return Document{}, ErrEmpty
	}
	if unsafeDocument([]byte(canonical)) {
		return Document{}, ErrUnsafeContent
	}
	format, err := normalizeFormat(expectedFormat, canonical)
	if err != nil {
		return Document{}, err
	}
	var cues []Cue
	switch format {
	case "srt":
		cues, err = parseSRT(canonical)
	case "ass", "ssa":
		cues, err = parseASS(canonical)
	default:
		err = ErrUnsupportedFormat
	}
	if err != nil {
		return Document{}, err
	}
	if len(cues) == 0 {
		return Document{}, ErrInvalidStructure
	}
	hash := sha256.Sum256([]byte(canonical))
	return Document{
		Format: format, Canonical: []byte(canonical), Cues: cues,
		ByteLength: len(canonical), CueCount: len(cues), ContentHash: hexHash(hash),
	}, nil
}

func decode(raw []byte) (string, error) {
	switch {
	case bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}):
		raw = raw[3:]
		if !utf8.Valid(raw) {
			return "", ErrInvalidEncoding
		}
		return string(raw), nil
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE}):
		return decodeUTF16(raw[2:], true)
	case bytes.HasPrefix(raw, []byte{0xFE, 0xFF}):
		return decodeUTF16(raw[2:], false)
	default:
		if !utf8.Valid(raw) {
			return "", ErrInvalidEncoding
		}
		return string(raw), nil
	}
}

func decodeUTF16(raw []byte, littleEndian bool) (string, error) {
	if len(raw)%2 != 0 {
		return "", ErrInvalidEncoding
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		if littleEndian {
			units[i] = binary.LittleEndian.Uint16(raw[i*2:])
		} else {
			units[i] = binary.BigEndian.Uint16(raw[i*2:])
		}
	}
	var builder strings.Builder
	for i := 0; i < len(units); i++ {
		unit := units[i]
		switch {
		case unit >= 0xD800 && unit <= 0xDBFF:
			if i+1 >= len(units) || units[i+1] < 0xDC00 || units[i+1] > 0xDFFF {
				return "", ErrInvalidEncoding
			}
			r := utf16.Decode([]uint16{unit, units[i+1]})[0]
			builder.WriteRune(r)
			i++
		case unit >= 0xDC00 && unit <= 0xDFFF:
			return "", ErrInvalidEncoding
		default:
			builder.WriteRune(rune(unit))
		}
	}
	return builder.String(), nil
}

func normalizeLineEndings(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\r", "\n")
}

func unsafeDocument(raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	trimmed := bytes.TrimSpace(raw)
	lower := strings.ToLower(string(trimmed))
	if bytes.HasPrefix(trimmed, []byte("PK\x03\x04")) || bytes.HasPrefix(trimmed, []byte{0x1F, 0x8B}) || bytes.HasPrefix(trimmed, []byte("7z\xBC\xAF\x27\x1C")) {
		return true
	}
	if strings.HasPrefix(lower, "<html") || strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<body") {
		return true
	}
	if strings.HasPrefix(lower, "{") || strings.HasPrefix(lower, "[{") {
		return true
	}
	var nonText int
	for _, r := range string(raw) {
		if r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\t') {
			return true
		}
		if !unicode.IsSpace(r) && unicode.IsControl(r) {
			nonText++
		}
	}
	return nonText > 0
}

func normalizeFormat(expected, content string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(expected))
	format = strings.TrimPrefix(format, ".")
	if format == "substation alpha" || format == "substationalpha" {
		format = "ssa"
	}
	if format != "" && format != "srt" && format != "ass" && format != "ssa" {
		return "", ErrUnsupportedFormat
	}
	if format == "" {
		trimmed := strings.TrimSpace(strings.ToLower(content))
		if strings.HasPrefix(trimmed, "[script info]") || strings.Contains(trimmed, "\n[events]") || strings.Contains(trimmed, "\ndialogue:") {
			format = "ass"
		} else {
			format = "srt"
		}
	}
	return format, nil
}

func parseSRT(content string) ([]Cue, error) {
	blocks := strings.Split(content, "\n\n")
	cues := make([]Cue, 0, min(len(blocks), MaxCueCount))
	var previousStart int64 = -1
	for _, block := range blocks {
		block = strings.Trim(block, "\n")
		if strings.TrimSpace(block) == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
			lines = lines[1:]
		}
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		if len(lines) < 3 {
			return nil, ErrInvalidStructure
		}
		index, err := strconv.Atoi(strings.TrimSpace(lines[0]))
		if err != nil || index < 0 {
			return nil, ErrInvalidStructure
		}
		start, end, ok := parseSRTTime(strings.TrimSpace(lines[1]))
		if !ok || end < start || (previousStart >= 0 && start < previousStart) {
			return nil, ErrInvalidStructure
		}
		text := strings.Join(lines[2:], "\n")
		if err := validateCueText(text); err != nil {
			return nil, err
		}
		if len(cues) >= MaxCueCount {
			return nil, ErrInvalidStructure
		}
		cues = append(cues, Cue{Index: index, StartMS: start, EndMS: end, Text: text})
		previousStart = start
	}
	return cues, nil
}

func parseSRTTime(value string) (int64, int64, bool) {
	matches := srtTimestamp.FindStringSubmatch(value)
	if len(matches) != 9 {
		return 0, 0, false
	}
	start, okStart := parseClock(matches[1:5])
	end, okEnd := parseClock(matches[5:9])
	return start, end, okStart && okEnd
}

func parseClock(parts []string) (int64, bool) {
	if len(parts) != 4 {
		return 0, false
	}
	hour, e1 := strconv.ParseInt(parts[0], 10, 64)
	minute, e2 := strconv.ParseInt(parts[1], 10, 64)
	second, e3 := strconv.ParseInt(parts[2], 10, 64)
	millisecond, e4 := strconv.ParseInt(parts[3], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || minute > 59 || second > 59 || millisecond > 999 {
		return 0, false
	}
	value := ((hour*60+minute)*60+second)*1000 + millisecond
	if value < 0 || value > math.MaxInt64 {
		return 0, false
	}
	return value, true
}

func parseASS(content string) ([]Cue, error) {
	const defaultFields = "Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text"
	fields := splitASSFields(defaultFields)
	inEvents := false
	seenEvents := false
	cues := make([]Cue, 0)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inEvents = strings.EqualFold(trimmed, "[Events]")
			seenEvents = seenEvents || inEvents
			continue
		}
		if !inEvents {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "format:") {
			candidateFields := splitASSFields(strings.TrimSpace(trimmed[len("format:"):]))
			if len(candidateFields) < 3 {
				return nil, ErrInvalidStructure
			}
			fields = candidateFields
			continue
		}
		if !strings.HasPrefix(lower, "dialogue:") {
			continue
		}
		values := strings.SplitN(strings.TrimSpace(trimmed[len("dialogue:"):]), ",", len(fields))
		if len(values) != len(fields) {
			return nil, ErrInvalidStructure
		}
		startIndex, endIndex, textIndex := -1, -1, -1
		for i, field := range fields {
			switch strings.ToLower(strings.TrimSpace(field)) {
			case "start":
				startIndex = i
			case "end":
				endIndex = i
			case "text":
				textIndex = i
			}
		}
		if startIndex < 0 || endIndex < 0 || textIndex < 0 {
			return nil, ErrInvalidStructure
		}
		start, okStart := parseASSTime(strings.TrimSpace(values[startIndex]))
		end, okEnd := parseASSTime(strings.TrimSpace(values[endIndex]))
		if !okStart || !okEnd || end < start {
			return nil, ErrInvalidStructure
		}
		text, err := plainASS(strings.TrimSpace(values[textIndex]))
		if err != nil {
			return nil, err
		}
		if err := validateCueText(text); err != nil {
			return nil, err
		}
		if len(cues) >= MaxCueCount {
			return nil, ErrInvalidStructure
		}
		cues = append(cues, Cue{Index: len(cues) + 1, StartMS: start, EndMS: end, Text: text})
	}
	if !seenEvents || len(cues) == 0 {
		return nil, ErrInvalidStructure
	}
	// ASS permits Dialogue records in any order. Keep Canonical unchanged for
	// write operations, while ordering the parsed view for useful pagination.
	sort.SliceStable(cues, func(i, j int) bool {
		return cues[i].StartMS < cues[j].StartMS
	})
	for i := range cues {
		cues[i].Index = i + 1
	}
	return cues, nil
}

func splitASSFields(value string) []string {
	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func parseASSTime(value string) (int64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, false
	}
	hour, errHour := strconv.ParseInt(parts[0], 10, 64)
	minute, errMinute := strconv.ParseInt(parts[1], 10, 64)
	secondParts := strings.Split(parts[2], ".")
	if len(secondParts) != 2 {
		return 0, false
	}
	second, errSecond := strconv.ParseInt(secondParts[0], 10, 64)
	fraction, errFraction := strconv.ParseInt(secondParts[1], 10, 64)
	if errHour != nil || errMinute != nil || errSecond != nil || errFraction != nil || minute > 59 || second > 59 || len(secondParts[1]) < 1 || len(secondParts[1]) > 3 {
		return 0, false
	}
	for digits := len(secondParts[1]); digits < 3; digits++ {
		fraction *= 10
	}
	return ((hour*60+minute)*60+second)*1000 + fraction, true
}

func plainASS(value string) (string, error) {
	var builder strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '{' {
			builder.WriteByte(value[i])
			continue
		}
		end := strings.IndexByte(value[i+1:], '}')
		if end < 0 {
			return "", ErrInvalidStructure
		}
		i += end + 1
	}
	value = builder.String()
	value = strings.ReplaceAll(value, `\N`, "\n")
	value = strings.ReplaceAll(value, `\n`, "\n")
	return strings.ReplaceAll(value, `\h`, " "), nil
}

func validateCueText(value string) error {
	if strings.TrimSpace(value) == "" {
		return ErrInvalidStructure
	}
	for _, line := range strings.Split(value, "\n") {
		if len([]byte(line)) > MaxLineBytes {
			return ErrInvalidStructure
		}
		for _, r := range line {
			if r == 0 || (unicode.IsControl(r) && r != '\t') {
				return ErrUnsafeContent
			}
		}
	}
	return nil
}

func hexHash(value [32]byte) string {
	const hex = "0123456789abcdef"
	result := make([]byte, 64)
	for i, b := range value {
		result[i*2] = hex[b>>4]
		result[i*2+1] = hex[b&0x0f]
	}
	return string(result)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
