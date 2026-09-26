package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const (
	fakeBattleVisible uint16 = 32 + iota
	fakeBattleColumn
	fakeBattleRow
)

type fakeGen2BattleMenuDecoder struct{}

func (fakeGen2BattleMenuDecoder) DecodeBattleMainMenu(r game.MemoryReader) game.BattleMainMenuState {
	return game.BattleMainMenuState{
		Visible: r.Peek8(fakeBattleVisible) != 0,
		Cursor: game.BattleMenuPosition{
			Column: int(r.Peek8(fakeBattleColumn)),
			Row:    int(r.Peek8(fakeBattleRow)),
		},
	}
}

func (fakeGen2BattleMenuDecoder) BattleMainMenuEntryPosition(entry game.BattleMenuEntry) (game.BattleMenuPosition, bool) {
	// Deliberately unlike Gen I: three columns and non-Gen-I ordering.
	switch entry {
	case game.BattleMenuFight:
		return game.BattleMenuPosition{Column: 2, Row: 0}, true
	case game.BattleMenuItems:
		return game.BattleMenuPosition{Column: 0, Row: 2}, true
	case game.BattleMenuPokemon:
		return game.BattleMenuPosition{Column: 1, Row: 1}, true
	case game.BattleMenuRun:
		return game.BattleMenuPosition{Column: 2, Row: 2}, true
	default:
		return game.BattleMenuPosition{}, false
	}
}

type fakeBattleMenuMachine struct {
	mem [64]byte
}

func (m *fakeBattleMenuMachine) Peek8(addr uint16) byte { return m.mem[addr] }

func (m *fakeBattleMenuMachine) PeekInto(addr uint16, dst []byte) {
	copy(dst, m.mem[int(addr):])
}

func (*fakeBattleMenuMachine) StepFrame() {}

func (*fakeBattleMenuMachine) StepFrames(int) {}

func (m *fakeBattleMenuMachine) Tap(btn emu.Button, _, _ int) {
	switch btn {
	case emu.Left:
		if m.mem[fakeBattleColumn] > 0 {
			m.mem[fakeBattleColumn]--
		}
	case emu.Right:
		if m.mem[fakeBattleColumn] < 2 {
			m.mem[fakeBattleColumn]++
		}
	case emu.Up:
		if m.mem[fakeBattleRow] > 0 {
			m.mem[fakeBattleRow]--
		}
	case emu.Down:
		if m.mem[fakeBattleRow] < 2 {
			m.mem[fakeBattleRow]++
		}
	}
}

func TestGenericBattleMainMenuUsesProfileLayout(t *testing.T) {
	for _, entry := range []game.BattleMenuEntry{
		game.BattleMenuFight,
		game.BattleMenuItems,
		game.BattleMenuPokemon,
		game.BattleMenuRun,
	} {
		t.Run(string(entry), func(t *testing.T) {
			m := &fakeBattleMenuMachine{}
			m.mem[fakeBattleVisible] = 1
			m.mem[fakeBattleColumn] = 1
			m.mem[fakeBattleRow] = 0

			decoder := fakeGen2BattleMenuDecoder{}
			if err := selectBattleMainMenuEntryWithDecoder(m, decoder, entry); err != nil {
				t.Fatalf("select %s: %v", entry, err)
			}
			got := decoder.DecodeBattleMainMenu(m).Cursor
			want, _ := decoder.BattleMainMenuEntryPosition(entry)
			if got != want {
				t.Fatalf("cursor = %+v, want %+v for %s", got, want, entry)
			}
		})
	}
}

func TestGenericBattleMainMenuRequiresVisibleMenu(t *testing.T) {
	m := &fakeBattleMenuMachine{}
	err := selectBattleMainMenuEntryWithDecoder(m, fakeGen2BattleMenuDecoder{}, game.BattleMenuRun)
	if err == nil {
		t.Fatal("hidden battle menu was accepted")
	}
}

func TestGenericBattleMainMenuRejectsUnknownEntry(t *testing.T) {
	m := &fakeBattleMenuMachine{}
	m.mem[fakeBattleVisible] = 1
	err := selectBattleMainMenuEntryWithDecoder(m, fakeGen2BattleMenuDecoder{}, game.BattleMenuEntry("pack"))
	if err == nil {
		t.Fatal("unknown battle menu entry was accepted")
	}
}

// fakeDroppingBattleMenuMachine drops the first `drop` A presses, the way
// Gen I ignores input between drawing the battle menu and HandleMenuInput's
// first joypad poll. An accepted A hides the main menu (the submenu opened).
type fakeDroppingBattleMenuMachine struct {
	fakeBattleMenuMachine
	drop, aPresses int
}

func (m *fakeDroppingBattleMenuMachine) Tap(btn emu.Button, hold, gap int) {
	if btn != emu.A {
		m.fakeBattleMenuMachine.Tap(btn, hold, gap)
		return
	}
	m.aPresses++
	if m.aPresses > m.drop {
		m.mem[fakeBattleVisible] = 0
	}
}

func TestActivateBattleMainMenuEntryRetriesDroppedPress(t *testing.T) {
	m := &fakeDroppingBattleMenuMachine{drop: 1}
	m.mem[fakeBattleVisible] = 1
	opened := func() bool { return m.mem[fakeBattleVisible] == 0 }

	if err := activateBattleMainMenuEntryWithDecoder(m, fakeGen2BattleMenuDecoder{}, game.BattleMenuItems, 10, opened); err != nil {
		t.Fatalf("activate after a dropped A: %v", err)
	}
	if m.aPresses != 2 {
		t.Fatalf("A presses = %d, want 2 (one dropped, one accepted)", m.aPresses)
	}
}

func TestActivateBattleMainMenuEntryNeverRepressesInsideSubmenu(t *testing.T) {
	m := &fakeDroppingBattleMenuMachine{}
	m.mem[fakeBattleVisible] = 1
	// The press is accepted (main menu gone) but the submenu never reports
	// open: re-pressing A here would act inside whatever now owns input.
	if err := activateBattleMainMenuEntryWithDecoder(m, fakeGen2BattleMenuDecoder{}, game.BattleMenuItems, 10, func() bool { return false }); err == nil {
		t.Fatal("unopened submenu was accepted")
	}
	if m.aPresses != 1 {
		t.Fatalf("A presses = %d, want 1 once the main menu stopped owning input", m.aPresses)
	}
}
