package state

import "github.com/maestroi/pokepilot/red/sym"

// Mon is one party member decoded from RAM.
type Mon struct {
	Species uint8
	Level   uint8
	HP      uint16
	MaxHP   uint16
	Status  uint8
	Moves   [4]uint8
	PP      [4]uint8
}

// Fainted reports whether the mon's HP is 0.
func (m Mon) Fainted() bool { return m.HP == 0 }

// PartyState is the decoded party: Count mons, capped at 6.
type PartyState struct {
	Count uint8
	Mons  []Mon // len == Count, capped at 6
}

// DecodeParty reads the party from a RAM snapshot. A PartyCount larger than 6
// (corrupt or mid-init RAM) is clamped to 6 rather than panicking.
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
		}
		copy(mons[n].Moves[:], m.Slice(base+sym.MonMoves, 4))
		copy(mons[n].PP[:], m.Slice(base+sym.MonPP, 4))
	}
	return PartyState{Count: uint8(count), Mons: mons}
}
