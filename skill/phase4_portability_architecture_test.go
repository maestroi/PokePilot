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
var phase4PortableFiles = fileSet(\n\t"bag.go",\n\t"battle.go",\n\t"battle_escape_menu.go",\n\t"battle_execution.go",\n\t"battle_forced_choice_recovery.go",\n\t"battle_medicine_execution.go",\n\t"battle_menu.go",\n\t"battle_policy.go",\n\t"battle_resource_policy.go",\n\t"battle_resource_state.go",\n\t"battle_resources.go",\n\t"battle_runtime.go",\n\t"battle_settlement.go",\n\t"battle_state.go",\n\t"capture_runtime.go",\n\t"catch.go",\n\t"combat_strategy.go",\n\t"connection_field_path.go",\n\t"elevator.go",\n\t"field_action.go",\n\t"field_action_runtime.go",\n\t"field_item.go",\n\t"field_item_runtime.go",\n\t"field_move_runtime.go",\n\t"field_path_bridge.go",\n\t"flee.go",\n\t"goto.go",\n\t"heal.go",\n\t"heal_runtime.go",\n\t"heal_transaction.go",\n\t"intra_map_warp.go",\n\t"inventory.go",\n\t"list_menu.go",\n\t"live_topology.go",\n\t"menu.go",\n\t"move.go",\n\t"navigation_overworld.go",\n\t"overworld.go",\n\t"overworld_blackout.go",\n\t"party.go",\n\t"party_menu.go",\n\t"policy.go",\n\t"prompt.go",\n\t"repel.go",\n\t"routable.go",\n\t"routing_runtime.go",\n\t"shop.go",\n\t"start_menu.go",\n\t"switch_policy.go",\n\t"travel.go",\n\t"warp.go",\n)

// gen1AdapterExceptions are explicit Gen-I adapters/compatibility controllers.
// They may interpret Red data because their job is to translate or execute a
// Gen-I-specific mechanism; generic Phase-4 drivers must not depend on them for
// cartridge layout knowledge.
var gen1AdapterExceptions = fileSet(\n\t"bag_gen1_compat.go",\n\t"bag_space.go",\n\t"battle_decision.go",\n\t"battle_resource_gen1_compat.go",\n\t"bicycle.go",\n\t"capture_storage.go",\n\t"catch_acquire.go",\n\t"catch_gen1_compat.go",\n\t"choice_rewards.go",\n\t"cut.go",\n\t"dialogue_recovery.go",\n\t"evolution_item.go",\n\t"field_item_gen1_compat.go",\n\t"field_move_red_compat.go",\n\t"field_path.go",\n\t"fishing.go",\n\t"flee_safari_gen1_compat.go",\n\t"gift_pokemon.go",\n\t"heal_gen1_compat.go",\n\t"in_game_trade.go",\n\t"interact.go",\n\t"interaction.go",\n\t"item_storage.go",\n\t"link_trade.go",\n\t"live_topology_gen1_compat.go",\n\t"move_learning.go",\n\t"party_gen1_compat.go",\n\t"pc.go",\n\t"pc_box_switch.go",\n\t"pickup.go",\n\t"policy_gen1_compat.go",\n\t"required_battle.go",\n\t"routable_gen1_compat.go",\n\t"shop_gen1_compat.go",\n\t"starter_request.go",\n\t"static_capture.go",\n\t"switch_policy_gen1_compat.go",\n\t"tmhm.go",\n\t"tmhm_slot.go",\n\t"trainer.go",\n\t"trainer_live_status.go",\n\t"utility_field_candidate.go",\n\t"water_catch.go",\n)

// redOwnedStoryPolicyExceptions are deliberately Red-owned story, progression,
// recovery, and route-policy executors. #974 explicitly excludes this layer;
// Gen II gets its own story/progression implementation rather than sharing it.
var redOwnedStoryPolicyExceptions = fileSet(\n\t"bag_pressure.go",\n\t"bill.go",\n\t"boulder_progression.go",\n\t"boulder_puzzle.go",\n\t"celadon_gym.go",\n\t"cinnabar_gym.go",\n\t"cinnabar_secret_key.go",\n\t"cutscene.go",\n\t"elite_four.go",\n\t"elite_four_stages.go",\n\t"errand.go",\n\t"evolution_stone_shop.go",\n\t"field_roster.go",\n\t"fossil.go",\n\t"fuchsia_progression.go",\n\t"gift_dojo.go",\n\t"gift_eevee.go",\n\t"gift_lapras.go",\n\t"gym.go",\n\t"inventory_recovery.go",\n\t"league_items.go",\n\t"league_resources.go",\n\t"mt_moon.go",\n\t"objective_recovery.go",\n\t"pokemon_center_checkpoint.go",\n\t"rocket_b1f_transition.go",\n\t"rocket_hideout.go",\n\t"rocket_hideout_elevator.go",\n\t"rocket_spinner.go",\n\t"route16_fly.go",\n\t"route_gate.go",\n\t"route_gate_audit.go",\n\t"route_transition.go",\n\t"saffron_gate.go",\n\t"saffron_gym.go",\n\t"seafoam_current.go",\n\t"side_route_semantics.go",\n\t"silph_card_key.go",\n\t"silph_clear.go",\n\t"ss_anne.go",\n\t"story.go",\n\t"surge_progression.go",\n\t"topology_interaction.go",\n\t"vermilion_gym_entry.go",\n\t"victory_road_boulders.go",\n\t"victory_road_progression.go",\n\t"viridian_gym.go",\n)

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
