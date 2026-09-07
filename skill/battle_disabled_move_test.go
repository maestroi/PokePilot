package skill

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// disabledMoveMarker has to survive the way the game actually draws the
// refusal. MoveSelectionMenu prints it into the box that normally holds the
// TYPE/ panel, so the marker is the ONLY thing telling Battle that the move
// list is up and waiting rather than that some text is playing. If it stops
// matching, Battle falls back to tapping A, which re-selects the disabled move
// and spins until the 60000-frame cap — a run-ending hang, not a lost turn.
//
// Both renderings are checked because the box wraps: ScreenText joins the
// lines with single spaces, which is why the marker deliberately spans no
// punctuation and no line break point.
func TestDisabledMoveMarkerMatchesTheDrawnRefusal(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []string
	}{
		{"one line", []string{"The move is disabled!"}},
		{"wrapped after move", []string{"The move", "is disabled!"}},
		{"wrapped after is", []string{"The move is", "disabled!"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &state.Mem{}
			for row, text := range tc.rows {
				for i, c := range text {
					if c < 0x100 {
						m[sym.TileMap+uint16(row*20+i)] = textTile(byte(c))
					}
				}
			}
			screen := state.ScreenText(m)
			if !strings.Contains(screen, disabledMoveMarker) {
				t.Fatalf("screen %q does not contain marker %q", screen, disabledMoveMarker)
			}
		})
	}
}

// The marker must not fire on ordinary battle text, or Battle would treat a
// playing message as a menu awaiting input and press cursor keys into it.
func TestDisabledMoveMarkerIgnoresOrdinaryBattleText(t *testing.T) {
	for _, text := range []string{
		"IVYSAUR used TACKLE!",
		"Enemy JIGGLYPUFF used DISABLE!",
		"IVYSAUR TACKLE was disabled!",
	} {
		m := &state.Mem{}
		for i, c := range text {
			if c < 0x100 {
				m[sym.TileMap+uint16(i)] = textTile(byte(c))
			}
		}
		if screen := state.ScreenText(m); strings.Contains(screen, disabledMoveMarker) {
			t.Errorf("marker matched ordinary text %q (screen %q)", text, screen)
		}
	}
}
