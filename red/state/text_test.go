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
