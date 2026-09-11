package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

const postSurgeCeladonTravelEngagements = 100

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

	center, ok := Place("celadon pokemon center")
	if !ok {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: celadon pokemon center place missing")
	}
	if _, err := TravelFlee(m, romData, center, policy, postSurgeCeladonTravelEngagements); err != nil {
		return fmt.Errorf("skill: PostSurgeCeladonProgression: reach Celadon Pokemon Center: %w", err)
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
