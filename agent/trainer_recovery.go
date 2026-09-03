package agent

import (
	"errors"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

// trainerLossFailurePrefix marks failures where the objective reached a
// mandatory trainer battle and the party blacked out. These are different
// from ordinary ErrBlackedOut failures: a poison wipe or a lost wild battle
// does not prove that retrying the same route will hit the same unavoidable
// opponent again.
const trainerLossFailurePrefix = "trainer loss while attempting "

// trainerLossFailureKey gives a trainer loss a stable logical objective
// identity. Travel's fight/flee variants intentionally collapse to the same
// key: fleeing changes only wild encounters, while a trainer cannot be fled,
// so both variants hit the same blocker. KindGym is scoped by its internal
// Place so a loss to a gym trainer in Pewter does not disable Cerulean.
func trainerLossFailureKey(o Objective) string {
	base := o
	base.Note, base.Intent = "", ""
	if base.Kind == KindGoTo || (base.Kind == KindHeal && base.Place != "") {
		base.Flee = false
	}
	if base.Kind == KindGym && base.Place != "" {
		return trainerLossFailurePrefix + "beat the gym leader at " + strings.ToUpper(base.Place)
	}
	return trainerLossFailurePrefix + base.String()
}

// trainerLossFailureName classifies only the typed trainer-blackout outcome.
// ErrTrainerBlackedOut still unwraps to ErrBlackedOut for Run's existing
// recoverable-blackout handling, but this narrower type is the evidence that
// a specific mandatory trainer beat the unchanged party.
func trainerLossFailureName(o Objective, err error) (string, bool) {
	if err == nil || !errors.Is(err, skill.ErrTrainerBlackedOut) {
		return "", false
	}
	return trainerLossFailureKey(o), true
}

func (k *Knowledge) clearTrainerLossFailures() {
	for name := range k.Failures {
		if strings.HasPrefix(name, trainerLossFailurePrefix) {
			delete(k.Failures, name)
		}
	}
}

// filterTrainerLossBlocked removes only objectives that have already blacked
// out against a mandatory trainer since the last successful training step.
// The marker is persisted in the ordinary Failure tally, so checkpoint resume
// cannot forget it. Unrelated routes and actions remain available.
func filterTrainerLossBlocked(out []Objective, known *Knowledge) []Objective {
	filtered := make([]Objective, 0, len(out))
	for _, o := range out {
		if _, blocked := known.Failures[trainerLossFailureKey(o)]; blocked {
			continue
		}
		filtered = append(filtered, o)
	}
	return filtered
}
