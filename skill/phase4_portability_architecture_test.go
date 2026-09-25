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
var phase4PortableFiles = map[string]bool{
	"bag.go":                           true,
	"battle.go":                        true,
	"battle_escape_menu.go":            true,
	"battle_execution.go":              true,
	"battle_forced_choice_recovery.go": true,
	"battle_medicine_execution.go":     true,
	"battle_menu.go":                   true,
	"battle_policy.go":                 true,
	"battle_resource_policy.go":        true,
	"battle_resource_state.go":         true,
	"battle_resources.go":              true,
	"battle_runtime.go":                true,
	"battle_settlement.go":             true,
	"battle_state.go":                  true,
	"capture_runtime.go":               true,
	"catch.go":                         true,
	"combat_strategy.go":               true,
	"connection_field_path.go":         true,
	"elevator.go":                      true,
	"field_action.go":                  true,
	"field_action_runtime.go":          true,
	"field_item.go":                    true,
	"field_item_runtime.go":            true,
	"field_move_runtime.go":            true,
	"field_path_bridge.go":             true,
	"flee.go":                          true,
	"goto.go":                          true,
	"heal.go":                          true,
	"heal_runtime.go":                  true,
	"heal_transaction.go":              true,
	"intra_map_warp.go":                true,
	"inventory.go":                     true,
	"list_menu.go":                     true,
	"live_topology.go":                 true,
	"menu.go":                          true,
	"move.go":                          true,
	"navigation_overworld.go":          true,
	"overworld.go":                     true,
	"overworld_blackout.go":            true,
	"party.go":                         true,
	"party_menu.go":                    true,
	"policy.go":                        true,
	"prompt.go":                        true,
	"repel.go":                         true,
	"routable.go":                      true,
	"routing_runtime.go":               true,
	"shop.go":                          true,
	"start_menu.go":                    true,
	"switch_policy.go":                 true,
	"travel.go":                        true,
	"warp.go":                          true,
}

// gen1AdapterExceptions are explicit Gen-I adapters/compatibility controllers.
// They may interpret Red data because their job is to translate or execute a
// Gen-I-specific mechanism; generic Phase-4 drivers must not depend on them for
// cartridge layout knowledge.
var gen1AdapterExceptions = map[string]bool{
	"bag_gen1_compat.go":             true,
	"bag_space.go":                   true,
	"battle_decision.go":             true,
	"battle_resource_gen1_compat.go": true,
	"bicycle.go":                    true,
	"capture_storage.go":             true,
	"catch_acquire.go":               true,
	"catch_gen1_compat.go":           true,
	"choice_rewards.go":              true,
	"cut.go":                         true,
	"dialogue_recovery.go":           true,
	"evolution_item.go":              true,
	"field_item_gen1_compat.go":      true,
	"field_move_red_compat.go":       true,
	"field_path.go":                  true,
	"fishing.go":                     true,
	"flee_safari_gen1_compat.go":     true,
	"gift_pokemon.go":                true,
	"heal_gen1_compat.go":            true,
	"in_game_trade.go":               true,
	"interact.go":                    true,
	"interaction.go":                 true,
	"item_storage.go":                true,
	"link_trade.go":                  true,
	"live_topology_gen1_compat.go":   true,
	"move_learning.go":               true,
	"party_gen1_compat.go":           true,
	"pc.go":                          true,
	"pc_box_switch.go":               true,
	"pickup.go":                      true,
	"policy_gen1_compat.go":          true,
	"required_battle.go":             true,
	"routable_gen1_compat.go":        true,
	"shop_gen1_compat.go":            true,
	"starter_request.go":             true,
	"static_capture.go":              true,
	"switch_policy_gen1_compat.go":   true,
	"tmhm.go":                        true,
	"tmhm_slot.go":                   true,
	"trainer.go":                     true,
	"trainer_live_status.go":         true,
	"utility_field_candidate.go":     true,
	"water_catch.go":                 true,
}

// redOwnedStoryPolicyExceptions are deliberately Red-owned story, progression,
// recovery, and route-policy executors. #974 explicitly excludes this layer;
// Gen II gets its own story/progression implementation rather than sharing it.
var redOwnedStoryPolicyExceptions = map[string]bool{
	"bag_pressure.go":               true,
	"bill.go":                       true,
	"boulder_progression.go":        true,
	"boulder_puzzle.go":             true,
	"celadon_gym.go":                true,
	"cinnabar_gym.go":               true,
	"cinnabar_secret_key.go":        true,
	"cutscene.go":                   true,
	"elite_four.go":                 true,
	"elite_four_stages.go":          true,
	"errand.go":                     true,
	"evolution_stone_shop.go":       true,
	"field_roster.go":               true,
	"fossil.go":                     true,
	"fuchsia_progression.go":        true,
	"gift_dojo.go":                  true,
	"gift_eevee.go":                 true,
	"gift_lapras.go":                true,
	"gym.go":                        true,
	"inventory_recovery.go":         true,
	"league_items.go":               true,
	"league_resources.go":           true,
	"mt_moon.go":                    true,
	"objective_recovery.go":         true,
	"pokemon_center_checkpoint.go":  true,
	"rocket_b1f_transition.go":      true,
	"rocket_hideout.go":             true,
	"rocket_hideout_elevator.go":    true,
	"rocket_spinner.go":             true,
	"route16_fly.go":                true,
	"route_gate.go":                 true,
	"route_gate_audit.go":           true,
	"route_transition.go":           true,
	"saffron_gate.go":               true,
	"saffron_gym.go":                true,
	"seafoam_current.go":            true,
	"side_route_semantics.go":       true,
	"silph_card_key.go":             true,
	"silph_clear.go":                true,
	"ss_anne.go":                    true,
	"story.go":                      true,
	"surge_progression.go":          true,
	"topology_interaction.go":       true,
	"vermilion_gym_entry.go":        true,
	"victory_road_boulders.go":      true,
	"victory_road_progression.go":   true,
	"viridian_gym.go":               true,
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
