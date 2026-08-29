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

// TestS86NewDestinations asserts the S8-6 additions to the place table: every
// new name resolves through Place, appears in PlaceNames(), and carries the
// map id derived from constants/map_constants.asm (CERULEAN_CITY $03,
// ROUTE_3 $0E, ROUTE_4 $0F, MT_MOON_1F $3B, MT_MOON_B1F $3C, MT_MOON_B2F $3D,
// CERULEAN_POKECENTER $40, CERULEAN_GYM $41, MT_MOON_POKECENTER $44).
// Standability and object-home clearance are covered by
// TestPlaceDestinationsStandable, which iterates PlaceNames() and therefore
// picks these up automatically; the probe evidence for every coordinate is in
// RUNNOTES S8-6.
func TestS86NewDestinations(t *testing.T) {
	want := map[string]uint8{
		"route 3":                 0x0E,
		"route 4":                 0x0F,
		"cerulean city":           0x03,
		"mt moon 1f":              0x3B,
		"mt moon b1f":             0x3C,
		"mt moon b2f":             0x3D,
		"mt moon pokemon center":  0x44,
		"cerulean pokemon center": 0x40,
		"cerulean gym":            0x41,
	}
	listed := map[string]bool{}
	for _, n := range skill.PlaceNames() {
		listed[n] = true
	}
	for name, id := range want {
		if !listed[name] {
			t.Errorf("PlaceNames() missing %q", name)
			continue
		}
		d, ok := skill.Place(name)
		if !ok {
			t.Errorf("Place(%q): not found", name)
			continue
		}
		if d.Map != id {
			t.Errorf("Place(%q).Map = %#04x, want %#04x", name, d.Map, id)
		}
	}
}

// TestRouteThroughMtMoon pins the S8-6 measurement at the router level: the
// map graph connects Route 3 to Route 4 THROUGH Mt. Moon's cave floors — the
// first destination chain to cross a multi-floor indoor dungeon. It answers
// the slice's question, "can the router route Route 3 -> Route 4 through the
// cave?", deterministically, without an emulator.
//
// Routes measured with TestProbe (RUNNOTES S8-6). The descent skips 1F:
// Route 4 has a direct warp (24,5) to B1F, so the shortest path is
//
//	Route 3 (0x0E) -north edge-> Route 4 (0x0F)
//	Route 4 (0x0F) -warp(24,5)-> B1F (0x3C)
//	B1F (0x3C)     -ladder warp(17,11)-> B2F (0x3D)
//
// The ascent is the reverse via B1F's own exit warp:
//
//	B2F (0x3D) -ladder warp(25,9)-> B1F (0x3C)
//	B1F (0x3C) -warp(27,3)-> Route 4 (0x0F)
//
// The 1F floor is connected separately: Route 4's cave-entrance warp (18,5)
// reaches it and it ladders down to B1F.
//
// NOTE (RUNNOTES S8-6): a full emulator WALK of the descent currently fails
// on Mt. Moon 1F — world.Build mislabels grid cell (9,22) as walkable (the
// ROM blocks the step; no sprite is there), so the intra-map BFS dead-ends.
// That is a localized collision-grid defect, not a routing failure, tracked
// separately; this test pins the routing, which is correct.
func TestRouteThroughMtMoon(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	// Descent: Route 3 reaches the deepest cave floor only by going through
	// the cave; the route must pass through Route 4, B1F, then B2F.
	down, err := world.FindRoute(g, 0x0E, 0x3D)
	if err != nil {
		t.Fatalf("FindRoute(Route 3 -> B2F): %v", err)
	}
	if got := routeMaps(0x0E, down); !containsSubseq(got, []uint8{0x0F, 0x3C, 0x3D}) {
		t.Fatalf("descent Route 3 -> B2F = %v; expected it to pass through Route 4, B1F, B2F", got)
	}

	// Ascent: the deepest floor routes back out to Route 4 through B1F.
	up, err := world.FindRoute(g, 0x3D, 0x0F)
	if err != nil {
		t.Fatalf("FindRoute(B2F -> Route 4): %v", err)
	}
	if got := routeMaps(0x3D, up); !containsSubseq(got, []uint8{0x3C, 0x0F}) {
		t.Fatalf("ascent B2F -> Route 4 = %v; expected it to pass through B1F", got)
	}

	// The 1F floor is connected: Route 4's cave-entrance warp reaches it and
	// it ladders down to B1F.
	if _, err := world.FindRoute(g, 0x0F, 0x3B); err != nil {
		t.Fatalf("FindRoute(Route 4 -> Mt Moon 1F): %v", err)
	}
	if _, err := world.FindRoute(g, 0x3B, 0x3C); err != nil {
		t.Fatalf("FindRoute(Mt Moon 1F -> B1F): %v", err)
	}

	t.Logf("router routes Route 3 -> through Mt Moon -> Route 4: down %v, up %v",
		routeMaps(0x0E, down), routeMaps(0x3D, up))
}

// routeMaps expands a FindRoute edge list into the sequence of map ids it
// visits, starting at start.
func routeMaps(start uint8, edges []world.Edge) []uint8 {
	maps := []uint8{start}
	for _, e := range edges {
		maps = append(maps, e.To)
	}
	return maps
}

// containsSubseq reports whether sub appears in a as an ordered subsequence
// (a may visit extra maps between the expected ones).
func containsSubseq(a, sub []uint8) bool {
	i := 0
	for _, v := range a {
		if i < len(sub) && v == sub[i] {
			i++
		}
	}
	return i == len(sub)
}
