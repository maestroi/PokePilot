package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// The ROM Tilesets table must reproduce every hand-verified collision table,
// so maps outside the slice get the same collision the slice would use.
func TestROMTilesetCollisionMatchesVerifiedTables(t *testing.T) {
	for _, env := range []string{"POKEMON_GOLD_ROM", "POKEMON_SILVER_ROM"} {
		t.Run(env, func(t *testing.T) {
			rom := loadROM(t, env)
			tilesets, err := locateTilesets(rom)
			if err != nil {
				t.Fatal(err)
			}
			for tileset := uint8(1); tileset <= numTilesets; tileset++ {
				want, ok := firstBadgeCollisionTable(tileset)
				if !ok {
					continue
				}
				got, err := tilesetCollisionAt(rom, tilesets, tileset)
				if err != nil || len(got) < len(want) || !bytesEqual(got[:len(want)], want) {
					t.Fatalf("tileset %#02x ROM collision differs from the verified table (err=%v)", tileset, err)
				}
			}
		})
	}
}

// Post-League story travel needs maps the first-badge slice never had.
func TestRuntimeWorldRoutesPostLeagueStory(t *testing.T) {
	for _, env := range []string{"POKEMON_GOLD_ROM", "POKEMON_SILVER_ROM"} {
		t.Run(env, func(t *testing.T) {
			rom := loadROM(t, env)
			provider, err := NewRuntimeWorldProvider(rom)
			if err != nil {
				t.Fatal(err)
			}
			graph, err := world.BuildNativeGraph(provider)
			if err != nil {
				t.Fatal(err)
			}
			newBark := nativeID(t, "NEW_BARK_TOWN")
			for _, dest := range []string{"OLIVINE_PORT", "GOLDENROD_MAGNET_TRAIN_STATION", "SILVER_CAVE_ROOM_3"} {
				if _, err := world.FindNativeRoute(graph, newBark, nativeID(t, dest)); err != nil {
					t.Errorf("New Bark -> %s: %v", dest, err)
				}
			}
			vermilion := nativeID(t, "VERMILION_PORT")
			for _, dest := range []string{"VERMILION_GYM", "POWER_PLANT", "CERULEAN_GYM", "PEWTER_GYM", "VIRIDIAN_GYM", "SAFFRON_MAGNET_TRAIN_STATION", "LAV_RADIO_TOWER_1F"} {
				if _, err := world.FindNativeRoute(graph, vermilion, nativeID(t, dest)); err != nil {
					t.Errorf("Vermilion Port -> %s: %v", dest, err)
				}
			}
			for _, id := range provider.MapIDs() {
				if _, err := provider.Grid(id, nil, worldmodel.TraversalLand); err != nil {
					t.Errorf("grid %#04x: %v", id, err)
				}
			}
		})
	}
}
