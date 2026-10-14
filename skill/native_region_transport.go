package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

// nativeRegionTransportProfile adds only the static place and region
// translators a profile needs for planning. Its interactive travel scripts
// remain game-owned and deliberately separate from this read-only API.
type nativeRegionTransportProfile interface {
	nativeRoutingProfile
	game.RegionTransportProvider
	NativeRegionTransportPlace(game.PlaceID) (uint16, bool)
	NativeRegionTransportRegion([]byte, uint16) (game.RegionID, bool)
}

// PlanNativeRegionTransport returns the native route to a legal boarding map
// followed by its distinct semantic transport leg. It makes no inputs or
// state mutations. In particular, it does NOT walk a ROM train-station warp,
// which could bypass an officer's ticket/power/schedule checks.
//
// The result is a plan, not permission to mark the region journey complete:
// the eventual adapter executor must board using the script and positively
// observe arrival in the requested region.
func PlanNativeRegionTransport(m *emu.Emu, romData []byte, destination game.RegionID) (world.NativeRegionRoute, error) {
	if m == nil || len(romData) == 0 {
		return world.NativeRegionRoute{}, world.ErrRegionTransportUnsupported
	}
	routing, err := nativeRoutingProfileForROM(romData)
	if err != nil {
		return world.NativeRegionRoute{}, fmt.Errorf("%w: %v", world.ErrRegionTransportUnsupported, err)
	}
	profile, ok := routing.(nativeRegionTransportProfile)
	if !ok {
		return world.NativeRegionRoute{}, world.ErrRegionTransportUnsupported
	}
	obs, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return world.NativeRegionRoute{}, fmt.Errorf("skill: native region transport observation: %w", err)
	}
	if obs.Region == "" {
		return world.NativeRegionRoute{}, world.ErrRegionTransportUnsupported
	}
	provider := profile.NativeMapProvider(romData)
	if provider == nil {
		return world.NativeRegionRoute{}, world.ErrRegionTransportUnsupported
	}
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		return world.NativeRegionRoute{}, fmt.Errorf("skill: native region transport topology: %w", err)
	}
	regionOf := func(mapID uint16) (game.RegionID, bool) {
		return profile.NativeRegionTransportRegion(romData, mapID)
	}
	plan, err := world.PlanNativeRegionRoute(graph, obs.NativeMapID, obs.Region, destination,
		profile.DecodeRegionTransports(m), profile.NativeRegionTransportPlace, regionOf)
	if err != nil {
		// RouteBlockedError remains discoverable with errors.As by callers;
		// status strings must never be the authority for recovery policy.
		var blocked *world.RouteBlockedError
		if errors.As(err, &blocked) {
			return world.NativeRegionRoute{}, blocked
		}
		return world.NativeRegionRoute{}, err
	}
	return plan, nil
}
