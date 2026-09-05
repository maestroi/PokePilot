package agent_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestObservationFieldCapabilitiesReachPlannerJSON(t *testing.T) {
	obs := agent.Observation{
		FieldCapabilities: []agent.FieldCapability{
			{
				Name:       "SURF",
				Badge:      "Soul",
				BadgeOwned: true,
				HMOwned:    true,
				Learned:    false,
				PartySlot:  -1,
				Usable:     false,
			},
		},
	}
	b, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	text := string(b)
	for _, want := range []string{`"FieldCapabilities"`, `"Name":"SURF"`, `"HMOwned":true`, `"Learned":false`, `"Usable":false`} {
		if !strings.Contains(text, want) {
			t.Fatalf("planner JSON %s does not contain %s", text, want)
		}
	}

	var roundTrip agent.Observation
	if err := json.Unmarshal(b, &roundTrip); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(roundTrip.FieldCapabilities) != 1 || roundTrip.FieldCapabilities[0].PartySlot != -1 {
		t.Fatalf("round-trip field capabilities = %+v", roundTrip.FieldCapabilities)
	}
}
