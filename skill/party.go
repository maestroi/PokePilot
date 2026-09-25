package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// partyMenuMarker identifies the FORCED battle party menu from wTileMap.
// It is the footer line _PartyMenuBattleText ("Bring out which #MON?") that
// DrawPartyMenu prints for BATTLE_PARTY_MENU (engine/menus/party_menu.asm).
const partyMenuMarker = "Bring out"

func partyMenuUp(m *emu.Emu) bool {
	return battleScreenHas(m, partyMenuMarker)
}

const useItemPartyMenuMarker = "Use item"

func useItemPartyMenuUp(m *emu.Emu) bool {
	return battleScreenHas(m, useItemPartyMenuMarker)
}

// SetLead reorders the party through the start menu's POKEMON list so that
// the member currently in slot is slot 0, the lead. A party of one needs no
// decisions: SetLead(m, 0) verifies the range and returns without touching
// a button.
//
// The game's only reorder is a two-way swap between the lead and the
// selected member (SwitchPartyMon), so any wanted slot reaches the front in
// one swap — which is exactly what PromoteToLead drives, step-and-verify,
// from START -> PKMN to the overworld again. POSITIVE postcondition:
// state.DecodeParty's first member is the species that sat in slot before
// the call; PromoteToLead returns an error when it does not hold.
func SetLead(m *emu.Emu, slot int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	if slot < 0 || slot >= int(party.Count) {
		return fmt.Errorf("skill: SetLead: slot %d out of range for a party of %d", slot, party.Count)
	}
	if slot == 0 {
		return nil // already the lead; nothing to decide
	}
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: SetLead: player not controllable on map %#04x", m.Peek8(sym.CurMap))
	}
	return PromoteToLead(m, slot)
}

// SwitchActive performs the VOLUNTARY mid-battle switch through the battle
// menu's POKéMON branch: A opens the FIGHT/ITEM/PKMN/RUN menu, RIGHT moves
// the cursor to the right column (POKéMON), A opens the party menu, the
// wanted slot is selected, and the SWITCH/STATS/CANCEL box that follows is
// answered SWITCH. POSITIVE postcondition: state.DecodeBattle's ActiveSpecies
// is the species that sat in party slot before the call — a nil return with
// the same active mon would be a lie, so the species is re-read from RAM at
// the end and asserted.
//
// This is the half of the battle party menu that Battle does not drive:
// the forced switch after a faint (S6-5b) is answered inside Battle's state
// machine; this one is opened by the player. The ordinary battle command is
// selected semantically through the active profile; party-slot navigation is
// likewise profile-driven, while Gen-I battle/party data still owns the postcondition.
func SwitchActive(m *emu.Emu, slot int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) == nil {
		return fmt.Errorf("skill: SwitchActive: no battle in progress on map %#04x", m.Peek8(sym.CurMap))
	}
	party := state.DecodeParty(&mem)
	if slot < 0 || slot >= int(party.Count) {
		return fmt.Errorf("skill: SwitchActive: slot %d out of range for a party of %d", slot, party.Count)
	}
	if party.Mons[slot].Fainted() {
		return fmt.Errorf("skill: SwitchActive: slot %d is fainted; the ROM bounces a fainted pick back to the menu", slot)
	}
	want := party.Mons[slot].Species
	if int(m.Peek8(sym.PlayerMonNumber)) == slot {
		// Already out: the ROM would print "ALREADY OUT!" and bounce.
		b := state.DecodeBattle(&mem)
		if b.ActiveSpecies == want {
			return nil
		}
		return fmt.Errorf("skill: SwitchActive: slot %d is marked active but the active species is %#02x, want %#02x", slot, b.ActiveSpecies, want)
	}

	// 1. The FIGHT/ITEM/PKMN/RUN menu: waitBattleMainMenu advances the
	// encounter text that precedes it, exactly as Battle's default branch
	// does, and stops the moment the menu is drawn.
	if err := waitBattleMainMenu(m); err != nil {
		return fmt.Errorf("skill: SwitchActive: %w", err)
	}

	// 2. Select the semantic POKéMON entry. The active profile owns the
	// battle-menu layout; this driver no longer knows Gen I cursor columns.
	if err := selectBattleMainMenuEntry(m, game.BattleMenuPokemon); err != nil {
		return fmt.Errorf("skill: SwitchActive: select POKéMON: %w", err)
	}

	// 3. A on POKéMON opens the party menu. The VOLUNTARY menu prints the
	// NORMAL_PARTY_MENU footer "Choose a #MON." (core.asm .partyMenuWasSelected
	// sets wPartyMenuTypeOrMessageID to NORMAL_PARTY_MENU); if the FORCED
	// menu's "Bring out" appears instead, the lead fainted and the screen
	// belongs to Battle, not this function — fail rather than drive it.
	for i := 0; i < 24; i++ {
		if battleSwitchMenuUp(m) {
			break
		}
		if partyMenuUp(m) {
			return fmt.Errorf("skill: SwitchActive: the forced switch menu appeared (the lead fainted); that screen belongs to Battle, not a voluntary switch")
		}
		m.Tap(emu.A, 3, 7)
		if _, err := m.StepUntil(25, battleSwitchMenuUp); err == nil {
			break
		}
	}
	if !battleSwitchMenuUp(m) {
		return fmt.Errorf("skill: SwitchActive: the voluntary party menu did not appear after selecting POKéMON")
	}

	// 4. The slot, step-and-verify on wCurrentMenuItem as everywhere else.
	if err := SelectPartySlot(m, slot); err != nil {
		return fmt.Errorf("skill: SwitchActive: %w", err)
	}

	// 5. The SWITCH/STATS/CANCEL box (SWITCH_STATS_CANCEL_MENU_TEMPLATE):
	// A selects SWITCH (index 0). The switch itself is the positive fact —
	// LoadBattleMonFromParty rewrites wBattleMonSpecies before the send-out
	// animation — so the loop ends on the species, never on a press count.
	activeIs := func(m *emu.Emu) bool {
		var s state.Mem
		state.Snapshot(m, &s)
		b := state.DecodeBattle(&s)
		return b != nil && b.ActiveSpecies == want
	}
	for i := 0; i < 24; i++ {
		if activeIs(m) {
			break
		}
		m.Tap(emu.A, 3, 7)
		if _, err := m.StepUntil(25, activeIs); err == nil {
			break
		}
	}

	// Postcondition: the active battle mon's species IS the wanted one.
	state.Snapshot(m, &mem)
	b := state.DecodeBattle(&mem)
	if b == nil || b.ActiveSpecies != want {
		got := uint8(0)
		if b != nil {
			got = b.ActiveSpecies
		}
		return fmt.Errorf("skill: SwitchActive: active species %#02x after selecting slot %d, want %#02x", got, slot, want)
	}
	return nil
}

