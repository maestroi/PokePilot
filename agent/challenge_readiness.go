package agent

import (
	"sort"
	"strings"
)

// ChallengeReadinessAction is the next preparation class for a concrete combat
// challenge. It is deliberately game-agnostic: adapters can add a small profile
// when they know useful matchup facts, while generic policy still works from
// observed party/resources and typed combat-loss evidence alone.
type ChallengeReadinessAction string

const (
	ChallengeReady       ChallengeReadinessAction = "ready"
	ChallengeHeal        ChallengeReadinessAction = "heal"
	ChallengeTrain       ChallengeReadinessAction = "train"
	ChallengeRestock     ChallengeReadinessAction = "restock"
	ChallengeChangeParty ChallengeReadinessAction = "change_party"
)

// ChallengeReadinessProfile contains optional adapter-owned knowledge about one
// challenge. Zero values mean "unknown", never "no requirement". This keeps the
// generic evaluator usable by future games without baking Red bosses into it.
type ChallengeReadinessProfile struct {
	MinimumReadiness   int      `json:"minimum_readiness,omitempty"`
	MinimumUsableMons  int      `json:"minimum_usable_mons,omitempty"`
	PreferredMoveTypes []string `json:"preferred_move_types,omitempty"`
	// MinimumSupportLevel is the floor for the strongest non-lead members: a
	// single carry cannot cover a gauntlet alone.
	MinimumSupportLevel int `json:"minimum_support_level,omitempty"`
	// RecoveryFights counts consecutive fights with no free recovery between
	// them; each needs its own share of bag healing stock.
	RecoveryFights int `json:"recovery_fights,omitempty"`
}

// ChallengeReadiness is the structured planner-facing assessment for one
// challenge. Reasons are compact semantic facts intended for explanation and
// telemetry; policy consumes Action and the numeric fields rather than parsing
// those strings.
type ChallengeReadiness struct {
	Objective         ObjectiveKey             `json:"objective"`
	Action            ChallengeReadinessAction `json:"action"`
	CurrentReadiness  int                      `json:"current_readiness,omitempty"`
	TargetReadiness   int                      `json:"target_readiness,omitempty"`
	Losses            int                      `json:"losses,omitempty"`
	PartyCount        int                      `json:"party_count,omitempty"`
	UsableParty       int                      `json:"usable_party,omitempty"`
	RecoveryAvailable bool                     `json:"recovery_available,omitempty"`
	EmergencyHeals    int                      `json:"emergency_heals,omitempty"`
	HealTarget        int                      `json:"heal_target,omitempty"`
	TrainSlot         int                      `json:"train_slot,omitempty"`
	Reasons           []string                 `json:"reasons,omitempty"`
}

func challengePreparationFor(k *Knowledge, obs Observation, challenge Objective) combatPreparationState {
	state := combatPreparationState{Current: partyCombatReadiness(obs)}
	if k == nil {
		return state
	}
	want := map[ObjectiveKey]bool{}
	for _, peer := range combatChainPeers(objectiveCatalogForObservation(obs), challenge) {
		want[peer.Key()] = true
	}
	for storage, failure := range k.Failures {
		key, mode, ok := parseFailureStorageKey(storage)
		if !ok || !want[combatRecoveryObjective(key.Objective()).Key()] {
			continue
		}
		switch mode {
		case failureModeCombatLoss, legacyFailureModeTrainerLoss, legacyFailureModeGymLoss:
			state.Active = true
			if failure.ReadinessTarget > state.Target {
				state.Target = failure.ReadinessTarget
			}
		case failureModeCombatRetry, legacyFailureModeGymRetry:
			// Retry-ready still means "lost before": keep the loss visible so
			// logistics (restock) apply, but its target was already met.
		default:
			continue
		}
		if failure.Times > state.Losses {
			state.Losses = failure.Times
		}
	}
	return state
}

// combatChainPeers returns the recovery identities sharing o's adapter-declared
// challenge chain, including o itself. Standalone objectives return only o.
func combatChainPeers(catalog ObjectiveCatalog, o Objective) []Objective {
	base := combatRecoveryObjective(o)
	key := base.Key()
	chain := ""
	for _, profile := range catalog.ChallengeProfiles {
		if profile.Objective == key {
			chain = profile.Chain
			break
		}
	}
	if chain == "" {
		return []Objective{base}
	}
	peers := []Objective{base}
	for _, profile := range catalog.ChallengeProfiles {
		if profile.Chain == chain && profile.Objective != key {
			peers = append(peers, profile.Objective.Objective())
		}
	}
	return peers
}

// weakSupportSlot returns the party slot of the weakest of the two strongest
// usable non-lead-level members when it is below floor.
func weakSupportSlot(obs Observation, floor int) (int, bool) {
	if floor <= 0 {
		return 0, false
	}
	slots := make([]int, 0, len(obs.Party))
	for slot, mon := range obs.Party {
		if mon.HP > 0 || mon.MaxHP == 0 {
			slots = append(slots, slot)
		}
	}
	if len(slots) < 2 {
		return 0, false
	}
	sort.SliceStable(slots, func(i, j int) bool { return obs.Party[slots[i]].Level > obs.Party[slots[j]].Level })
	support := slots[1:minInt(3, len(slots))]
	weakest := support[len(support)-1]
	if int(obs.Party[weakest].Level) >= floor {
		return 0, false
	}
	return weakest, true
}

func challengeProfileFor(obs Observation, challenge Objective) ChallengeReadinessProfile {
	catalog := objectiveCatalogForObservation(obs)
	key := combatRecoveryObjective(challenge).Key()
	for _, candidate := range catalog.ChallengeProfiles {
		if candidate.Objective == key {
			return candidate.Readiness
		}
	}
	if challenge.Kind == KindGym && challenge.Place != "" {
		for _, candidate := range catalog.Challenges {
			if candidate.Place == challenge.Place {
				return candidate.Readiness
			}
		}
	}
	return ChallengeReadinessProfile{}
}

func challengeProfileKnown(profile ChallengeReadinessProfile) bool {
	return profile.MinimumReadiness > 0 || profile.MinimumUsableMons > 0 || len(profile.PreferredMoveTypes) > 0 ||
		profile.MinimumSupportLevel > 0 || profile.RecoveryFights > 0
}

func challengeUsableParty(obs Observation) int {
	usable := 0
	for _, mon := range obs.Party {
		if mon.HP > 0 {
			usable++
		}
	}
	return usable
}

func challengePartyNeedsRecovery(obs Observation) bool {
	if leadOutOfPP(obs) {
		return true
	}
	for _, mon := range obs.Party {
		if mon.HP == 0 || monHurt(mon) || mon.Status != "" {
			return true
		}
	}
	return false
}

func challengeRecoveryItemAvailable(obs Observation) bool {
	for _, item := range obs.Bag {
		if item.Quantity <= 0 {
			continue
		}
		if _, ok := ppRestoreItems[item.Name]; ok && leadOutOfPP(obs) {
			return true
		}
		want, ok := fieldMedStatus[item.Name]
		if !ok {
			continue
		}
		for _, mon := range obs.Party {
			if medReaches(mon, want) {
				return true
			}
		}
	}
	return false
}

func challengeKnownCenter(obs Observation, known *Knowledge) bool {
	catalog := objectiveCatalogForObservation(obs)
	if catalog.CurrentCenter || obs.RecoveryCheckpoint != "" {
		return true
	}
	if known == nil {
		return false
	}
	for _, destination := range catalog.Destinations {
		if destination.Center && destination.Location != "" && known.Visited[destination.Location] {
			return true
		}
	}
	return false
}

func challengeCanRestockRecovery(obs Observation) bool {
	for _, raw := range obs.MartStock {
		name := strings.ToLower(strings.TrimSpace(raw))
		if _, ok := hpHealingItems[name]; ok {
			return true
		}
		if _, ok := fieldMedStatus[name]; ok {
			return true
		}
		if _, ok := ppRestoreItems[name]; ok {
			return true
		}
	}
	return false
}

func challengeLeadCoverageKnown(obs Observation) bool {
	return len(obs.LeadMoves) > 0
}

func challengeLeadHasPreferredMove(obs Observation, preferred []string) bool {
	if len(preferred) == 0 {
		return true
	}
	want := make(map[string]bool, len(preferred))
	for _, raw := range preferred {
		if value := strings.ToLower(strings.TrimSpace(raw)); value != "" {
			want[value] = true
		}
	}
	for _, move := range obs.LeadMoves {
		if move.Power > 0 && want[strings.ToLower(strings.TrimSpace(move.Type))] {
			return true
		}
	}
	return false
}

func maxReadinessTarget(values ...int) int {
	best := 0
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}

// EvaluateChallengeReadiness converts current party/resource facts, optional
// adapter matchup knowledge, and typed loss history into one actionable outcome.
// It does not predict a win: "ready" means no currently-observed preparation
// blocker is known.
func EvaluateChallengeReadiness(obs Observation, known *Knowledge, challenge Objective, profile ChallengeReadinessProfile) ChallengeReadiness {
	preparation := challengePreparationFor(known, obs, challenge)
	partyCount := len(obs.Party)
	if partyCount == 0 {
		partyCount = obs.PartyCount
	}
	usable := challengeUsableParty(obs)
	recoveryAvailable := challengeKnownCenter(obs, known) || challengeRecoveryItemAvailable(obs)
	target := maxReadinessTarget(profile.MinimumReadiness, preparation.Target)

	result := ChallengeReadiness{
		Objective:         combatRecoveryObjective(challenge).Key(),
		Action:            ChallengeReady,
		CurrentReadiness:  preparation.Current,
		TargetReadiness:   target,
		Losses:            preparation.Losses,
		PartyCount:        partyCount,
		UsableParty:       usable,
		RecoveryAvailable: recoveryAvailable,
		EmergencyHeals:    emergencyHealStock(obs),
	}

	if partyCount == 0 {
		result.Action = ChallengeChangeParty
		result.Reasons = []string{"no usable party is available for a combat challenge"}
		return result
	}

	if challengePartyNeedsRecovery(obs) {
		switch {
		case recoveryAvailable:
			result.Action = ChallengeHeal
			result.Reasons = []string{"party HP/status/PP needs recovery before committing to the challenge"}
		case challengeCanRestockRecovery(obs):
			result.Action = ChallengeRestock
			result.Reasons = []string{"party needs recovery, no known recovery is available, and the current shop can supply recovery items"}
		default:
			result.Action = ChallengeHeal
			result.Reasons = []string{"party HP/status/PP needs recovery but no known recovery source is currently available"}
		}
		return result
	}

	if profile.MinimumUsableMons > 0 && partyCount < profile.MinimumUsableMons {
		result.Action = ChallengeChangeParty
		result.Reasons = []string{"party is smaller than the challenge profile's usable-party floor"}
		return result
	}

	if len(profile.PreferredMoveTypes) > 0 && challengeLeadCoverageKnown(obs) &&
		!challengeLeadHasPreferredMove(obs, profile.PreferredMoveTypes) {
		result.Action = ChallengeChangeParty
		result.Reasons = []string{"observed lead moves do not cover any preferred challenge move type"}
		return result
	}

	// A loss with zero emergency healing stock is a useful logistics signal even
	// after blackout/Center recovery made the party healthy again. If a shop is
	// currently available, stock the bounded emergency reserve before retrying.
	if preparation.Losses > 0 && emergencyHealStock(obs) == 0 && challengeCanRestockRecovery(obs) {
		result.Action = ChallengeRestock
		result.Reasons = []string{"recent challenge loss and zero emergency healing stock"}
		return result
	}

	// A gauntlet with no free recovery between fights is decided by bag stock:
	// stock it before committing, not after the first loss.
	if heals := chainHealTarget(profile.RecoveryFights); heals > 0 && emergencyHealStock(obs) < heals && challengeCanRestockRecovery(obs) {
		result.Action = ChallengeRestock
		result.HealTarget = heals
		result.Reasons = []string{"chained fights without free recovery need bag healing stock before committing"}
		return result
	}

	if target > 0 && preparation.Current < target {
		result.Action = ChallengeTrain
		result.Reasons = []string{"measured party readiness is below the current challenge preparation target"}
		return result
	}

	if slot, ok := weakSupportSlot(obs, profile.MinimumSupportLevel); ok {
		result.Action = ChallengeTrain
		result.TrainSlot = slot
		result.Reasons = []string{"a supporting party member is below the challenge's support-level floor"}
		return result
	}

	if preparation.Losses > 0 {
		result.Reasons = []string{"prior loss recorded, but the escalated preparation target is now satisfied"}
	} else {
		result.Reasons = []string{"no observed recovery, party-composition, resource, or readiness blocker"}
	}
	return result
}

func isReadinessChallenge(obs Observation, o Objective) bool {
	return o.Kind == KindGym || o.Kind == KindTrainer || challengeProfileKnown(challengeProfileFor(obs, o))
}

func challengeReadinessForOffer(obs Observation, known *Knowledge, offer ObjectiveOffer) []ChallengeReadiness {
	keys := map[string]Objective{}
	for _, objective := range offer.Candidates {
		if isReadinessChallenge(obs, objective) {
			base := combatRecoveryObjective(objective)
			keys[base.Key().ID()] = base
		}
	}
	for _, block := range offer.Blocked {
		if block.Objective == nil {
			continue
		}
		objective := block.Objective.Objective()
		if isReadinessChallenge(obs, objective) {
			base := combatRecoveryObjective(objective)
			keys[base.Key().ID()] = base
		}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)

	out := make([]ChallengeReadiness, 0, len(ordered))
	for _, key := range ordered {
		challenge := keys[key]
		out = append(out, EvaluateChallengeReadiness(obs, known, challenge, challengeProfileFor(obs, challenge)))
	}
	return out
}
