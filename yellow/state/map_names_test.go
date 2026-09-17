package state

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const mapConstantsFile = "../../pokeyellow/constants/map_constants.asm"

var mapConstName = regexp.MustCompile(`^[A-Z0-9_]+$`)

// parseMapConstants reads the vendored pokeyellow decomp and returns each
// map's constant name keyed by the explicit hex ID on its map_const line.
// The decomp is the inventory this ROM ships with: 249 lines, one per map.
// A line looks like
//
//	map_const PALLET_TOWN,                   10,  9 ; $00
//
// and the ID is the $XX comment after the first ';'. One Yellow line carries
// a second ';' with a note after the ID
//
//	map_const UNDERGROUND_PATH_NORTH_SOUTH,   4, 24 ; $77 ; ...is actually 4x23
//
// and everything past the ID is ignored, so fields[5] is always the ID.
//
// The parser fails the test on a malformed line, a duplicate ID, or a
// duplicate name.
func parseMapConstants(t *testing.T) map[uint8]string {
	t.Helper()
	data, err := os.ReadFile(mapConstantsFile)
	if err != nil {
		t.Fatal("reading vendored map constants: ", err)
	}

	maps := make(map[uint8]string)
	byName := make(map[string]uint8)
	for n, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "map_const" {
			continue
		}
		if len(fields) < 6 {
			t.Fatalf("%s:%d: malformed map_const line: %s", mapConstantsFile, n+1, line)
		}
		name, width, height, sep, idHex := fields[1], fields[2], fields[3], fields[4], fields[5]
		name = strings.TrimSuffix(name, ",")
		if !mapConstName.MatchString(name) {
			t.Fatalf("%s:%d: bad map name %q", mapConstantsFile, n+1, name)
		}
		width = strings.TrimSuffix(width, ",")
		if _, err := strconv.Atoi(width); err != nil {
			t.Fatalf("%s:%d: bad map width %q", mapConstantsFile, n+1, width)
		}
		if _, err := strconv.Atoi(height); err != nil {
			t.Fatalf("%s:%d: bad map height %q", mapConstantsFile, n+1, height)
		}
		if sep != ";" {
			t.Fatalf("%s:%d: missing ';' before the ID: %s", mapConstantsFile, n+1, line)
		}
		if !strings.HasPrefix(idHex, "$") {
			t.Fatalf("%s:%d: bad map id %q", mapConstantsFile, n+1, idHex)
		}
		id, err := strconv.ParseUint(idHex[1:], 16, 8)
		if err != nil {
			t.Fatalf("%s:%d: bad map id %q: %v", mapConstantsFile, n+1, idHex, err)
		}
		mid := uint8(id)
		if prev, ok := maps[mid]; ok {
			t.Fatalf("%s:%d: duplicate ID $%02X: %s and %s", mapConstantsFile, n+1, mid, prev, name)
		}
		if prev, ok := byName[name]; ok {
			t.Fatalf("%s:%d: duplicate name %s: $%02X and $%02X", mapConstantsFile, n+1, name, prev, mid)
		}
		maps[mid] = name
		byName[name] = mid
	}
	return maps
}

// TestMapNamesMatchDecomp keeps the generated mapNames table honest against
// the vendored decomp, the same way red/state's test does. It compares the
// WHOLE table in both directions: every decomp entry must be present in
// mapNames with the same name, and every non-empty mapNames entry must be
// present in the decomp. A spot check would pass against a table with 200
// wrong entries; this does not.
func TestMapNamesMatchDecomp(t *testing.T) {
	decomp := parseMapConstants(t)
	if len(decomp) == 0 {
		t.Fatalf("parsed no map_const lines from %s", mapConstantsFile)
	}
	if len(decomp) != 249 {
		t.Errorf("%s defines %d maps, want 249", mapConstantsFile, len(decomp))
	}

	for id, name := range decomp {
		if got := mapNames[id]; got != name {
			t.Errorf("mapNames[0x%02X] = %q, want %q (from %s)", id, got, name, mapConstantsFile)
		}
	}
	for i, name := range mapNames {
		id := uint8(i)
		if name == "" {
			continue
		}
		want, ok := decomp[id]
		if !ok {
			t.Errorf("mapNames[0x%02X] = %q, but %s has no map with ID $%02X", id, name, mapConstantsFile, id)
			continue
		}
		if name != want {
			t.Errorf("mapNames[0x%02X] = %q, want %q (from %s)", id, name, want, mapConstantsFile)
		}
	}
}

// TestMapNameUnknownID is the contract behind MapName: an ID the ROM does not
// define yields "". 0xF9 is a genuine gap after SUMMER_BEACH_HOUSE ($F8).
func TestMapNameUnknownID(t *testing.T) {
	if got := MapName(0xF9); got != "" {
		t.Errorf("MapName(0xF9) = %q, want \"\"", got)
	}
	if got := MapName(0xFF); got != "" {
		t.Errorf("MapName(0xFF) = %q, want \"\"", got)
	}
}
