package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestMCPStartRunCarriesRunPolicy(t *testing.T) {
	var queued farm.Spec
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		json.NewDecoder(req.Body).Decode(&queued)                          //nolint:errcheck
		json.NewEncoder(res).Encode(map[string]string{"status": "queued"}) //nolint:errcheck
	}))
	defer wall.Close()
	control := &mcpControl{wallBase: wall.URL, http: wall.Client()}

	if _, _, err := control.startRun(context.Background(), nil, mcpStartRunInput{
		Game: "pokemon-red", PlayStyle: "Completionist", Purpose: "full_e2e", RiskTolerance: "cautious",
		WildEncounters: "fight", LLMDeployment: "gpu-a", RecoveryProfile: "resilient",
	}); err != nil {
		t.Fatal(err)
	}
	if queued.PlayStyle != "completionist" || queued.Purpose != "full_e2e" || queued.RiskTolerance != "cautious" ||
		queued.WildEncounters != "fight" || queued.LLMDeployment != "gpu-a" || queued.RecoveryProfile != "resilient" {
		t.Fatalf("queued spec = %+v", queued)
	}

	// The runner would silently play a typo as speedrun.
	queued = farm.Spec{}
	if _, _, err := control.startRun(context.Background(), nil, mcpStartRunInput{Game: "pokemon-red", PlayStyle: "dex"}); err == nil || queued.RunID != "" {
		t.Fatalf("unknown play_style err = %v, queued %q", err, queued.RunID)
	}
}
