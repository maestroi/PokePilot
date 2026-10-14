package world

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

// ErrRegionTransportUnsupported means the adapter cannot resolve this
// cartridge's native map identities or region boundaries.
var ErrRegionTransportUnsupported = errors.New("world: native region transport is unsupported")

// NativeRegionRoute deliberately ends at the boarding map. A transport is a
// script-owned action, not a warp that native walking may silently traverse.
// ArrivalMap is informational until a transport executor confirms arrival.
type NativeRegionRoute struct {
	Approach    []NativeEdge
	BoardingMap uint16
	ArrivalMap  uint16
	Transport   game.RegionTransportOption
}

// PlanNativeRegionRoute finds the closest reachable departure in the current
// region, then returns its separate semantic transport action. The static
// graph is filtered to current-region maps before finding the approach. This
// is crucial: a decomp ROM may list the Magnet Train's remote station as an
// unconditional warp even while its officer denies boarding. That warp must
// never act as a shortcut to a departure in the other region.
//
// resolvePlace translates portable endpoints to native map ids, and regionOf
// derives region identity from the adapter's map/landmark data. Unknown
// identities fail closed; generic routing never decodes Gold/Silver ids.
func PlanNativeRegionRoute(
	graph *NativeGraph,
	currentMap uint16,
	fromRegion, toRegion game.RegionID,
	options []game.RegionTransportOption,
	resolvePlace func(game.PlaceID) (uint16, bool),
	regionOf func(uint16) (game.RegionID, bool),
) (NativeRegionRoute, error) {
	if graph == nil || resolvePlace == nil || regionOf == nil || fromRegion == "" || toRegion == "" {
		return NativeRegionRoute{}, ErrRegionTransportUnsupported
	}
	if fromRegion == toRegion {
		return NativeRegionRoute{}, fmt.Errorf("%w: crossing requires distinct regions", ErrNoRoute)
	}
	// Keep only confirmed maps in this region. A special-map landmark with no
	// known region is not permission to traverse it.
	sameRegion := &NativeGraph{Edges: make(map[uint16][]NativeEdge)}
	for mapID := range graph.Edges {
		if region, ok := regionOf(mapID); ok && region == fromRegion {
			sameRegion.Edges[mapID] = nil
		}
	}
	if _, ok := sameRegion.Edges[currentMap]; !ok {
		return NativeRegionRoute{}, fmt.Errorf("%w: current map %#04x is not in %s", ErrNoRoute, currentMap, fromRegion)
	}
	for from, edges := range graph.Edges {
		if _, ok := sameRegion.Edges[from]; !ok {
			continue
		}
		for _, edge := range edges {
			if _, ok := sameRegion.Edges[edge.To]; ok {
				sameRegion.Edges[from] = append(sameRegion.Edges[from], edge)
			}
		}
	}

	var best NativeRegionRoute
	haveBest := false
	var blocked []game.TransitionBlockage
	for _, option := range options {
		if option.FromRegion != fromRegion || option.ToRegion != toRegion {
			continue
		}
		board, okBoard := resolvePlace(option.From)
		arrive, okArrive := resolvePlace(option.To)
		if !okBoard || !okArrive {
			continue
		}
		if region, ok := regionOf(board); !ok || region != fromRegion {
			continue
		}
		if region, ok := regionOf(arrive); !ok || region != toRegion {
			continue
		}
		if _, ok := sameRegion.Edges[board]; !ok {
			continue
		}
		approach, err := FindNativeRoute(sameRegion, currentMap, board)
		if err != nil {
			continue
		}
		if !option.Available || len(option.Missing) > 0 {
			if len(option.Missing) > 0 {
				blocked = append(blocked, game.TransitionBlockage{
					Transition: game.Transition{ID: option.Service, From: option.From, To: option.To},
					Missing:    append([]game.CapabilityID(nil), option.Missing...),
				})
			}
			continue
		}
		// Nearest boarding map wins; equal-distance services tie by their
		// stable semantic identities rather than map iteration order.
		if !haveBest || len(approach) < len(best.Approach) ||
			(len(approach) == len(best.Approach) && (option.Service < best.Transport.Service ||
				(option.Service == best.Transport.Service && option.From < best.Transport.From))) {
			best = NativeRegionRoute{
				Approach: approach, BoardingMap: board, ArrivalMap: arrive,
				Transport: option,
			}
			haveBest = true
		}
	}
	if haveBest {
		return best, nil
	}
	if len(blocked) > 0 {
		return NativeRegionRoute{}, &RouteBlockedError{Blockages: blocked}
	}
	return NativeRegionRoute{}, fmt.Errorf("%w: no reachable %s-to-%s boarding point", ErrNoRoute, fromRegion, toRegion)
}
