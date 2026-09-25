package data

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestMoveTableMatchesDecomp keeps the generated move vocabulary honest: the
// ids are re-derived from the vendored pret constants rather than trusted.
func TestMoveTableMatchesDecomp(t *testing.T) {
	src, err := os.ReadFile("../../pokered/constants/move_constants.asm")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`^\s*const (\w+)\s*;\s*([0-9a-f]+)`)
	seen := 0
	for _, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, "NUM_ATTACKS") {
			break
		}
		m := re.FindStringSubmatch(line)
		if m == nil || m[1] == "NO_MOVE" {
			continue
		}
		id, err := strconv.ParseUint(m[2], 16, 8)
		if err != nil {
			t.Fatal(err)
		}
		want := strings.ToLower(strings.ReplaceAll(m[1], "_", " "))
		got, ok := MoveName(uint8(id))
		if !ok || got != want {
			t.Errorf("move %#02x = %q, %v; want %q", id, got, ok, want)
		}
		seen++
	}
	if seen != 165 || len(moveTable) != seen {
		t.Fatalf("decomp moves=%d table=%d, want 165", seen, len(moveTable))
	}
	if raw, ok := MoveRaw("Thunderbolt"); !ok || raw != 0x55 {
		t.Fatalf("MoveRaw(Thunderbolt) = %#02x, %v", raw, ok)
	}
	if name, ok := TypeName(0x15); !ok || name != "water" {
		t.Fatalf("TypeName(water) = %q, %v", name, ok)
	}
}
