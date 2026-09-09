package skill_test

import (
	"errors"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
	"github.com/maestroi/pokepilot/world"
)

// domeFossilItemID is DOME_FOSSIL ($29) from
// pokered/constants/item_constants.asm.
const domeFossilItemID uint8 = 0x29

// TestMtMoonFossilOpensTheEasternExit replays the defect that stalled farm
// run run-17rjs2d1uf1kw3 for eighty rounds: "go to cerulean city" from inside
// Mt. Moon, over and over, because Mt. Moon's deepest floor cannot be crossed
// on foot until its Super Nerd has been beaten AND paid.
//
// The gate is his body, measured rather than assumed. B2F's fossil corridor
// narrows to two tiles; he stands on one of them
// (pokered/data/maps/objects/MtMoonB2F.asm: object_event 12, 8) and stepping
// on the other is what triggers his battle:
//
//	PROBE_MAP=0x3d PROBE_AT=21,17 PROBE_TO=5,7 PROBE_BLOCK=12,8;13,8
//	  -> world: no path
//
// Beating him does not move him. MtMoonB2FMoveSuperNerdScript runs only after
// a fossil is taken, so "defeated" is not the postcondition that opens the
// road; "defeated, paid, and the map script back at zero" is. Travel alone
// walks into that corridor and dead-ends there with a bare "no route", which
// is what the farm run repeated: with the fossil objective removed the same
// checkpoint stops at (12,9), one tile below him.
//
// So this test asserts the whole transaction, in the order a run meets it:
// the exit refuses with a NAMED missing capability rather than a dead end,
// the owned objective satisfies it, running the objective twice does not buy
// a second fossil, and the road to Cerulean is then ordinary travel.
func TestMtMoonFossilOpensTheEasternExit(t *testing.T) {
	if testing.Short() {
		t.Skip("emulator journey; not part of the -short gate")
	}
	e := fixture.Load(t, "mt_moon_b2f")
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)
	cerulean, ok := skill.Place("cerulean city")
	if !ok {
		t.Fatal(`Place "cerulean city" not found`)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	if got := mem.U8(sym.CurMap); got != 0x3D {
		t.Fatalf("fixture contract: map=%#04x, want 0x3d (Mt. Moon B2F)", got)
	}
	if facts := storyFacts(&mem); facts.MtMoonFossilAcquired {
		t.Fatal("fixture contract: the fossil is already taken; the blocked half cannot be observed")
	}

	// Blocked, but not silently: the planner learns which capability is
	// missing, which is what turns this from an endless retry into one
	// progression objective.
	_, err := skill.TravelFlee(e, romData, cerulean, policy, 20)
	var blocked *world.RouteBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("travel before the fossil = %v, want a structured world.RouteBlockedError", err)
	}
	if !hasCapability(blocked.MissingCapabilities(), "can_exit_mt_moon") {
		t.Fatalf("missing capabilities = %v, want can_exit_mt_moon named", blocked.MissingCapabilities())
	}
	state.Snapshot(e, &mem)
	if !state.Controllable(&mem) {
		t.Fatal("a refused route left the player uncontrollable")
	}

	if err := skill.MtMoonFossil(e, romData, policy); err != nil {
		t.Fatalf("MtMoonFossil: %v", err)
	}

	// The positive postcondition, read back from RAM independently of the
	// skill's own check: the fight is recorded, the fossil is in the bag,
	// and the map script has run to completion rather than being abandoned
	// mid-sequence with the Super Nerd still walking.
	state.Snapshot(e, &mem)
	if !state.HasEvent(&mem, state.EventBeatMtMoonSuperNerd) {
		t.Error("EVENT_BEAT_MT_MOON_EXIT_SUPER_NERD is not set after the objective")
	}
	if q := bagCount(&mem, domeFossilItemID); q != 1 {
		t.Errorf("bag holds %d DOME_FOSSIL, want 1", q)
	}
	if s := mem.U8(sym.MtMoonB2FCurScript); s != 0 {
		t.Errorf("wMtMoonB2FCurScript = %d after the objective, want 0 (the script finished)", s)
	}
	if !storyFacts(&mem).MtMoonFossilAcquired {
		t.Fatal("StoryFacts.MtMoonFossilAcquired is false after the objective")
	}

	// Idempotent: the planner may re-issue a progression objective it
	// believes is unfinished, and a second fossil would be a second choice
	// answered on its own initiative.
	if err := skill.MtMoonFossil(e, romData, policy); err != nil {
		t.Fatalf("MtMoonFossil is not idempotent: %v", err)
	}
	state.Snapshot(e, &mem)
	if q := bagCount(&mem, domeFossilItemID); q != 1 {
		t.Errorf("bag holds %d DOME_FOSSIL after a second run, want 1", q)
	}
	if state.HasEvent(&mem, state.EventGotHelixFossil) {
		t.Error("the second run also took the Helix Fossil")
	}

	res, err := skill.TravelFlee(e, romData, cerulean, policy, 20)
	if err != nil {
		t.Fatalf("travel to Cerulean after the fossil: %v (result %+v)", err, res)
	}
	state.Snapshot(e, &mem)
	if got := mem.U8(sym.CurMap); got != cerulean.Map {
		t.Fatalf("ended on map %#04x, want %#04x (Cerulean City)", got, cerulean.Map)
	}
	t.Logf("crossed Mt. Moon: %+v", res)
}

func storyFacts(m *state.Mem) state.StoryFacts {
	return state.DecodeStoryFacts(m, state.DecodeInventory(m))
}

func bagCount(m *state.Mem, item uint8) int {
	for _, entry := range state.DecodeInventory(m).Items {
		if entry.ID == item {
			return int(entry.Quantity)
		}
	}
	return 0
}

func hasCapability(ids []gameruntime.CapabilityID, want gameruntime.CapabilityID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
