package skill

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

const postSurgeCeladonTravelEngagements = 100

type postSurgeCeladonTravelLeg struct {
	place string
	label string
}

var postSurgeCeladonTravelLegs = []postSurgeCeladonTravelLeg{
	{place: "lavender town", label: "reach Lavender Town"},
	{place: "celadon pokemon center", label: "reach Celadon Pokemon Center"},
}

// postSurgePastLavender reports whether a resumed run is already on the
// Lavender -> Route 8 -> Underground Path -> Route 7 -> Celadon suffix of the
// badge-four route. In that case forcing the Lavender checkpoint again would
// backtrack a run that already made forward progress.
//
// Saffron is included deliberately: Adventure/Completionist play can make the
// city reachable before this story objective. If a resumed run is already
// there, the router can take the short legal route to Celadon instead of being
// forced east to Lavender first.
func postSurgePastLavender(mapID uint8) bool {
	name := state.MapName(mapID)
	switch mapID {
	case 0x04, // Lavender Town
		0x8D, // Lavender Pokemon Center
		0x13, // Route 8
		0x4F, // Route 8 gate
		0x50, // Underground Path Route 8
		0x79, // Underground Path west-east
		0x4D, // Underground Path Route 7
		0x4E, // Underground Path Route 7 copy
		0x4C, // Route 7 gate
		0x12, // Route 7
		0x0A, // Saffron City
		0x06, // Celadon City
		0x85, // Celadon Pokemon Center
		0x86: // Celadon Gym
		return true
	}
	return strings.HasPrefix(name, "LAVENDER_") ||
		strings.HasPrefix(name, "POKEMON_TOWER_") ||
		name == "MR_FUJIS_HOUSE" ||
		strings.HasPrefix(name, "SAFFRON_") ||
		strings.HasPrefix(name, "SILPH_CO_") ||
		postSurgeCeladonArea(mapID)
}

func postSurgeCeladonArea(mapID uint8) bool {
	name := state.MapName(mapID)
	return strings.HasPrefix(name, "CELADON_") ||
		name == "GAME_CORNER" ||
		strings.HasPrefix(name, "GAME_CORNER_") ||
		strings.HasPrefix(name, "ROCKET_HIDEOUT_")
}

// travelPostSurgeCeladon is retained as the composed journey helper used by
// the milestone qualification tests. Runtime progression now invokes the two
// travel legs as separate objective transactions so planner feedback and
// failure attribution happen at Lavender and Celadon instead of only after
// the entire badge-three -> badge-four route.
func travelPostSurgeCeladon(currentMap uint8, travel func(Destination) (TravelResult, error)) error {
	legs := postSurgeCeladonTravelLegs
	if postSurgePastLavender(currentMap) {
		legs = legs[1:]
	}
	for _, leg := range legs {
		dest, ok := Place(leg.place)
		if !ok {
			return fmt.Errorf("skill: PostSurgeCeladonProgression: %s place missing", leg.place)
		}
		if _, err := travel(dest); err != nil {
			return fmt.Errorf("skill: PostSurgeCeladonProgression: %s: %w", leg.label, err)
		}
	}
	return nil
}

func postSurgePrerequisites(m *emu.Emu, policy MovePolicy) (state.Mem, error) {
	if policy == nil {
		return state.Mem{}, fmt.Errorf("skill: PostSurgeCeladonProgression: nil policy")
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	badges := ram(m).DecodeProgress(&mem)
	if badges.Has(state.BadgeRainbow) {
		return mem, nil
	}
	if !badges.Has(state.BadgeThunder) {
		return state.Mem{}, fmt.Errorf("skill: PostSurgeCeladonProgression: Thunder Badge is required")
	}
	return mem, nil
}

// PostSurgeReachLavender owns only the first bounded story leg after Surge:
// repair/retain Cut and cross Route 9 + Rock Tunnel to Lavender. A resumed run
// already on the west side of Lavender satisfies the stage without walking
// backward just to replay its checkpoint.
func PostSurgeReachLavender(m *emu.Emu, romData []byte, policy MovePolicy) error {
	mem, err := postSurgePrerequisites(m, policy)
	if err != nil {
		return err
	}
	if ram(m).DecodeProgress(&mem).Has(state.BadgeRainbow) {
		return nil
	}
	currentMap := ram(m).DecodePlayer(&mem).MapID
	if postSurgePastLavender(currentMap) {
		return nil
	}
	if err := RepairUtilityFieldCapability(m, romData, policy, FieldCut); err != nil {
		return fmt.Errorf("skill: PostSurgeReachLavender: prepare Cut carrier: %w", err)
	}
	lavender, ok := Place("lavender town")
	if !ok {
		return fmt.Errorf("skill: PostSurgeReachLavender: lavender town place missing")
	}
	if _, err := TravelFlee(m, romData, lavender, policy, postSurgeCeladonTravelEngagements); err != nil {
		return fmt.Errorf("skill: PostSurgeReachLavender: reach Lavender Town: %w", err)
	}
	return nil
}

// PostSurgeReachCeladon owns the second bounded leg: from Lavender or later,
// traverse Route 8/7's Underground Path to the Celadon Center and recover the
// party. Keeping recovery in this stage gives the next Erika transaction a
// clean, positively observable starting state.
func PostSurgeReachCeladon(m *emu.Emu, romData []byte, policy MovePolicy) error {
	mem, err := postSurgePrerequisites(m, policy)
	if err != nil {
		return err
	}
	if ram(m).DecodeProgress(&mem).Has(state.BadgeRainbow) {
		return nil
	}
	currentMap := ram(m).DecodePlayer(&mem).MapID
	if !postSurgePastLavender(currentMap) {
		return fmt.Errorf("skill: PostSurgeReachCeladon: Lavender stage is incomplete from map %#04x", currentMap)
	}
	center, ok := Place("celadon pokemon center")
	if !ok {
		return fmt.Errorf("skill: PostSurgeReachCeladon: celadon pokemon center place missing")
	}
	if _, err := TravelFlee(m, romData, center, policy, postSurgeCeladonTravelEngagements); err != nil {
		return fmt.Errorf("skill: PostSurgeReachCeladon: reach Celadon Pokemon Center: %w", err)
	}
	if err := Heal(m); err != nil {
		return fmt.Errorf("skill: PostSurgeReachCeladon: heal in Celadon: %w", err)
	}
	return nil
}

// PostSurgeDefeatErika is the final local stage. It revalidates Cut because
// party/storage work can change the carrier between transactions, then owns
// only the short Center/city -> Celadon Gym approach and Erika battle.
func PostSurgeDefeatErika(m *emu.Emu, romData []byte, policy MovePolicy) error {
	mem, err := postSurgePrerequisites(m, policy)
	if err != nil {
		return err
	}
	if ram(m).DecodeProgress(&mem).Has(state.BadgeRainbow) {
		return nil
	}
	currentMap := ram(m).DecodePlayer(&mem).MapID
	if !postSurgeCeladonArea(currentMap) {
		return fmt.Errorf("skill: PostSurgeDefeatErika: Celadon-ready stage is incomplete from map %#04x", currentMap)
	}
	if err := RepairUtilityFieldCapability(m, romData, policy, FieldCut); err != nil {
		return fmt.Errorf("skill: PostSurgeDefeatErika: prepare Cut carrier: %w", err)
	}

	city, ok := Place("celadon city")
	if !ok {
		return fmt.Errorf("skill: PostSurgeDefeatErika: celadon city place missing")
	}
	if _, err := TravelFlee(m, romData, city, policy, 10); err != nil {
		return fmt.Errorf("skill: PostSurgeDefeatErika: leave Center for Celadon City: %w", err)
	}
	outcome, err := Gym(m, romData, policy)
	if err != nil {
		return fmt.Errorf("skill: PostSurgeDefeatErika: Erika: %w", err)
	}
	if outcome == state.ResultLost {
		return fmt.Errorf("skill: PostSurgeDefeatErika: %w against Erika", ErrTrainerBlackedOut)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: PostSurgeDefeatErika: Erika battle ended with outcome %d", outcome)
	}

	state.Snapshot(m, &mem)
	if !ram(m).DecodeProgress(&mem).Has(state.BadgeRainbow) {
		return fmt.Errorf("skill: PostSurgeDefeatErika: Rainbow Badge missing after Erika")
	}
	return nil
}

// PostSurgeCeladonProgression remains as a composed milestone helper for ROM
// qualification and callers outside the objective runtime. The runtime offers
// the three functions above as separate semantic progression stages.
func PostSurgeCeladonProgression(m *emu.Emu, romData []byte, policy MovePolicy) error {
	stages := []struct {
		name string
		run  func(*emu.Emu, []byte, MovePolicy) error
	}{
		{name: "reach Lavender", run: PostSurgeReachLavender},
		{name: "reach and recover in Celadon", run: PostSurgeReachCeladon},
		{name: "defeat Erika", run: PostSurgeDefeatErika},
	}
	for _, stage := range stages {
		if err := stage.run(m, romData, policy); err != nil {
			return fmt.Errorf("skill: PostSurgeCeladonProgression: %s: %w", stage.name, err)
		}
	}
	return nil
}
