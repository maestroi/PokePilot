package rom

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

func TestIsInertWarpMatchesDecomp(t *testing.T) {
	for _, tc := range []struct {
		file        string
		mapID, x, y uint8
	}{
		{"CeladonCity", 0x06, 39, 19},
		{"SilphCo1F", 0xb5, 16, 10},
		{"SilphCo11F", 0xeb, 5, 5},
	} {
		src, err := os.ReadFile("../../pokeyellow/data/maps/objects/" + tc.file + ".asm")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), fmt.Sprintf("warp_event %2d, %2d,", tc.x, tc.y)) || !strings.Contains(string(src), "; inaccessible") {
			t.Fatalf("%s: decomp no longer marks (%d,%d) inaccessible", tc.file, tc.x, tc.y)
		}
		if !IsInertWarp(tc.mapID, tc.x, tc.y) {
			t.Fatalf("IsInertWarp(%#02x,%d,%d) = false", tc.mapID, tc.x, tc.y)
		}
	}
	if IsInertWarp(0xeb, 4, 5) {
		t.Fatal("ordinary Silph 11F tile was classified inert")
	}
}

// The routing header is what GoTo plans with; dropping Inert there bans the
// Silph 11F pad and cuts the president's office off from the boss room.
func TestYellowWorldHeaderMarksInertWarps(t *testing.T) {
	h := projectWorldHeader(MapHeader{ID: 0xeb, Warps: []Warp{{X: 5, Y: 5}, {X: 3, Y: 2}}})
	if want := []worldmodel.Warp{{X: 5, Y: 5, Inert: true}, {X: 3, Y: 2}}; h.Warps[0] != want[0] || h.Warps[1] != want[1] {
		t.Fatalf("warps = %+v, want %+v", h.Warps, want)
	}
}
