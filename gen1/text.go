// Package gen1 contains mechanics shared by compatible Generation I games.
//
// It must not own ROM/RAM addresses, map ids, event flags or version-specific
// tables. Profiles supply bytes; this package only interprets formats that are
// actually shared.
package gen1

import (
	"strconv"
	"strings"
	"unicode"
)

// textChars maps the English Gen-I font tile IDs to rendered text. Red, Blue
// and Yellow use the same character encoding for names and tilemap text.
var textChars = buildTextChars()

func buildTextChars() map[byte]string {
	m := map[byte]string{
		0x7f: " ",
		0x9a: "(", 0x9b: ")", 0x9c: ":", 0x9d: ";", 0x9e: "[", 0x9f: "]",
		0xba: "é",
		0xbb: "'d", 0xbc: "'l", 0xbd: "'s", 0xbe: "'t", 0xbf: "'v",
		0xe0: "'", 0xe3: "-", 0xe4: "'r", 0xe5: "'m",
		0xe6: "?", 0xe7: "!", 0xe8: ".",
		0xef: "♂", 0xf0: "¥", 0xf1: "×", 0xf2: ".", 0xf3: "/", 0xf4: ",", 0xf5: "♀",
	}
	for i := byte(0); i < 26; i++ {
		m[0x80+i] = string(rune('A' + i))
		m[0xa0+i] = string(rune('a' + i))
	}
	for i := byte(0); i < 10; i++ {
		m[0xf6+i] = string(rune('0' + i))
	}
	return m
}

const nameTerminator = 0x50

// DecodeName renders a Gen-I name buffer and stops at the '@' terminator.
func DecodeName(buf []byte) string {
	for i, b := range buf {
		if b == nameTerminator {
			buf = buf[:i]
			break
		}
	}
	return DecodeTiles(buf)
}

// DecodeTiles renders English Gen-I tile/text bytes with whitespace collapsed.
func DecodeTiles(tiles []byte) string {
	var b strings.Builder
	for _, tile := range tiles {
		if s, ok := textChars[tile]; ok {
			b.WriteString(s)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

const pathologicalDisplayRun = 6

// NormalizeDisplayText abbreviates pathological repeated-character legacy
// names in presentation text without mutating RAM or raw decoded strings.
func NormalizeDisplayText(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	var previous rune
	run := 0
	flush := func() {
		if run == 0 {
			return
		}
		if run >= pathologicalDisplayRun && (unicode.IsLetter(previous) || unicode.IsDigit(previous)) {
			b.WriteRune(previous)
			b.WriteRune('×')
			b.WriteString(strconv.Itoa(run))
		} else {
			for i := 0; i < run; i++ {
				b.WriteRune(previous)
			}
		}
	}

	for _, r := range s {
		if run != 0 && r == previous {
			run++
			continue
		}
		flush()
		previous = r
		run = 1
	}
	flush()
	return b.String()
}

// PresetMenuNames extracts the three uppercase built-in names from Oak's
// "NEW NAME" menu. It operates on semantic screen text, never RAM addresses.
func PresetMenuNames(screenText string) []string {
	fields := strings.Fields(screenText)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] != "NEW" || fields[i+1] != "NAME" {
			continue
		}
		var names []string
		for _, field := range fields[i+2:] {
			if !presetWord(field) {
				break
			}
			names = append(names, field)
		}
		return names
	}
	return nil
}

func presetWord(field string) bool {
	if len(field) < 2 {
		return false
	}
	for _, r := range field {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
