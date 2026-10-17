package profile

import "github.com/maestroi/pokepilot/game"

// RadioTowerVictory keeps battle events, Rocket departure and the Director's
// gift independent. In RadioTower5F.asm the game sets EVENT_CLEARED_RADIO_TOWER
// before the Gold/Silver-specific wing script has finished. A checkpoint at
// that point is not yet a settled takeover.
type RadioTowerVictory struct {
	ExecutiveM2Beaten bool
	ExecutiveF1Beaten bool
	BossBeaten        bool
	Cleared           bool
	WingReceived      bool
}

func (v RadioTowerVictory) Complete() bool {
	return v.ExecutiveM2Beaten && v.ExecutiveF1Beaten && v.BossBeaten &&
		v.Cleared && v.WingReceived
}

func (p *Profile) DecodeRadioTowerVictory(reader game.MemoryReader) RadioTowerVictory {
	result := RadioTowerVictory{
		ExecutiveM2Beaten: hasGSEvent(reader, eventRadioTowerExecutiveM2Beaten),
		ExecutiveF1Beaten: hasGSEvent(reader, eventRadioTowerExecutiveF1Beaten),
		BossBeaten:        hasGSEvent(reader, eventRadioTowerBossBeaten),
		Cleared:           hasGSEvent(reader, eventRadioTowerCleared),
	}
	// The Director gives Rainbow Wing in Gold, Silver Wing in Silver.
	// Never accept the other game's wing as proof of this script.
	if p != nil {
		switch p.id {
		case GoldGameID:
			result.WingReceived = hasGSEvent(reader, eventRadioTowerGoldWingReceived)
		case SilverGameID:
			result.WingReceived = hasGSEvent(reader, eventRadioTowerSilverWingReceived)
		}
	}
	return result
}
