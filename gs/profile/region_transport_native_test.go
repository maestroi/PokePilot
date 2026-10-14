package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
)

func TestGoldRegionTransportNativePlaceResolver(t *testing.T) {
	p := NewGold()
	for _, tc := range []struct {
		place   game.PlaceID
		mapName string
	}{
		{"olivine-port", "OLIVINE_PORT"},
		{"vermilion-port", "VERMILION_PORT"},
		{"goldenrod-magnet-train-station", "GOLDENROD_MAGNET_TRAIN_STATION"},
		{"saffron-magnet-train-station", "SAFFRON_MAGNET_TRAIN_STATION"},
	} {
		info, ok := gsdata.MapByName(tc.mapName)
		if !ok {
			t.Fatalf("missing native map %s", tc.mapName)
		}
		got, ok := p.NativeRegionTransportPlace(tc.place)
		if !ok || got != gsdata.NativeMapID(info.Group, info.Number) {
			t.Fatalf("%s => %#04x, ok=%v, want map %s", tc.place, got, ok, tc.mapName)
		}
	}
	if _, ok := p.NativeRegionTransportPlace("other-game-location"); ok {
		t.Fatal("unknown place must not resolve")
	}
	if _, ok := p.NativeRegionTransportPlace(""); ok {
		t.Fatal("empty place must not resolve")
	}
	if _, ok := p.NativeRegionTransportRegion(nil, 0); ok {
		t.Fatal("no cartridge cannot establish a region")
	}
}
