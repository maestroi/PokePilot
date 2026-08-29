package state

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const mapConstantsFile = "../../pokered/constants/map_constants.asm"

var mapConstName = regexp.MustCompile(`^[A-Z0-9_]+$`)

// parseMapConstants reads the vendored decomp and returns each map's
// decomp constant name keyed by the explicit hex ID on its map_const
// line. The decomp is the inventory this ROM ships with: 248 lines, one
// per map. A line looks like
//
//	map_const PALLET_TOWN,                   10,  9 ; $00
//
// and the ID is the $XX comment after the first ';'. (One line carries a
// second ';' with a note after the ID; everything past the ID is ignored.)
//
// The parser fails the test on a duplicate ID, a duplicate name, or a
// map_const line that does not have the expected shape.
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
		// map_const NAME, WIDTH, HEIGHT ; $ID [; note]
		if len(fields) < 6 {
			t.Fatalf("%s:%d: malformed map_const line: %s", mapConstantsFile, n+1, line)
		}
		name, width, height, sep, idHex := fields[1], fields[2], fields[3], fields[4], fields[5]
		// The name is written with its trailing comma ("PALLET_TOWN,").
		if !strings.HasSuffix(name, ",") {
			t.Fatalf("%s:%d: missing ',' after map name: %s", mapConstantsFile, n+1, line)
		}
		name = name[:len(name)-1]
		if !mapConstName.MatchString(name) {
			t.Fatalf("%s:%d: bad map name %q", mapConstantsFile, n+1, name)
		}
		// The width is written with its trailing comma too ("10,").
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

// TestMapNamesMatchDecomp keeps the hand-transcribed mapNames table honest
// against the vendored decomp, the same way TestEventIndicesMatchDecomp
// keeps the hand-counted event bit indices honest. It compares the WHOLE
// table in both directions: every decomp entry must be present in mapNames
// with the same name, and every non-empty mapNames entry must be present in
// the decomp with the same name. A spot check would pass against a table
// with 200 wrong entries; this does not.
func TestMapNamesMatchDecomp(t *testing.T) {
	decomp := parseMapConstants(t)
	if len(decomp) == 0 {
		t.Fatalf("parsed no map_const lines from %s", mapConstantsFile)
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

// TestMapNameUnknownID is the contract behind Observation.MapName: an ID
// the ROM does not define yields "" and is never an error. 0xF1-0xF4 are
// the decomp's own UNUSED_MAP_F1..F4 placeholders, so the ROM does define
// them; 0xFA is a genuine gap between AGATHAS_ROOM ($F7) and the end of
// the table.
func TestMapNameUnknownID(t *testing.T) {
	if got := MapName(0xFA); got != "" {
		t.Errorf("MapName(0xFA) = %q, want \"\"", got)
	}
	if got := MapName(0xFF); got != "" {
		t.Errorf("MapName(0xFF) = %q, want \"\"", got)
	}
}
