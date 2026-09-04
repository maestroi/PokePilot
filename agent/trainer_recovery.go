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

// ppRecoveryDue reports whether Offer already proved that attacking PP needs
// recovery by constructing a recovery objective. This deliberately consumes
// Offer's factual result rather than re-decoding move semantics here: a Center
// heal carries the PP reason on its note, and a finite Ether/Elixer objective
// exists only when leadOutOfPP was true. If neither recovery path is actually
// available, this returns false so the planner is not left with an artificially
// empty menu.
func ppRecoveryDue(out []Objective) bool {
	for _, o := range out {
		if o.Kind == KindHeal && strings.Contains(o.Note, "lead has no PP") {
			return true
		}
		if o.Kind != KindUseItem {
			continue
		}
		for _, id := range ppRestoreItems {
			if o.Item == id {
				return true
			}
		}
	}
	return false
}

// filterTrainerLossBlocked removes objectives that are currently disproved by
// observed combat outcomes. Mandatory-trainer losses stay blocked until one
// successful Train rung as before. Gym recovery adds one bounded phase: after
// that rung, another Train objective is withheld until the ready gym retry is
// actually attempted. If the retry loses, the scoped gym-loss marker returns
// and training becomes legal again; if it wins, Knowledge.Done consumes the
// ready marker. This prevents an LLM from climbing L10 -> L12 -> ... -> L22
// without ever testing whether the last material change was already enough.
//
// The same final filter also handles PP recovery. If Offer has constructed a
// Center/Ether recovery because every real attacking move is exhausted, Train
// and Gym are withheld until that resource is restored. Status PP (for example
// GROWL) is not a reason to enter another combat objective that cannot deal
// damage.
func filterTrainerLossBlocked(out []Objective, known *Knowledge) []Objective {
	retryPlace, retryDue := gymRetryPending(known)
	ppDue := ppRecoveryDue(out)
	filtered := make([]Objective, 0, len(out))
	for _, o := range out {
		if _, blocked := known.Failures[trainerLossFailureKey(o)]; blocked {
			continue
		}
		// Offer normally removes a gym whose scoped leader-loss marker exists
		// before this filter runs. Keep the same invariant here too: recovery
		// filtering is also used directly in tests and should never allow an
		// unchanged leader rechallenge merely because it was handed a raw
		// candidate list.
		if o.Kind == KindGym && o.Place != "" {
			if _, blocked := known.Failures[gymLossFailureKey(o.Place)]; blocked {
				continue
			}
		}
		if ppDue && (o.Kind == KindTrain || o.Kind == KindGym) {
			continue
		}
		if retryDue && o.Kind == KindTrain {
			continue
		}
		if retryDue {
			switch {
			case o.Kind == KindGym:
				o = appendObjectiveNote(o, "(retry due after successful training; test the stronger party now)")
			case o.Kind == KindGoTo && retryPlace != "" && strings.EqualFold(o.Place, retryPlace):
				o = appendObjectiveNote(o, "(return for gym retry after successful training)")
			}
		}
		filtered = append(filtered, o)
	}
	return filtered
}
