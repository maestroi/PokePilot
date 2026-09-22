package skill

import (
	"errors"
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
		safariZoneWestMap, safariZoneCenterMap, safariZoneCenterRestHouseMap,
		safariZoneSecretHouse, safariZoneWestRestHouseMap, safariZoneEastRestHouseMap,
		safariZoneNorthRestHouseMap,
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

// TestFuchsiaKogaOutcomeErr pins the triage c4db7cfafa4b263c defect: a Koga
// loss must keep the typed trainer-blackout signal so the planner can train or
// grow the party and retry the slice, not classify as an unknown,
// unrecoverable failure.
func TestFuchsiaKogaOutcomeErr(t *testing.T) {
	if !errors.Is(fuchsiaKogaOutcomeErr(state.ResultLost), ErrTrainerBlackedOut) {
		t.Fatal("Koga loss must unwrap to ErrTrainerBlackedOut so the planner can train and retry")
	}
	if errors.Is(fuchsiaKogaOutcomeErr(state.ResultDraw), ErrTrainerBlackedOut) {
		t.Fatal("a draw must not be reported as a trainer blackout")
	}
	if errors.Is(fuchsiaKogaOutcomeErr(state.ResultWon), ErrTrainerBlackedOut) {
		t.Fatal("a win must not be reported as a trainer blackout")
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
	setTestEvent(&mem, eventGaveGoldTeeth)
	if needsSafariRewards(&mem) {
		t.Fatal("after giving Gold Teeth away, HM03 alone should satisfy Safari collection")
	}
}

func TestSafariGateJoinChoiceIndex(t *testing.T) {
	const prompt = "MONEY ¥43868  YES  NO\nWould you like to join the hunt?"

	if got, ok := safariGateJoinChoiceIndex(safariZoneGateMap, prompt, true); !ok || got != 0 {
		t.Fatalf("enter Safari choice = (%d,%v), want YES index 0", got, ok)
	}
	if got, ok := safariGateJoinChoiceIndex(safariZoneGateMap, prompt, false); !ok || got != 1 {
		t.Fatalf("leave Safari choice = (%d,%v), want NO index 1", got, ok)
	}
	if _, ok := safariGateJoinChoiceIndex(fuchsiaCityMap, prompt, false); ok {
		t.Fatal("Safari prompt text outside the gate map must not be auto-answered")
	}
	if _, ok := safariGateJoinChoiceIndex(safariZoneGateMap, "Would you like to leave early?", false); ok {
		t.Fatal("unrelated Safari gate choices must not be auto-answered")
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

func setTestEvent(mem *state.Mem, event state.Event) {
	off := sym.EventFlags + uint16(event)/8
	mem[off] |= 1 << (uint16(event) % 8)
}
