package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func TestFirstShipVoyageRequiresCartridgeEventsAndUsableTicket(t *testing.T) {
	for _, profile := range []*Profile{NewGold(), NewSilver()} {
		t.Run(string(profile.ID()), func(t *testing.T) {
			mem := fakeGSReader{}
			mem[sym.NumKeyItems] = 1
			mem[sym.KeyItems] = gsSSTicket
			put := func(event uint16) {
				mem[sym.EventFlags+event/8] |= 1 << uint(event%8)
			}
			read := func() FirstShipVoyageState { return profile.DecodeFirstShipVoyage(mem) }
			if read().FirstTrip || read().Arrived || read().FoundGirl {
				t.Fatal("a ticket alone cannot prove boarding or landing")
			}
			if !profile.DecodeFirstBadgeProgress(mem).Has(ProgressSSTicketReceived) {
				t.Fatal("ticket in the key-item pocket should be usable")
			}
			if profile.DecodeFirstBadgeProgress(mem).Has(ProgressKantoFirstLanding) {
				t.Fatal("ticket is not proof of a Kanto landing")
			}
			put(gsEventBeatEliteFour)
			put(gsEventShipInformedSailor)
			put(gsEventShipLazySailor)
			put(gsEventShipFoundGirl)
			put(gsEventShipArrived)
			if !read().LeagueWon || !read().InformedSailor || !read().RecoveredSailor || !read().FoundGirl || !read().Arrived {
				t.Fatalf("voyage stage flags were not decoded: %+v", read())
			}
			if profile.DecodeFirstBadgeProgress(mem).Has(ProgressKantoFirstLanding) {
				t.Fatal("ship arrival while still aboard is not disembarkation")
			}
			put(gsEventFirstShipTrip)
			if !read().FirstTrip || !profile.DecodeFirstBadgeProgress(mem).Has(ProgressKantoFirstLanding) {
				t.Fatal("Vermilion port exit must prove the first Kanto landing")
			}
			mem[sym.NumKeyItems] = 0
			if read().HasTicket || profile.DecodeFirstBadgeProgress(mem).Has(ProgressSSTicketReceived) {
				t.Fatal("receipt event alone is not a usable S.S. Ticket")
			}
			if got := profile.DecodeFirstShipVoyage(nil); got != (FirstShipVoyageState{}) {
				t.Fatalf("nil memory invented voyage state: %+v", got)
			}
		})
	}
}
