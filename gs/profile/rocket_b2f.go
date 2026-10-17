package profile

import "github.com/maestroi/pokepilot/game"

// Events from the Gold/Silver retail event flag table (zero-based).
// The executive's transient hide-object event must not stand in for the
// post-battle EVENT_BEAT_ROCKET_EXECUTIVEF_2.
const (
	eventRocketTransmitterDoorOpened uint16 = 768
	eventRocketExecutiveFBeaten      uint16 = 1392
	eventRocketElectrode1            uint16 = 1760
	eventRocketElectrode2            uint16 = 1761
	eventRocketElectrode3            uint16 = 1762
)

type RocketB2FStory struct {
	TransmitterDoorOpened bool
	ExecutiveFBeaten      bool
	Electrodes            [3]bool
	Cleared               bool
}

// DecodeRocketB2FStory observes durable cartridge event bits only. In
// particular, all three Electrodes vanishing does not mean that Lance's
// dialogue/HM06 reward script has set EVENT_CLEARED_ROCKET_HIDEOUT.
func (*Profile) DecodeRocketB2FStory(reader game.MemoryReader) RocketB2FStory {
	if reader == nil {
		return RocketB2FStory{}
	}
	return RocketB2FStory{
		TransmitterDoorOpened: hasGSEvent(reader, eventRocketTransmitterDoorOpened),
		ExecutiveFBeaten:      hasGSEvent(reader, eventRocketExecutiveFBeaten),
		Electrodes: [3]bool{
			hasGSEvent(reader, eventRocketElectrode1),
			hasGSEvent(reader, eventRocketElectrode2),
			hasGSEvent(reader, eventRocketElectrode3),
		},
		Cleared: hasGSEvent(reader, eventClearedRocketHideout),
	}
}
