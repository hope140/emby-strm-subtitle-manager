package subtitle

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestValidateAndParseSRTUTF8AndUTF16(t *testing.T) {
	const srt = "1\r\n00:00:01,000 --> 00:00:02,500\r\n你好\r\n\r\n2\r\n00:00:03.000 --> 00:00:04.000\r\n世界\r\n"
	for name, raw := range map[string][]byte{
		"utf8":    []byte("\xEF\xBB\xBF" + srt),
		"utf16le": utf16Bytes(srt, true),
		"utf16be": utf16Bytes(srt, false),
	} {
		t.Run(name, func(t *testing.T) {
			document, err := ValidateAndParse(raw, "srt", DefaultMaxBytes)
			if err != nil {
				t.Fatal(err)
			}
			if document.Format != "srt" || document.CueCount != 2 || document.ByteLength == 0 || len(document.ContentHash) != 64 {
				t.Fatalf("document = %#v", document)
			}
			if document.Cues[0].StartMS != 1000 || document.Cues[0].EndMS != 2500 || document.Cues[0].Text != "你好" {
				t.Fatalf("cues = %#v", document.Cues)
			}
		})
	}
}

func TestValidateAndParseASSPlainText(t *testing.T) {
	content := `[Script Info]
Title: fixture
[Events]
Format: Layer, Start, End, Style, Text
	Dialogue: 0,0:00:01.00,0:00:02.50,Default,{\i1}第一\N第二`
	document, err := ValidateAndParse([]byte(content), "ass", DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if document.Format != "ass" || len(document.Cues) != 1 || document.Cues[0].Text != "第一\n第二" {
		t.Fatalf("ASS document = %#v", document)
	}
}

func TestValidateRejectsUnsafeInvalidAndOversizedContent(t *testing.T) {
	valid := []byte("1\n00:00:01,000 --> 00:00:02,000\ntext\n")
	for name, raw := range map[string][]byte{
		"empty":        nil,
		"html":         []byte("<html><body>error</body></html>"),
		"json":         []byte(`{"error":"provider"}`),
		"gzip":         []byte{0x1f, 0x8b, 0x08, 0x00},
		"binary":       []byte("1\n00:00:01,000 --> 00:00:02,000\ntext\x00\n"),
		"bad-encoding": {0xff, 0xfe, 0x00},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateAndParse(raw, "srt", DefaultMaxBytes); err == nil {
				t.Fatal("invalid subtitle was accepted")
			}
		})
	}
	if _, err := ValidateAndParse(valid, "vtt", DefaultMaxBytes); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("unsupported format error = %v", err)
	}
	if _, err := ValidateAndParse(valid, "srt", int64(len(valid)-1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("size error = %v", err)
	}
	longLine := "1\n00:00:01,000 --> 00:00:02,000\n" + strings.Repeat("x", MaxLineBytes+1) + "\n"
	if _, err := ValidateAndParse([]byte(longLine), "srt", DefaultMaxBytes); !errors.Is(err, ErrInvalidStructure) {
		t.Fatalf("line length error = %v", err)
	}
}

func TestValidateRejectsInvalidTimelineAndCueLimit(t *testing.T) {
	for _, value := range []string{
		"1\n00:00:02,000 --> 00:00:01,000\nbackwards\n",
		"1\n00:61:00,000 --> 00:01:02,000\ninvalid\n",
		"1\n00:00:01,000 --> 00:00:02,000\nfirst\n\n2\n00:00:00,500 --> 00:00:02,000\nreordered\n",
	} {
		if _, err := ValidateAndParse([]byte(value), "srt", DefaultMaxBytes); !errors.Is(err, ErrInvalidStructure) {
			t.Fatalf("invalid timeline error = %v", err)
		}
	}
	var builder strings.Builder
	for i := 1; i <= MaxCueCount+1; i++ {
		builder.WriteString("1\n00:00:01,000 --> 00:00:02,000\ntext\n\n")
	}
	if _, err := ValidateAndParse([]byte(builder.String()), "srt", DefaultMaxBytes); !errors.Is(err, ErrInvalidStructure) {
		t.Fatalf("cue count error = %v", err)
	}
}

func utf16Bytes(value string, littleEndian bool) []byte {
	units := utf16.Encode([]rune(value))
	result := make([]byte, 2+len(units)*2)
	if littleEndian {
		result[0], result[1] = 0xff, 0xfe
	} else {
		result[0], result[1] = 0xfe, 0xff
	}
	for i, unit := range units {
		if littleEndian {
			binary.LittleEndian.PutUint16(result[2+i*2:], unit)
		} else {
			binary.BigEndian.PutUint16(result[2+i*2:], unit)
		}
	}
	return bytes.Clone(result)
}
