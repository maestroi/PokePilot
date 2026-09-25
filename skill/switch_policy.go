package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

const (
	// A voluntary switch spends a whole turn, so a merely equal bench member
	// is not enough. Requiring a 50% score improvement is also the anti-ping-
	// pong rule: equivalent candidates deterministically stay put.
	switchGainNumerator   int64 = 3
	switchGainDenominator int64 = 2

	// A bench member at or below 20% HP is not a voluntary switch-in. Forced
	// replacement is different: when the active mon fainted, any live member
	// is legal and the best remaining one must be chosen.
	voluntaryMinHPDivisor uint16 = 5
)

// switchEvaluation explains how one party member looks against the current
// opponent. Generation-specific damage and typing mechanics are projected by
// game.BattleCombatStrategy; this file only combines that score with portable
// HP/status/resource considerations.
type switchEvaluation struct {
	Slot         int
	Species      uint16
	Level        uint8
	HP           uint16
	MaxHP        uint16
	Status       string
	BestMoveSlot int
	BestMove     game.BattleMoveEvaluation
	IncomingRisk int // tenths: worst opponent STAB type into this member
	FieldMoves   int
	Score        int64
}

func (e switchEvaluation) String() string {
	status := e.Status
	if status == "" {
		status = "healthy"
	}
	return fmt.Sprintf("slot=%d species=%d level=%d hp=%d/%d status=%s best-slot=%d best={%s} incoming=%0.1fx field=%d score=%d",
		e.Slot, e.Species, e.Level, e.HP, e.MaxHP, status, e.BestMoveSlot,
		e.BestMove.String(), float64(e.IncomingRisk)/10, e.FieldMoves, e.Score)
}

// switchDecision is the policy seam Battle uses before committing to FIGHT.
// Legal describes whether a voluntary switch is possible at all; Switch says
// whether the best legal candidate is materially better enough to spend the
// turn. Keeping these separate makes "stay" a deliberate decision rather
// than an absence of candidates.
type switchDecision struct {
	Legal     bool
	Switch    bool
	Slot      int
	Reason    string
	Active    switchEvaluation
	Candidate switchEvaluation
}

func chooseTacticalSwitchState(romData []byte, resources game.BattleResourcesState, b game.BattleState) switchDecision {
	strategy, err := combatStrategyForROM(romData)
	if err != nil {
		return switchDecisionWithoutStrategy(resources, "combat-strategy-unavailable")
	}
	return chooseTacticalSwitchWithStrategy(strategy, romData, resources, b)
}

// chooseTacticalSwitchWithStrategy compares the active mon with every healthy
// bench member using the selected generation's combat mechanics.
func chooseTacticalSwitchWithStrategy(
	strategy game.BattleCombatStrategy,
	romData []byte,
	resources game.BattleResourcesState,
	b game.BattleState,
) switchDecision {
	party := resources.Party
	activeSlot := resources.ActiveSlot
	decision := switchDecision{Slot: -1, Reason: "no-live-bench"}
	if strategy == nil || len(party) < 2 || activeSlot < 0 || activeSlot >= len(party) {
		return decision
	}

	decision.Active = evaluateActiveForSwitch(strategy, romData, party, activeSlot, b)
	_, defender := battleCombatants(b)
	bestSet := false
	liveBench := false
	for slot, mon := range party {
		if slot == activeSlot || mon.Fainted() {
			continue
		}
		liveBench = true
		if criticallyWeak(mon) || mon.Status == "frozen" {
			continue
		}
		eval := evaluatePartyMonForSwitch(strategy, romData, slot, mon, defender)
		// A voluntary switch into a member that cannot currently deal known
		// damage is not tactical recovery; emergency PP/faint handling remains
		// responsible for its own legality/fallback behavior.
		if eval.BestMoveSlot < 0 || eval.BestMove.ExpectedScore <= 0 {
			continue
		}
		if !bestSet || betterSwitchEvaluation(eval, decision.Candidate) {
			decision.Candidate = eval
			decision.Slot = slot
			bestSet = true
		}
	}
	decision.Legal = liveBench
	if !liveBench {
		return decision
	}
	if !bestSet {
		decision.Reason = "no-viable-bench"
		return decision
	}
	if decision.Active.BestMoveSlot < 0 || decision.Active.BestMove.ExpectedScore <= 0 {
		decision.Switch = true
		decision.Reason = "active-has-no-effective-offense"
		return decision
	}
	if decision.Candidate.Score*switchGainDenominator > decision.Active.Score*switchGainNumerator {
		decision.Switch = true
		decision.Reason = "material-matchup-improvement"
		return decision
	}
	decision.Reason = "candidate-not-materially-better"
	return decision
}

func chooseTrainingCarrySwitchState(
	romData []byte,
	resources game.BattleResourcesState,
	b game.BattleState,
	minLevel uint8,
) switchDecision {
	strategy, err := combatStrategyForROM(romData)
	if err != nil {
		return switchDecisionWithoutStrategy(resources, "combat-strategy-unavailable")
	}
	return chooseTrainingCarrySwitchWithStrategy(strategy, romData, resources, b, minLevel)
}

// chooseTrainingCarrySwitchWithStrategy is the deliberate switch-training
// variant. The weak target already earned participation; hand the fight to a
// healthy carry without applying the normal 50% material-gain threshold.
func chooseTrainingCarrySwitchWithStrategy(
	strategy game.BattleCombatStrategy,
	romData []byte,
	resources game.BattleResourcesState,
	b game.BattleState,
	minLevel uint8,
) switchDecision {
	party := resources.Party
	activeSlot := resources.ActiveSlot
	decision := switchDecision{Slot: -1, Reason: "no-training-carry"}
	if strategy == nil || len(party) < 2 || activeSlot < 0 || activeSlot >= len(party) {
		return decision
	}

	decision.Active = evaluateActiveForSwitch(strategy, romData, party, activeSlot, b)
	_, defender := battleCombatants(b)
	for slot, mon := range party {
		if slot == activeSlot || mon.Fainted() || mon.Level < minLevel || mon.Status == "frozen" {
			continue
		}
		// Training is optional work: do not send a carry below the same 50% HP
		// retreat line used by Train merely to squeeze out one more encounter.
		if mon.MaxHP > 0 && mon.HP*2 < mon.MaxHP {
			continue
		}
		eval := evaluatePartyMonForSwitch(strategy, romData, slot, mon, defender)
		if eval.BestMoveSlot < 0 || eval.BestMove.ExpectedScore <= 0 {
			continue
		}
		if decision.Slot < 0 || betterSwitchEvaluation(eval, decision.Candidate) {
			decision.Slot = slot
			decision.Candidate = eval
		}
	}
	decision.Legal = decision.Slot >= 0
	decision.Switch = decision.Legal
	if decision.Switch {
		decision.Reason = "switch-training-carry"
	}
	return decision
}

func bestReplacementSlotState(
	romData []byte,
	resources game.BattleResourcesState,
	b game.BattleState,
) (int, switchEvaluation) {
	strategy, err := combatStrategyForROM(romData)
	if err != nil {
		return firstLiveReplacement(resources)
	}
	return bestReplacementSlotWithStrategy(strategy, romData, resources, b)
}

// bestReplacementSlotWithStrategy ranks every live party member for the current
// opponent. Forced replacement does not apply voluntary HP/frozen filters.
func bestReplacementSlotWithStrategy(
	strategy game.BattleCombatStrategy,
	romData []byte,
	resources game.BattleResourcesState,
	b game.BattleState,
) (int, switchEvaluation) {
	party := resources.Party
	_, defender := battleCombatants(b)
	bestSlot := -1
	var best switchEvaluation
	for slot, mon := range party {
		if mon.Fainted() {
			continue
		}
		eval := evaluatePartyMonForSwitch(strategy, romData, slot, mon, defender)
		if bestSlot < 0 || betterSwitchEvaluation(eval, best) {
			bestSlot, best = slot, eval
		}
	}
	return bestSlot, best
}

func evaluateActiveForSwitch(
	strategy game.BattleCombatStrategy,
	romData []byte,
	party []game.BattlePartyMon,
	activeSlot int,
	b game.BattleState,
) switchEvaluation {
	attacker, defender := battleCombatants(b)
	mon := party[activeSlot]
	moves := [4]uint16{}
	pp := [4]uint8{}
	for i := range b.Moves {
		moves[i], pp[i] = uint16(b.Moves[i].ID), b.Moves[i].PP
		if b.Moves[i].Disabled {
			// Disable makes the slot unusable right now. Counting it in the
			// stay score would make the choices Battle can select look stronger
			// than they really are this turn.
			pp[i] = 0
		}
	}
	return evaluateSwitchCombatant(
		strategy, romData, activeSlot, mon.NativeSpeciesID, mon.Status,
		moves, pp, attacker, defender,
	)
}

func evaluatePartyMonForSwitch(
	strategy game.BattleCombatStrategy,
	romData []byte,
	slot int,
	mon game.BattlePartyMon,
	defender game.BattleCombatant,
) switchEvaluation {
	attacker := battlePartyCombatant(mon)
	var moves [4]uint16
	var pp [4]uint8
	for i, move := range mon.Moves {
		moves[i], pp[i] = move.NativeMoveID, move.PP
	}
	return evaluateSwitchCombatant(
		strategy, romData, slot, mon.NativeSpeciesID, mon.Status,
		moves, pp, attacker, defender,
	)
}

func evaluateSwitchCombatant(
	strategy game.BattleCombatStrategy,
	romData []byte,
	slot int,
	species uint16,
	status string,
	moves [4]uint16,
	pp [4]uint8,
	attacker, defender game.BattleCombatant,
) switchEvaluation {
	e := switchEvaluation{
		Slot: slot, Species: species, Level: attacker.Level,
		HP: attacker.HP, MaxHP: attacker.MaxHP, Status: status,
		BestMoveSlot: -1,
		IncomingRisk: strategy.IncomingTypeRisk(romData, defender, attacker),
		FieldMoves:   fieldMoveCount(strategy, moves),
	}
	for i, id := range moves {
		if id == 0 || pp[i] == 0 {
			continue
		}
		moveEval, _, err := strategy.EvaluateCombatMove(romData, attacker, defender, id, pp[i])
		if err != nil {
			continue
		}
		if e.BestMoveSlot < 0 || game.BetterBattleMove(moveEval, e.BestMove) {
			e.BestMoveSlot, e.BestMove = i, moveEval
		}
	}

	offense := e.BestMove.ExpectedScore
	if offense <= 0 {
		// Forced replacement still needs a stable ordering when a member has
		// no ordinary damaging move. Level is a bounded fallback, far below
		// real move scores but better than treating every such mon identical.
		offense = int64(attacker.Level) * 1000
	}
	hpPermille := int64(1000)
	if attacker.MaxHP > 0 {
		hpPermille = int64(attacker.HP) * 1000 / int64(attacker.MaxHP)
	}
	statusPermille := int64(statusSwitchFactor(status))
	defensePermille := int64(defensiveSwitchFactor(e.IncomingRisk))

	e.Score = offense
	e.Score = e.Score * hpPermille / 1000
	e.Score = e.Score * statusPermille / 1000
	e.Score = e.Score * defensePermille / 1000
	// One field move is normal in story parties; multiple field moves are a
	// utility signal. The active generation decides which native moves count.
	for n := 1; n < e.FieldMoves; n++ {
		e.Score = e.Score * 85 / 100
	}
	return e
}

func switchDecisionWithoutStrategy(resources game.BattleResourcesState, reason string) switchDecision {
	decision := switchDecision{Slot: -1, Reason: reason}
	if resources.ActiveSlot < 0 || resources.ActiveSlot >= len(resources.Party) {
		return decision
	}
	for slot, mon := range resources.Party {
		if slot != resources.ActiveSlot && !mon.Fainted() {
			decision.Legal = true
			break
		}
	}
	return decision
}

func firstLiveReplacement(resources game.BattleResourcesState) (int, switchEvaluation) {
	for slot, mon := range resources.Party {
		if mon.Fainted() {
			continue
		}
		eval := switchEvaluation{
			Slot: slot, Species: mon.NativeSpeciesID, Level: mon.Level,
			HP: mon.HP, MaxHP: mon.MaxHP, Status: mon.Status, BestMoveSlot: -1,
			Score: int64(mon.Level) * 1000,
		}
		return slot, eval
	}
	return -1, switchEvaluation{Slot: -1, BestMoveSlot: -1}
}

func defensiveSwitchFactor(risk int) int {
	// Neutral risk (10) -> 1000. 2x -> 500. 0.5x -> 2000. Immunity is
	// valuable but capped at 2.5x because this is a typing prior, not knowledge
	// of the opponent's exact move set.
	if risk <= 0 {
		return 2500
	}
	factor := 10000 / risk
	if factor > 2500 {
		factor = 2500
	}
	return factor
}

func statusSwitchFactor(status string) int {
	switch status {
	case "frozen":
		return 150
	case "asleep":
		return 450
	case "poisoned", "burned":
		return 700
	case "paralyzed":
		return 800
	default:
		return 1000
	}
}

func criticallyWeak(mon game.BattlePartyMon) bool {
	return mon.MaxHP > 0 && mon.HP*voluntaryMinHPDivisor <= mon.MaxHP
}

func betterSwitchEvaluation(candidate, incumbent switchEvaluation) bool {
	if candidate.Score != incumbent.Score {
		return candidate.Score > incumbent.Score
	}
	if candidate.HP != incumbent.HP {
		return candidate.HP > incumbent.HP
	}
	if candidate.BestMove.CurrentPP != incumbent.BestMove.CurrentPP {
		return candidate.BestMove.CurrentPP > incumbent.BestMove.CurrentPP
	}
	return candidate.Slot < incumbent.Slot
}

func fieldMoveCount(strategy game.BattleCombatStrategy, moves [4]uint16) int {
	count := 0
	for _, id := range moves {
		if id != 0 && strategy.IsFieldMove(id) {
			count++
		}
	}
	return count
}
