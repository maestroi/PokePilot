package agent

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/gs/sym"
	"github.com/maestroi/pokepilot/skill"
)

func TestGSDepositSlotChoiceKeepsLeagueMedicine(t *testing.T) {
	items := []gsItemStack{{0x08, 1}, {gsItemPotion, 3}, {gsItemRevive, 2}}
	if got := gsDepositSlotChoice(items); got != 0 {
		t.Fatalf("deposit choice = %d, want the Moon Stone stack (0)", got)
	}
	if got := gsDepositSlotChoice(items[1:]); got != 1 {
		t.Fatalf("all-medicine choice = %d, want last stack (1)", got)
	}
}

// ROM-backed: the bedroom PC runs the same PlayersPC menu as the Center's
// player row. Play the real opening (Elm's aide gives a Potion on the way
// out), walk home, and store the Potion with the production deposit.
func TestGoldDepositItemStackThroughPlayersPC(t *testing.T) {
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("POKEMON_GOLD_ROM not set")
	}
	m, err := emu.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	if _, err := skill.BootToOverworld(m); err != nil {
		t.Fatalf("BootToOverworld: %v", err)
	}
	if err := executeGSOpening(m, m.ROM(), skill.StarterCyndaquil); err != nil {
		t.Fatalf("opening: %v", err)
	}
	profile, err := gsOpeningProfile(m.ROM())
	if err != nil {
		t.Fatal(err)
	}
	bedroom, err := gsOpeningMapID("PLAYERS_HOUSE_2F")
	if err != nil {
		t.Fatal(err)
	}
	if err := gsSecondBadgeFaceFrom(m, m.ROM(), profile,
		skill.ExactNativeDestination(bedroom, 2, 2), 2, 1, "test:bedroom-pc"); err != nil {
		t.Fatalf("reach bedroom PC: %v", err)
	}
	if n, pc := m.Peek8(sym.NumItems), m.Peek8(sym.NumPCItems); n != 1 || pc != 0 {
		t.Fatalf("bag/PC = %d/%d items, want Elm's Potion only (1/0)", n, pc)
	}
	m.Tap(emu.A, gsPCMenuHold, 7)
	if !gsPCWait(m, func() bool {
		if gsPlayersPCMenuUp(m, profile) {
			return true
		}
		if gsPCText(m, profile) != "" {
			m.Tap(emu.A, gsPCMenuHold, 7)
		}
		return false
	}) {
		t.Fatalf("player's PC menu did not open: %q", gsPCText(m, profile))
	}
	if err := gsDepositItemStack(m, profile, 0); err != nil {
		t.Fatal(err)
	}
	if err := gsCloseBillsPC(m, profile); err != nil {
		t.Fatal(err)
	}
	if n, pc := m.Peek8(sym.NumItems), m.Peek8(sym.NumPCItems); n != 0 || pc != 1 {
		t.Fatalf("after deposit bag/PC = %d/%d items, want 0/1", n, pc)
	}
}
