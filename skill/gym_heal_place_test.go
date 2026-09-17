package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// TestGymHealPlaceResolves locks in the pre-leader heal wiring: every gym
// challenge that promises a city Pokemon Center must name a place the table
// resolves, that place must be off the gym's own map (you cannot heal inside
// a gym), and the stand tile must still resolve. A gym with an empty
// HealPlace keeps the caller-heals-before-entry contract it already
// satisfied, so it is skipped here rather than failed.
func TestGymHealPlaceResolves(t *testing.T) {
	seen := 0
	for mapID, g := range gyms {
		if g.HealPlace == "" {
			continue
		}
		seen++
		center, ok := Place(g.HealPlace)
		if !ok {
			t.Errorf("gym %#04x (%s): heal place %q is not registered", mapID, g.Leader, g.HealPlace)
			continue
		}
		if center.Map == g.Map {
			t.Errorf("gym %#04x (%s): heal place %q is on the gym map itself", mapID, g.Leader, g.HealPlace)
		}
		if _, ok := Place(g.Place); !ok {
			t.Errorf("gym %#04x (%s): stand place %q is not registered", mapID, g.Leader, g.Place)
		}
	}
	if seen == 0 {
		t.Fatal("no gym carries a HealPlace, so the pre-leader heal is unreachable")
	}
}

// TestFuchsiaGymHealsBeforeKoga is the regression anchor for
// run-3bnej6i2rtpct1rjr9hm0trdpy round 1, whose Koga battle was lost because
// the Fuchsia trainer gauntlet consumed the pre-gym heal and left Venusaur at
// 41/153 HP for the leader. Fuchsia must carry a registered heal place.
func TestFuchsiaGymHealsBeforeKoga(t *testing.T) {
	g, ok := GymAt(fuchsiaGymMap)
	if !ok {
		t.Fatalf("map %#02x has no gym challenge", fuchsiaGymMap)
	}
	if g.HealPlace == "" {
		t.Fatal("fuchsia gym has no pre-leader heal place; the gauntlet-depleted party is handed to Koga")
	}
	center, ok := Place(g.HealPlace)
	if !ok {
		t.Fatalf("fuchsia heal place %q is not registered", g.HealPlace)
	}
	if center.Map == fuchsiaGymMap {
		t.Fatalf("fuchsia heal place %q heals inside the gym", g.HealPlace)
	}
}

// TestGymHealsBeforeKogaRealROM replays the exact pre-Koga checkpoint from
// run-3bnej6i2rtpct1rjr9hm0trdpy round 1: inside Fuchsia Gym after the
// gauntlet, Venusaur at 41/153 with two party members fainted. Before the
// pre-leader heal this state lost to Koga deterministically
// (state.ResultLost, full party wipe); after it, Gym must win the Soul Badge.
// POKEPILOT_KOGA_TEST_STATE points at that .state; like every other real-ROM
// prepared-state test, it is skipped when the ROM-derived artifact is absent.
func TestGymHealsBeforeKogaRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_KOGA_TEST_STATE")

	var before state.Mem
	state.Snapshot(m, &before)
	g, ok := GymAt(m.Peek8(ram(m).CurMap))
	if !ok {
		t.Fatalf("prepared state is on map %#02x, which has no gym challenge", m.Peek8(ram(m).CurMap))
	}
	if g.HealPlace == "" {
		t.Fatalf("gym on map %#02x carries no heal place", g.Map)
	}
	if allPartyCenterRecovered(&before, ram(m)) {
		t.Fatal("prepared Koga state is already at full strength; it does not exercise the pre-leader heal")
	}

	outcome, err := Gym(m, m.ROM(), StatAwareMove(m.ROM()))
	if err != nil {
		t.Fatalf("Gym: %v", err)
	}
	if outcome != state.ResultWon {
		t.Fatalf("Koga battle outcome %d, want won (ResultWon)", outcome)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if !ram(m).DecodeProgress(&after).Has(state.BadgeSoul) {
		t.Fatal("Soul Badge is not set after beating Koga")
	}
}
