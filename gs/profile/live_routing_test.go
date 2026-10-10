package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestDecodeGen2LiveObjectsMarksSmashableRocksOnly(t *testing.T) {
	var mem fakeMemory
	put := func(i int, sprite byte, slot byte, x, y byte) {
		base := sym.ObjectStructs + uint16(i*sym.ObjectStructLen)
		mem[base], mem[base+1], mem[base+0x10], mem[base+0x11] = sprite, slot, x, y
	}
	put(1, gen2SpriteRock, 1, 4+4, 3+4)
	put(2, 0x5a, 2, 5+4, 3+4)              // SPRITE_BOULDER: Strength, not Rock Smash
	put(3, 0x10, 3, 6+4, 3+4)              // an ordinary NPC
	put(4, gen2SpriteBoulder, 4, 7+4, 3+4) // live Strength boulder
	mem[sym.ObjectStructs+uint16(4*sym.ObjectStructLen)+gen2ObjectMovementTypeOffset] = gen2StrengthBoulderMovement
	objects, _ := decodeGen2LiveObjects(&mem)
	if len(objects) != 4 {
		t.Fatalf("objects = %+v, want 4", objects)
	}
	for _, o := range objects {
		want := game.FieldMoveID("")
		if o.Slot == 1 {
			want = game.FieldMoveRockSmash
		}
		if o.Clearable != want {
			t.Fatalf("object slot %d clearable = %q, want %q", o.Slot, o.Clearable, want)
		}
		if o.Pushable != (o.Slot == 4) {
			t.Fatalf("object slot %d Pushable=%t; only movement-data Strength boulders push", o.Slot, o.Pushable)
		}
	}
}
