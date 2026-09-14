package agent

import "testing"

func TestJourneyProgressionBlockedRoute25UntilBillTicket(t *testing.T) {
	obs := Observation{}
	if !journeyProgressionBlocked(obs, route25Map) {
		t.Fatal("Route 25 should stay behind Bill progression before the S.S. Ticket")
	}

	obs.Story = ProgressState{{ID: redProgressSSTicketAcquired, Complete: true}}
	if journeyProgressionBlocked(obs, route25Map) {
		t.Fatal("Route 25 should open to generic travel after Bill awards the S.S. Ticket")
	}
}

func TestOfferLeavesPreBillRoute25ToBillProgression(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{
		0x23: {route25Map},
		route25Map: {0x23},
	})
	known.SawMap(0x23)
	obs := Observation{
		Map:        0x23,
		MapName:    "ROUTE_24",
		X:          10,
		Y:          14,
		PartyCount: 1,
		Party:      []PartyMon{{Level: 18, HP: 50, MaxHP: 50}},
	}

	for _, objective := range Offer(obs, known) {
		if objective.Kind == KindGoTo && objective.Place == "route 25" {
			t.Fatalf("pre-Bill generic Route 25 journey leaked into Offer: %+v", objective)
		}
	}

	billOffered := false
	for _, objective := range redProgressionObjectives(obs) {
		if objective.Kind == KindProgress && objective.Progress == redProgressSSTicketAcquired {
			billOffered = true
			break
		}
	}
	if !billOffered {
		t.Fatal("pre-Bill Route 24 must keep the owned Bill/S.S.-Ticket progression objective available")
	}

	obs.Story = ProgressState{{ID: redProgressSSTicketAcquired, Complete: true}}
	plain, flee := false, false
	for _, objective := range Offer(obs, known) {
		if objective.Kind != KindGoTo || objective.Place != "route 25" {
			continue
		}
		if objective.Flee {
			flee = true
		} else {
			plain = true
		}
	}
	if !plain || !flee {
		t.Fatalf("post-Bill Route 25 = plain:%v flee:%v, want both generic journeys restored", plain, flee)
	}
}
