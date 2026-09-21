package skill

import (
	"errors"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
)

type destinationRouteTarget struct {
	X, Y int
}

func destinationRouteTargets(dest Destination) []destinationRouteTarget {
	switch dest.Kind {
	case DestinationMap:
		return nil
	case DestinationArea:
		out := make([]destinationRouteTarget, 0,
			(int(dest.Area.MaxX)-int(dest.Area.MinX)+1)*(int(dest.Area.MaxY)-int(dest.Area.MinY)+1))
		for y := int(dest.Area.MinY); y <= int(dest.Area.MaxY); y++ {
			for x := int(dest.Area.MinX); x <= int(dest.Area.MaxX); x++ {
				out = append(out, destinationRouteTarget{X: x, Y: y})
			}
		}
		return out
	case DestinationInteraction:
		x, y := int(dest.X), int(dest.Y)
		var out []destinationRouteTarget
		// Ordinary interaction is one tile away. Service counters use the
		// same axis two tiles away, with the counter between actor and player.
		for _, d := range [][2]int{{0, -1}, {-1, 0}, {1, 0}, {0, 1}, {0, -2}, {-2, 0}, {2, 0}, {0, 2}} {
			tx, ty := x+d[0], y+d[1]
			if tx >= 0 && tx <= 0xff && ty >= 0 && ty <= 0xff {
				out = append(out, destinationRouteTarget{X: tx, Y: ty})
			}
		}
		return out
	default:
		return []destinationRouteTarget{{X: int(dest.X), Y: int(dest.Y)}}
	}
}

func findRoutePlanForDestination(
	g *world.Graph,
	from uint8,
	x, y int,
	dest Destination,
	blockedHere map[world.Edge]bool,
	prereqs world.RoutePrerequisites,
) ([]world.RouteStep, error) {
	targets := destinationRouteTargets(dest)
	if dest.Kind == DestinationMap || len(targets) == 0 {
		return world.FindRoutePlanAtDestinationWithCapabilities(
			g, from, dest.Map, x, y, -1, -1, blockedHere, prereqs,
		)
	}
	var firstErr error
	for _, target := range targets {
		route, err := world.FindRoutePlanAtDestinationWithCapabilities(
			g, from, dest.Map, x, y, target.X, target.Y, blockedHere, prereqs,
		)
		if err == nil || errors.Is(err, world.ErrRouteReplanRequired) {
			return route, err
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = world.ErrNoRoute
	}
	return nil, firstErr
}

func routePlanToDestinationByTravelPolicy(
	m *emu.Emu,
	g *world.Graph,
	from uint8,
	x, y int,
	dest Destination,
	blockedHere map[world.Edge]bool,
	prereqs world.RoutePrerequisites,
) (world.RouteCostResult, error) {
	targets := destinationRouteTargets(dest)
	if dest.Kind == DestinationMap || len(targets) == 0 {
		return routePlanByTravelPolicy(
			m, g, from, dest.Map, x, y, -1, -1, blockedHere, prereqs,
		)
	}

	var (
		best     world.RouteCostResult
		bestErr  error
		firstErr error
		haveBest bool
	)
	for _, target := range targets {
		result, err := routePlanByTravelPolicy(
			m, g, from, dest.Map, x, y, target.X, target.Y, blockedHere, prereqs,
		)
		if err != nil && !errors.Is(err, world.ErrRouteReplanRequired) {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !haveBest || result.Cost < best.Cost {
			best, bestErr, haveBest = result, err, true
		}
	}
	if haveBest {
		return best, bestErr
	}
	if firstErr == nil {
		firstErr = world.ErrNoRoute
	}
	return world.RouteCostResult{}, firstErr
}
