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
		// 50% is the existing Offer threshold. Aggressive does not add any
		// recovery preference, preserving old planner menus exactly.
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
// recovery only annotates an already-offered heal or Pokemon Center journey;
// fight-everything simply removes the Flee execution variant.
func ApplyRunPolicy(obs Observation, offered []Objective, riskTolerance, wildEncounters string) []Objective {
	out := suppressRepeatedSuccessfulFiller(obs, offered)
	if risk := RiskTolerance(riskTolerance); risk.Name != RiskToleranceAggressive {
		out = applyRiskTolerance(obs, out, risk)
	}
	if NormalizeWildEncounters(wildEncounters) == WildEncountersFight {
		out = forceFightWildEncounters(out)
		out = removeRepelEncounterAvoidance(out)
	}
	return out
}

// suppressRepeatedSuccessfulFiller is the mode-independent loop breaker for
// successful objectives that can churn forever without advancing the game.
// The long stagnation watchdog already asks a strategic planner to reconsider,
// but that replan used to receive the exact same completed travel/shop filler
// that created the stall. A model could therefore make a different-looking
// plan that still alternated Route 22/Viridian or repeatedly bought the same
// optional item until the second watchdog window killed the run.
//
// History is intentionally the evidence boundary: only an objective completed
// at least twice in the recent bounded history is suppressed. Fight/flee travel
// variants share one semantic key because choosing the other encounter policy
// is not a new destination. Progression and recovery actions stay repeatable;
// an unvisited frontier also stays eligible. If suppression would remove the
// whole menu, keep the legal menu unchanged rather than manufacturing a dead
// end.
func suppressRepeatedSuccessfulFiller(obs Observation, offered []Objective) []Objective {
	if len(offered) <= 1 || len(obs.History) < 2 {
		return append([]Objective(nil), offered...)
	}

	completed := map[string]int{}
	for _, round := range obs.History {
		if round.Outcome != "done" {
			continue
		}
		completed[runPolicyHistoryKey(round.Objective)]++
	}

	repeated := map[string]bool{}
	for key, count := range completed {
		if count >= 2 {
			repeated[key] = true
		}
	}
	if len(repeated) == 0 {
		return append([]Objective(nil), offered...)
	}

	out := make([]Objective, 0, len(offered))
	for _, objective := range offered {
		if repeated[runPolicyHistoryKey(objective.String())] && !runPolicyRepeatableAfterSuccess(obs, objective) {
			continue
		}
		out = append(out, objective)
	}
	if len(out) == 0 {
		return append([]Objective(nil), offered...)
	}
	return out
}

func runPolicyHistoryKey(objective string) string {
	key := strings.ToLower(strings.TrimSpace(objective))
	return strings.TrimSuffix(key, ", fleeing wild battles")
}

func runPolicyRepeatableAfterSuccess(obs Observation, objective Objective) bool {
	switch objective.Kind {
	case KindProgress, KindGym, KindHeal, KindUseItem, KindTrain, KindCatch:
		return true
	case KindGoTo:
		return strings.Contains(strings.ToLower(objective.Note), "unvisited adjacent map")
	case KindBuy:
		// Repeated resupply can be real recovery after battles. Optional shop
		// churn (the farm's repeated PARLYZ HEAL case) has no such evidence.
		economy := EconomyContext(obs)
		return economy != nil && economy.ResupplyNeeded
	default:
		return false
	}
}

func applyRiskTolerance(obs Observation, offered []Objective, profile RiskToleranceProfile) []Objective {
	if !riskRecoveryRecommended(obs, profile.HealThreshold) {
		return offered
	}

	note := riskRecoveryNote(profile.Name)
	out := append([]Objective(nil), offered...)
	hasRecovery := false
	for i := range out {
		switch {
		case out[i].Kind == KindHeal:
			out[i] = appendObjectiveNote(out[i], note)
			hasRecovery = true
		case out[i].Kind == KindGoTo && naturalCenterPlace(strings.ToLower(string(out[i].Place))):
			// Keep the objective itself exactly on the raw Offer menu. Persistent
			// strategic plans are validated against that raw menu, so inventing a
			// synthetic remote KindHeal here would make a good defensive plan look
			// invalid. Going to the known Center is the legal first half; once
			// there, ordinary Offer exposes the local Heal action.
			out[i] = appendObjectiveNote(out[i], note)
			hasRecovery = true
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
		if mon.HP == 0 || mon.Status != "" {
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


func removeRepelEncounterAvoidance(offered []Objective) []Objective {
	out := make([]Objective, 0, len(offered))
	for _, o := range offered {
		if !isSpeedrunRepelObjective(o) {
			out = append(out, o)
		}
	}
	return out
}
