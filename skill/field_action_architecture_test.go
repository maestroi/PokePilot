package skill

import (
	"os"
	"strings"
	"testing"
)

func TestGenericFieldActionRuntimeHasNoConcreteGameDependencies(t *testing.T) {
	src, err := os.ReadFile("field_action_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"github.com/maestroi/pokepilot/red/",
		"github.com/maestroi/pokepilot/blue/",
		"github.com/maestroi/pokepilot/yellow/",
		"github.com/maestroi/pokepilot/gs/",
	} {
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("field_action_runtime.go imports concrete game package %q", forbidden)
		}
	}
}

func TestFieldActionExecutionDoesNotDecodeConcreteEffectFlags(t *testing.T) {
	src, err := os.ReadFile("field_action.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"mem.U8(sym.TileInFrontOfPlayer)",
		"mem.U8(sym.WalkBikeSurfState)",
		"mem.U8(sym.StatusFlags1)",
		"mem.U8(sym.MapPalOffset)",
		"mem.U8(sym.ActionResult)",
	} {
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("field_action.go contains concrete runtime-effect read %q", forbidden)
		}
	}
}

func TestFieldPathDoesNotReadConcreteSurfMode(t *testing.T) {
	src, err := os.ReadFile("field_path.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"sym.WalkBikeSurfState",
		"fieldSurfingState",
		"observeFrontTile(",
		"cuttableFrontTile(",
	} {
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("field_path.go contains concrete field-action state dependency %q", forbidden)
		}
	}
}

func TestSurfStrengthConsumersDoNotReadConcreteModeFlags(t *testing.T) {
	for _, path := range []string{
		"water_catch.go",
		"warp.go",
		"route_transition.go",
		"side_route_semantics.go",
		"boulder_puzzle.go",
		"component_restage.go",
		"victory_road_progression.go",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"sym.WalkBikeSurfState",
			"fieldSurfingState",
			"fieldStrengthActiveBit",
			"sym.StatusFlags1",
		} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s contains concrete Surf/Strength runtime dependency %q", path, forbidden)
			}
		}
	}
}

func TestLiveMapGridRuntimeDoesNotReadConcreteSurfMode(t *testing.T) {
	src, err := os.ReadFile("live_topology.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "func liveMapGrid(m *emu.Emu")
	end := strings.Index(body, "// LiveMapGridFromMem")
	if start < 0 || end <= start {
		t.Fatal("liveMapGrid runtime block not found")
	}
	body = body[start:end]
	for _, forbidden := range []string{"sym.WalkBikeSurfState", "fieldSurfingState"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("liveMapGrid runtime contains concrete Surf dependency %q", forbidden)
		}
	}
}

func TestFieldMoveLaneDoesNotDecodeRedPrerequisitesOrMenuIDs(t *testing.T) {
	for _, path := range []string{"field_action.go", "field_move_runtime.go"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"sym.ObtainedBadges",
			"sym.FieldMoves",
			"sym.PartyMon1",
			"state.DecodeProgress(",
			"bagEntry(",
			"partyMoveSlot(",
			"fieldHM02Item",
			"fieldFlyMenuID",
		} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s contains Red field-move prerequisite/menu dependency %q", path, forbidden)
			}
		}
	}
}

func TestGenericFieldMoveProfileResolverHasNoConcreteGameImports(t *testing.T) {
	src, err := os.ReadFile("field_move_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"github.com/maestroi/pokepilot/red/",
		"github.com/maestroi/pokepilot/blue/",
		"github.com/maestroi/pokepilot/yellow/",
		"github.com/maestroi/pokepilot/gs/",
	} {
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("field_move_runtime.go imports concrete game package %q", forbidden)
		}
	}
}
