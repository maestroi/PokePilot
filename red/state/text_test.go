package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeTiles(t *testing.T) {
	tiles := make([]byte, sym.TileMapLen)
	// "OAK: HI" followed by filler that must decode as spaces.
	text := []byte{0x8e, 0x80, 0x8a, 0x9c, 0x7f, 0x87, 0x88}
	copy(tiles, text)
	for i := len(text); i < len(tiles); i++ {
		tiles[i] = 0x00 // <NULL>, not in the char table
	}

	if got := DecodeTiles(tiles); got != "OAK: HI" {
		t.Fatalf("DecodeTiles = %q, want %q", got, "OAK: HI")
	}
}

func TestDecodeTilesContractions(t *testing.T) {
	// 'd 'l 's 't 'v and the standalone apostrophe are single tiles.
	tiles := []byte{0x83, 0x8e, 0x8d, 0xbe, 0x7f, 0x88, 0xbe}
	if got := DecodeTiles(tiles); got != "DON't I't" {
		t.Fatalf("DecodeTiles = %q, want %q", got, "DON't I't")
	}
}

func TestDecodeNameStopsAtTerminator(t *testing.T) {
	// GetDefaultName copies NAME_BUFFER_LENGTH bytes, so wPlayerName after
	// picking the RED preset is RED@ plus the rest of the name list.
	got := DecodeName([]byte{0x91, 0x84, 0x83, 0x50, 0x80, 0x92, 0x87, 0x50, 0x89, 0x80, 0x82})
	if got != "RED" {
		t.Fatalf("DecodeName(RED@ASH@JAC) = %q, want RED", got)
	}
	if got := DecodeName([]byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x50}); got != "AAAAAAA" {
		t.Fatalf("DecodeName(AAAAAAA@) = %q, want AAAAAAA", got)
	}
}

func TestNormalizeDisplayText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "pathological player name", in: "AAAAAAAAAAAAA used CUT!", want: "A×13 used CUT!"},
		{name: "pathological rival name", in: "BBBBBBBB challenged you!", want: "B×8 challenged you!"},
		{name: "short expressive run", in: "NOOO!", want: "NOOO!"},
		{name: "ordinary long word", in: "CONGRATULATIONS!", want: "CONGRATULATIONS!"},
		{name: "digits", in: "1111111", want: "1×7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeDisplayText(tt.in); got != tt.want {
				t.Fatalf("NormalizeDisplayText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestScreenTextNormalizesPathologicalRuns(t *testing.T) {
	var m Mem
	for i := 0; i < 13; i++ {
		m[sym.TileMap+uint16(i)] = 0x80 // A
	}

	if got := ScreenText(&m); got != "A×13" {
		t.Fatalf("ScreenText = %q, want %q", got, "A×13")
	}
}

func TestScreenTextEmptyWhenNothingDrawn(t *testing.T) {
	var m Mem // all zero: no tile is in the char table
	if got := ScreenText(&m); got != "" {
		t.Fatalf("ScreenText = %q, want empty", got)
	}
}

func TestDecodeDialogueCarriesText(t *testing.T) {
	var m Mem
	m[sym.FontLoaded] = 1
	copy(m[sym.TileMap:], []byte{0x8e, 0x80, 0x8a})

	d := DecodeDialogue(&m)
	if d == nil {
		t.Fatal("DecodeDialogue = nil, want a dialogue state")
	}
	if d.Text != "OAK" {
		t.Fatalf("Text = %q, want %q", d.Text, "OAK")
	}
}
