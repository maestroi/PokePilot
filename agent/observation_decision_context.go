package agent

import "encoding/json"

// DecisionContext is additive planner-facing context derived from facts that
// are already present in Observation plus the semantics of offered skills.
// It does not choose an objective: it makes comparisons the model was
// previously expected to perform across distant parts of the prompt explicit.
type DecisionContext struct {
	Training *TrainingDecisionContext `json:",omitempty"`
	Catch    *CatchDecisionContext    `json:",omitempty"`
	Economy  *EconomyDecisionContext  `json:",omitempty"`
}

// TrainingDecisionContext puts the lead and the current map's wild level band
// beside each other. A positive LeadLevelMinusWildMax means the lead already
// outlevels even the strongest wild encounter on this map; whether training is
// still worthwhile remains the planner's decision.
type TrainingDecisionContext struct {
	LeadLevel             uint8
	WildMinLevel          uint8
	WildMaxLevel          uint8
	LeadLevelMinusWildMax int
}

// CatchDecisionContext states what the current Catch skill actually does to a
// wanted target. Catch deliberately does not reuse the normal battle policy on
// a wanted encounter: it neither attacks nor weakens it and throws balls from
// full HP. This keeps the planner from reasoning as if choosing Catch will let
// an overlevelled lead chip a low-level target first.
type CatchDecisionContext struct {
	WantedTargetsAttacked bool
	WantedTargetsWeakened bool
	BallsThrownAtFullHP   bool
}

// MarshalJSON keeps Observation's existing wire shape byte-for-byte for its
// original fields and adds DecisionContext only when there is relevant local
// encounter or economy information. llmUserPrompt already json.Marshal's
// Observation, so this reaches every planner without another prompt layer.
func (o Observation) MarshalJSON() ([]byte, error) {
	type plain Observation
	return json.Marshal(struct {
		plain
		DecisionContext *DecisionContext `json:",omitempty"`
	}{
		plain:           plain(o),
		DecisionContext: decisionContextFor(o),
	})
}

func decisionContextFor(o Observation) *DecisionContext {
	ctx := &DecisionContext{}

	if o.HasGrass && len(o.Party) > 0 && len(o.WildGrass) > 0 {
		minLevel, maxLevel := o.WildGrass[0].MinLevel, o.WildGrass[0].MaxLevel
		for _, wild := range o.WildGrass[1:] {
			if wild.MinLevel < minLevel {
				minLevel = wild.MinLevel
			}
			if wild.MaxLevel > maxLevel {
				maxLevel = wild.MaxLevel
			}
		}
		lead := o.Party[0]
		ctx.Training = &TrainingDecisionContext{
			LeadLevel:             lead.Level,
			WildMinLevel:          minLevel,
			WildMaxLevel:          maxLevel,
			LeadLevelMinusWildMax: int(lead.Level) - int(maxLevel),
		}
	}

	if hasBalls(o) && len(o.WildGrass) > 0 {
		ctx.Catch = &CatchDecisionContext{
			WantedTargetsAttacked: false,
			WantedTargetsWeakened: false,
			BallsThrownAtFullHP:   true,
		}
	}

	ctx.Economy = plannerEconomyContext(o)
	if ctx.Training == nil && ctx.Catch == nil && ctx.Economy == nil {
		return nil
	}
	return ctx
}
