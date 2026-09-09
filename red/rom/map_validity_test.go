package rom

import (
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestMapValidityMatchesDecomp(t *testing.T) {
	constants, err := os.ReadFile("../../pokered/constants/map_constants.asm")
	if err != nil {
		t.Fatal(err)
	}
	entries := regexp.MustCompile(`(?m)^\s*map_const ([A-Z0-9_]+),[^\n]*;\s*\$([0-9A-F]{2})`).FindAllSubmatch(constants, -1)
	if len(entries) != 0xf8 {
		t.Fatalf("decomp map count = %d, want 248", len(entries))
	}
	used := map[uint8]bool{}
	for _, entry := range entries {
		id, err := strconv.ParseUint(string(entry[2]), 16, 8)
		if err != nil {
			t.Fatal(err)
		}
		used[uint8(id)] = !strings.HasPrefix(string(entry[1]), "UNUSED_MAP_")
	}
	for id := 0; id < 256; id++ {
		if got := validMapID(uint8(id)); got != used[uint8(id)] {
			t.Errorf("map %02x validity = %v, decomp says %v", id, got, used[uint8(id)])
		}
		if !used[uint8(id)] {
			if _, err := ParseMap(nil, uint8(id)); !errors.Is(err, ErrInvalidMapID) {
				t.Errorf("map %02x: %v, want ErrInvalidMapID before reading ROM", id, err)
			}
		}
	}
}

// Unused table slots may contain readable garbage or alias a real header.
// Parseability must not turn them into maps that participate in routing.
func TestParseMapRejectsUnusedDecompSlots(t *testing.T) {
	data := loadROM(t)
	constants, err := os.ReadFile("../../pokered/constants/map_constants.asm")
	if err != nil {
		t.Fatal(err)
	}
	entries := regexp.MustCompile(`map_const UNUSED_MAP_([0-9A-F]{2}),`).FindAllSubmatch(constants, -1)
	if len(entries) == 0 {
		t.Fatal("no unused map slots found in decomp")
	}
	for _, entry := range entries {
		id, err := strconv.ParseUint(string(entry[1]), 16, 8)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(string(entry[1]), func(t *testing.T) {
			if _, err := ParseMap(data, uint8(id)); err == nil {
				t.Fatalf("unused map %02x accepted", id)
			}
		})
	}
}
