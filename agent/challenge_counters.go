package agent

import (
	"sort"

	"github.com/maestroi/pokepilot/game"
)

// counterLevelMargin is how far under the opposing team's top level a counter
// may sit and still be judged strong enough to carry the matchup. It is a
// portable policy constant, not a per-challenge fact.
const counterLevelMargin = 3

// challengeCounter is a usable party member whose known moves include a
// damaging move of one of the challenge's preferred attack types.
type challengeCounter struct {
	Slot  int
	Level int
	Type  string
}

func moveDamages(m PartyMove) bool { return m.Power > 0 && m.PP > 0 }

// partyCounters lists usable counters, strongest first (ties: earlier slot).
func partyCounters(obs Observation, m game.Matchup) []challengeCounter {
	var out []challengeCounter
	for slot, mon := range obs.Party {
		if mon.HP == 0 || mon.IsEgg {
			continue
		}
		for _, mv := range mon.Moves {
			if moveDamages(mv) && m.Prefers(game.TypeID(mv.Type)) {
				out = append(out, challengeCounter{Slot: slot, Level: int(mon.Level), Type: mv.Type})
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Level > out[j].Level })
	return out
}

// partyHasUsefulDamage reports whether any usable member can deal more than
// token damage: a damaging move whose type the matchup does not mark useless.
// A party whose every attack is useless (Thunder Shock into Brock) cannot
// make progress by levels alone.
func partyHasUsefulDamage(obs Observation, m game.Matchup) bool {
	for _, mon := range obs.Party {
		if mon.HP == 0 || mon.IsEgg {
			continue
		}
		for _, mv := range mon.Moves {
			if moveDamages(mv) && !m.IsUseless(game.TypeID(mv.Type)) {
				return true
			}
		}
	}
	return false
}

// partyMovesKnown is false for observations that do not decode per-member
// moves; matchup readiness then stays silent instead of guessing.
func partyMovesKnown(obs Observation) bool {
	for _, mon := range obs.Party {
		if len(mon.Moves) > 0 {
			return true
		}
	}
	return false
}

func counterLevelTarget(m game.Matchup) int {
	if t := m.MaxLevel - counterLevelMargin; t > 1 {
		return t
	}
	return 1
}

// CounterNeed is an open counter campaign: a challenge the party lost whose
// ROM matchup it cannot carry, and the deterministic next step.
type CounterNeed struct {
	Challenge ObjectiveKey
	Action    ChallengeReadinessAction // acquire_counter or train_counter
	Matchup   game.Matchup
	Slot      int // train_counter: the party slot to train
	Target    int // the level a counter should reach
}

// counterNeedFor returns the first (by key) recorded combat loss whose
// readiness asks for a counter. A campaign is only opened by evidence: a
// loss, not merely a profile, so early routes are never stalled by a League
// matchup the party will not face for hours.
func counterNeedFor(obs Observation, known *Knowledge) (CounterNeed, bool) {
	if known == nil {
		return CounterNeed{}, false
	}
	var keys []ObjectiveKey
	seen := map[string]bool{}
	for storage := range known.Failures {
		key, mode, ok := parseFailureStorageKey(storage)
		if !ok {
			continue
		}
		switch mode {
		case failureModeCombatLoss, legacyFailureModeTrainerLoss, legacyFailureModeGymLoss:
		default:
			continue
		}
		key = combatRecoveryObjective(key.Objective()).Key()
		if !seen[key.ID()] {
			seen[key.ID()] = true
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].ID() < keys[j].ID() })
	for _, key := range keys {
		obj := key.Objective()
		profile := challengeProfileFor(obs, obj)
		r := EvaluateChallengeReadiness(obs, known, obj, profile)
		if r.Action == ChallengeAcquireCounter || r.Action == ChallengeTrainCounter {
			return CounterNeed{Challenge: key, Action: r.Action, Matchup: profile.Matchup, Slot: r.CounterSlot, Target: r.CounterTarget}, true
		}
	}
	return CounterNeed{}, false
}

// counterLearnLevel is the earliest level at which species learns a damaging
// move of a preferred type no later than target; ok is false otherwise.
func counterLearnLevel(learnsets map[SpeciesID][]LearnableMove, species SpeciesID, m game.Matchup, target int) (LearnableMove, bool) {
	var best LearnableMove
	found := false
	for _, mv := range learnsets[species] {
		if mv.Level > target || !m.Prefers(game.TypeID(mv.Type)) {
			continue
		}
		if !found || mv.Level < best.Level {
			best, found = mv, true
		}
	}
	return best, found
}

// counterPreparationObjective chooses the next counter-campaign step from the
// offered menu. It never manufactures objectives: a catch must already be an
// offered, executable catch; training must already be offered or reachable.
func counterPreparationObjective(obs Observation, offered []Objective, known *Knowledge, need CounterNeed) (Objective, bool) {
	switch need.Action {
	case ChallengeAcquireCounter:
		var pick Objective
		found, bestLevel := false, 0
		for _, o := range offered {
			if o.Kind != KindCatch || o.Species == "" {
				continue
			}
			mv, ok := counterLearnLevel(obs.Learnsets, o.Species, need.Matchup, need.Target)
			if ok && (!found || mv.Level < bestLevel) {
				pick, found, bestLevel = o, true, mv.Level
			}
		}
		return pick, found
	case ChallengeTrainCounter:
		for _, o := range offered {
			if o.Kind != KindTrain {
				continue
			}
			if (need.Slot == 0 && o.Species == "" && o.Slot == 0) || (need.Slot > 0 && o.Slot == need.Slot) {
				return o, true
			}
		}
		if journey, _, ok := bestKnownTrainingJourney(obs, known, offered); ok {
			return journey, true
		}
	}
	return Objective{}, false
}

// counterCampaignActionable reports whether the open campaign can act from
// here: a reachable counter catch is known, or a counter only needs levels
// that can actually be gained from here. A train-counter campaign is not
// actionable in a sealed room (no grass, no routable habitat); treating it as
// always actionable locked the fight and emptied the menu into "nothing is
// possible from here" (run-2g0fgqbf: Lance loss in Agatha's room, regression
// of #2514).
func counterCampaignActionable(obs Observation, need CounterNeed) bool {
	if need.Action == ChallengeTrainCounter {
		return trainingPossibleHere(obs)
	}
	return len(obs.CounterCandidates) > 0
}

// trainingPossibleHere reports whether the party can make training progress
// from the current location: local grass yields XP, or a learned habitat is
// currently routable and viable. It is the honest answer to "can we train a
// counter from here", which a sealed room must answer no.
func trainingPossibleHere(obs Observation) bool {
	if trainingYieldsXP(obs) {
		return true
	}
	for _, area := range obs.TrainingAreaChoices {
		if area.Selected {
			return true
		}
	}
	return false
}

// challengeCounterReady reports that the party already holds a counter at
// the matchup's level target for challenge, or that the challenge has no
// matchup to satisfy.
func challengeCounterReady(obs Observation, challenge Objective) bool {
	m := challengeProfileFor(obs, challenge).Matchup
	if !m.Known() || len(m.Preferred) == 0 || !partyMovesKnown(obs) {
		return true
	}
	counters := partyCounters(obs, m)
	return len(counters) > 0 && counters[0].Level >= counterLevelTarget(m)
}

func failureObjectiveKey(f Failure) (ObjectiveKey, bool) {
	if f.Key == nil {
		return ObjectiveKey{}, false
	}
	return *f.Key, true
}
