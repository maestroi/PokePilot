package skill_test

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
	"github.com/maestroi/pokepilot/world"
)

// TestGoToViridianPokecenter is the slice-2 milestone: from Pallet Town,
// the player walks to the Viridian Pokémon Center, crossing Route 1 and
// Viridian City, then the player faces the nurse and talks. The name keeps
// "GoTo" as the milestone's identity even though the walk uses Travel: the
// route crosses Route 1's tall grass, GoTo aborts on a wild battle by
// design (MEASURED ~1 encounter on the Pallet -> Viridian leg), and Travel
// fights it and resumes, so the test is deterministic. Setup is the cached
// pallet_town checkpoint instead of replaying GetStarter; post_starter is
// NOT the start point: it ends in Oak's lab, not Pallet Town.
func TestGoToViridianPokecenter(t *testing.T) {
	e := fixture.Load(t, "pallet_town")

	dest, ok := skill.Place("viridian pokemon center")
	if !ok {
		t.Fatal("Place: \"viridian pokemon center\" not found")
	}
	if dest.Map != 0x29 || dest.X != 3 || dest.Y != 3 {
		t.Fatalf("Place = %+v, want {Map:0x29 X:3 Y:3}", dest)
	}

	res, err := skill.Travel(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), 20)
	if err != nil {
		t.Fatalf("Travel: %v", err)
	}
	t.Logf("reached the Viridian Pokémon Center after %d battles (BlackedOut=%v)", res.Battles, res.BlackedOut)

	var mem state.Mem
	state.Snapshot(e, &mem)
	p := state.DecodePlayer(&mem)
	if p.MapID != 0x29 {
		t.Fatalf("CurMap = %#04x, want 0x29", p.MapID)
	}
	if p.X != 3 || p.Y != 3 {
		t.Errorf("player = (%d,%d), want (3,3)", p.X, p.Y)
	}
	if !state.Controllable(&mem) {
		t.Error("player not controllable after GoTo")
	}

	// The nurse stands at (3,1) behind the counter at (3,2). The player
	// cannot stand on a counter tile, so the interaction is to stand below
	// it at (3,3) and face the counter; the game reaches the nurse across
	// it. Talk asserts a text box actually opened, so its success is what
	// proves she responded.
	if err := skill.Face(e, 3, 2); err != nil {
		t.Fatalf("Face(3,2): %v", err)
	}
	presses, err := skill.Talk(e)
	if err != nil {
		t.Fatalf("Talk: %v", err)
	}
	if presses < 1 {
		t.Errorf("Talk presses = %d, want >= 1", presses)
	}
}

// TestPlaceDestinationsStandable proves that every name PlaceNames returns
// resolves to a tile the player can actually stand on: in bounds, walkable on
// the map's collision grid, and not an object's home tile (an NPC blocks its
// own tile, so a destination there is unreachable by walking). The test
// iterates PlaceNames() rather than a hand-written list, so any place added to
// goto.go is covered automatically.
func TestPlaceDestinationsStandable(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM %s: %v", romPath, err)
	}

	for _, name := range skill.PlaceNames() {
		t.Run(name, func(t *testing.T) {
			dest, ok := skill.Place(name)
			if !ok {
				t.Fatalf("Place(%q): not found", name)
			}
			h, err := rom.ParseMap(romData, dest.Map)
			if err != nil {
				t.Fatalf("ParseMap(0x%02x): %v", dest.Map, err)
			}
			grid, err := world.Build(romData, h)
			if err != nil {
				t.Fatalf("Build(0x%02x): %v", dest.Map, err)
			}
			if !grid.InBounds(int(dest.X), int(dest.Y)) {
				t.Fatalf("(%d,%d) is not in bounds on map 0x%02x (%dx%d)", dest.X, dest.Y, dest.Map, grid.Width, grid.Height)
			}
			if !grid.Walkable(int(dest.X), int(dest.Y)) {
				t.Fatalf("(%d,%d) is not walkable on map 0x%02x", dest.X, dest.Y, dest.Map)
			}
			for _, o := range h.Objects {
				if o.X == dest.X && o.Y == dest.Y {
					t.Fatalf("(%d,%d) is the home tile of object sprite %d on map 0x%02x", dest.X, dest.Y, o.SpriteID, dest.Map)
				}
			}
		})
	}
}
