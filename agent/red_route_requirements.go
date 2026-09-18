package agent

import (
	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

const (
	lavenderTownMap              uint8 = 0x04
	celadonCityMap               uint8 = 0x06
	fuchsiaCityMap               uint8 = 0x07
	saffronCityMap               uint8 = 0x0a
	route2Map                    uint8 = 0x0d
	route3Map                    uint8 = 0x0e
	route9Map                    uint8 = 0x14
	route10Map                   uint8 = 0x15
	route12Map                   uint8 = 0x17
	route13Map                   uint8 = 0x18
	route14Map                   uint8 = 0x19
	route15Map                   uint8 = 0x1a
	route25Map                   uint8 = 0x24
	viridianGymMap               uint8 = 0x2d
	vermilionGymMap              uint8 = 0x5c
	fuchsiaMartMap               uint8 = 0x98
	fuchsiaBillsGrandpasHouseMap uint8 = 0x99
	fuchsiaPokemonCenterMap      uint8 = 0x9a
	wardensHouseMap              uint8 = 0x9b
	safariZoneGateMap            uint8 = 0x9c
	fuchsiaGymMap                uint8 = 0x9d
	fuchsiaMeetingRoomMap        uint8 = 0x9e
	fuchsiaGoodRodHouseMap       uint8 = 0xa4
	cinnabarGymMap               uint8 = 0xa6
	saffronGymMap                uint8 = 0xb2
	route15Gate1FMap             uint8 = 0xb8
	safariZoneEastMap            uint8 = 0xd9
	safariZoneNorthMap           uint8 = 0xda
	safariZoneWestMap            uint8 = 0xdb
	safariZoneCenterMap          uint8 = 0xdc
	safariZoneCenterRestHouseMap uint8 = 0xdd
	safariZoneSecretHouseMap     uint8 = 0xde
	safariZoneWestRestHouseMap   uint8 = 0xdf
	safariZoneEastRestHouseMap   uint8 = 0xe0
	safariZoneNorthRestHouseMap  uint8 = 0xe1
)

// routeAvailabilityFor is Red's live topology projection. Generic blockage
// collection accepts a capability-link resolver; Red owns the meaning of its
// route capability ids and how they relate to story/badge facts.
func routeAvailabilityFor(m *emu.Emu, romData []byte) routeAvailability {
	planner, err := skill.NewRoutePlanner(m, romData)
	if err != nil {
		return routeAvailability{}
	}
	return collectRouteAvailability(planner, redRouteAvailabilityPlaceNames(), redRoutePrerequisiteLink)
}

func (a *redObjectiveAdapter) RouteRequirements(obs Observation) []RouteBlockage {
	return redRouteRequirements(obs)
}

// redRouteRequirements translates Pokémon Red's named campaign gates into the
// portable semantic destination/prerequisite contract. No generic provider has
// to know the native map byte, event flag, badge enum, or compound story owner.
func redRouteRequirements(obs Observation) []RouteBlockage {
	out := make([]RouteBlockage, 0, 24)
	blockMap := func(mapID uint8, transition string, prerequisite RoutePrerequisiteLink) {
		// A route requirement governs entry. If a checkpoint/resume is already
		// inside the map, local actions must remain available so recovery cannot
		// strand the player behind a gate that is now physically behind them.
		if mapID == obs.Map {
			return
		}
		for _, name := range skill.PlaceNames() {
			destination, ok := skill.Place(name)
			if !ok || destination.Map != mapID {
				continue
			}
			out = append(out, redStoryRouteBlockage(PlaceID(name), transition, prerequisite))
		}
	}

	if !redHasBadge(obs, state.BadgeBoulder) {
		blockMap(route3Map, "red:story:route3_boulder", RoutePrerequisiteLink{
			Capability: "can_leave_pewter_east",
			Badge:      state.BadgeBoulder.String(),
			Progress:   redProgressBoulderBadge,
		})
	}
	if !observedEvent(obs, state.EventGotPokedex.String()) {
		blockMap(route2Map, "red:story:route2_pokedex", RoutePrerequisiteLink{Progress: redProgressPokedexAcquired})
	}
	if !obs.Story.Has(redProgressSSTicketAcquired) {
		blockMap(route25Map, "red:story:route25_bill", RoutePrerequisiteLink{Progress: redProgressSSTicketAcquired})
	}
	if !redHasBadge(obs, state.BadgeThunder) {
		prerequisite := RoutePrerequisiteLink{Badge: state.BadgeThunder.String()}
		for _, mapID := range []uint8{route9Map, route10Map, lavenderTownMap, celadonCityMap} {
			blockMap(mapID, "red:story:post_surge", prerequisite)
		}
	}
	if !obs.Story.Has(redProgressPokeFluteAcquired) {
		// Route 13 and the connected south/east Fuchsia corridor are physically
		// beyond the Route 12 Snorlax gate. A resumed checkpoint on any of these
		// maps is therefore stronger evidence that the gate is already behind the
		// player than a missing derived story fact is evidence that it is closed.
		// This includes Fuchsia interiors and Safari maps: a save in the Pokémon
		// Center, for example, must be allowed to step back into Fuchsia City and
		// Route 14 even when the carried story projection lost the Poké Flute fact.
		gateBehindPlayer := redCheckpointBeyondRoute12Snorlax(obs.Map)
		if !gateBehindPlayer {
			prerequisite := RoutePrerequisiteLink{Progress: redProgressPokeFluteAcquired}
			for _, mapID := range []uint8{route12Map, route13Map, route14Map, route15Map, fuchsiaCityMap} {
				blockMap(mapID, "red:story:poke_flute", prerequisite)
			}
			out = append(out,
				redStoryRouteBlockage("route 12 snorlax", "red:story:route12_snorlax", prerequisite),
				redStoryRouteBlockage("route 12 south of snorlax", "red:story:route12_snorlax", prerequisite),
			)
		}
	}
	if !obs.Story.Has(ProgressSaffronGateOpen) {
		blockMap(saffronCityMap, "red:story:saffron_gate", RoutePrerequisiteLink{Progress: ProgressSaffronGateOpen})
	}
	if !obs.Story.Has(redProgressSilphRescueComplete) {
		blockMap(saffronGymMap, "red:story:silph_rescue", RoutePrerequisiteLink{Progress: redProgressSilphRescueComplete})
	}
	if !obs.Story.Has(ProgressSecretKeyOwned) {
		blockMap(cinnabarGymMap, "red:story:secret_key", RoutePrerequisiteLink{Progress: ProgressSecretKeyOwned})
	}
	if !obs.Story.Has(ProgressViridianGymOpen) {
		blockMap(viridianGymMap, "red:story:viridian_gym_open", RoutePrerequisiteLink{Progress: ProgressViridianGymOpen})
	}
	if obs.Map != vermilionGymMap {
		blockMap(vermilionGymMap, "red:story:vermilion_gym_cut_owner", RoutePrerequisiteLink{Capability: "can_cut", FieldCapability: "cut", Progress: redProgressHM01Acquired})
	}
	return dedupeRouteBlockages(out)
}

func redCheckpointBeyondRoute12Snorlax(mapID uint8) bool {
	switch mapID {
	case route13Map, route14Map, route15Map, route15Gate1FMap,
		fuchsiaCityMap, fuchsiaMartMap, fuchsiaBillsGrandpasHouseMap,
		fuchsiaPokemonCenterMap, wardensHouseMap, safariZoneGateMap,
		fuchsiaGymMap, fuchsiaMeetingRoomMap, fuchsiaGoodRodHouseMap,
		safariZoneEastMap, safariZoneNorthMap, safariZoneWestMap, safariZoneCenterMap,
		safariZoneCenterRestHouseMap, safariZoneSecretHouseMap, safariZoneWestRestHouseMap,
		safariZoneEastRestHouseMap, safariZoneNorthRestHouseMap:
		return true
	default:
		return false
	}
}

func redStoryRouteBlockage(destination PlaceID, transition string, prerequisite RoutePrerequisiteLink) RouteBlockage {
	blockage := RouteBlockage{Destination: destination}
	if transition != "" {
		blockage.Transitions = []string{transition}
	}
	if prerequisite != (RoutePrerequisiteLink{}) {
		blockage.Prerequisites = []RoutePrerequisiteLink{prerequisite}
	}
	return blockage
}

func dedupeRouteBlockages(in []RouteBlockage) []RouteBlockage {
	out := make([]RouteBlockage, 0, len(in))
	seen := map[string]bool{}
	for _, blockage := range in {
		if blockage.Destination == "" {
			continue
		}
		key := string(blockage.Destination) + "\x00" + routeBlockageIdentity(blockage)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, blockage)
	}
	return out
}

func redHasBadge(obs Observation, badge state.Badge) bool {
	want := badge.String()
	for _, owned := range obs.Badges {
		if owned == want {
			return true
		}
	}
	return false
}

// These native-map probes are retained for Red-focused compatibility tests.
// Production generic providers no longer call them; they consume the semantic
// RouteBlockage values emitted above.
func journeyProgressionBlocked(obs Observation, destinationMap uint8) bool {
	switch destinationMap {
	case route3Map:
		return !redHasBadge(obs, state.BadgeBoulder)
	case route2Map:
		return !observedEvent(obs, state.EventGotPokedex.String())
	case route25Map:
		return !obs.Story.Has(redProgressSSTicketAcquired)
	case route9Map, route10Map, lavenderTownMap, celadonCityMap:
		return !redHasBadge(obs, state.BadgeThunder)
	case route12Map, route13Map, route14Map, route15Map, fuchsiaCityMap:
		return !obs.Story.Has(redProgressPokeFluteAcquired)
	case saffronCityMap:
		return !obs.Story.Has(ProgressSaffronGateOpen)
	case saffronGymMap:
		return !obs.Story.Has(redProgressSilphRescueComplete)
	case cinnabarGymMap:
		return !obs.Story.Has(ProgressSecretKeyOwned)
	case viridianGymMap:
		return !obs.Story.Has(ProgressViridianGymOpen)
	case vermilionGymMap:
		return obs.Map != vermilionGymMap
	default:
		return false
	}
}

func placeProgressionBlocked(obs Observation, placeName string) bool {
	switch placeName {
	case "route 12 snorlax", "route 12 south of snorlax":
		return !obs.Story.Has(redProgressPokeFluteAcquired)
	default:
		return false
	}
}

func redRoutePrerequisiteLink(id CapabilityID) (RoutePrerequisiteLink, bool) {
	switch gameruntime.CapabilityID(id) {
	case "can_leave_viridian_north":
		return RoutePrerequisiteLink{Capability: id, Progress: redProgressPokedexAcquired}, true
	case "can_leave_pewter_east":
		return RoutePrerequisiteLink{Capability: id, Badge: state.BadgeBoulder.String(), Progress: redProgressBoulderBadge}, true
	case "can_exit_mt_moon":
		return RoutePrerequisiteLink{Capability: id, Progress: redProgressMtMoonFossilAcquired}, true
	case "can_pass_cerulean_robbed_house":
		return RoutePrerequisiteLink{Capability: id, Progress: redProgressSSTicketAcquired}, true
	case "can_board_ss_anne":
		return RoutePrerequisiteLink{Capability: id, Progress: redProgressSSTicketAcquired}, true
	case "can_cut":
		return RoutePrerequisiteLink{Capability: id, FieldCapability: "cut", Progress: redProgressHM01Acquired}, true
	case "can_surf":
		return RoutePrerequisiteLink{Capability: id, FieldCapability: "surf"}, true
	case "can_move_boulders":
		return RoutePrerequisiteLink{Capability: id, FieldCapability: "strength"}, true
	case "can_clear_snorlax":
		return RoutePrerequisiteLink{Capability: id, Progress: redProgressPokeFluteAcquired}, true
	case "can_ride_cycling_road":
		return RoutePrerequisiteLink{Capability: id, Progress: redProgressBicycleAcquired}, true
	case "can_enter_saffron":
		return RoutePrerequisiteLink{Capability: id, Progress: ProgressSaffronGateOpen}, true
	default:
		return RoutePrerequisiteLink{}, false
	}
}
