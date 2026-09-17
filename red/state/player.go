package state

import (
	"fmt"
)

// Facing is the direction the player sprite faces.
type Facing uint8

const (
	FacingDown  Facing = 0
	FacingUp    Facing = 4
	FacingLeft  Facing = 8
	FacingRight Facing = 12
)

// String renders the Gen 1 direction encoding; unknown values render as
// "unknown(N)".
func (f Facing) String() string {
	switch f {
	case FacingDown:
		return "down"
	case FacingUp:
		return "up"
	case FacingLeft:
		return "left"
	case FacingRight:
		return "right"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(f))
	}
}

// PlayerState is the player's position and animation state.
type PlayerState struct {
	MapID   uint8
	X, Y    uint8 // tile coordinates within the current map
	Facing  Facing
	Walking bool // true while a step animation is in progress
}

// WorldState describes the map the player is currently on.
type WorldState struct {
	MapID         uint8
	Width, Height uint8 // in tiles
	Tileset       uint8
}

// DecodePlayer reads the player's position and facing from a RAM snapshot.
func (a Addresses) DecodePlayer(m *Mem) PlayerState {
	return PlayerState{
		MapID:   m.U8(a.CurMap),
		X:       m.U8(a.XCoord),
		Y:       m.U8(a.YCoord),
		Facing:  Facing(m.U8(a.SpritePlayerFacing)),
		Walking: m.U8(a.WalkCounter) != 0,
	}
}

// DecodeWorld reads the current map's dimensions and tileset.
func (a Addresses) DecodeWorld(m *Mem) WorldState {
	return WorldState{
		MapID:   m.U8(a.CurMap),
		Width:   m.U8(a.CurMapWidth),
		Height:  m.U8(a.CurMapHeight),
		Tileset: m.U8(a.CurMapTileset),
	}
}
