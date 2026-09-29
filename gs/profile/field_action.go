package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gsStrengthBoulderMovement = 0x19
	gsObjectMovementOffset    = 0x03
	gsObjectMapXOffset        = 0x10
	gsObjectMapYOffset        = 0x11
)

func gsCuttableCollision(c byte) bool {
	switch c {
	case 0x10, 0x12, 0x14, 0x18, 0x1a, 0x1c:
		return true
	default:
		return false
	}
}

func gsFacingTileCollision(reader game.MemoryReader) byte {
	switch reader.Peek8(sym.PlayerDirection) & 0x0c {
	case 0x00:
		return reader.Peek8(sym.TileDown)
	case 0x04:
		return reader.Peek8(sym.TileUp)
	case 0x08:
		return reader.Peek8(sym.TileLeft)
	default:
		return reader.Peek8(sym.TileRight)
	}
}

func gsBoulderAhead(reader game.MemoryReader) bool {
	x := int(reader.Peek8(sym.XCoord)) + 4
	y := int(reader.Peek8(sym.YCoord)) + 4
	switch reader.Peek8(sym.PlayerDirection) & 0x0c {
	case 0x00:
		y++
	case 0x04:
		y--
	case 0x08:
		x--
	case 0x0c:
		x++
	}
	for i := 0; i < sym.NumObjectStructs; i++ {
		base := sym.ObjectStructs + uint16(i*sym.ObjectStructLen)
		if reader.Peek8(base+gsObjectMovementOffset) != gsStrengthBoulderMovement {
			continue
		}
		if int(reader.Peek8(base+gsObjectMapXOffset)) == x && int(reader.Peek8(base+gsObjectMapYOffset)) == y {
			return true
		}
	}
	return false
}

func (*Profile) DecodeFieldAction(reader game.MemoryReader) game.FieldActionState {
	if reader == nil {
		return game.FieldActionState{}
	}
	playerState := reader.Peek8(sym.PlayerState)
	_, choice := (&Profile{}).DecodeTwoOption(reader)
	return game.FieldActionState{
		Controllable:     gsControllable(reader),
		CuttableAhead:    gsCuttableCollision(gsFacingTileCollision(reader)),
		BoulderAhead:     gsBoulderAhead(reader),
		Surfing:          playerState == 4 || playerState == 8,
		StrengthActive:   reader.Peek8(sym.BikeFlags)&1 != 0,
		Lit:              reader.Peek8(sym.StatusFlags)&(1<<2) != 0,
		ActionSucceeded:  reader.Peek8(sym.FieldMoveSucceeded) != 0,
		ResultTextActive: reader.Peek8(sym.StateFlags)&(1<<6) != 0,
		ChoiceVisible:    choice,
		DebugText:        gsScreenText(reader),
	}
}

var _ game.FieldActionDecoder = (*Profile)(nil)
