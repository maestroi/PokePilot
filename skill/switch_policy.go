package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/combat"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
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
// opponent. Score combines its best expected-damage move with current HP,
// major status, defensive typing against the opponent's STAB types, and a
// small penalty for carrying multiple field moves. The detailed best-move
// evaluation remains the shared red/combat model from #49.
type switchEvaluation struct {
	Slot         int
	Species      uint8
	Level        uint8
	HP           uint16
	MaxHP        uint16
	Status       string
	BestMoveSlot int
	BestMove     combat.MoveEvaluation
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

// chooseTacticalSwitch compares the active mon with every healthy bench
// member. It is called only while the real battle main menu is up, which is
// the ROM's practical switch-legality boundary: trapping/multi-turn states
// that deny a choice never present this menu to Battle in the first place.
func chooseTacticalSwitch(romData []byte, mem *state.Mem, b state.BattleState) switchDecision {
	party := state.DecodeParty(mem)
	activeSlot := int(mem.U8(sym.PlayerMonNumber))
	decision := switchDecision{Slot: -1, Reason: "no-live-bench"}
	if len(party.Mons) < 2 || activeSlot < 0 || activeSlot >= len(party.Mons) {
		return decision
	}

	decision.Active = evaluateActiveForSwitch(romData, party, activeSlot, b)
	_, defender := combat.PlayerMatchup(b)
	bestSet := false
	liveBench := false
	for slot, mon := range party.Mons {
		if slot == activeSlot || mon.Fainted() {
			continue
		}
		liveBench = true
		if criticallyWeak(mon) || mon.StatusName() == "frozen" {
			continue
		}
		eval := evaluatePartyMonForSwitch(romData, slot, mon, defender)
		// A voluntary switch into a member that cannot currently deal known
		// damage is not tactical recovery; the emergency all-PP path in
		// Battle remains responsible for its own legality/fallback behavior.
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

// bestReplacementSlot ranks every live party member for the current opponent.
// Forced replacement does not apply the voluntary HP/frozen filters because
// the player must send something out. If ROM scoring cannot distinguish the
// candidates, deterministic party order breaks the tie.
func bestReplacementSlot(romData []byte, mem *state.Mem, b state.BattleState) (int, switchEvaluation) {
	party := state.DecodeParty(mem)
	_, defender := combat.PlayerMatchup(b)
	bestSlot := -1
	var best switchEvaluation
	for slot, mon := range party.Mons {
		if mon.Fainted() {
			continue
		}
		eval := evaluatePartyMonForSwitch(romData, slot, mon, defender)
		if bestSlot < 0 || betterSwitchEvaluation(eval, best) {
			bestSlot, best = slot, eval
		}
	}
	return bestSlot, best
}

func evaluateActiveForSwitch(romData []byte, party state.PartyState, activeSlot int, b state.BattleState) switchEvaluation {
	attacker, defender := combat.PlayerMatchup(b)
	mon := party.Mons[activeSlot]
	moves := [4]uint8{}
	pp := [4]uint8{}
	for i := range b.Moves {
		moves[i], pp[i] = b.Moves[i].ID, b.Moves[i].PP
		if b.DisabledMove == uint8(i+1) {
			// Disable makes the slot unusable right now. Counting it in the
			// stay score would make a trapped weak move set look healthier than
			// the choices Battle can actually select this turn.
			pp[i] = 0
		}
	}
	return evaluateSwitchCombatant(romData, activeSlot, mon.Species, mon.StatusName(), moves, pp, attacker, defender)
}

func evaluatePartyMonForSwitch(romData []byte, slot int, mon state.Mon, defender combat.Combatant) switchEvaluation {
	attacker := combat.Combatant{
		Level: mon.Level, HP: mon.HP, MaxHP: mon.MaxHP,
		Attack: mon.Attack, Defense: mon.Defense, Special: mon.Special,
		Type1: mon.Type1, Type2: mon.Type2,
	}
	return evaluateSwitchCombatant(romData, slot, mon.Species, mon.StatusName(), mon.Moves, mon.PP, attacker, defender)
}

func evaluateSwitchCombatant(romData []byte, slot int, species uint8, status string, moves, pp [4]uint8, attacker, defender combat.Combatant) switchEvaluation {
	e := switchEvaluation{
		Slot: slot, Species: species, Level: attacker.Level,
		HP: attacker.HP, MaxHP: attacker.MaxHP, Status: status,
		BestMoveSlot: -1, IncomingRisk: incomingTypeRisk(romData, defender, attacker),
		FieldMoves: fieldMoveCount(moves),
	}
	for i, id := range moves {
		if id == 0 || pp[i] == 0 {
			continue
		}
		mv, err := rom.LookupMove(romData, id)
		if err != nil {
			continue
		}
		moveEval, err := combat.EvaluateMove(romData, attacker, defender, mv, pp[i])
		if err != nil {
			continue
		}
		if e.BestMoveSlot < 0 || combat.BetterMove(moveEval, e.BestMove) {
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
	// useful signal that this member is serving an overworld role. Penalize
	// only the extras, and only modestly: Surf/Strength can still make an HM
	// carrier the best battler when their actual combat score warrants it.
	for n := 1; n < e.FieldMoves; n++ {
		e.Score = e.Score * 85 / 100
	}
	return e
}

func incomingTypeRisk(romData []byte, enemy, candidate combat.Combatant) int {
	risk := rom.NeutralEffect
	if v, err := rom.TypeEffectiveness(romData, enemy.Type1, candidate.Type1, candidate.Type2); err == nil {
		risk = v
	}
	if enemy.Type2 != enemy.Type1 {
		if v, err := rom.TypeEffectiveness(romData, enemy.Type2, candidate.Type1, candidate.Type2); err == nil && v > risk {
			risk = v
		}
	}
	return risk
}

func defensiveSwitchFactor(risk int) int {
	// Neutral risk (10) -> 1000. 2x -> 500. 0.5x -> 2000. Immunity is
	// valuable but capped at 2.5x because we do not know the opponent's exact
	// move set here; its species typing is a defensive prior, not clairvoyance.
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

func criticallyWeak(mon state.Mon) bool {
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

func fieldMoveCount(moves [4]uint8) int {
	count := 0
	for _, id := range moves {
		switch id {
		case 0x0f, // CUT
			0x13, // FLY
			0x39, // SURF
			0x46, // STRENGTH
			0x94: // FLASH
			count++
		}
	}
	return count
}
