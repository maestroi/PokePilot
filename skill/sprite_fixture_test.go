package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestDecodeSpritesFixture is S5c-4's real-ROM anchor: it loads the
// viridian_pokecenter fixture (the player on the counter approach tile, map
// 0x29), snapshots RAM, decodes the live sprites, and compares slot 1 (the
// stationary nurse) against the ROM's own map header. ParseMap removes the
// ROM's +4 bias, so the two must agree exactly. A synthetic test cannot catch
// a wrong stride, a wrong offset, or a missed -4 (its writer and its decoder
// share the same wrong assumption); this comparison is the only thing that can.
func TestDecodeSpritesFixture(t *testing.T) {
	e := fixture.Load(t, "viridian_pokecenter")

	var mem state.Mem
	state.Snapshot(e, &mem)

	// The fixture is on the Viridian Pokemon Center (map 0x29).
	if p := state.DecodePlayer(&mem); p.MapID != 0x29 {
		t.Fatalf("fixture map = %#04x, want 0x29", p.MapID)
	}

	header, err := rom.ParseMap(e.ROM(), 0x29)
	if err != nil {
		t.Fatalf("ParseMap(0x29): %v", err)
	}
	if len(header.Objects) == 0 {
		t.Fatal("ParseMap(0x29): no objects; the anchor has nothing to compare against")
	}

	sprites := state.DecodeSprites(&mem)

	// Find the decoded slot 1 (the nurse).
	var nurse *state.SpriteState
	for i := range sprites {
		if sprites[i].Slot == 1 {
			nurse = &sprites[i]
			break
		}
	}
	if nurse == nil {
		t.Fatal("slot 1 nurse inactive: DecodeSprites returned no slot 1; the liveness predicate is wrong")
	}

	// header.Objects[0] is the nurse (the pokecenter's first object).
	obj := header.Objects[0]
	if nurse.X != int(obj.X) || nurse.Y != int(obj.Y) {
		t.Fatalf("slot 1 nurse at (%d,%d), want (%d,%d) from map header", nurse.X, nurse.Y, int(obj.X), int(obj.Y))
	}
	if nurse.PictureID != obj.SpriteID {
		t.Errorf("slot 1 nurse picture ID = %#02x, want %#02x from map header", nurse.PictureID, obj.SpriteID)
	}
}
