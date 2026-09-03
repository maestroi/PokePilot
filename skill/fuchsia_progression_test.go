package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestFuchsiaProgressionAvailable(t *testing.T) {
	for _, mapID := range []uint8{
		mrFujisHouseMap, lavenderTownMap, lavenderPokemonCenterMap,
		route12Map, route13Map, route14Map, route15Map, route15Gate1FMap,
		fuchsiaCityMap, fuchsiaPokemonCenterMap, wardensHouseMap,
		safariZoneGateMap, fuchsiaGymMap, safariZoneEastMap, safariZoneNorthMap,
		safariZoneWestMap, safariZoneCenterMap, safariZoneSecretHouse,
	} {
		if !FuchsiaProgressionAvailable(mapID) {
			t.Fatalf("expected map %#02x to be resumable", mapID)
		}
	}
	if FuchsiaProgressionAvailable(0x00) {
		t.Fatal("Pallet Town must not be part of the Fuchsia progression slice")
	}
}

func TestFuchsiaProgressionReady(t *testing.T) {
	var mem state.Mem
	if FuchsiaProgressionReady(&mem) {
		t.Fatal("progression must require the Poke Flute")
	}
	setTestBag(&mem, [2]uint8{pokeFluteItemFuchsia, 1})
	if !FuchsiaProgressionReady(&mem) {
		t.Fatal("Poke Flute ownership should satisfy the #32 handoff")
	}
}

func TestFuchsiaProgressionComplete(t *testing.T) {
	var mem state.Mem
	setTestBag(&mem, [2]uint8{hm03SurfItem, 1}, [2]uint8{hm04StrengthItem, 1})
	if FuchsiaProgressionComplete(&mem) {
		t.Fatal("HM03 + HM04 without the Soul Badge must not complete the slice")
	}

	mem[sym.ObtainedBadges] = soulBadgeMask
	if !FuchsiaProgressionComplete(&mem) {
		t.Fatal("Soul Badge + HM03 + HM04 should complete the slice")
	}

	setTestBag(&mem, [2]uint8{hm03SurfItem, 1})
	if FuchsiaProgressionComplete(&mem) {
		t.Fatal("missing HM04 must keep the slice incomplete")
	}
}

func TestFuchsiaGymRegistration(t *testing.T) {
	g, ok := GymAt(fuchsiaGymMap)
	if !ok {
		t.Fatal("Fuchsia Gym is not registered")
	}
	if g.Leader != "KOGA" || g.LeaderX != 4 || g.LeaderY != 10 || g.Badge != state.BadgeSoul {
		t.Fatalf("Fuchsia Gym = %+v, want KOGA at (4,10) with Soul Badge", g)
	}
	d, ok := Place(g.Place)
	if !ok || d.Map != fuchsiaGymMap || d.X != 4 || d.Y != 11 {
		t.Fatalf("Koga approach place = %+v,%v, want map %#02x at (4,11)", d, ok, fuchsiaGymMap)
	}
}

func TestNeedsSafariRewards(t *testing.T) {
	var mem state.Mem
	if !needsSafariRewards(&mem) {
		t.Fatal("missing both Safari rewards must require a Safari session")
	}
	setTestBag(&mem, [2]uint8{hm03SurfItem, 1}, [2]uint8{goldTeethItem, 1})
	if needsSafariRewards(&mem) {
		t.Fatal("HM03 + Gold Teeth should satisfy Safari collection")
	}
	setTestBag(&mem, [2]uint8{hm03SurfItem, 1})
	state.SetEvent(&mem, eventGaveGoldTeeth)
	if needsSafariRewards(&mem) {
		t.Fatal("after giving Gold Teeth away, HM03 alone should satisfy Safari collection")
	}
}

func setTestBag(mem *state.Mem, entries ...[2]uint8) {
	mem[sym.NumBagItems] = uint8(len(entries))
	for i, entry := range entries {
		off := sym.BagItems + uint16(i)*2
		mem[off] = entry[0]
		mem[off+1] = entry[1]
	}
}
