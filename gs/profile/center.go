package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

var _ game.CenterDecoder = (*Profile)(nil)

// DecodeCenter projects Gold/Silver nurse-transaction state onto the portable
// Center contract so shared Heal can own YES/NO and the recovered-party
// postcondition without Red RAM.
func (p *Profile) DecodeCenter(reader game.MemoryReader) game.CenterState {
	if reader == nil {
		return game.CenterState{}
	}
	_, promptOpen := p.DecodeTwoOption(reader)
	return game.CenterState{
		PartyPresent: reader.Peek8(sym.PartyCount) > 0,
		PromptOpen:   promptOpen,
		Recovered:    gsPartyCenterRecovered(reader),
		Controllable: gsControllable(reader),
		TextOpen:     gsTextboxVisible(reader) && !promptOpen,
		MenuOpen:     promptOpen,
	}
}

func gsPartyCenterRecovered(reader game.MemoryReader) bool {
	if reader == nil {
		return false
	}
	count := int(reader.Peek8(sym.PartyCount))
	if count <= 0 {
		return false
	}
	if count > 6 {
		count = 6
	}
	seen := false
	for i := 0; i < count; i++ {
		// Retail Gold/Silver keep SPECIES_EGG in wPartySpecies while the
		// party_struct still holds the hatch species and typically 0 HP.
		// Center recovery must key off the species list — treating the egg
		// slot as a fainted mon makes Heal wait forever.
		if gsPartySlotIsEgg(reader, i) {
			continue
		}
		base := sym.PartyMon1 + uint16(i)*sym.PartyMonSize
		seen = true
		hp := battleBE16(reader, base+gen2PartyHPOffset)
		maxHP := battleBE16(reader, base+gen2PartyMaxHPOffset)
		if maxHP == 0 || hp != maxHP || reader.Peek8(base+gen2PartyStatusOffset) != 0 {
			return false
		}
		for slot := 0; slot < 4; slot++ {
			move := reader.Peek8(base + gen2PartyMovesOffset + uint16(slot))
			pp := reader.Peek8(base+gen2PartyPPOffset+uint16(slot)) & gen2PPMask
			if move != 0 && pp == 0 {
				return false
			}
		}
	}
	return seen
}

// gsPartySlotIsEgg reports whether party slot i is an Egg. Prefer wPartySpecies
// (the cartridge's authoritative list); also accept SPECIES_EGG in the
// party_struct for fixtures that only populate that field.
func gsPartySlotIsEgg(reader game.MemoryReader, slot int) bool {
	if reader == nil || slot < 0 || slot > 5 {
		return false
	}
	if gsdata.IsEgg(reader.Peek8(sym.PartySpecies + uint16(slot))) {
		return true
	}
	base := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize
	return gsdata.IsEgg(reader.Peek8(base))
}
