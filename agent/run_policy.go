package agent

import "strings"

const (
	RiskToleranceAggressive = "aggressive"
	RiskToleranceBalanced   = "balanced"
	RiskToleranceCautious   = "cautious"

	WildEncountersPlanner = "planner"
	WildEncountersFight   = "fight"
)

// RiskToleranceProfile controls how early a run considers a free recovery
// stop worthwhile. It is deliberately separate from PlayStyleProfile: a
// speedrun can protect money/progress, and an Adventure run can still choose
// to play aggressively.
type RiskToleranceProfile struct {
	Name          string
	HealThreshold float64
}

// NormalizeRiskTolerance keeps legacy/omitted specs on the historical policy.
// Product surfaces opt new runs into Balanced explicitly.
func NormalizeRiskTolerance(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case RiskToleranceBalanced:
		return RiskToleranceBalanced
	case RiskToleranceCautious, "defensive":
		return RiskToleranceCautious
	case "", RiskToleranceAggressive:
		fallthrough
	default:
		return RiskToleranceAggressive
	}
}

func RiskTolerance(name string) RiskToleranceProfile {
	switch NormalizeRiskTolerance(name) {
	case RiskToleranceBalanced:
		return RiskToleranceProfile{Name: RiskToleranceBalanced, HealThreshold: 0.70}
	case RiskToleranceCautious:
		return RiskToleranceProfile{Name: RiskToleranceCautious, HealThreshold: 0.85}
	default:
		// 50% is the existing Offer threshold. Aggressive does not synthesize
		// any extra recovery objectives, preserving old planner menus exactly.
		return RiskToleranceProfile{Name: RiskToleranceAggressive, HealThreshold: 0.50}
	}
}

// NormalizeWildEncounters returns the stable wire value. Planner is the
// compatibility default: the ordinary menu may offer both fighting and
// fleeing travel variants. Fight removes voluntary fleeing from that menu.
func NormalizeWildEncounters(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case WildEncountersFight, "fight_all", "fight-all", "never_flee", "never-flee":
		return WildEncountersFight
	case "", WildEncountersPlanner:
		fallthrough
	default:
		return WildEncountersPlanner
	}
}

// ApplyRunPolicy applies orthogonal run controls to an already-legal semantic
// objective menu. It never creates a route or bypasses progression. Defensive
// recovery is derived only from an already-offered Pokemon Center journey;
// fight-everything simply removes the Flee execution variant.
func ApplyRunPolicy(obs Observation, offered []Objective, riskTolerance, wildEncounters string) []Objective {
	out := append([]Objective(nil), offered...)
	if risk := RiskTolerance(riskTolerance); risk.Name != RiskToleranceAggressive {
		out = applyRiskTolerance(obs, out, risk)
	}
	if NormalizeWildEncounters(wildEncounters) == WildEncountersFight {
		out = forceFightWildEncounters(out)
	}
	return out
}

func applyRiskTolerance(obs Observation, offered []Objective, profile RiskToleranceProfile) []Objective {
	if !riskRecoveryRecommended(obs, profile.HealThreshold) {
		return offered
	}

	note := riskRecoveryNote(profile.Name)
	out := append([]Objective(nil), offered...)
	hasRecovery := false
	for i := range out {
		if out[i].Kind != KindHeal {
			continue
		}
		out[i] = appendObjectiveNote(out[i], note)
		hasRecovery = true
	}

	// When ordinary Offer has not crossed its historical 50% hurt line yet,
	// derive a Heal from a Center journey that is already known legal. This is
	// what lets Balanced/Cautious recover between trainer battles without
	// changing the compatibility menu for Aggressive/legacy runs.
	if !hasRecovery {
		seen := make(map[Objective]bool, len(out))
		for _, o := range out {
			seen[policyObjectiveKey(o)] = true
		}
		recovery := make([]Objective, 0, 2)
		for _, o := range out {
			if o.Kind != KindGoTo || !naturalCenterPlace(strings.ToLower(string(o.Place))) {
				continue
			}
			heal := Objective{Kind: KindHeal, Place: o.Place, Flee: o.Flee, Note: o.Note}
			heal = appendObjectiveNote(heal, note)
			key := policyObjectiveKey(heal)
			if seen[key] {
				continue
			}
			seen[key] = true
			recovery = append(recovery, heal)
		}
		if len(recovery) > 0 {
			hasRecovery = true
			out = append(recovery, out...)
		}
	}

	if hasRecovery {
		battleNote := riskBattleNote(profile.Name)
		for i := range out {
			switch out[i].Kind {
			case KindTrainer, KindGym:
				out[i] = appendObjectiveNote(out[i], battleNote)
			}
		}
	}
	return out
}

func riskRecoveryRecommended(obs Observation, threshold float64) bool {
	if len(obs.Party) == 0 {
		return false
	}
	if leadOutOfPP(obs) {
		return true
	}
	for _, mon := range obs.Party {
		if mon.MaxHP == 0 {
			continue
		}
		if mon.HP == 0 || mon.Status != 0 {
			return true
		}
		if float64(mon.HP)/float64(mon.MaxHP) <= threshold {
			return true
		}
	}
	return false
}

func riskRecoveryNote(name string) string {
	if name == RiskToleranceCautious {
		return "(cautious risk: recover while the party is worn down; avoid a preventable blackout and protect campaign resources)"
	}
	return "(balanced risk: recover before more hard battles when practical; protect progress and resources from a preventable blackout)"
}

func riskBattleNote(name string) string {
	if name == RiskToleranceCautious {
		return "(cautious risk: a recovery stop is available and preferred before another trainer battle)"
	}
	return "(balanced risk: consider the available recovery stop before risking another trainer battle)"
}

func forceFightWildEncounters(offered []Objective) []Objective {
	out := make([]Objective, 0, len(offered))
	seen := make(map[Objective]int, len(offered))
	for _, original := range offered {
		o := original
		o.Flee = false
		key := policyObjectiveKey(o)
		if idx, ok := seen[key]; ok {
			out[idx].Note = mergePolicyNotes(out[idx].Note, o.Note)
			continue
		}
		seen[key] = len(out)
		out = append(out, o)
	}
	return out
}

func policyObjectiveKey(o Objective) Objective {
	o.Note = ""
	return o
}

func mergePolicyNotes(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "" || strings.Contains(a, b):
		return a
	default:
		return a + " " + b
	}
}
