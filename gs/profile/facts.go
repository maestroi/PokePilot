package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/gs/sym"
)

// Landmark ids from pokegold constants/landmark_constants.asm. The comments
// in that file are the const values; RegionCheck in engine/overworld/landmarks.asm
// is the rule below.
const (
	landmarkSpecial     uint8 = 0x00
	landmarkKanto       uint8 = 0x2e
	landmarkVictoryRoad uint8 = 0x57
)

// box_struct offsets from pokegold macros/ram.asm. Level at 0x1f is the same
// layout DecodeObservation already reads.
const (
	partyDVOffset        = 0x15
	partyHappinessOffset = 0x1b
	partyPokerusOffset   = 0x1c
)

// wTimeOfDay stores MORN_F/DAY_F/NITE_F (0/1/2). DARKNESS_F is a palette, not
// a clock period, so any other byte stays undecoded.
func timeOfDay(raw byte) game.TimeOfDay {
	switch raw {
	case 0:
		return game.TimeMorning
	case 1:
		return game.TimeDay
	case 2:
		return game.TimeNight
	default:
		return ""
	}
}

// regionFromLandmark is RegionCheck without the special-map backup lookup.
// Landmark 0 is a special map; the caller resolves wBackupMapGroup first.
func regionFromLandmark(landmark uint8) game.RegionID {
	if landmark == landmarkSpecial {
		return ""
	}
	if landmark < landmarkKanto || landmark >= landmarkVictoryRoad {
		return game.RegionJohto
	}
	return game.RegionKanto
}

func regionOf(reader game.MemoryReader, rom []byte) game.RegionID {
	if reader == nil || len(rom) == 0 {
		return ""
	}
	landmark, err := gsrom.MapLandmark(rom, gsdata.NativeMapID(reader.Peek8(sym.MapGroup), reader.Peek8(sym.MapNumber)))
	if err != nil {
		return ""
	}
	if landmark == landmarkSpecial {
		landmark, err = gsrom.MapLandmark(rom, gsdata.NativeMapID(reader.Peek8(sym.BackupMapGroup), reader.Peek8(sym.BackupMapNumber)))
		if err != nil {
			return ""
		}
	}
	return regionFromLandmark(landmark)
}

// shinyDV is CheckShininess: attack DV bit 1 set, and defense, speed, and
// special DVs all equal to 10.
func shinyDV(dv0, dv1 byte) bool {
	const shinyAtk = 0x20 // SHINY_ATK_MASK << 4
	return dv0&shinyAtk != 0 && dv0&0x0f == 10 && dv1>>4 == 10 && dv1&0x0f == 10
}

// pokerusStatus matches the stats screen: a nonzero low nibble is an active
// infection, a nonzero high nibble alone is the cured immunity dot.
func pokerusStatus(raw byte) game.Pokerus {
	if raw&0x0f != 0 {
		return game.PokerusInfected
	}
	if raw&0xf0 != 0 {
		return game.PokerusCured
	}
	return ""
}

func partyGender(rom []byte, species uint8, egg bool, dv0, dv1 byte) game.Gender {
	if egg || len(rom) == 0 || species == 0 {
		return ""
	}
	atkSpd := (dv0 & 0xf0) | (dv1 >> 4)
	gender, err := gsrom.Gender(rom, species, atkSpd)
	if err != nil {
		return ""
	}
	return gender
}
