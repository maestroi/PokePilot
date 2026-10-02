package rom

import (
	"fmt"

	"github.com/maestroi/pokepilot/worldmodel"
)

// Object types from pokegold constants/script_constants.asm.
const (
	ObjectTypeScript   uint8 = 0
	ObjectTypeItemBall uint8 = 1
	ObjectTypeTrainer  uint8 = 2
)

// Movement classes from pokegold constants/map_object_constants.asm.
const (
	moveStill         uint8 = 0x01
	moveWander        uint8 = 0x02
	moveSpinSlow      uint8 = 0x03
	moveWalkUpDown    uint8 = 0x04
	moveWalkLeftRight uint8 = 0x05
	moveStandingDown  uint8 = 0x06
	moveStandingUp    uint8 = 0x07
	moveStandingLeft  uint8 = 0x08
	moveStandingRight uint8 = 0x09
	moveSpinFast      uint8 = 0x0a
	moveSwimWander    uint8 = 0x24
)

// Object is one def_object_events record with sprite coordinates un-biased
// (the ROM stores them +4).
type Object struct {
	Slot         int
	Sprite       uint8
	X, Y         uint8
	Movement     uint8
	RangeX       uint8
	RangeY       uint8
	Hour1        uint8
	Hour2        uint8
	Palette      uint8
	Type         uint8
	Sight        uint8
	Script       uint16
	EventFlag    uint16
	ItemID       uint8
	ItemQty      uint8
	TrainerClass uint8
	TrainerSet   uint8
}

func (o Object) Portable() worldmodel.MapObject {
	out := worldmodel.MapObject{
		Slot:               o.Slot,
		X:                  o.X,
		Y:                  o.Y,
		Movement:           objectMovement(o.Movement),
		NativeSpriteID:     uint16(o.Sprite),
		NativeItemID:       uint16(o.ItemID),
		NativeTrainerClass: uint16(o.TrainerClass),
		NativeTrainerSet:   uint16(o.TrainerSet),
	}
	return out
}

func objectMovement(raw uint8) worldmodel.ObjectMovement {
	switch raw {
	case moveStill, moveStandingDown, moveStandingUp, moveStandingLeft, moveStandingRight:
		return worldmodel.ObjectMovementStay
	case moveWander, moveSpinSlow, moveWalkUpDown, moveWalkLeftRight, moveSpinFast, moveSwimWander:
		return worldmodel.ObjectMovementWalk
	default:
		return worldmodel.ObjectMovementUnknown
	}
}

func decodeObject(rom []byte, off, slot int, scriptBank uint8) (Object, error) {
	if err := mustInROM(rom, off, objectEventLen, "object event"); err != nil {
		return Object{}, err
	}
	y := rom[off+1]
	x := rom[off+2]
	if y < objectSpriteYOffset || x < objectSpriteXOffset {
		return Object{}, fmt.Errorf("gs/rom: object %d has un-biased sprite coords (%d,%d)", slot, x, y)
	}
	o := Object{
		Slot:      slot,
		Sprite:    rom[off],
		Y:         y - objectSpriteYOffset,
		X:         x - objectSpriteXOffset,
		Movement:  rom[off+3],
		RangeY:    rom[off+4] >> 4,
		RangeX:    rom[off+4] & 0x0f,
		Hour1:     rom[off+5],
		Hour2:     rom[off+6],
		Palette:   rom[off+7] >> 4,
		Type:      rom[off+7] & 0x0f,
		Sight:     rom[off+8],
		Script:    uint16(rom[off+9]) | uint16(rom[off+10])<<8,
		EventFlag: uint16(rom[off+11]) | uint16(rom[off+12])<<8,
	}
	if o.Type == ObjectTypeItemBall || o.Type == ObjectTypeTrainer {
		rec, err := bankedOffset(scriptBank, o.Script)
		if err == nil && rec+2 <= len(rom) {
			switch o.Type {
			case ObjectTypeItemBall:
				o.ItemID = rom[rec]
				o.ItemQty = rom[rec+1]
			case ObjectTypeTrainer:
				if rec+4 <= len(rom) {
					o.TrainerClass = rom[rec+2]
					o.TrainerSet = rom[rec+3]
				}
			}
		}
	}
	return o, nil
}
