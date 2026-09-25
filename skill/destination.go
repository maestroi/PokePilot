package skill

import "strings"

// DestinationKind states what arriving at a destination means. ExactTile is
// deliberately the zero value so existing internal coordinates remain exact
// unless a caller opts into broader semantics.
type DestinationKind uint8

const (
	DestinationExactTile DestinationKind = iota
	DestinationMap
	DestinationArea
	DestinationInteraction
)

// DestinationArea is an inclusive rectangular set of acceptable standing
// tiles. It is intentionally comparable so Destination remains usable as a
// map key and in equality-based regression tests.
type DestinationBounds struct {
	MinX, MinY uint8
	MaxX, MaxY uint8
}

// Destination is a navigation goal. X/Y mean an exact standing tile for
// DestinationExactTile, and the target object/NPC tile for
// DestinationInteraction. DestinationMap ignores X/Y. DestinationArea uses
// Area and chooses the cheapest reachable tile in that region.
type Destination struct {
	Map  uint8
	X, Y uint8
	Kind DestinationKind
	Area DestinationBounds
}

func ExactDestination(mapID, x, y uint8) Destination {
	return Destination{Map: mapID, X: x, Y: y, Kind: DestinationExactTile}
}

func MapDestination(mapID uint8) Destination {
	return Destination{Map: mapID, Kind: DestinationMap}
}

func AreaDestination(mapID, minX, minY, maxX, maxY uint8) Destination {
	return Destination{
		Map:  mapID,
		Kind: DestinationArea,
		Area: DestinationBounds{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY},
	}
}

func InteractionDestination(mapID, targetX, targetY uint8) Destination {
	return Destination{Map: mapID, X: targetX, Y: targetY, Kind: DestinationInteraction}
}

// Reached reports whether a plain player coordinate positively satisfies the
// destination without needing live collision/object data. Interaction targets
// deliberately return false: adjacency may be one tile, across a service
// counter, or otherwise topology-dependent and is verified by GoTo itself.
func (d Destination) Reached(mapID, x, y uint8) bool {
	if mapID != d.Map {
		return false
	}
	switch d.Kind {
	case DestinationMap:
		return true
	case DestinationArea:
		return x >= d.Area.MinX && x <= d.Area.MaxX &&
			y >= d.Area.MinY && y <= d.Area.MaxY
	case DestinationInteraction:
		return false
	default:
		return x == d.X && y == d.Y
	}
}

// routeCoordinates converts destination semantics to the world planner's
// existing exact-vs-map contract. Map and interaction goals first route to
// the correct map; interaction approach is resolved from live geometry after
// arrival. Area goals likewise enter the map before choosing the cheapest live
// tile in the region.
func (d Destination) routeCoordinates() (int, int) {
	switch d.Kind {
	case DestinationMap, DestinationArea, DestinationInteraction:
		return -1, -1
	default:
		return int(d.X), int(d.Y)
	}
}

func (d Destination) KindName() string {
	switch d.Kind {
	case DestinationMap:
		return "map"
	case DestinationArea:
		return "area"
	case DestinationInteraction:
		return "interaction"
	default:
		return "exact_tile"
	}
}

// namedPlaceKind makes broad geographic names semantic map-arrival goals while
// retaining exact legacy coordinates for service/script places. The stored X/Y
// remain useful as deterministic hints/tests, but generic navigation no longer
// treats a city or route name as an instruction to revisit one arbitrary tile.
func namedPlaceKind(name string, stored Destination) DestinationKind {
	if stored.Kind != DestinationExactTile {
		return stored.Kind
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(name, "route ") ||
		strings.HasSuffix(name, " city") ||
		strings.HasSuffix(name, " town") ||
		strings.HasSuffix(name, " island") {
		return DestinationMap
	}
	switch name {
	case "viridian forest",
		"mt moon 1f", "mt moon b1f", "mt moon b2f",
		"rock tunnel 1f",
		"pokemon mansion":
		return DestinationMap
	default:
		return DestinationExactTile
	}
}
