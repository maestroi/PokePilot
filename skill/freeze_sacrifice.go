package skill

import "github.com/maestroi/pokepilot/game"

// freezeSacrificeQuietMenus is how many battle menus a frozen sacrifice may
// face one foe with no HP loss before that foe is treated as unwilling to
// faint it. Gen I never decrements enemy PP, so a foe can loop non-damaging
// moves forever. A frozen Pokémon cannot act, and once it is the last
// conscious member the battle cannot faint it or end.
const freezeSacrificeQuietMenus = 3

type freezeSacrificeAction int

const (
	freezeSacrificeNone freezeSacrificeAction = iota
	freezeSacrificeSend
	freezeSacrificeRecall
	freezeSacrificeHold
)

// freezeSacrificeWatch sends an incurable frozen party member out while some
// other conscious member can still fight, so the foe faints it under the
// game's own damage instead of leaving a stall for the stall cap. A foe that
// does not change its HP is abandoned after freezeSacrificeQuietMenus menus
// and not tried again until the enemy species changes.
type freezeSacrificeWatch struct {
	pending bool
	slot    int
	enemy   uint8
	hp      uint16
	quiet   int
	failed  map[uint8]bool
}

func (w *freezeSacrificeWatch) fail(enemy uint8) {
	w.pending = false
	if w.failed == nil {
		w.failed = map[uint8]bool{}
	}
	w.failed[enemy] = true
}

func (w *freezeSacrificeWatch) decide(resources game.BattleResourcesState, enemy uint8, activeHP uint16) (freezeSacrificeAction, int) {
	active := resources.ActiveSlot
	if active < 0 || active >= len(resources.Party) {
		w.pending = false
		return freezeSacrificeNone, 0
	}
	mon := resources.Party[active]
	if w.pending && (active != w.slot || mon.Status != "frozen" || mon.Fainted() || enemy != w.enemy) {
		w.pending = false
	}
	if w.pending {
		if activeHP < w.hp {
			w.hp = activeHP
			w.quiet = 0
			return freezeSacrificeHold, active
		}
		w.quiet++
		if w.quiet < freezeSacrificeQuietMenus {
			return freezeSacrificeHold, active
		}
		w.fail(enemy)
		if slot, ok := nonFrozenFighterSlot(resources); ok {
			return freezeSacrificeRecall, slot
		}
		return freezeSacrificeNone, 0
	}
	if w.failed[enemy] || bagCanThawFreeze(resources) {
		return freezeSacrificeNone, 0
	}
	if mon.Status == "frozen" && !mon.Fainted() {
		if _, ok := nonFrozenFighterSlot(resources); !ok {
			return freezeSacrificeNone, 0
		}
		w.pending = true
		w.slot = active
		w.enemy = enemy
		w.hp = activeHP
		w.quiet = 0
		return freezeSacrificeHold, active
	}
	slot, ok := incurableFrozenBenchSlot(resources)
	if !ok {
		return freezeSacrificeNone, 0
	}
	w.pending = true
	w.slot = slot
	w.enemy = enemy
	w.hp = resources.Party[slot].HP
	w.quiet = 0
	return freezeSacrificeSend, slot
}

func bagCanThawFreeze(resources game.BattleResourcesState) bool {
	_, ok := chooseStatusMedicineState(resources, "frozen")
	return ok
}

// incurableFrozenBenchSlot is a conscious frozen bench member to send out
// while the active member can still fight and the bag cannot thaw freeze.
func incurableFrozenBenchSlot(resources game.BattleResourcesState) (int, bool) {
	if bagCanThawFreeze(resources) {
		return 0, false
	}
	active := resources.ActiveSlot
	if active < 0 || active >= len(resources.Party) {
		return 0, false
	}
	if resources.Party[active].Fainted() || resources.Party[active].Status == "frozen" {
		return 0, false
	}
	for i, mon := range resources.Party {
		if i == active || mon.Fainted() || mon.Status != "frozen" {
			continue
		}
		return i, true
	}
	return 0, false
}

func nonFrozenFighterSlot(resources game.BattleResourcesState) (int, bool) {
	for i, mon := range resources.Party {
		if i == resources.ActiveSlot || mon.Fainted() || mon.Status == "frozen" {
			continue
		}
		return i, true
	}
	return 0, false
}
