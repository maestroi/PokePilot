package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestNextNonFullBoxChoosesFirstAvailableAfterCurrent(t *testing.T) {
	counts := [gen1BoxCount]uint8{}
	for i := range counts {
		counts[i] = gen1BoxCapacity
	}
	counts[3] = gen1BoxCapacity - 1
	counts[7] = 0

	got, ok := nextNonFullBox(counts, 1)
	if !ok || got != 3 {
		t.Fatalf("nextNonFullBox = (%d,%v), want (3,true)", got, ok)
	}
}

func TestNextNonFullBoxWrapsAround(t *testing.T) {
	counts := [gen1BoxCount]uint8{}
	for i := range counts {
		counts[i] = gen1BoxCapacity
	}
	counts[0] = 0

	got, ok := nextNonFullBox(counts, gen1BoxCount-2)
	if !ok || got != 0 {
		t.Fatalf("nextNonFullBox = (%d,%v), want (0,true)", got, ok)
	}
}

func TestNextNonFullBoxReportsAllFull(t *testing.T) {
	counts := [gen1BoxCount]uint8{}
	for i := range counts {
		counts[i] = gen1BoxCapacity
	}
	if got, ok := nextNonFullBox(counts, 4); ok || got != -1 {
		t.Fatalf("nextNonFullBox = (%d,%v), want (-1,false)", got, ok)
	}
}

func TestNextNonFullBoxNeverReturnsCurrent(t *testing.T) {
	counts := [gen1BoxCount]uint8{}
	for i := range counts {
		counts[i] = gen1BoxCapacity
	}
	counts[5] = 0
	if got, ok := nextNonFullBox(counts, 5); ok || got != -1 {
		t.Fatalf("nextNonFullBox reused current box: (%d,%v)", got, ok)
	}
}

// changeBoxPromptMem draws rows of text plus a live two-option cursor. The
// cursor sits on a blank row-0 tile so it never splits a word.
func changeBoxPromptMem(rows ...string) *state.Mem {
	m := &state.Mem{}
	m[sym.FontLoaded] = 1
	m[sym.MaxMenuItem] = 1
	for row, text := range rows {
		for i, c := range []byte(text) {
			m[sym.TileMap+uint16((row+1)*20+i)] = textTile(c)
		}
	}
	m[sym.TileMap+19] = 0xED
	cursor := sym.TileMap + 19
	m[sym.MenuCursorLocation] = uint8(cursor)
	m[sym.MenuCursorLocation+1] = uint8(cursor >> 8)
	return m
}

// run-1v98zy914jk23p: _WhenYouChangeBoxText's `para` clears "will be saved."
// before YesNoChoice, so only the question and the PC menu behind it remain.
func TestChangeBoxSavePromptMatchesDrawnQuestion(t *testing.T) {
	m := changeBoxPromptMem("WITHDRAW", "DEPOSIT", "RELEASE", "CHANGE BOX", "SEE YA", "Is that okay")
	if !pcChangeBoxSavePrompt(m) {
		t.Fatalf("Change Box YES/NO rejected: screen=%q", state.ScreenText(m))
	}
}

func TestChangeBoxSavePromptRejectsOtherTwoOptionPrompts(t *testing.T) {
	m := changeBoxPromptMem("Is that okay")
	if pcChangeBoxSavePrompt(m) {
		t.Fatalf("YES/NO outside Bill's PC accepted: screen=%q", state.ScreenText(m))
	}
}
