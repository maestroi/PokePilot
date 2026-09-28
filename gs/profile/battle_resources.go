package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gen2PartyMovesOffset   uint16 = 0x02
	gen2PartyPPOffset      uint16 = 0x17
	gen2PartyLevelOffset   uint16 = 0x1f
	gen2PartyStatusOffset  uint16 = 0x20
	gen2PartyHPOffset      uint16 = 0x22
	gen2PartyMaxHPOffset   uint16 = 0x24
	gen2PartyAttackOffset  uint16 = 0x26
	gen2PartyDefenseOffset uint16 = 0x28
	gen2PartySpeedOffset   uint16 = 0x2a
	gen2PartySpAtkOffset   uint16 = 0x2c
	gen2PartySpDefOffset   uint16 = 0x2e
)

func gen2Status(raw byte) string {
	switch {
	case raw&0x07 != 0:
		return "asleep"
	case raw&0x08 != 0:
		return "poisoned"
	case raw&0x10 != 0:
		return "burned"
	case raw&0x20 != 0:
		return "frozen"
	case raw&0x40 != 0:
		return "paralyzed"
	default:
		return ""
	}
}

func (*Profile) DecodeBattleResources(reader game.MemoryReader) game.BattleResourcesState {
	if reader == nil {
		return game.BattleResourcesState{ActiveSlot: -1}
	}
	count := int(reader.Peek8(sym.PartyCount))
	if count < 0 {
		count = 0
	}
	if count > 6 {
		count = 6
	}
	out := game.BattleResourcesState{
		InBattle:   reader.Peek8(sym.BattleMode) != 0,
		ActiveSlot: int(reader.Peek8(sym.CurBattleMon)),
		Party:      make([]game.BattlePartyMon, 0, count),
	}
	for i := 0; i < count; i++ {
		base := sym.PartyMon1 + uint16(i)*sym.PartyMonSize
		mon := game.BattlePartyMon{
			NativeSpeciesID: uint16(reader.Peek8(base)),
			Level:           reader.Peek8(base + gen2PartyLevelOffset),
			HP:              battleBE16(reader, base+gen2PartyHPOffset),
			MaxHP:           battleBE16(reader, base+gen2PartyMaxHPOffset),
			Status:          gen2Status(reader.Peek8(base + gen2PartyStatusOffset)),
			Attack:          battleBE16(reader, base+gen2PartyAttackOffset),
			Defense:         battleBE16(reader, base+gen2PartyDefenseOffset),
			Speed:           battleBE16(reader, base+gen2PartySpeedOffset),
			SpecialAttack:   battleBE16(reader, base+gen2PartySpAtkOffset),
			SpecialDefense:  battleBE16(reader, base+gen2PartySpDefOffset),
		}
		mon.Special = mon.SpecialAttack
		for slot := 0; slot < 4; slot++ {
			mon.Moves[slot] = game.BattlePartyMove{
				NativeMoveID: uint16(reader.Peek8(base + gen2PartyMovesOffset + uint16(slot))),
				PP:           reader.Peek8(base+gen2PartyPPOffset+uint16(slot)) & gen2PPMask,
			}
		}
		// Party structs do not carry species types. The active battle struct
		// does, so project those exact live types for the active slot and leave
		// inactive types unknown rather than reconstructing ROM data in policy.
		if i == out.ActiveSlot && out.InBattle {
			mon.Type1 = uint16(reader.Peek8(sym.BattleMonType1))
			mon.Type2 = uint16(reader.Peek8(sym.BattleMonType2))
		}
		out.Party = append(out.Party, mon)
	}
	// Bag policy remains intentionally empty in this slice. Gen-II item IDs
	// differ from Gen-I's medicine table; exposing native inventory before the
	// shared medicine vocabulary is generation-neutral would make Battle use
	// the wrong items.
	return out
}

var _ game.BattleResourcesDecoder = (*Profile)(nil)
