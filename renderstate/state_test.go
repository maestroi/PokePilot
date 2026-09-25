package renderstate

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestFromProfileObservationBuildsRepresentativeOverworldState(t *testing.T) {
	state := FromProfileObservation(FrameMeta{
		GameID:           game.GameID("pokemon-red"),
		Revision:         game.RevisionID("en-us-rev0"),
		Frame:            1234,
		Cycle:            5678,
		CapturedAtUnixMS: 1_800_000_000_000,
	}, game.ProfileObservation{
		Location:     "pallet town",
		MapName:      "PALLET_TOWN",
		X:            5,
		Y:            6,
		Facing:       "down",
		Controllable: true,
	})

	if err := Validate(state); err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != SchemaVersion {
		t.Fatalf("schema=%d want=%d", state.SchemaVersion, SchemaVersion)
	}
	if state.Scene != SceneOverworld {
		t.Fatalf("scene=%q want=%q", state.Scene, SceneOverworld)
	}
	if state.Game.ID != "pokemon-red" || state.Game.Revision != "en-us-rev0" {
		t.Fatalf("game=%+v", state.Game)
	}
	if state.Clock.Frame != 1234 || state.Clock.Cycle != 5678 {
		t.Fatalf("clock=%+v", state.Clock)
	}
	if state.Map == nil || state.Map.ID != "pallet town" || state.Map.Name != "PALLET_TOWN" {
		t.Fatalf("map=%+v", state.Map)
	}
	if state.Player == nil || state.Player.Kind != EntityPlayer || state.Player.Position != (Position{X: 5, Y: 6}) || state.Player.Facing != DirectionDown {
		t.Fatalf("player=%+v", state.Player)
	}
	if !state.HasCapability(CapabilityMap) || !state.HasCapability(CapabilityPlayer) {
		t.Fatalf("capabilities=%v", state.Capabilities)
	}
	if state.Battle != nil {
		t.Fatalf("unexpected battle=%+v", state.Battle)
	}
}

func TestFromProfileObservationMarksBattleWithoutInventingBattleDetails(t *testing.T) {
	state := FromProfileObservation(FrameMeta{
		GameID:   "pokemon-red",
		Revision: "en-us-rev0",
	}, game.ProfileObservation{InBattle: true})

	if state.Scene != SceneBattle || state.Battle == nil {
		t.Fatalf("scene=%q battle=%+v", state.Scene, state.Battle)
	}
	if !state.HasCapability(CapabilityBattle) {
		t.Fatalf("capabilities=%v", state.Capabilities)
	}
	if len(state.Battle.Actors) != 0 || state.Battle.Phase != "" {
		t.Fatalf("baseline observation invented battle detail: %+v", state.Battle)
	}
}

type fakeProfile struct{}

func (fakeProfile) ID() game.GameID           { return "pokemon-red" }
func (fakeProfile) Revision() game.RevisionID { return "en-us-rev0" }
func (fakeProfile) DecodeObservation(game.MemoryReader, []byte) (game.ProfileObservation, error) {
	return game.ProfileObservation{Location: "pallet town", MapName: "PALLET_TOWN", X: 4, Y: 9, Facing: "up"}, nil
}

func TestFromProfileUsesAdapterIdentityAndSemanticObservation(t *testing.T) {
	state, err := FromProfile(fakeProfile{}, nil, nil, FrameMeta{
		GameID:   "wrong-game",
		Revision: "wrong-revision",
		Frame:    77,
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.Game.ID != "pokemon-red" || state.Game.Revision != "en-us-rev0" {
		t.Fatalf("game=%+v", state.Game)
	}
	if state.Map == nil || state.Map.ID != "pallet town" || state.Player == nil || state.Player.Position != (Position{X: 4, Y: 9}) {
		t.Fatalf("state=%+v", state)
	}
}

func TestJSONRoundTripPreservesSchemaAndSemanticLayers(t *testing.T) {
	state := RenderState{
		SchemaVersion: SchemaVersion,
		Game:          GameRef{ID: "pokemon-red", Revision: "en-us-rev0"},
		Clock:         Clock{Frame: 99, Cycle: 1200},
		Scene:         SceneOverworld,
		Capabilities:  []Capability{CapabilityMap, CapabilityPlayer, CapabilityLayers, CapabilityEntities},
		Map:           &MapState{ID: "viridian city", Name: "VIRIDIAN_CITY", Width: 2, Height: 1},
		Player:        &ActorState{ID: "player", Kind: EntityPlayer, Position: Position{X: 1, Y: 0}, Facing: DirectionLeft},
		Entities:      []ActorState{{ID: "npc:1", Kind: EntityNPC, Position: Position{X: 0, Y: 0}, Facing: DirectionRight}},
		Layers: []TileLayer{{
			ID:     "ground",
			Kind:   LayerTerrain,
			Origin: Position{},
			Width:  2,
			Height: 1,
			Cells:  []TileCell{{Kind: TilePath}, {Kind: TileGrass, Variant: "tall"}},
		}},
	}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, state); err != nil {
		t.Fatal(err)
	}
	got, err := ReadJSON(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != SchemaVersion || got.Scene != SceneOverworld || got.Clock.Frame != 99 {
		t.Fatalf("got=%+v", got)
	}
	if len(got.Layers) != 1 || len(got.Layers[0].Cells) != 2 || got.Layers[0].Cells[1].Kind != TileGrass {
		t.Fatalf("layers=%+v", got.Layers)
	}
	if len(got.Entities) != 1 || got.Entities[0].ID != "npc:1" {
		t.Fatalf("entities=%+v", got.Entities)
	}
}

func TestUnknownAdditiveFieldsAndSemanticValuesDegradeGracefully(t *testing.T) {
	raw := `{
		"schema_version":1,
		"game":{"id":"future-game","revision":"rev0"},
		"clock":{"frame":42,"future_clock_field":7},
		"scene":"cinematic",
		"capabilities":["map","weather"],
		"map":{"id":"future place","future_map_field":true},
		"entities":[{"kind":"vehicle","position":{"x":3,"y":4},"future_entity_field":"ok"}],
		"future_top_level":{"anything":true}
	}`

	state, err := ReadJSON(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if state.Scene != Scene("cinematic") {
		t.Fatalf("scene=%q", state.Scene)
	}
	if len(state.Capabilities) != 2 || state.Capabilities[1] != Capability("weather") {
		t.Fatalf("capabilities=%v", state.Capabilities)
	}
	if len(state.Entities) != 1 || state.Entities[0].Kind != EntityKind("vehicle") {
		t.Fatalf("entities=%+v", state.Entities)
	}
}

func TestSchemaV1RequiredWireKeysRemainStable(t *testing.T) {
	state := FromProfileObservation(FrameMeta{GameID: "pokemon-red", Revision: "en-us-rev0", Frame: 7}, game.ProfileObservation{
		Location: "pallet town",
		MapName:  "PALLET_TOWN",
		X:        5,
		Y:        6,
		Facing:   "down",
	})
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema_version", "game", "clock", "scene", "map", "player"} {
		if _, ok := wire[key]; !ok {
			t.Fatalf("schema v1 missing required key %q: %s", key, b)
		}
	}
	if bytes.Contains(b, []byte("NativeMapID")) || bytes.Contains(b, []byte("native_map")) {
		t.Fatalf("native map identity leaked onto render wire: %s", b)
	}
}

func TestValidateRejectsBrokenLayerShapeAndSchema(t *testing.T) {
	base := RenderState{
		SchemaVersion: SchemaVersion,
		Game:          GameRef{ID: "pokemon-red", Revision: "en-us-rev0"},
		Scene:         SceneOverworld,
	}

	broken := base
	broken.Layers = []TileLayer{{ID: "ground", Kind: LayerTerrain, Width: 2, Height: 2, Cells: make([]TileCell, 3)}}
	if err := Validate(broken); err == nil || !strings.Contains(err.Error(), "has 3 cells, want 4") {
		t.Fatalf("layer validation err=%v", err)
	}

	future := base
	future.SchemaVersion = SchemaVersion + 1
	if err := Validate(future); err == nil || !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("schema validation err=%v", err)
	}
}
