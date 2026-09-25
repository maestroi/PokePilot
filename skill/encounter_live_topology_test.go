package skill

import "testing"

func TestEncounterCellsForWalkabilityUsesLiveIndoorTopology(t *testing.T) {
	static := []cell{{x: 0, y: 0}, {x: 1, y: 0}, {x: 2, y: 0}}
	open := map[[2]int]bool{
		{0, 0}: true,
		{2, 0}: true,
	}
	got := encounterCellsForWalkability(static, 3, 1, true, func(x, y int) bool {
		return open[[2]int{x, y}]
	})
	if len(got) != 2 || got[0] != (cell{x: 0, y: 0}) || got[1] != (cell{x: 2, y: 0}) {
		t.Fatalf("live indoor encounter cells = %+v, want only currently walkable cells", got)
	}
}

func TestEncounterCellsForWalkabilityFiltersOutdoorStaticCells(t *testing.T) {
	static := []cell{{x: 0, y: 0}, {x: 1, y: 0}, {x: 2, y: 0}}
	got := encounterCellsForWalkability(static, 3, 1, false, func(x, y int) bool {
		return x != 1
	})
	if len(got) != 2 || got[0] != static[0] || got[1] != static[2] {
		t.Fatalf("filtered encounter cells = %+v, want static cells still live-walkable", got)
	}
}
