package profile

import "github.com/maestroi/pokepilot/game"

// The switch-room state is volatile. GoldenrodUndergroundResetSwitchesCallback
// resets the switches and door flags on entering the parent tunnel, so these
// MUST NOT be modeled as durable campaign milestones. Read the live cartridge
// bits after each interaction; never infer an open door from switch taps.
const (
	eventUndergroundSwitch1 uint16 = 723
	eventUndergroundSwitch2 uint16 = 724
	eventUndergroundSwitch3 uint16 = 725
	eventUndergroundEmergencySwitch uint16 = 726
	eventUndergroundFinalDoorOpen uint16 = 737
)

type RadioTowerSwitches struct {
	Switch1 bool
	Switch2 bool
	Switch3 bool
	Emergency bool
	FinalDoorOpen bool
}

func (*Profile) DecodeRadioTowerSwitches(reader game.MemoryReader) RadioTowerSwitches {
	return RadioTowerSwitches{
		Switch1: hasGSEvent(reader, eventUndergroundSwitch1),
		Switch2: hasGSEvent(reader, eventUndergroundSwitch2),
		Switch3: hasGSEvent(reader, eventUndergroundSwitch3),
		Emergency: hasGSEvent(reader, eventUndergroundEmergencySwitch),
		FinalDoorOpen: hasGSEvent(reader, eventUndergroundFinalDoorOpen),
	}
}
