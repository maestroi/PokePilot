package renderstate

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func replayTestState(frame uint64, x int, tile TileKind) RenderState {
	return RenderState{
		SchemaVersion: SchemaVersion,
		Game:          GameRef{ID: game.GameID("pokemon-red"), Revision: game.RevisionID("test")},
		Clock:         Clock{Frame: frame},
		Scene:         SceneOverworld,
		Capabilities:  []Capability{CapabilityMap, CapabilityPlayer, CapabilityLayers},
		Map:           &MapState{ID: game.PlaceID("route-1"), Name: "Route 1", Width: 2, Height: 1},
		Player:        &ActorState{ID: "player", Kind: EntityPlayer, Position: Position{X: x, Y: 0}},
		Layers: []TileLayer{{
			ID: "terrain", Kind: LayerTerrain, Width: 2, Height: 1,
			Cells: []TileCell{{Kind: tile}, {Kind: TilePath}},
		}},
	}
}

func TestReplayTimelineDeduplicatesLayersAndSeeks(t *testing.T) {
	builder := NewReplayTimelineBuilder("run-1", 59.7275)
	if err := builder.Append(0, 1, replayTestState(10, 0, TileGrass)); err != nil {
		t.Fatal(err)
	}
	if err := builder.Append(100, 1, replayTestState(16, 1, TileGrass)); err != nil {
		t.Fatal(err)
	}
	if err := builder.Append(200, 1, replayTestState(22, 1, TileWater)); err != nil {
		t.Fatal(err)
	}
	timeline, err := builder.Build(250)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(timeline.LayerSets); got != 2 {
		t.Fatalf("layer sets = %d, want 2", got)
	}
	if timeline.Samples[0].LayerSet != timeline.Samples[1].LayerSet {
		t.Fatalf("unchanged layer grids should share one set: %+v", timeline.Samples)
	}
	if timeline.Samples[2].LayerSet == timeline.Samples[1].LayerSet {
		t.Fatalf("changed layer grid reused old set: %+v", timeline.Samples)
	}
	if len(timeline.Samples[0].State.Layers) != 0 {
		t.Fatal("sample retained inline layers")
	}

	state, ok := timeline.StateAtMS(150)
	if !ok {
		t.Fatal("StateAtMS returned no state")
	}
	if state.Clock.Frame != 16 || state.Player == nil || state.Player.Position.X != 1 {
		t.Fatalf("seek state = %+v", state)
	}
	if len(state.Layers) != 1 || state.Layers[0].Cells[0].Kind != TileGrass {
		t.Fatalf("seek layers = %+v", state.Layers)
	}
}

func TestReplayTimelineSeekClampsAndValidationRejectsBadLayerReference(t *testing.T) {
	builder := NewReplayTimelineBuilder("run-2", 60)
	if err := builder.Append(50, 2, replayTestState(100, 1, TilePath)); err != nil {
		t.Fatal(err)
	}
	timeline, err := builder.Build(100)
	if err != nil {
		t.Fatal(err)
	}
	before, ok := timeline.StateAtMS(0)
	if !ok || before.Clock.Frame != 100 {
		t.Fatalf("seek before first = %+v, %v", before, ok)
	}
	after, ok := timeline.StateAtMS(999)
	if !ok || after.Clock.Frame != 100 {
		t.Fatalf("seek after last = %+v, %v", after, ok)
	}

	timeline.Samples[0].LayerSet = 99
	if err := timeline.Validate(); err == nil {
		t.Fatal("invalid layer reference was accepted")
	}
}
