package profile

import "github.com/maestroi/pokepilot/game"

// These zero-based cartridge events are pinned to the vendored
// pokegold/constants/event_flags.asm. Password, door, boss and Electrode
// events are independently persistent: reboot/replay must not have to fight
// trainers or guess script state to continue the Rocket Hideout.
const (
	rocketSlowpokeTailEvent      uint16 = 769
	rocketRaticateTailEvent      uint16 = 770
	rocketGiovanniDoorEvent      uint16 = 771
	rocketExecutiveMEvent        uint16 = 1396
	rocketHailGiovanniEvent      uint16 = 767
	rocketTransmitterDoorEvent   uint16 = 768
	rocketExecutiveFEvent        uint16 = 1392
	rocketElectrode1Event        uint16 = 1760
	rocketElectrode2Event        uint16 = 1761
	rocketElectrode3Event        uint16 = 1762
)

type RocketBaseStory struct {
	SlowpokeTail      bool
	RaticateTail      bool
	GiovanniDoor      bool
	ExecutiveM        bool
	HailGiovanni      bool
	TransmitterDoor   bool
	ExecutiveF        bool
	Electrodes        [3]bool
	Cleared           bool
}

// DecodeRocketBaseStory reads only durable ROM-owned events. It must not
// infer completion from arrival, dialogue state or a battle animation.
func (*Profile) DecodeRocketBaseStory(reader game.MemoryReader) RocketBaseStory {
	if reader == nil {
		return RocketBaseStory{}
	}
	return RocketBaseStory{
		SlowpokeTail:    hasGSEvent(reader, rocketSlowpokeTailEvent),
		RaticateTail:    hasGSEvent(reader, rocketRaticateTailEvent),
		GiovanniDoor:    hasGSEvent(reader, rocketGiovanniDoorEvent),
		ExecutiveM:      hasGSEvent(reader, rocketExecutiveMEvent),
		HailGiovanni:    hasGSEvent(reader, rocketHailGiovanniEvent),
		TransmitterDoor: hasGSEvent(reader, rocketTransmitterDoorEvent),
		ExecutiveF:      hasGSEvent(reader, rocketExecutiveFEvent),
		Electrodes: [3]bool{
			hasGSEvent(reader, rocketElectrode1Event),
			hasGSEvent(reader, rocketElectrode2Event),
			hasGSEvent(reader, rocketElectrode3Event),
		},
		Cleared: hasGSEvent(reader, eventClearedRocketHideout),
	}
}
