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
// Route-16 corridor facts only for Gen-I profiles.
func applyGoToCompatibilityLocalTopology(
	m *emu.Emu,
	cur uint8,
	blockers map[[2]int]bool,
	grid *world.Grid,
	romData []byte,
) map[[2]int]bool {
	if !gen1GoToCompatibilityEnabled(m) || cur != route16Map {
		return blockers
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, eventBeatRoute16Snorlax) {
		if blockers == nil {
			blockers = map[[2]int]bool{}
		}
		blockers[[2]int{route16SnorlaxX, route16SnorlaxY}] = true
	}
	openRoute16CutPassage(grid, romData, &mem)
	return blockers
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
// ownership on Mansion B1F (run-3dxv0agt5di8x1ddvib3x0enk5). This stays out of
// plain GoTo so story helpers that intentionally handle ErrNoRoute
// (returnToCinnabarIsland) keep that signal.
func recoverGoToCompatibilitySwitchSeal(m *emu.Emu, romData []byte, policy MovePolicy) (bool, error) {
	if !gen1GoToCompatibilityEnabled(m) || policy == nil || m == nil {
		return false, nil
	}
	switch m.Peek8(sym.CurMap) {
	case pokemonMansionB1FMap:
		if mansionTileReachable(m, romData, mansionB1FExitX, mansionB1FExitY) {
			return false, nil
		}
		if err := openMansionBasementExit(m, romData, policy); err != nil {
			return false, fmt.Errorf("skill: Travel: open Mansion B1F switch exit: %w", err)
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
