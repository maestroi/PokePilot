package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestFilterGrassCatchesDropsUnreachableHabitats(t *testing.T) {
	route23, _ := skill.Place("route 23")
	offered := []Objective{
		{Kind: KindCatch, Species: "fearow", Place: "route 23", Flee: true},
		{Kind: KindCatch, Species: "pidgey", Place: "route 1", Flee: true},
		{Kind: KindCatch, Species: "slowpoke", Place: "route 23", Intent: dexFishingIntent},
		{Kind: KindGoTo, Place: "route 23"},
	}
	got := filterGrassCatches(offered, func(mapID uint8) bool { return mapID != route23.Map })
	if len(got) != 3 || got[0].Species != "pidgey" || got[1].Intent != dexFishingIntent || got[2].Kind != KindGoTo {
		t.Fatalf("filtered = %+v, want pidgey catch, fishing hunt and travel kept", got)
	}
}
