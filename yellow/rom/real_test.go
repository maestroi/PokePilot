package rom

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/yellow/sym"
)

func loadRealYellowROM(t *testing.T) []byte {
	t.Helper()
	if testing.Short() {
		t.Skip("ROM-backed Yellow world test")
	}
	path := os.Getenv("POKEMON_YELLOW_ROM")
	if path == "" {
		path = "roms/pokemon_yellow.gb"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("POKEMON_YELLOW_ROM: %v", err)
	}
	if got := game.InspectROM(data).SHA1; got != sym.ROMSHA1 {
		t.Fatalf("Yellow ROM sha1 = %s, want %s", got, sym.ROMSHA1)
	}
	return data
}

func TestRealYellowWorldParsesEveryPlayableMap(t *testing.T) {
	data := loadRealYellowROM(t)
	for _, id := range MapIDs() {
		h, err := ParseMap(data, id)
		if err != nil {
			t.Fatalf("ParseMap(%02x %s): %v", id, MapName(id), err)
		}
		for _, c := range h.Connections {
			if !validMapID(c.MapID) {
				t.Errorf("%02x %s connection -> invalid %02x", id, MapName(id), c.MapID)
			}
		}
		_, elevator := lookupElevator(id)
		for _, w := range h.Warps {
			// Elevator warps carry a placeholder destination that the
			// elevator script rewrites; the graph models them through
			// LookupElevator instead.
			if elevator {
				continue
			}
			if w.DestMap != 0xff && !validMapID(w.DestMap) {
				t.Errorf("%02x %s warp -> invalid %02x", id, MapName(id), w.DestMap)
			}
		}
		if _, err := h.WorldGridSpec(data, nil, 0); err != nil {
			t.Fatalf("grid %02x %s: %v", id, MapName(id), err)
		}
	}
}

func TestRealYellowGraphIncludesBeachHouse(t *testing.T) {
	data := loadRealYellowROM(t)
	graph, err := world.BuildGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Edges) != yellowPlayableMapCount {
		t.Fatalf("graph maps = %d, want %d", len(graph.Edges), yellowPlayableMapCount)
	}
	if _, ok := graph.Edges[0xF8]; !ok {
		t.Fatal("SUMMER_BEACH_HOUSE (F8) missing from graph")
	}
}

// Regression: Yellow keeps tileset collision lists in bank 1, not in the
// tileset's GFX bank. Reading them from the wrong bank made every overworld
// tile walkable and left most of Kanto unreachable from Pallet Town.
func TestRealYellowOverworldCollisionUsesYellowCollisionBank(t *testing.T) {
	data := loadRealYellowROM(t)
	h, err := ParseMap(data, 0x00) // PALLET_TOWN
	if err != nil {
		t.Fatal(err)
	}
	spec, err := h.WorldGridSpec(data, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	blocked := 0
	for _, walkable := range spec.Walkable {
		if !walkable {
			blocked++
		}
	}
	if blocked < len(spec.Walkable)/4 {
		t.Fatalf("Pallet Town has %d/%d blocked tiles; collision list read from the wrong bank", blocked, len(spec.Walkable))
	}
	graph, err := world.BuildGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	reached := map[world.MapID]bool{}
	for _, e := range graph.Edges[0x00] {
		reached[e.To] = true
	}
	if !reached[0x25] { // REDS_HOUSE_1F
		t.Fatalf("Pallet Town exits = %v, want a warp into REDS_HOUSE_1F", graph.Edges[0x00])
	}
}

func TestRealYellowStaticTables(t *testing.T) {
	data := loadRealYellowROM(t)
	encounters, err := WildEncounters(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(encounters) == 0 {
		t.Fatal("no Yellow wild encounters parsed")
	}
	if got, err := SpeciesName(data, 0x54); err != nil || got != "PIKACHU" {
		t.Fatalf("Pikachu species name = %q, %v", got, err)
	}
	if got, err := MoveName(data, 85); err != nil || got != "THUNDERBOLT" {
		t.Fatalf("Thunderbolt name = %q, %v", got, err)
	}
	move, err := LookupMove(data, 85)
	if err != nil || move.ID != 85 {
		t.Fatalf("Thunderbolt move = %+v, %v", move, err)
	}
	if got, err := ItemName(data, 0x31); err != nil || got != "NUGGET" {
		t.Fatalf("Nugget item name = %q, %v", got, err)
	}
	if _, err := TrainerParty(data, 1, 1); err != nil {
		t.Fatalf("Youngster party: %v", err)
	}
}
