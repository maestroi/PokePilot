package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestRegionTransportObservationCarriesStructuredPrerequisites(t *testing.T) {
	obs := Observation{
		Region: "johto",
		RegionTransports: []game.RegionTransportOption{{
			Service: "magnet_train", FromRegion: game.RegionJohto, ToRegion: game.RegionKanto,
			From: "goldenrod", To: "saffron",
			Missing: []game.CapabilityID{"power_restored", "rail_pass"},
		}},
	}
	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"region_transports"`,`"service":"magnet_train"`,`"missing":["power_restored","rail_pass"]`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("observation %s missing %s", data, want)
		}
	}
	without := Observation{Region: "kanto"}
	data, err = json.Marshal(without)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "region_transports") {
		t.Fatalf("clockless or unsupported adapter invented region transport: %s", data)
	}
}
