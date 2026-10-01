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
		base := sym.PartyMon1 + uint16(i)*sym.PartyMonSize
		if gsdata.IsEgg(reader.Peek8(base)) {
			continue
		}
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
