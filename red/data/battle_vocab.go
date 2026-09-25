package data

import "github.com/maestroi/pokepilot/game"

var moveByID = reverse(moveTable)

// typeNames are Gen I's type bytes (pokered/constants/type_constants.asm).
// BIRD ($06) is the unused placeholder type; nothing in the shipped game
// carries it, and it reads as flying for any portable consumer.
var typeNames = map[uint8]string{
	0x00: "normal",
	0x01: "fighting",
	0x02: "flying",
	0x03: "poison",
	0x04: "ground",
	0x05: "rock",
	0x06: "flying",
	0x07: "bug",
	0x08: "ghost",
	0x14: "fire",
	0x15: "water",
	0x16: "grass",
	0x17: "electric",
	0x18: "psychic",
	0x19: "ice",
	0x1a: "dragon",
}

func MoveName(raw uint8) (string, bool) {
	name, ok := moveByID[raw]
	return name, ok
}

func Move(raw uint8) (game.MoveID, bool) {
	name, ok := MoveName(raw)
	if !ok {
		return "", false
	}
	return game.MoveID(name), true
}

func MoveRaw(id game.MoveID) (uint8, bool) {
	raw, ok := moveTable[game.CanonicalID(string(id))]
	return raw, ok
}

// TypeName returns the portable name of a Gen I type byte.
func TypeName(raw uint8) (string, bool) {
	name, ok := typeNames[raw]
	return name, ok
}
