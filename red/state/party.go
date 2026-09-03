package state

import "github.com/maestroi/pokepilot/red/sym"

// Mon is one party member decoded from RAM.
type Mon struct {
	Species uint8
	Level   uint8
	HP      uint16
	MaxHP   uint16
	Status  uint8
	Type1   uint8
	Type2   uint8
	Attack  uint16
	Defense uint16
	Speed   uint16
	Special uint16
	Moves   [4]uint8
	PP      [4]uint8 // current PP; PP Up count bits are stripped during decode
}

// Fainted reports whether the mon's HP is 0.
func (m Mon) Fainted() bool { return m.HP == 0 }

// The status byte's bits, derived from constants/battle_constants.asm:62-67.
// The low three bits are a SLEEP COUNTER, not a flag: any non-zero value
// means the mon is asleep, and the value is the number of turns left. The
// rest are single-bit flags.
const (
	statusSleepMask = 0b00000111
	statusPoison    = 1 << 3
	statusBurn      = 1 << 4
	statusFreeze    = 1 << 5
	statusParalyze  = 1 << 6
)

// Poisoned reports whether the mon has the poison status.
func (m Mon) Poisoned() bool { return m.Status&statusPoison != 0 }

// Asleep reports whether the mon is asleep. Sleep lives in the low three
// bits as a counter, so this tests the whole mask, never a single bit: a
// 3-turn sleep (0b011) and a 7-turn sleep (0b111) are both asleep, and a
// single-bit test (status == 0b001) would read them as something else.
func (m Mon) Asleep() bool { return m.Status&statusSleepMask != 0 }

// StatusName reports the mon's status as a name for prompts and logs, or
// "" when the mon is healthy. Several bits can be set at once (a mon can
// be poisoned and asleep), so the name is the first match in the order
// sleep, poison, burn, freeze, paralyze.
func (m Mon) StatusName() string {
	switch {
	case m.Asleep():
		return "asleep"
	case m.Poisoned():
		return "poisoned"
	case m.Status&statusBurn != 0:
		return "burned"
	case m.Status&statusFreeze != 0:
		return "frozen"
	case m.Status&statusParalyze != 0:
		return "paralyzed"
	}
	return ""
}

// PartyState is the decoded party: Count mons, capped at 6.
type PartyState struct {
	Count uint8
	Mons  []Mon // len == Count, capped at 6
}

// DecodeParty reads the party from a RAM snapshot. A PartyCount larger than 6
// (corrupt or mid-init RAM) is clamped to 6 rather than panicking. Gen 1's
// party entries already carry their current calculated stats and types, so
// switch policy can score bench members directly without reconstructing base
// stats from species data. Party PP bytes pack the PP Up count into the high
// two bits; Mon.PP exposes only current remaining PP.
func DecodeParty(m *Mem) PartyState {
	count := int(m.U8(sym.PartyCount))
	if count > 6 {
		count = 6
	}
	mons := make([]Mon, count)
	for n := 0; n < count; n++ {
		base := sym.PartyMon1 + uint16(n)*sym.PartyMonSize
		mons[n] = Mon{
			Species: m.U8(base + sym.MonSpecies),
			Level:   m.U8(base + sym.MonLevel),
			HP:      m.U16BE(base + sym.MonHP),
			MaxHP:   m.U16BE(base + sym.MonMaxHP),
			Status:  m.U8(base + sym.MonStatus),
			Type1:   m.U8(base + sym.MonType1),
			Type2:   m.U8(base + sym.MonType2),
			Attack:  m.U16BE(base + sym.MonAttack),
			Defense: m.U16BE(base + sym.MonDefense),
			Speed:   m.U16BE(base + sym.MonSpeed),
			Special: m.U16BE(base + sym.MonSpecial),
		}
		copy(mons[n].Moves[:], m.Slice(base+sym.MonMoves, 4))
		copy(mons[n].PP[:], m.Slice(base+sym.MonPP, 4))
		for i := range mons[n].PP {
			mons[n].PP[i] &= CurrentPPMask
		}
	}
	return PartyState{Count: uint8(count), Mons: mons}
}
