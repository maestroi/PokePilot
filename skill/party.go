package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

func partyMenuUp(m *emu.Emu) bool {
	decoder, err := partyMenuDecoderFor(m)
	if err != nil {
		return false
	}
	live := decoder.DecodePartyMenu(m)
	return live.Visible && live.Kind == game.PartyMenuForcedBattle
}

func useItemPartyMenuUp(m *emu.Emu) bool {
	decoder, err := partyMenuDecoderFor(m)
	if err != nil {
		return false
	}
	live := decoder.DecodePartyMenu(m)
	return live.Visible && live.Kind == game.PartyMenuItemUse
}

// SwitchActive performs a voluntary mid-battle switch through profile-owned
// battle-menu, party-menu, execution, and roster semantics.
func SwitchActive(m *emu.Emu, slot int) error {
	battleMenu, err := battleMenuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: SwitchActive: %w", err)
	}
	party, err := partyMenuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: SwitchActive: %w", err)
	}
	execution, err := battleExecutionDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: SwitchActive: %w", err)
	}
	resources, err := battleResourcesDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: SwitchActive: %w", err)
	}
	runtime, err := battleRuntimeDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: SwitchActive: %w", err)
	}

	if !resources.DecodeBattleResources(m).InBattle {
		return fmt.Errorf("skill: SwitchActive: no battle in progress on %s",
			battleRuntimeContext(runtime.DecodeBattleRuntime(m)))
	}
	if err := switchActiveWithDecoders(m, slot, battleMenu, party, execution, resources); err != nil {
		return fmt.Errorf("skill: SwitchActive: %w", err)
	}
	return nil
}

// switchActiveWithDecoders is the cartridge-neutral switch transaction. It is
// decoder-injected so a Gen-II-shaped fake can exercise the full flow without
// importing any concrete game RAM layout.
func switchActiveWithDecoders(
	m menuMachine,
	slot int,
	battleMenu game.BattleMenuDecoder,
	party game.PartyMenuDecoder,
	execution game.BattleExecutionDecoder,
	resources game.BattleResourcesDecoder,
) error {
	if battleMenu == nil || party == nil || execution == nil || resources == nil {
		return fmt.Errorf("missing battle switch decoder")
	}

	before := resources.DecodeBattleResources(m)
	if !before.InBattle {
		return fmt.Errorf("no battle in progress")
	}
	if slot < 0 || slot >= len(before.Party) {
		return fmt.Errorf("slot %d out of range for a party of %d", slot, len(before.Party))
	}
	if before.Party[slot].Fainted() {
		return fmt.Errorf("slot %d is fainted; the game refused a fainted switch target", slot)
	}
	if before.ActiveSlot == slot {
		return nil
	}

	if !waitMenuUntil(m, bagMainMenuBudget, func() bool {
		return battleMenu.DecodeBattleMainMenu(m).Visible
	}) {
		return fmt.Errorf("battle main menu did not open within %d frames", bagMainMenuBudget)
	}
	if err := activateBattleMainMenuEntryWithDecoder(m, battleMenu, game.BattleMenuPokemon, moveMenuBudget, func() bool {
		return party.DecodePartyMenu(m).Visible
	}); err != nil {
		return fmt.Errorf("voluntary party menu did not appear: %w", err)
	}
	partyMenu := party.DecodePartyMenu(m)
	if partyMenu.Kind != game.PartyMenuVoluntaryBattle {
		return fmt.Errorf("party menu %q appeared while opening a voluntary switch", partyMenu.Kind)
	}

	if err := selectPartySlotWithDecoder(m, party, slot); err != nil {
		return err
	}

	if !waitMenuUntil(m, moveMenuBudget, func() bool {
		live := resources.DecodeBattleResources(m)
		if live.InBattle && live.ActiveSlot == slot {
			return true
		}
		return execution.DecodeBattleExecution(m).Phase == game.BattleExecutionSwitchBox
	}) {
		return fmt.Errorf("switch confirmation did not appear")
	}

	afterSelect := resources.DecodeBattleResources(m)
	if afterSelect.ActiveSlot != slot {
		if execution.DecodeBattleExecution(m).Phase != game.BattleExecutionSwitchBox {
			return fmt.Errorf("party selection left no switch confirmation")
		}
		m.Tap(emu.A, 3, 7)
		if !waitMenuUntil(m, moveMenuBudget, func() bool {
			live := resources.DecodeBattleResources(m)
			return !live.InBattle || live.ActiveSlot == slot
		}) {
			return fmt.Errorf("active slot did not become %d", slot)
		}
	}

	after := resources.DecodeBattleResources(m)
	if !after.InBattle || after.ActiveSlot != slot {
		return fmt.Errorf("active slot %d after selecting slot %d, want %d", after.ActiveSlot, slot, slot)
	}
	return nil
}
