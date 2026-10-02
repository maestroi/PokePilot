package main

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestSpectatorMarshalAdvertisesPublicCapabilities(t *testing.T) {
	tests := []struct {
		game         string
		wantWorldMap bool
	}{
		{game: "pokemon-red", wantWorldMap: true},
		{game: "pokemon-yellow", wantWorldMap: false},
		{game: "pokemon-gold", wantWorldMap: true},
		{game: "pokemon-silver", wantWorldMap: true},
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

func TestSpectatorMarshalSanitizesGen2SemanticMap(t *testing.T) {
	run := spectatorRun{
		RunID: "gold", Status: "running", Game: "pokemon-gold",
		NativeMap: 0x1807, X: 3, Y: 4,
		Sprites: []farm.MapSprite{{X: 4, Y: 4, PictureID: 0xaa, Slot: 2}},
		MapAsset: &farm.SemanticMapAsset{
			ID: 0x1807, Width: 3, Height: 2, Cells: ".#W...",
			Warps:       []farm.SemanticMapWarp{{X: 2, Y: 0, Dest: 0x1808}},
			Connections: []string{"1808"},
		},
	}
	body, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		NativeMap uint16 `json:"native_map"`
		MapAsset  *struct {
			ID    uint16 `json:"id"`
			Cells string `json:"cells"`
			Warps []struct {
				Dest uint16 `json:"dest"`
			} `json:"warps"`
		} `json:"map_asset"`
		Sprites []map[string]any `json:"sprites"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.NativeMap != 0x1807 || got.MapAsset == nil || got.MapAsset.ID != 0x1807 || got.MapAsset.Cells != ".#W..." {
		t.Fatalf("public semantic map = %+v", got)
	}
	if len(got.MapAsset.Warps) != 1 || got.MapAsset.Warps[0].Dest != 0x1808 {
		t.Fatalf("public warps = %+v", got.MapAsset.Warps)
	}
	if len(got.Sprites) != 1 {
		t.Fatalf("public sprites = %+v", got.Sprites)
	}
	if _, leaked := got.Sprites[0]["picture_id"]; leaked {
		t.Fatalf("private sprite picture id crossed public boundary: %s", body)
	}

	run.MapAsset.ID = 0x1808
	body, err = json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal mismatched asset: %v", err)
	}
	var mismatched map[string]json.RawMessage
	if err := json.Unmarshal(body, &mismatched); err != nil {
		t.Fatalf("decode mismatch: %v", err)
	}
	if _, exists := mismatched["map_asset"]; exists {
		t.Fatalf("mismatched map asset must be omitted: %s", body)
	}
}
