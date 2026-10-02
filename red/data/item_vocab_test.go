package data

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestItemTableMatchesDecomp keeps the canonical Red item vocabulary honest:
// the ids are re-derived from the vendored pret constants rather than trusted.
// The planner's executable item whitelist is this table, so a real world pickup
// whose name is missing here is rejected at validation as an "unknown Red item"
// and stops the run (#2388: carbos in Safari Zone East).
func TestItemTableMatchesDecomp(t *testing.T) {
	src, err := os.ReadFile("../../pokered/constants/item_constants.asm")
	if err != nil {
		t.Fatal(err)
	}
	// A few player-facing names intentionally differ from the naive
	// lowercased-underscore decomp form.
	nameOverride := map[string]string{
		"poke ball":  "pokeball",
		"s s ticket": "s.s. ticket",
	}
	// Deliberately resolved through a specific trusted path (dex fishing, the
	// old-rod NPC reward) or withheld as a sensitive progression item, so they
	// are not in the generic executable whitelist (see the itemTable note).
	excluded := map[string]bool{
		"master ball": true,
		"old rod":     true,
		"good rod":    true,
		"super rod":   true,
	}
	re := regexp.MustCompile(`^\s*const (\w+)\s*;\s*\$?([0-9a-fA-F]+)`)
	seen := 0
	for _, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, "NUM_ITEMS") {
			break
		}
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name, idHex := m[1], m[2]
		if name == "NO_ITEM" {
			continue
		}
		id, err := strconv.ParseUint(idHex, 16, 8)
		if err != nil {
			t.Fatal(err)
		}
		// Badge event-flag overloads and unused slots are not bag items.
		if (id >= 0x15 && id <= 0x1C) || id == 0x2C || id == 0x32 {
			continue
		}
		want := strings.ToLower(strings.ReplaceAll(name, "_", " "))
		if override, ok := nameOverride[want]; ok {
			want = override
		}
		if excluded[want] {
			continue
		}
		got, ok := itemTable[want]
		if !ok || got != uint8(id) {
			t.Errorf("item %q: table = %#02x, %v; want %#02x", want, got, ok, id)
		}
		seen++
	}
	// HMs are defined by the add_hm macro (not const lines) and named hm01-hm05.
	for i, raw := range []uint8{0xC4, 0xC5, 0xC6, 0xC7, 0xC8} {
		want := fmt.Sprintf("hm%02d", i+1)
		if got, ok := itemTable[want]; !ok || got != raw {
			t.Errorf("item %q: table = %#02x, %v; want %#02x", want, got, ok, raw)
		}
	}
	if len(itemTable) != seen+5 {
		t.Fatalf("decomp items=%d table=%d, want %d", seen, len(itemTable), seen+5)
	}
	// The exact regression: carbos is a real Safari Zone East pickup.
	if raw, ok := ItemRaw("carbos"); !ok || raw != 0x26 {
		t.Fatalf("ItemRaw(carbos) = %#02x, %v; want 0x26", raw, ok)
	}
}
