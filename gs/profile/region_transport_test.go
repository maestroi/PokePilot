package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func gsSetTransitEvent(mem *fakeMemory, event uint16) {
	mem[sym.EventFlags+event/8] |= 1 << uint(event%8)
}

func gsWithTransportKeys(mem *fakeMemory) {
	mem[sym.NumKeyItems] = 2
	mem[sym.KeyItems] = gsSSTicket
	mem[sym.KeyItems+1] = gsRailPass
}

func gsTransport(t *testing.T, options []game.RegionTransportOption, service string, from game.RegionID) game.RegionTransportOption {
	t.Helper()
	for _, option := range options {
		if option.Service == service && option.FromRegion == from {
			return option
		}
	}
	t.Fatalf("missing %s from %s in %+v", service, from, options)
	return game.RegionTransportOption{}
}

func gsHasMissing(option game.RegionTransportOption, cap game.CapabilityID) bool {
	for _, id := range option.Missing {
		if id == cap {
			return true
		}
	}
	return false
}

func TestGoldRegionTransportFirstVoyageAndSchedules(t *testing.T) {
	var mem fakeMemory
	profile := NewGold()
	mem[sym.CurDay] = 2 // Tuesday: neither repeat departure day.
	gsSetTransitEvent(&mem, gsEventBeatEliteFour)
	gsWithTransportKeys(&mem)

	first := profile.DecodeRegionTransports(&mem)
	boatEast := gsTransport(t, first, "ss_aqua", game.RegionJohto)
	boatWest := gsTransport(t, first, "ss_aqua", game.RegionKanto)
	if !boatEast.Available || !boatEast.AnyDay || !boatEast.OneWay {
		t.Fatalf("first Olivine sailing = %+v, want one-way boarding today", boatEast)
	}
	if boatWest.Available || !gsHasMissing(boatWest, gsCapFirstShipTrip) || !gsHasMissing(boatWest, gsCapShipVermilionDay) {
		t.Fatalf("before first trip return = %+v, want first-trip and weekday block", boatWest)
	}
	gsSetTransitEvent(&mem, gsEventFirstShipTrip)
	after := profile.DecodeRegionTransports(&mem)
	if east := gsTransport(t, after, "ss_aqua", game.RegionJohto); east.Available || !gsHasMissing(east, gsCapShipOlivineDay) || east.OneWay {
		t.Fatalf("repeat eastbound on Tuesday = %+v, want blocked", east)
	}

	for _, tc := range []struct {
		day        byte
		east, west bool
	}{
		{0, false, true}, // Sunday: Vermilion only
		{1, true, false}, // Monday: Olivine only
		{3, false, true}, // Wednesday: Vermilion only
		{5, true, false}, // Friday: Olivine only
		{6, false, false},
		{8, true, false}, // wCurDay modulo 7
	} {
		mem[sym.CurDay] = tc.day
		options := profile.DecodeRegionTransports(&mem)
		east := gsTransport(t, options, "ss_aqua", game.RegionJohto)
		west := gsTransport(t, options, "ss_aqua", game.RegionKanto)
		if east.Available != tc.east || west.Available != tc.west {
			t.Fatalf("day %d: east=%+v west=%+v", tc.day, east, west)
		}
	}
}

func TestSilverRegionTransportTicketPowerAndRedCompatibility(t *testing.T) {
	var mem fakeMemory
	profile := NewSilver()
	mem[sym.CurDay] = 3
	gsSetTransitEvent(&mem, gsEventFirstShipTrip)
	options := profile.DecodeRegionTransports(&mem)
	for _, region := range []game.RegionID{game.RegionJohto, game.RegionKanto} {
		train := gsTransport(t, options, "magnet_train", region)
		if train.Available || !gsHasMissing(train, gsCapKantoPower) || !gsHasMissing(train, gsCapRailPass) {
			t.Fatalf("unpowered/un-ticketed train from %s = %+v", region, train)
		}
	}
	gsWithTransportKeys(&mem)
	gsSetTransitEvent(&mem, gsEventRestoredKantoPower)
	options = profile.DecodeRegionTransports(&mem)
	for _, region := range []game.RegionID{game.RegionJohto, game.RegionKanto} {
		train := gsTransport(t, options, "magnet_train", region)
		if !train.Available || !train.AnyDay || train.OneWay {
			t.Fatalf("restored train from %s = %+v", region, train)
		}
	}
	if boat := gsTransport(t, options, "ss_aqua", game.RegionJohto); boat.Available || !gsHasMissing(boat, gsCapBeatLeague) {
		t.Fatalf("S.S. Aqua before League = %+v", boat)
	}
	if got := profile.DecodeRegionTransports(nil); got != nil {
		t.Fatalf("nil memory must not invent routes: %+v", got)
	}
}
