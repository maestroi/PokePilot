package skill

import "github.com/maestroi/pokepilot/game"

const (
	// A future specialist should matter, but not make the current fight
	// unwinnable. FutureValue is accumulated in roughly-permille units; cap its
	// switch-in penalty so a genuinely superior current counter can still be
	// used when needed.
	futureSwitchPenaltyCap int64 = 2000

	// If the active member is materially more important to a later matchup
	// than the heal target, do not deliberately spend its turn as the
	// sacrificial body. A value of 500 is the smallest unique-counter award.
	futureSacrificeProtectionGap int64 = 500
)

// BattleSequenceContext carries only information that is useful across a
// sequence of major battles. FutureOpponents are representative combat
// matchups, expressed in the same generation-neutral shape consumed by the
// combat strategy. MinimumViableParty prevents sacrifice-heal from gambling a
// turn once the usable roster is already at the caller's safety floor.
type BattleSequenceContext struct {
	FutureOpponents    []game.BattleCombatant
	MinimumViableParty int
}

func (c BattleSequenceContext) empty() bool {
	return len(c.FutureOpponents) == 0 && c.MinimumViableParty <= 0
}

// futurePreservationValues identifies unique future counters. For each future
// matchup, every live member is evaluated at full HP and with status cleared:
// the question is whether that member is strategically worth preserving after
// between-battle recovery, not whether it happens to be hurt right now.
//
// Only a strictly best member earns preservation value. A tie means the team
// has redundant coverage and neither member needs special protection. The
// award grows with the best-vs-second-best margin, so a unique hard counter is
// protected more strongly than a marginally better alternative.
func futurePreservationValues(
	strategy game.BattleCombatStrategy,
	romData []byte,
	party []game.BattlePartyMon,
	context BattleSequenceContext,
) []int64 {
	values := make([]int64, len(party))
	if strategy == nil || len(context.FutureOpponents) == 0 {
		return values
	}

	for _, defender := range context.FutureOpponents {
		bestSlot := -1
		var bestScore, secondScore int64
		for slot, mon := range party {
			if mon.Fainted() || mon.MaxHP == 0 {
				continue
			}
			futureMon := mon
			futureMon.HP = futureMon.MaxHP
			futureMon.Status = ""
			eval := evaluatePartyMonForSwitch(strategy, romData, slot, futureMon, defender)
			if eval.BestMoveSlot < 0 || eval.BestMove.ExpectedScore <= 0 || eval.Score <= 0 {
				continue
			}
			switch {
			case bestSlot < 0 || eval.Score > bestScore:
				secondScore = bestScore
				bestScore = eval.Score
				bestSlot = slot
			case eval.Score > secondScore:
				secondScore = eval.Score
			}
		}
		if bestSlot < 0 || bestScore <= secondScore {
			continue
		}

		marginPermille := int64(1000)
		if secondScore > 0 {
			marginPermille = (bestScore - secondScore) * 1000 / bestScore
		}
		values[bestSlot] += 500 + marginPermille
	}
	return values
}

func addFutureValue(eval switchEvaluation, values []int64) switchEvaluation {
	if eval.Slot >= 0 && eval.Slot < len(values) {
		eval.FutureValue = values[eval.Slot]
	}
	return eval
}

// sequenceSwitchScore discounts a future specialist as a switch-in candidate.
// The same discount on the active member makes it easier to switch a valuable
// future counter *out* of a bad current matchup.
func sequenceSwitchScore(eval switchEvaluation) int64 {
	if eval.Score <= 0 || eval.FutureValue <= 0 {
		return eval.Score
	}
	penalty := eval.FutureValue
	if penalty > futureSwitchPenaltyCap {
		penalty = futureSwitchPenaltyCap
	}
	factor := int64(1000) + penalty/2
	return eval.Score * 1000 / factor
}

func betterSequenceSwitchEvaluation(candidate, incumbent switchEvaluation) bool {
	candidateScore := sequenceSwitchScore(candidate)
	incumbentScore := sequenceSwitchScore(incumbent)
	if candidateScore != incumbentScore {
		return candidateScore > incumbentScore
	}
	if candidate.FutureValue != incumbent.FutureValue {
		return candidate.FutureValue < incumbent.FutureValue
	}
	return betterSwitchEvaluation(candidate, incumbent)
}

func sequenceViablePartyCount(party []game.BattlePartyMon) int {
	count := 0
	for _, mon := range party {
		if !mon.Fainted() && mon.HasCurrentPP() {
			count++
		}
	}
	return count
}

// bestSequenceLeadSlotWithStrategy chooses an out-of-battle lead for a known
// major-trainer matchup envelope. It uses the current HP/status/PP state, then
// applies future-preservation pressure so an almost-as-good expendable counter
// starts ahead of a specialist needed later in the gauntlet.
func bestSequenceLeadSlotWithStrategy(
	strategy game.BattleCombatStrategy,
	romData []byte,
	resources game.BattleResourcesState,
	currentOpponents []game.BattleCombatant,
	context BattleSequenceContext,
) (int, switchEvaluation) {
	if strategy == nil || len(resources.Party) == 0 || len(currentOpponents) == 0 {
		return -1, switchEvaluation{Slot: -1, BestMoveSlot: -1}
	}
	futureValues := futurePreservationValues(strategy, romData, resources.Party, context)

	bestSlot := -1
	var best switchEvaluation
	var bestScore int64
	for slot, mon := range resources.Party {
		if mon.Fainted() || criticallyWeak(mon) || mon.Status == "frozen" {
			continue
		}
		var total int64
		var representative switchEvaluation
		effective := 0
		for _, defender := range currentOpponents {
			eval := evaluatePartyMonForSwitch(strategy, romData, slot, mon, defender)
			if eval.BestMoveSlot < 0 || eval.BestMove.ExpectedScore <= 0 {
				continue
			}
			total += eval.Score
			effective++
			if representative.BestMoveSlot < 0 || betterSwitchEvaluation(eval, representative) {
				representative = eval
			}
		}
		if effective == 0 {
			continue
		}
		representative.FutureValue = futureValues[slot]
		// Average over the whole matchup envelope, including archetypes this
		// member cannot currently damage. Otherwise a narrow specialist looks
		// artificially perfect because its bad matchups disappear from the
		// denominator.
		representative.Score = total / int64(len(currentOpponents))
		score := sequenceSwitchScore(representative)
		if bestSlot < 0 || score > bestScore ||
			(score == bestScore && betterSequenceSwitchEvaluation(representative, best)) {
			bestSlot, best, bestScore = slot, representative, score
		}
	}
	return bestSlot, best
}
