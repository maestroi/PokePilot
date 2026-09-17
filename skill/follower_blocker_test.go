package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// The Pikachu follower is not a wall. Yellow's CollisionCheckOnLand lets the
// player bump it wPikachuCollisionCounter times and then step onto its tile
// (or pass immediately while holding B), so planning around it as a blocker
// would route every Yellow walk away from a tile that is genuinely walkable.
//
// This pins the decision where it actually lives: spriteBlockers must drop
// exactly the sprite slot romTableSet names for the ROM, and no others. The
// emulator has no memory-write API, so the snapshot is built the same way the
// live one is and decoded through the same path, rather than patched in RAM.
func TestSpriteBlockersExcludesFollower(t *testing.T) {
	yellowData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	redData, err := os.ReadFile("../roms/pokemon_red.gb")
	if err != nil {
		t.Skip("pokemon_red.gb not available")
	}
	// Boot Yellow far enough to have a live overworld snapshot to build on.
	st, err := os.ReadFile("failure/yellow_overworld.state")
	if err != nil {
		t.Skip("yellow_overworld.state not available")
	}

	// An ordinary NPC in slot 5 at (2,2) and the follower at (1,1). If the
	// follower is wrongly treated as solid the plan detours; if the NPC is
	// wrongly dropped the plan walks through a real object. Both must be
	// decided correctly on a Yellow ROM.
	fill := func(mem *state.Mem, slot int, pic uint8, x, y int) {
		d1 := 0xC100 + slot*0x10
		d2 := 0xC200 + slot*0x10
		(*mem)[d1] = pic           // picture id; 0 marks the slot unused
		(*mem)[d1+2] = 0x01        // image index; 0xff marks it offscreen
		(*mem)[d2+4] = byte(y + 4) // sprite coords are stored with +4
		(*mem)[d2+5] = byte(x + 4)
	}

	t.Run("yellow drops the follower only", func(t *testing.T) {
		e, err := emu.OpenCGBBytes(yellowData)
		if err != nil {
			t.Fatal(err)
		}
		defer e.Close()
		if err := e.LoadState(st); err != nil {
			t.Fatal(err)
		}
		var mem state.Mem
		state.Snapshot(e, &mem)
		fill(&mem, 5, 0x01, 2, 2)
		fill(&mem, pikaFollowerSlot, 0x3D, 1, 1)

		blocked := spritesBlocked(&mem, tablesForROM(yellowData))
		if blocked[[2]int{1, 1}] {
			t.Error("Pikachu follower tile (1,1) is blocked; the follower tile must be walkable")
		}
		if !blocked[[2]int{2, 2}] {
			t.Error("NPC tile (2,2) is not blocked; an ordinary sprite must still block")
		}
	})

	// Red has no follower, so the same slot must remain a solid blocker
	// there: the exclusion is a per-ROM fact, not a Gen I constant.
	t.Run("red blocks every sprite", func(t *testing.T) {
		e, err := emu.OpenCGBBytes(redData)
		if err != nil {
			t.Fatal(err)
		}
		defer e.Close()
		var mem state.Mem
		state.Snapshot(e, &mem)
		fill(&mem, 5, 0x01, 2, 2)
		fill(&mem, pikaFollowerSlot, 0x3D, 1, 1)

		blocked := spritesBlocked(&mem, tablesForROM(redData))
		if !blocked[[2]int{1, 1}] {
			t.Error("Red blocked the follower slot; Red has no passable follower")
		}
		if !blocked[[2]int{2, 2}] {
			t.Error("NPC tile (2,2) is not blocked on Red")
		}
	})
}
