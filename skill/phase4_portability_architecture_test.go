package skill

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// phase4PortableFiles are the reusable #974 execution/runtime files. They are
// intentionally protected from every concrete game package: generation-specific
// decoding and mechanics belong in profiles/adapters, never back in these drivers.
var phase4PortableFiles = fileSet(
	"bag.go",
	"battle.go",
	"battle_escape_menu.go",
	"battle_execution.go",
	"battle_forced_choice_recovery.go",
	"battle_medicine_execution.go",
	"battle_menu.go",
	"battle_policy.go",
	"battle_resource_policy.go",
	"battle_resource_state.go",
	"battle_resources.go",
	"battle_runtime.go",
	"battle_settlement.go",
	"battle_state.go",
	"capture_runtime.go",
	"catch.go",
	"combat_strategy.go",
	"connection_field_path.go",
	"elevator.go",
	"field_action.go",
	"field_action_runtime.go",
	"field_item.go",
	"field_item_runtime.go",
	"field_move_runtime.go",
	"field_path_bridge.go",
	"flee.go",
	"goto.go",
	"heal.go",
	"heal_runtime.go",
	"heal_transaction.go",
	"intra_map_warp.go",
	"inventory.go",
	"list_menu.go",
	"live_topology.go",
	"menu.go",
	"move.go",
	"navigation_overworld.go",
	"overworld.go",
	"overworld_blackout.go",
	"party.go",
	"party_menu.go",
	"policy.go",
	"prompt.go",
	"repel.go",
	"routable.go",
	"routing_runtime.go",
	"shop.go",
	"start_menu.go",
	"switch_policy.go",
	"travel.go",
	"warp.go",
)

// gen1AdapterExceptions are explicit Gen-I adapters/compatibility controllers.
// They may interpret Red data because their job is to translate or execute a
// Gen-I-specific mechanism; generic Phase-4 drivers must not depend on them for
// cartridge layout knowledge.
var gen1AdapterExceptions = fileSet(
	"bag_gen1_compat.go",
	"bag_space.go",
	"battle_decision.go",
	"battle_resource_gen1_compat.go",
	"bicycle.go",
	"capture_storage.go",
	"catch_acquire.go",
	"catch_gen1_compat.go",
	"choice_rewards.go",
	"cut.go",
	"dialogue_recovery.go",
	"evolution_item.go",
	"field_item_gen1_compat.go",
	"field_move_red_compat.go",
	"field_path.go",
	"fishing.go",
	"flee_safari_gen1_compat.go",
	"gift_pokemon.go",
	"heal_gen1_compat.go",
	"in_game_trade.go",
	"interact.go",
	"interaction.go",
	"item_storage.go",
	"link_trade.go",
	"live_topology_gen1_compat.go",
	"move_learning.go",
	"party_gen1_compat.go",
	"pc.go",
	"pc_box_switch.go",
	"pickup.go",
	"policy_gen1_compat.go",
	"required_battle.go",
	"routable_gen1_compat.go",
	"shop_gen1_compat.go",
	"starter_request.go",
	"static_capture.go",
	"switch_policy_gen1_compat.go",
	"tmhm.go",
	"tmhm_slot.go",
	"trainer.go",
	"trainer_live_status.go",
	"utility_field_candidate.go",
	"water_catch.go",
)

// redOwnedStoryPolicyExceptions are deliberately Red-owned story, progression,
// recovery, and route-policy executors. #974 explicitly excludes this layer;
// Gen II gets its own story/progression implementation rather than sharing it.
var redOwnedStoryPolicyExceptions = fileSet(
	"bag_pressure.go",
	"bill.go",
	"boulder_progression.go",
	"boulder_puzzle.go",
	"celadon_gym.go",
	"cinnabar_gym.go",
	"cinnabar_secret_key.go",
	"cutscene.go",
	"elite_four.go",
	"elite_four_stages.go",
	"errand.go",
	"evolution_stone_shop.go",
	"field_roster.go",
	"fossil.go",
	"fuchsia_progression.go",
	"gift_dojo.go",
	"gift_eevee.go",
	"gift_lapras.go",
	"gym.go",
	"inventory_recovery.go",
	"league_items.go",
	"league_resources.go",
	"mt_moon.go",
	"objective_recovery.go",
	"pokemon_center_checkpoint.go",
	"rocket_b1f_transition.go",
	"rocket_hideout.go",
	"rocket_hideout_elevator.go",
	"rocket_spinner.go",
	"route16_fly.go",
	"route_gate.go",
	"route_gate_audit.go",
	"route_transition.go",
	"saffron_gate.go",
	"saffron_gym.go",
	"seafoam_current.go",
	"side_route_semantics.go",
	"silph_card_key.go",
	"silph_clear.go",
	"ss_anne.go",
	"story.go",
	"surge_progression.go",
	"topology_interaction.go",
	"vermilion_gym_entry.go",
	"victory_road_boulders.go",
	"victory_road_progression.go",
	"viridian_gym.go",
)

func fileSet(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}

func concreteGameImport(path string) bool {
	for _, prefix := range []string{
		"github.com/maestroi/pokepilot/red/",
		"github.com/maestroi/pokepilot/blue/",
		"github.com/maestroi/pokepilot/yellow/",
		"github.com/maestroi/pokepilot/gs/",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func redImport(path string) bool {
	return strings.HasPrefix(path, "github.com/maestroi/pokepilot/red/")
}

func TestPhase4PortableSkillBoundary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	seenPortable := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		if phase4PortableFiles[name] {
			seenPortable[name] = true
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", name, err)
			}
			if phase4PortableFiles[name] && concreteGameImport(path) {
				t.Errorf("%s is a reusable Phase-4 driver but imports concrete game package %q", name, path)
			}
			if !redImport(path) {
				continue
			}
			if phase4PortableFiles[name] {
				continue
			}
			if gen1AdapterExceptions[name] || redOwnedStoryPolicyExceptions[name] {
				continue
			}
			t.Errorf("%s imports concrete Red package %q but is not classified as a Gen-I adapter/compatibility file or Red-owned story/progression code", name, path)
		}
	}

	for name := range phase4PortableFiles {
		if !seenPortable[name] {
			t.Errorf("portable Phase-4 boundary references missing file %s", name)
		}
	}
	for name := range gen1AdapterExceptions {
		if redOwnedStoryPolicyExceptions[name] {
			t.Errorf("%s is classified as both Gen-I adapter and Red-owned story/policy", name)
		}
	}
}
