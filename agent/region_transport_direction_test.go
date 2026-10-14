package agent

import (
	"encoding/json"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestRegionTravelWireOnlyShowsCurrentRegionDepartures(t *testing.T) {
	options := []game.RegionTransportOption{
		{Service: "ship", FromRegion: game.RegionJohto, ToRegion: game.RegionKanto,
			From: "olivine", To: "vermilion", Available: true, OneWay: true},
		{Service: "ship", FromRegion: game.RegionKanto, ToRegion: game.RegionJohto,
			From: "vermilion", To: "olivine", Missing: []game.CapabilityID{"departure_day"}},
	}
	obs := Observation{Region: string(game.RegionJohto), RegionTransports: options}
	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		RegionTransports []game.RegionTransportOption `json:"region_transports"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.RegionTransports) != 1 || wire.RegionTransports[0].From != "olivine" {
		t.Fatalf("planner saw unavailable reverse leg: %+v", wire.RegionTransports)
	}
	// Filtering the prompt must not modify the full runtime catalog.
	if len(obs.RegionTransports) != 2 {
		t.Fatalf("runtime lost reverse journey: %+v", obs.RegionTransports)
	}
	obs.Region = string(game.RegionKanto)
	data, err = json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.RegionTransports) != 1 || wire.RegionTransports[0].From != "vermilion" ||
		len(wire.RegionTransports[0].Missing) != 1 {
		t.Fatalf("Kanto departure blocked on sailing day: %+v", wire.RegionTransports)
	}
	obs.Region = ""
	data, err = json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	// Unmarshal does not zero absent optional fields on an existing struct.
	// Decode into a fresh wire value before testing the omitted-field case.
	wire.RegionTransports = nil
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.RegionTransports) != 0 {
		t.Fatalf("unknown current region must not offer travel: %+v", wire.RegionTransports)
	}
}
