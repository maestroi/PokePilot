package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func gsWriteMenuText(mem fakeGSReader, row int, text string) {
	for i, r := range text {
		var tile byte = 0x7f
		switch {
		case r >= 'A' && r <= 'Z':
			tile = 0x80 + byte(r-'A')
		case r == '-':
			tile = 0xe3
		}
		mem[sym.TileMap+uint16(row*20+i)] = tile
	}
}

func TestGoldSilverPostCreditsMenuRequiresRenderedContinue(t *testing.T) {
	for _, profile := range []*Profile{NewGold(), NewSilver()} {
		t.Run(string(profile.ID()), func(t *testing.T) {
			mem := fakeGSReader{
				sym.SaveFileExists:  1,
				sym.TwoDMenuNumRows: 3,
				sym.MenuCursorY:     2,
			}
			if got := profile.DecodePostCreditsMenu(mem); got.ContinueVisible || got.SaveInfoVisible {
				t.Fatalf("stale menu RAM falsely recognized CONTINUE: %+v", got)
			}
			gsWriteMenuText(mem, 0, "CONTINUE")
			gsWriteMenuText(mem, 1, "NEW GAME")
			gsWriteMenuText(mem, 2, "OPTION")
			if got := profile.DecodePostCreditsMenu(mem); !got.ContinueVisible || got.Cursor != 2 || got.SaveInfoVisible {
				t.Fatalf("rendered CONTINUE row 2 not recognized: %+v", got)
			}
			mem[sym.SaveFileExists] = 0
			if got := profile.DecodePostCreditsMenu(mem); got.ContinueVisible {
				t.Fatalf("CONTINUE with no battery save must be rejected: %+v", got)
			}
			mem[sym.SaveFileExists] = 1
			mem[sym.MenuCursorY] = 5
			if got := profile.DecodePostCreditsMenu(mem); got.ContinueVisible {
				t.Fatalf("out-of-bounds menu cursor accepted: %+v", got)
			}
			mem[sym.MenuCursorY] = 1
			mem[sym.BattleMode] = 1
			if got := profile.DecodePostCreditsMenu(mem); got.ContinueVisible {
				t.Fatalf("battle pixels must not count as title menu: %+v", got)
			}
		})
	}
}

func TestGoldSilverPostCreditsConfirmationIsSeparateFromMainMenu(t *testing.T) {
	for _, profile := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{sym.SaveFileExists: 1}
		gsWriteMenuText(mem, 1, "BADGES")
		gsWriteMenuText(mem, 2, "TIME")
		if got := profile.DecodePostCreditsMenu(mem); !got.SaveInfoVisible || got.ContinueVisible {
			t.Fatalf("%s save-info panel = %+v", profile.ID(), got)
		}
		gsWriteMenuText(mem, 3, "NEW GAME")
		if got := profile.DecodePostCreditsMenu(mem); got.SaveInfoVisible {
			t.Fatalf("%s unrelated menu labels accepted as save info: %+v", profile.ID(), got)
		}
	}
}
