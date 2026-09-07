package agent

import "github.com/maestroi/pokepilot/red/state"

const (
	route2Map uint8 = 0x0d
	route3Map uint8 = 0x0e
)

// journeyProgressionBlocked reports a journey the game itself cannot currently
// complete. These are scripted exits: an NPC force-stops the player every frame
// until a condition is met, so the destination is not merely risky, it is
// unreachable, and the walk cannot end any way but failing. Keep such an
// objective off the planner menu until its condition is visible in the live
// observation. These gates are independent of trainer loss recovery: successful
// training changes combat readiness, but it cannot open a scripted exit.
//
//   - Route 3: PewterCityDefaultScript force-stops the east exit until Brock
//     has been beaten.
//   - Route 2: the Viridian City guard blocks the north exit until Oak's parcel
//     has been delivered, which is the moment the Pokédex is handed over — so
//     GotPokedex, not GotOaksParcel, is the observable that says the errand
//     FINISHED rather than merely started. MEASURED on
//     run-3ld1hij6u4mpjerhar4j139tt (and one more): at Viridian (19,9) with
//     GotPokedex false, "go to route 2, fleeing wild battles" spent all ten
//     dialogue recoveries on "You can't go through here!" and failed the round.
func journeyProgressionBlocked(obs Observation, destinationMap uint8) bool {
	switch destinationMap {
	case route3Map:
		return !hasBadge(obs, state.BadgeBoulder)
	case route2Map:
		return !observedEvent(obs, state.EventGotPokedex.String())
	}
	return false
}
