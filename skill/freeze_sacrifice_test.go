package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func freezeSacrificeParty(active int, mons ...game.BattlePartyMon) game.BattleResourcesState {
	return game.BattleResourcesState{InBattle: true, ActiveSlot: active, Party: mons}
}

func healthyFighter() game.BattlePartyMon {
	return game.BattlePartyMon{HP: 40, MaxHP: 40}
}

func frozenMon() game.BattlePartyMon {
	return game.BattlePartyMon{HP: 30, MaxHP: 30, Status: "frozen"}
}

func TestFreezeSacrificeSendsIncurableBench(t *testing.T) {
	var watch freezeSacrificeWatch
	resources := freezeSacrificeParty(0, healthyFighter(), frozenMon())
	action, slot := watch.decide(resources, 0x16, resources.Party[0].HP)
	if action != freezeSacrificeSend || slot != 1 {
		t.Fatalf("action=%d slot=%d, want send frozen bench", action, slot)
	}
}

func TestFreezeSacrificeSkipsWhenBagCanThaw(t *testing.T) {
	for _, item := range []uint8{itemIceHeal, itemFullHeal, itemFullRestore} {
		var watch freezeSacrificeWatch
		resources := freezeSacrificeParty(0, healthyFighter(), frozenMon())
		resources.Bag = []game.InventoryItem{{NativeItemID: uint16(item), Quantity: 1}}
		action, slot := watch.decide(resources, 0x16, resources.Party[0].HP)
		if action != freezeSacrificeNone || slot != 0 {
			t.Fatalf("item %#02x: action=%d slot=%d, want no sacrifice", item, action, slot)
		}
	}
}

func TestFreezeSacrificeHoldsWhileFoeDealsDamage(t *testing.T) {
	var watch freezeSacrificeWatch
	resources := freezeSacrificeParty(0, healthyFighter(), frozenMon())
	if action, _ := watch.decide(resources, 9, 40); action != freezeSacrificeSend {
		t.Fatalf("setup action=%d, want send", action)
	}
	resources.ActiveSlot = 1
	action, slot := watch.decide(resources, 9, 12)
	if action != freezeSacrificeHold || slot != 1 {
		t.Fatalf("action=%d slot=%d, want hold while HP is falling", action, slot)
	}
}

func TestFreezeSacrificeRecallsWhenFoeWillNotDamage(t *testing.T) {
	var watch freezeSacrificeWatch
	resources := freezeSacrificeParty(0, healthyFighter(), frozenMon(), healthyFighter())
	if action, slot := watch.decide(resources, 7, 40); action != freezeSacrificeSend || slot != 1 {
		t.Fatalf("setup action=%d slot=%d, want send slot 1", action, slot)
	}
	resources.ActiveSlot = 1
	for i := 0; i < freezeSacrificeQuietMenus-1; i++ {
		action, _ := watch.decide(resources, 7, 30)
		if action != freezeSacrificeHold {
			t.Fatalf("menu %d action=%d, want hold", i, action)
		}
	}
	action, slot := watch.decide(resources, 7, 30)
	if action != freezeSacrificeRecall || slot != 0 {
		t.Fatalf("action=%d slot=%d, want recall to fighter 0", action, slot)
	}
	resources.ActiveSlot = 0
	if action, _ := watch.decide(resources, 7, 40); action != freezeSacrificeNone {
		t.Fatalf("same foe after recall action=%d, want none", action)
	}
	action, slot = watch.decide(resources, 8, 40)
	if action != freezeSacrificeSend || slot != 1 {
		t.Fatalf("new foe action=%d slot=%d, want send again", action, slot)
	}
}

func TestFreezeSacrificeHoldsFrozenActiveUntilDamageFails(t *testing.T) {
	var watch freezeSacrificeWatch
	resources := freezeSacrificeParty(0, frozenMon(), healthyFighter())
	action, _ := watch.decide(resources, 3, 30)
	if action != freezeSacrificeHold {
		t.Fatalf("action=%d, want hold so a damaging foe can faint the frozen lead", action)
	}
}
