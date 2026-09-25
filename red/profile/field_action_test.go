package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

type fieldActionTestMemory [0x10000]byte

func (m *fieldActionTestMemory) Peek8(addr uint16) byte { return m[addr] }

func (m *fieldActionTestMemory) PeekInto(addr uint16, dst []byte) {
	copy(dst, m[int(addr):])
}

func TestDecodeFieldActionOwnsGen1RuntimeEncodings(t *testing.T) {
	m := new(fieldActionTestMemory)
	m[sym.CurMapWidth] = 1
	m[sym.CurMapHeight] = 1
	m[sym.XCoord] = 10
	m[sym.YCoord] = 10
	m[sym.SpritePlayerFacing] = byte(state.FacingRight)
	m[sym.TileInFrontOfPlayer] = fieldCutTreeTile
	m[sym.WalkBikeSurfState] = fieldSurfingState
	m[sym.StatusFlags1] = fieldStrengthActiveBit
	m[sym.MapPalOffset] = 0
	m[sym.ActionResult] = 1

	// Sprite slot 1: live boulder at (11,10), directly in front.
	m[sym.SpritePlayerStateData1+0x10] = state.BoulderPictureID
	m[sym.SpriteStateData2+0x10+0x04] = 14
	m[sym.SpriteStateData2+0x10+0x05] = 15

	got := New().DecodeFieldAction(m)
	if !got.Controllable || !got.CuttableAhead || !got.BoulderAhead || !got.Surfing ||
		!got.StrengthActive || !got.Lit || !got.ActionSucceeded {
		t.Fatalf("decoded field action state = %+v, want all semantic facts true", got)
	}
}

func TestDecodeFieldActionRejectsNonAdjacentBoulder(t *testing.T) {
	m := new(fieldActionTestMemory)
	m[sym.XCoord] = 10
	m[sym.YCoord] = 10
	m[sym.SpritePlayerFacing] = byte(state.FacingRight)
	m[sym.SpritePlayerStateData1+0x10] = state.BoulderPictureID
	m[sym.SpriteStateData2+0x10+0x04] = 14
	m[sym.SpriteStateData2+0x10+0x05] = 16

	if got := New().DecodeFieldAction(m); got.BoulderAhead {
		t.Fatalf("non-adjacent boulder decoded as ahead: %+v", got)
	}
}
