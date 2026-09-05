package state

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/maestroi/pokepilot/red/sym"
)

// textChars maps font tile IDs to the text they render, per pokered's
// constants/charmap.asm. Tile IDs are the same values the game uses for text
// bytes, so a tilemap row decodes directly. Bytes not present here (control
// codes, box borders, unused glyphs) decode as a space.
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

// DecodeTiles renders a wTileMap snapshot to a string, trimmed and with
// whitespace collapsed to single spaces.
//
// This reads the tilemap in RAM, not the framebuffer. It is text the game
// already decided to draw, not pixels interpreted after the fact — see
// docs/DESIGN.md on why the framebuffer is never a source of truth.
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

// NormalizeDisplayText keeps presentation/logging output compact when an old
// save or a broken naming flow contains a pathological repeated-character
// name such as AAAAAAAAAAAAA. It does not mutate game RAM or DecodeTiles; it
// only abbreviates long runs of the same letter/digit in text shown to agents,
// traces and UIs. Short expressive runs remain untouched.
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

// ScreenText returns the text currently rendered on screen, decoded from the
// tilemap. It is empty when nothing readable is drawn. Presentation-only
// normalization prevents pathological legacy names from flooding every trace,
// observation and spectator surface that consumes screen text.
func ScreenText(m *Mem) string {
	return NormalizeDisplayText(DecodeTiles(m.Slice(sym.TileMap, sym.TileMapLen)))
}
