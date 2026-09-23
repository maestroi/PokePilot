package skill

import "github.com/maestroi/pokepilot/red/rom"

// PokemonCenterMap reports whether a map exposes the actual Pokemon Center
// nurse service. Service-role detection is stronger than map-name conventions:
// Indigo Plateau's lobby is a Center checkpoint even though its map name does
// not contain POKECENTER.
func PokemonCenterMap(romData []byte, mapID uint8) (bool, error) {
	_, ok, err := interactionDestinationForRole(romData, mapID, rom.InteractionPokemonCenterNurse)
	return ok, err
}

// RecoveryCheckpointPlace translates wLastBlackoutMap's exterior respawn map
// into the named Center that activated it. The cartridge records the map
// outside the Center, so derive the service destination from real world
// topology rather than assuming city/Center names line up.
func RecoveryCheckpointPlace(romData []byte, respawnMap uint8) (string, bool, error) {
	graph, err := cachedRouteGraph(romData)
	if err != nil {
		return "", false, err
	}
	for _, name := range PlaceNames() {
		dest, ok := Place(name)
		if !ok {
			continue
		}
		center, centerErr := PokemonCenterMap(romData, dest.Map)
		if centerErr != nil || !center {
			continue
		}
		if dest.Map == respawnMap {
			return name, true, nil
		}
		for _, edge := range graph.Edges[dest.Map] {
			if edge.To == respawnMap {
				return name, true, nil
			}
		}
	}
	return "", false, nil
}
