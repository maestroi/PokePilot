package agent

import "testing"

func TestDexCatchGrassSourceRespectsLiveCurrentMapHabitat(t *testing.T) {
	src := DexSource{Kind: AcquireWildGrass, Place: PlaceID("pokemon mansion")}
	obs := Observation{
		Map:      0xA5,
		HasGrass: false,
		Bag:      []Item{{Name: "pokeball", Quantity: 5}},
	}
	if _, _, ok := dexCatchGrassSource(obs, src, nil, nil, nil); ok {
		t.Fatal("sealed current-map habitat was offered despite live HasGrass=false")
	}

	obs.HasGrass = true
	if place, _, ok := dexCatchGrassSource(obs, src, nil, nil, nil); !ok || place != src.Place {
		t.Fatalf("reachable current-map habitat = %q ok=%v, want %q true", place, ok, src.Place)
	}
}
