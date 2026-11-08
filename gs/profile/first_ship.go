package profile

import "github.com/maestroi/pokepilot/game"

// FirstShipVoyageState is the cartridge-owned state machine behind the first
// S.S. Aqua journey. Event bits are deliberately decoded here instead of in
// generic route policy or the campaign planner.
//
// The arrival bit means the ship can dock, not that the player disembarked.
// The first-trip flag is only set by VermilionPortLeaveShipScript.
type FirstShipVoyageState struct {
	LeagueWon       bool
	HasTicket       bool
	FirstTrip       bool
	Arrived         bool
	FoundGirl       bool
	InformedSailor  bool
	RecoveredSailor bool
}

const (
	// Zero-based event indices from pokegold/constants/event_flags.asm,
	// including const_skip/const_next regions.
	gsEventShipArrived        uint16 = 49
	gsEventShipFoundGirl      uint16 = 50
	gsEventShipLazySailor     uint16 = 51
	gsEventShipInformedSailor uint16 = 52
)

func (*Profile) DecodeFirstShipVoyage(reader game.MemoryReader) FirstShipVoyageState {
	if reader == nil {
		return FirstShipVoyageState{}
	}
	return FirstShipVoyageState{
		LeagueWon:       hasGSEvent(reader, gsEventBeatEliteFour),
		HasTicket:       gsOwnsKeyItem(reader, gsSSTicket),
		FirstTrip:       hasGSEvent(reader, gsEventFirstShipTrip),
		Arrived:         hasGSEvent(reader, gsEventShipArrived),
		FoundGirl:       hasGSEvent(reader, gsEventShipFoundGirl),
		InformedSailor:  hasGSEvent(reader, gsEventShipInformedSailor),
		RecoveredSailor: hasGSEvent(reader, gsEventShipLazySailor),
	}
}
