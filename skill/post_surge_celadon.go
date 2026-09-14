package skill

import (
	"fmt"

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
	default:
		return false
	}
}

// travelPostSurgeCeladon breaks the unusually long badge-three -> badge-four
// walk into semantic legs. Travel intentionally caps dialogue recoveries per
// call to catch a route looping on the same box; the full Cerulean -> Route 9
// -> Rock Tunnel -> Lavender -> Route 8 -> Underground Path -> Celadon journey
// can legitimately encounter more boxes than that cap. Resetting the bounded
// recovery budget at Lavender keeps the generic anti-loop guard strict while
// still allowing the story route to complete.
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

// PostSurgeCeladonProgression owns the long handoff from the Thunder Badge to
// the Rainbow Badge as one resumable story action. The route planner is left to
// choose the concrete legal path, which means the normal semantic Saffron guard
// gate sends this journey back through Cerulean, Route 9, Rock Tunnel, Lavender,
// and the Route 8/7 Underground Path instead of letting the planner invent a
// shortcut through Saffron.
//
// Flash is deliberately not a prerequisite: Rock Tunnel darkness changes the
// presentation, not the collision topology PokePilot navigates from ROM data.
// Cut is a real prerequisite twice (Route 9 and Celadon Gym), so the stage
// repairs a compatible carrier before committing to the route. Blackouts are
// returned to the caller; because the semantic objective remains incomplete,
// the next round can resume it from whatever Pokemon Center the game selected.
func PostSurgeCeladonProgression(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: nil policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	badges := state.DecodeProgress(&mem)
	if badges.Has(state.BadgeRainbow) {
		return nil
	}
	if !badges.Has(state.BadgeThunder) {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: Thunder Badge is required")
	}

	// A Cut carrier that survived Surge is normally still present. Keep this
	// explicit anyway: party training/storage is allowed to change the roster,
	// and losing the carrier between Vermilion and Route 9 must be recoverable
	// rather than turning into another opaque navigation stall.
	if err := RepairUtilityFieldCapability(m, romData, policy, FieldCut); err != nil {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: prepare Cut carrier: %w", err)
	}

	// The complete post-Surge journey is much longer than an ordinary Travel
	// call. Stage it at Lavender so legitimate trainer/script dialogue across
	// several maps does not consume Travel's per-call anti-loop recovery budget.
	// On a retry from Route 8 or later, travelPostSurgeCeladon skips Lavender so
	// the objective remains resumable and never walks backward just to reset a
	// counter.
	state.Snapshot(m, &mem)
	currentMap := state.DecodePlayer(&mem).MapID
	if err := travelPostSurgeCeladon(currentMap, func(dest Destination) (TravelResult, error) {
		return TravelFlee(m, romData, dest, policy, postSurgeCeladonTravelEngagements)
	}); err != nil {
		return err
	}
	if err := Heal(m); err != nil {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: heal in Celadon: %w", err)
	}

	// Gym is intentionally started from the city alias so it owns Celadon's
	// exterior Cut tree and all trainer interruptions on the approach to Erika.
	city, ok := Place("celadon city")
	if !ok {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: celadon city place missing")
	}
	if _, err := TravelFlee(m, romData, city, policy, 10); err != nil {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: leave Center for Celadon City: %w", err)
	}
	outcome, err := Gym(m, romData, policy)
	if err != nil {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: Erika: %w", err)
	}
	if outcome == state.ResultLost {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: %w against Erika", ErrTrainerBlackedOut)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: Erika battle ended with outcome %d", outcome)
	}

	state.Snapshot(m, &mem)
	if !state.DecodeProgress(&mem).Has(state.BadgeRainbow) {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: Rainbow Badge missing after Erika")
	}
	return nil
}
