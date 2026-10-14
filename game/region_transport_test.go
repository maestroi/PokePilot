package game

import "testing"

func TestAssessRegionTransportsKeepsDirectionalBlockages(t *testing.T) {
	outbound := RegionTransport{Service: "boat", FromRegion: RegionJohto, ToRegion: RegionKanto,
		Leg: Transition{ID: "boat-east", From: "j-port", To: "k-port",
			Requires: []CapabilityID{"ticket", "wednesday"}, OneWay: true},
		DepartureDays: []Weekday{Wednesday}}
	returnTrip := RegionTransport{Service: "boat", FromRegion: RegionKanto, ToRegion: RegionJohto,
		Leg: Transition{ID: "boat-west", From: "k-port", To: "j-port",
			Requires: []CapabilityID{"ticket", "sunday"}}}
	options := AssessRegionTransports([]RegionTransport{outbound, returnTrip}, NewCapabilitySet("ticket", "wednesday"))
	forward := RegionRoutes(options, RegionJohto, RegionKanto)
	if len(forward) != 1 || !forward[0].Available || !forward[0].OneWay || forward[0].From != "j-port" {
		t.Fatalf("forward = %+v", forward)
	}
	backward := RegionRoutes(options, RegionKanto, RegionJohto)
	if len(backward) != 1 || backward[0].Available || len(backward[0].Missing) != 1 || backward[0].Missing[0] != "sunday" {
		t.Fatalf("backward = %+v", backward)
	}
	if len(RegionRoutes(options, RegionJohto, RegionJohto)) != 0 {
		t.Fatal("a crossing must not count as an intra-region path")
	}
	if got := AssessRegionTransports(nil, nil); got != nil {
		t.Fatalf("clockless transport catalog = %+v, want nil", got)
	}
}
