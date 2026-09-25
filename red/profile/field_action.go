package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	fieldCutTreeTile       uint8 = 0x3d
	fieldGymCutTreeTile    uint8 = 0x50
	fieldStrengthActiveBit       = 1 << 0
	fieldSurfingState       uint8 = 2
)

func redFieldActionFront(player state.PlayerState) (int, int, bool) {
	x, y := int(player.X), int(player.Y)
	switch player.Facing {
	case state.FacingUp:
		y--
	case state.FacingDown:
		y++
	case state.FacingLeft:
		x--
	case state.FacingRight:
		x++
	default:
		return 0, 0, false
	}
	return x, y, true
}

// DecodeFieldAction keeps Gen I tile ids, mode values, flags and live boulder
// identity on the Red/Blue profile side of the boundary.
func (*Profile) DecodeFieldAction(reader game.MemoryReader) game.FieldActionState {
	if reader == nil {
		return game.FieldActionState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])

	player := state.DecodePlayer(&mem)
	frontTile := mem.U8(sym.TileInFrontOfPlayer)
	cuttable := frontTile == fieldCutTreeTile || frontTile == fieldGymCutTreeTile

	boulderAhead := false
	if fx, fy, ok := redFieldActionFront(player); ok {
		for _, boulder := range state.DecodeBoulders(&mem) {
			if boulder.X == fx && boulder.Y == fy {
				boulderAhead = true
				break
			}
		}
	}

	return game.FieldActionState{
		Controllable:    state.Controllable(&mem),
		CuttableAhead:   cuttable,
		BoulderAhead:    boulderAhead,
		Surfing:         mem.U8(sym.WalkBikeSurfState) == fieldSurfingState,
		StrengthActive:  mem.U8(sym.StatusFlags1)&fieldStrengthActiveBit != 0,
		Lit:             mem.U8(sym.MapPalOffset) == 0,
		ActionSucceeded: mem.U8(sym.ActionResult) == 1,
	}
}
