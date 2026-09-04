package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/red/state"
)

// ErrIntentContradictsObservation rejects planner-authored memory that claims
// a completed gym milestone while the authoritative badge bit is still absent.
// Intent is useful strategy memory, but it must never become a parallel source
// of game state.
var ErrIntentContradictsObservation = errors.New("agent: intent contradicts the observation")

type intentGymFact struct {
	leader string
	badge  state.Badge
}

var intentGymFacts = []intentGymFact{
	{leader: "brock", badge: state.BadgeBoulder},
	{leader: "misty", badge: state.BadgeCascade},
	{leader: "lt. surge", badge: state.BadgeThunder},
	{leader: "erika", badge: state.BadgeRainbow},
	{leader: "koga", badge: state.BadgeSoul},
	{leader: "sabrina", badge: state.BadgeMarsh},
	{leader: "blaine", badge: state.BadgeVolcano},
	{leader: "giovanni", badge: state.BadgeEarth},
}

func validateIntentFacts(intent string, obs Observation) error {
	low := strings.ToLower(strings.TrimSpace(intent))
	if low == "" {
		return nil
	}
	for _, fact := range intentGymFacts {
		if hasBadge(obs, fact.badge) {
			continue
		}
		badge := strings.ToLower(fact.badge.String()) + " badge"
		leader := fact.leader
		unsupported := containsAny(low,
			"defeated "+leader,
			"having defeated "+leader,
			"after defeating "+leader,
			leader+" is defeated",
			leader+" defeated,",
			leader+" defeated;",
			leader+" defeated.",
			leader+" defeated)",
			"beaten "+leader,
			leader+" is beaten",
			"obtained "+badge,
			badge+" obtained",
			"earned "+badge,
			"got "+badge,
			"have the "+badge,
		)
		if unsupported {
			return fmt.Errorf("%w: %q claims %s/%s is complete, but Observation.Badges does not contain %s",
				ErrIntentContradictsObservation, intent, strings.ToUpper(leader), fact.badge, fact.badge)
		}
	}
	return nil
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
