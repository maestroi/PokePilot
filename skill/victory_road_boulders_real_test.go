package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestVictoryRoadBoulderSectionsRealROM is the private/local qualification for
// #110. Each env var points at a controllable external checkpoint on the named
// floor with the relevant section incomplete and Strength usable/preparable.
// The checkpoint may be captured at the puzzle entrance OR halfway through:
// SolveVictoryRoadBoulderSection ignores canonical starting coordinates and
// derives the continuation exclusively from current player/boulder RAM.
//
// The ROM and .state files are derived commercial-game artifacts and are not
// committed, matching the repository's existing prepared-state test policy.
func TestVictoryRoadBoulderSectionsRealROM(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		section VictoryRoadBoulderSection
	}{
		{"1F switch", "POKEPILOT_VICTORY_ROAD_1F_BOULDER_STATE", VictoryRoad1FSwitch},
		{"2F west switch", "POKEPILOT_VICTORY_ROAD_2F_SWITCH1_STATE", VictoryRoad2FSwitch1},
		{"3F switch", "POKEPILOT_VICTORY_ROAD_3F_SWITCH_STATE", VictoryRoad3FSwitch},
		{"3F hole", "POKEPILOT_VICTORY_ROAD_3F_HOLE_STATE", VictoryRoad3FHole},
		{"2F east switch", "POKEPILOT_VICTORY_ROAD_2F_SWITCH2_STATE", VictoryRoad2FSwitch2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := loadPreparedFieldActionState(t, tc.env)
			spec, ok := VictoryRoadBoulderSpec(tc.section)
			if !ok {
				t.Fatalf("no puzzle spec for %v", tc.section)
			}

			var before state.Mem
			state.Snapshot(m, &before)
			if got := before.U8(sym.CurMap); got != spec.Map {
				t.Fatalf("%s checkpoint map = %#02x, want %#02x", tc.env, got, spec.Map)
			}
			if state.HasEvent(&before, spec.CompleteEvent) {
				t.Fatalf("%s checkpoint already has completion event %#x set", tc.env, spec.CompleteEvent)
			}
			capability := FieldCapabilityFor(&before, FieldStrength)
			if !capability.Usable && !CanPrepareFieldMove(m.ROM(), &before, FieldStrength) {
				t.Fatalf("%s checkpoint cannot use or prepare Strength: %+v", tc.env, capability)
			}
			if len(state.DecodeBoulders(&before)) == 0 {
				t.Fatalf("%s checkpoint has no live boulders", tc.env)
			}

			result, err := SolveVictoryRoadBoulderSection(m, m.ROM(), StatAwareMove(m.ROM()), tc.section)
			if err != nil {
				t.Fatalf("SolveVictoryRoadBoulderSection(%v): %v", tc.section, err)
			}
			if result.Pushes == 0 {
				t.Fatalf("%v reported success without a verified push from an incomplete checkpoint: %+v", tc.section, result)
			}

			var after state.Mem
			state.Snapshot(m, &after)
			if !state.HasEvent(&after, spec.CompleteEvent) {
				t.Fatalf("%v completion event %#x is not set after solve", tc.section, spec.CompleteEvent)
			}

			target := spec.Targets[0]
			terminal := spec.TerminalTargets[[2]int{target.X, target.Y}]
			if terminal {
				for _, boulder := range state.DecodeBoulders(&after) {
					if boulder.X == target.X && boulder.Y == target.Y {
						t.Fatalf("terminal boulder still visible at hole (%d,%d): %+v", target.X, target.Y, boulder)
					}
				}
				return
			}

			found := false
			for _, boulder := range state.DecodeBoulders(&after) {
				if boulder.X == target.X && boulder.Y == target.Y {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("%v event is set but no live boulder occupies target (%d,%d): %v", tc.section, target.X, target.Y, state.DecodeBoulders(&after))
			}
		})
	}
}
