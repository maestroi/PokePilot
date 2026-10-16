package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func gen1GoToCompatibilityEnabled(m *emu.Emu) bool {
	if m == nil {
		return false
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return false
	}
	switch string(profile.ID()) {
	case "pokemon-red", "pokemon-blue", "pokemon-yellow":
		return true
	default:
		return false
	}
}

// applyGoToCompatibilityInitialTopology keeps Kanto-only route overlays out of
// the reusable GoTo driver. Future non-Gen-I profiles get the provider graph
// unchanged and can supply their own story/progression semantics separately.
func applyGoToCompatibilityInitialTopology(m *emu.Emu, g *world.Graph, romData []byte) (*world.Graph, error) {
	if !gen1GoToCompatibilityEnabled(m) {
		return g, nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	g, err := withAsleepRoute16Snorlax(g, romData, &mem)
	if err != nil {
		return nil, err
	}
	return withSurfSeaTopology(g, romData, &mem)
}

// applyGoToCompatibilityLocalTopology applies the remaining live Kanto
// scripted-tile facts only for Gen-I profiles.
func applyGoToCompatibilityLocalTopology(
	m *emu.Emu,
	cur uint8,
	blockers map[[2]int]bool,
	grid *world.Grid,
	romData []byte,
) map[[2]int]bool {
	if !gen1GoToCompatibilityEnabled(m) {
		return blockers
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	blockers = mergeBlockers(blockers, gen1ScriptedTileBlockers(cur, &mem))
	if cur == route16Map {
		openRoute16CutPassage(grid, romData, &mem)
	}
	return blockers
}

// gen1ScriptedTileBlockers returns walkable collision tiles whose map scripts
// still intercept the player. The ROM collision grid cannot represent them, so
// every Gen-I same-map walk and graph overlay must treat them as occupied until
// the matching bag/event precondition holds.
//
// Cinnabar Island (18,4) is the measured case: without SECRET_KEY in the bag the
// island script prints "The door is locked..." and pushes the player south.
// That tile sits on ordinary shortest paths to the Pokemon Mansion door, so a
// generic inventory-detour Travel back onto the island loops the text box
// (run-30sb0q7xjdkqc1psaltd1lywws / #2718). Match IsItemInBag: ownership in the
// Player PC alone does not silence the script.
func gen1ScriptedTileBlockers(cur uint8, mem *state.Mem) map[[2]int]bool {
	if mem == nil {
		return nil
	}
	switch cur {
	case route16Map:
		if state.HasEvent(mem, eventBeatRoute16Snorlax) {
			return nil
		}
		return map[[2]int]bool{{route16SnorlaxX, route16SnorlaxY}: true}
	case cinnabarIslandMap:
		if _, qty := bagEntry(mem, mansionSecretKeyItem); qty > 0 {
			return nil
		}
		return map[[2]int]bool{{int(cinnabarGymGateX), int(cinnabarGymGateY)}: true}
	default:
		return nil
	}
}

// gen1ScriptedTileBlockersFor snapshots the current map for walkAroundAvoidingObjects,
// which lives in the portable move driver and must not import red packages.
func gen1ScriptedTileBlockersFor(m *emu.Emu) map[[2]int]bool {
	if m == nil || !gen1GoToCompatibilityEnabled(m) {
		return nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	return gen1ScriptedTileBlockers(m.Peek8(sym.CurMap), &mem)
}

// goToRoutePrerequisites returns Kanto's semantic route gates for Gen-I
// profiles. Other generations start with no Red prerequisites; their adapter
// may define generation-owned route semantics without changing GoTo.
func goToRoutePrerequisites(m *emu.Emu, g *world.Graph, romData []byte) world.RoutePrerequisites {
	if !gen1GoToCompatibilityEnabled(m) {
		return world.RoutePrerequisites{}
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	return redRoutePrerequisites(g, romData, &mem)
}

// recoverGoToCompatibilitySwitchSeal opens a Gen-I Mansion switch-sealed
// pocket when Travel's recoveringGoTo sees world.ErrNoRoute. The static graph
// still knows the stairs/door warps, but statue state can leave the player in
// a component with no walkable path to those pads — measured after Secret Key
// ownership on Mansion B1F (run-3dxv0agt5di8x1ddvib3x0enk5). Same-map Travel
// back to a sealed B1F pickup tile (inventory detour return to the Secret Key
// stand) must open that destination, not only the stairs. This stays out of
// plain GoTo so story helpers that intentionally handle ErrNoRoute
// (returnToCinnabarIsland) keep that signal.
func recoverGoToCompatibilitySwitchSeal(m *emu.Emu, romData []byte, policy MovePolicy, dest Destination) (bool, error) {
	if !gen1GoToCompatibilityEnabled(m) || policy == nil || m == nil {
		return false, nil
	}
	switch m.Peek8(sym.CurMap) {
	case pokemonMansionB1FMap:
		tx, ty := mansionB1FExitX, mansionB1FExitY
		if dest.Map == pokemonMansionB1FMap {
			tx, ty = dest.X, dest.Y
		}
		if mansionTileReachable(m, romData, tx, ty) {
			return false, nil
		}
		if err := openMansionB1FTile(m, romData, policy, tx, ty); err != nil {
			return false, fmt.Errorf("skill: Travel: open Mansion B1F switch path to (%d,%d): %w", tx, ty, err)
		}
		return true, nil
	case pokemonMansion1FMap:
		if mansionTileReachable(m, romData, mansion1FExitX, mansion1FExitY) {
			return false, nil
		}
		if err := openMansion1FExit(m, romData, policy); err != nil {
			return false, fmt.Errorf("skill: Travel: open Mansion 1F switch exit: %w", err)
		}
		return true, nil
	default:
		return false, nil
	}
}

// enterGoToCompatibilitySealedDestination owns the one Gen-I destination the
// static graph cannot route into: Mansion B1F. The graph links the 1F door
// straight to the B1F stairs, but that stairs pocket is only entered through
// the 3F fall, so plain GoTo walked in, back out, and wandered Kanto until it
// stalled (run-4dzrf942k1sii0tpd3jygf1m). Travel first reaches 1F, then the
// Secret Key skill's own basement descent; ordinary GoTo owns the B1F tile.
func enterGoToCompatibilitySealedDestination(m *emu.Emu, romData []byte, dest Destination, policy MovePolicy) error {
	if dest.Map != pokemonMansionB1FMap || policy == nil || !gen1GoToCompatibilityEnabled(m) {
		return nil
	}
	switch m.Peek8(sym.CurMap) {
	case pokemonMansionB1FMap:
		return nil
	case pokemonMansion1FMap:
		if mansionTileReachable(m, romData, mansion1FToB1FWarp.WarpX, mansion1FToB1FWarp.WarpY) {
			return nil
		}
	case pokemonMansion2FMap, pokemonMansion3FMap:
	default:
		if err := cutAwareGoTo(m, romData, Destination{Map: pokemonMansion1FMap, X: mansion1FExitX, Y: mansion1FExitY}, policy)(); err != nil {
			return err
		}
	}
	return reachMansionBasement(m, romData, policy)
}
