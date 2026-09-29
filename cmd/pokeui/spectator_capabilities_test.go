package main

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestSpectatorMarshalAdvertisesPublicCapabilities(t *testing.T) {
	tests := []struct {
		game         string
		wantWorldMap bool
	}{
		{game: "pokemon-red", wantWorldMap: true},
		{game: "pokemon-yellow", wantWorldMap: false},
		{game: "pokemon-gold", wantWorldMap: false},
		{game: "tetris", wantWorldMap: false},
		{game: "future-game", wantWorldMap: false},
	}

	for _, tt := range tests {
		t.Run(tt.game, func(t *testing.T) {
			body, err := json.Marshal(spectatorRun{RunID: "run", Status: "running", Game: tt.game})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got struct {
				Capabilities []string `json:"public_capabilities"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !slices.Contains(got.Capabilities, "live") || !slices.Contains(got.Capabilities, "replay") {
				t.Fatalf("capabilities = %v, want live + replay", got.Capabilities)
			}
			if slices.Contains(got.Capabilities, "worldMap") != tt.wantWorldMap {
				t.Fatalf("worldMap in %v = %v, want %v", got.Capabilities, slices.Contains(got.Capabilities, "worldMap"), tt.wantWorldMap)
			}
		})
	}
}
