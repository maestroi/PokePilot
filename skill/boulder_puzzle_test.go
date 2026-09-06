package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func writePuzzleSprite(mem *state.Mem, slot, x, y int, picture uint8) {
	data1 := sym.SpritePlayerStateData1 + uint16(slot)*0x10
	data2 := sym.SpriteStateData2 + uint16(slot)*0x10
	mem[data1+0x00] = picture
	mem[data1+0x02] = 0x00
	mem[data2+0x04] = uint8(y + 4)
	mem[data2+0x05] = uint8(x + 4)
}

func TestLiveBoulderMovablesUseSpriteSlotAsStableNodeID(t *testing.T) {
	var mem state.Mem
	writePuzzleSprite(&mem, 3, 5, 15, state.BoulderPictureID)
	writePuzzleSprite(&mem, 7, 17, 13, state.BoulderPictureID)
	writePuzzleSprite(&mem, 8, 6, 6, 0x01)

	got := liveBoulderMovables(&mem)
	want := []world.Movable{
		{ID: 3, Pos: world.Point{X: 5, Y: 15}},
		{ID: 7, Pos: world.Point{X: 17, Y: 13}},
	}
	if len(got) != len(want) {
		t.Fatalf("liveBoulderMovables = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("movable %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	fixed := liveNonBoulderBlockers(&mem)
	if fixed[[2]int{5, 15}] || fixed[[2]int{17, 13}] {
		t.Fatalf("boulders leaked into fixed blockers: %v", fixed)
	}
	if !fixed[[2]int{6, 6}] {
		t.Fatalf("ordinary live sprite missing from fixed blockers: %v", fixed)
	}
}

func TestVictoryRoadBoulderSpecsMatchROMScriptGoals(t *testing.T) {
	cases := []struct {
		section  VictoryRoadBoulderSection
		mapID    uint8
		target   world.Point
		event    state.Event
		terminal bool
	}{
		{VictoryRoad1FSwitch, 0x6c, world.Point{X: 17, Y: 13}, 0x917, false},
		{VictoryRoad2FSwitch1, 0xc2, world.Point{X: 1, Y: 16}, 0x538, false},
		{VictoryRoad3FSwitch, 0xc6, world.Point{X: 3, Y: 5}, 0x660, false},
		{VictoryRoad3FHole, 0xc6, world.Point{X: 23, Y: 15}, 0x666, true},
		{VictoryRoad2FSwitch2, 0xc2, world.Point{X: 9, Y: 16}, 0x53f, false},
	}

	for _, tc := range cases {
		t.Run(tc.section.String(), func(t *testing.T) {
			spec, ok := VictoryRoadBoulderSpec(tc.section)
			if !ok {
				t.Fatal("spec not found")
			}
			if spec.Map != tc.mapID || len(spec.Targets) != 1 || spec.Targets[0] != tc.target {
				t.Fatalf("spec map/target = map %#02x targets=%v, want map %#02x target=%v", spec.Map, spec.Targets, tc.mapID, tc.target)
			}
			if !spec.HasCompleteEvent || spec.CompleteEvent != tc.event {
				t.Fatalf("completion event = enabled=%v event=%#x, want %#x", spec.HasCompleteEvent, spec.CompleteEvent, tc.event)
			}
			if got := spec.TerminalTargets[[2]int{tc.target.X, tc.target.Y}]; got != tc.terminal {
				t.Fatalf("terminal target = %v, want %v", got, tc.terminal)
			}
		})
	}
}
