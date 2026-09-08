package state

import "github.com/maestroi/pokepilot/red/sym"

// partyMonExperienceOffset is MON_EXP in Red's party struct. Experience is a
// three-byte big-endian cumulative value. Keep the raw offset inside the Red
// state boundary rather than exposing it to planner/runtime code.
const partyMonExperienceOffset uint16 = 0x0e

// PartyExperience returns one party member's cumulative experience. The bool
// is false for an invalid slot or a transient/corrupt party count.
func PartyExperience(m *Mem, slot int) (uint32, bool) {
	count := int(m.U8(sym.PartyCount))
	if count > 6 {
		count = 6
	}
	if slot < 0 || slot >= count {
		return 0, false
	}
	base := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize + partyMonExperienceOffset
	return uint32(m.U8(base))<<16 | uint32(m.U8(base+1))<<8 | uint32(m.U8(base+2)), true
}
