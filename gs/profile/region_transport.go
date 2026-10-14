package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

// Cartridge-owned item/event ids from pret/pokegold at gs/data.SourceRevision.
// The two boarding scripts call checkitem; possession in the key-item pocket
// matters, not a similarly named item stored elsewhere.
const (
	gsSSTicket uint8 = 0x44
	gsRailPass uint8 = 0x86

	gsEventBeatEliteFour      uint16 = 68
	gsEventFirstShipTrip      uint16 = 48
	gsEventRestoredKantoPower uint16 = 205
)

const (
	gsCapBeatLeague       game.CapabilityID = "gs_beat_elite_four"
	gsCapSSTicket         game.CapabilityID = "gs_s_s_ticket_in_bag"
	gsCapShipOlivineDay   game.CapabilityID = "gs_ss_aqua_olivine_departure"
	gsCapShipVermilionDay game.CapabilityID = "gs_ss_aqua_vermilion_departure"
	gsCapFirstShipTrip    game.CapabilityID = "gs_ss_aqua_first_trip_complete"
	gsCapRailPass         game.CapabilityID = "gs_magnet_train_pass_in_bag"
	gsCapKantoPower       game.CapabilityID = "gs_kanto_power_restored"
)

func gsOwnsKeyItem(reader game.MemoryReader, item uint8) bool {
	count := int(reader.Peek8(sym.NumKeyItems))
	if count > sym.MaxKeyItems {
		count = sym.MaxKeyItems
	}
	for i := 0; i < count; i++ {
		if reader.Peek8(sym.KeyItems+uint16(i)) == item {
			return true
		}
	}
	return false
}

var gsWeekdays = [...]game.Weekday{
	game.Sunday, game.Monday, game.Tuesday, game.Wednesday,
	game.Thursday, game.Friday, game.Saturday,
}

// From GetWeekday, which reduces wCurDay modulo seven (Sunday = 0).
func gsWeekday(reader game.MemoryReader) game.Weekday {
	return gsWeekdays[int(reader.Peek8(sym.CurDay))%len(gsWeekdays)]
}

func gsDepartureDay(day game.Weekday, allowed []game.Weekday) bool {
	for _, allowedDay := range allowed {
		if day == allowedDay {
			return true
		}
	}
	return false
}

// DecodeRegionTransports projects the scripts in OlivinePort,
// VermilionPort, GoldenrodMagnetTrainStation and SaffronMagnetTrainStation.
// The initial S.S. Aqua crossing from Olivine is permitted any weekday after
// the League; subsequent Olivine sailings run Monday/Friday, while Vermilion
// sails Wednesday/Sunday. The rail service is daily once power is restored.
func (*Profile) DecodeRegionTransports(reader game.MemoryReader) []game.RegionTransportOption {
	if reader == nil {
		return nil
	}
	firstTrip := hasGSEvent(reader, gsEventFirstShipTrip)
	day := gsWeekday(reader)
	olivineDays := []game.Weekday{game.Monday, game.Friday}
	vermilionDays := []game.Weekday{game.Wednesday, game.Sunday}
	caps := game.NewCapabilitySet()
	if hasGSEvent(reader, gsEventBeatEliteFour) {
		caps[gsCapBeatLeague] = true
	}
	if firstTrip {
		caps[gsCapFirstShipTrip] = true
	}
	if gsOwnsKeyItem(reader, gsSSTicket) {
		caps[gsCapSSTicket] = true
	}
	if gsOwnsKeyItem(reader, gsRailPass) {
		caps[gsCapRailPass] = true
	}
	if hasGSEvent(reader, gsEventRestoredKantoPower) {
		caps[gsCapKantoPower] = true
	}
	if !firstTrip || gsDepartureDay(day, olivineDays) {
		caps[gsCapShipOlivineDay] = true
	}
	if gsDepartureDay(day, vermilionDays) {
		caps[gsCapShipVermilionDay] = true
	}

	location := func(name string) game.PlaceID {
		info, ok := gsdata.MapByName(name)
		if !ok {
			return ""
		}
		return info.Location
	}
	olivine, vermilion := location("OLIVINE_PORT"), location("VERMILION_PORT")
	goldenrod, saffron := location("GOLDENROD_MAGNET_TRAIN_STATION"), location("SAFFRON_MAGNET_TRAIN_STATION")
	if olivine == "" || vermilion == "" || goldenrod == "" || saffron == "" {
		// Fail closed if the generated native map catalog does not cover a port.
		return nil
	}
	ship := func(from, to game.PlaceID, regionFrom, regionTo game.RegionID, extra []game.CapabilityID, days []game.Weekday, anyDay, oneWay bool) game.RegionTransport {
		requires := append([]game.CapabilityID{gsCapBeatLeague, gsCapSSTicket}, extra...)
		return game.RegionTransport{
			Service: "ss_aqua", FromRegion: regionFrom, ToRegion: regionTo,
			Leg: game.Transition{
				ID: "ss_aqua", From: from, To: to, Requires: requires,
				OneWay: oneWay, ChoiceRequired: true,
			},
			DepartureDays: days, AnyDay: anyDay,
		}
	}
	train := func(from, to game.PlaceID, regionFrom, regionTo game.RegionID) game.RegionTransport {
		return game.RegionTransport{
			Service: "magnet_train", FromRegion: regionFrom, ToRegion: regionTo,
			Leg: game.Transition{
				ID: "magnet_train", From: from, To: to,
				Requires:       []game.CapabilityID{gsCapKantoPower, gsCapRailPass},
				ChoiceRequired: true,
			},
			AnyDay: true,
		}
	}
	routes := []game.RegionTransport{
		ship(olivine, vermilion, game.RegionJohto, game.RegionKanto,
			[]game.CapabilityID{gsCapShipOlivineDay}, olivineDays, !firstTrip, !firstTrip),
		ship(vermilion, olivine, game.RegionKanto, game.RegionJohto,
			[]game.CapabilityID{gsCapFirstShipTrip, gsCapShipVermilionDay}, vermilionDays, false, false),
		train(goldenrod, saffron, game.RegionJohto, game.RegionKanto),
		train(saffron, goldenrod, game.RegionKanto, game.RegionJohto),
	}
	return game.AssessRegionTransports(routes, caps)
}

var _ game.RegionTransportProvider = (*Profile)(nil)
