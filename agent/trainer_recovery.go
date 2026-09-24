package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

func combatRecoveryObjective(o Objective) Objective {
	base := o
	base.Note = ""
	if base.Kind == KindGoTo || (base.Kind == KindHeal && base.Place != "") {
		// Flee changes only wild-encounter policy. A mandatory trainer cannot
		// be fled, so both journey variants share one combat recovery identity.
		base.Flee = false
	}
	return base
}

func combatLossFailureKey(o Objective) string {
	return failureStorageKey(combatRecoveryObjective(o).Key(), failureModeCombatLoss)
}

func combatRetryReadyKey(o Objective) string {
	return failureStorageKey(combatRecoveryObjective(o).Key(), failureModeCombatRetry)
}

// combatLossFailureName recognizes only typed combat evidence. It deliberately
// does not classify a plain ErrBlackedOut: wild losses and poison wipes are
// logistics outcomes, not proof that the objective is blocked by combat.
func combatLossFailureName(o Objective, err error) (string, bool) {
	if err == nil {
		return "", false
	}
	var required *skill.RequiredBattleError
	if errors.As(err, &required) && errors.Is(err, skill.ErrBlackedOut) {
		return combatLossFailureKey(o), true
	}
	if errors.Is(err, skill.ErrTrainerBlackedOut) {
		return combatLossFailureKey(o), true
	}
	return "", false
}

func combatLossRecorded(k *Knowledge, o Objective) bool {
	if k == nil {
		return false
	}
	base := combatRecoveryObjective(o)
	if _, ok := k.Failures[combatLossFailureKey(base)]; ok {
		return true
	}
	// Read-only migration support for structured pre-generic modes.
	if _, ok := k.Failures[legacyTrainerLossStorageKey(base)]; ok {
		return true
	}
	if _, ok := k.Failures[legacyTrainerLossStringKey(base)]; ok {
		return true
	}
	if base.Kind == KindGym && base.Place != "" {
		if _, ok := k.Failures[legacyGymLossStorageKey(string(base.Place))]; ok {
			return true
		}
		if _, ok := k.Failures[legacyGymLossStringKey(string(base.Place))]; ok {
			return true
		}
	}
	return false
}

func mergeCombatRetryFailure(a, b Failure) Failure {
	if b.Times > a.Times || a.Objective == "" {
		return b
	}
	return a
}

// Combat preparation uses a portable party-readiness score rather than
// boss-specific level tables. The strongest three party members contribute
// with diminishing weight, so a solo run can satisfy the target by making its
// one carry substantially stronger while a broader party can improve through
// useful secondary members too.
const maxCombatReadiness = 700 // 4*100 + 2*100 + 1*100

type combatPreparationState struct {
	Current int
	Target  int
	Losses  int
	Active  bool
}

func partyCombatReadiness(obs Observation) int {
	var top [3]int
	for _, mon := range obs.Party {
		level := int(mon.Level)
		switch {
		case level > top[0]:
			top[2], top[1], top[0] = top[1], top[0], level
		case level > top[1]:
			top[2], top[1] = top[1], level
		case level > top[2]:
			top[2] = level
		}
	}
	return top[0]*4 + top[1]*2 + top[2]
}

func combatPreparationLevelGain(losses, partyCount int) int {
	if losses < 1 {
		losses = 1
	}
	// A first full combat loss asks for roughly three lead levels of real
	// improvement. Repeated losses escalate by two levels instead of retrying
	// after another token +1/+2 grind. Thin parties get extra margin because
	// they have no healthy fallback when the carry hits a bad matchup.
	levels := 3 + (losses-1)*2
	switch {
	case partyCount <= 1:
		levels += 2
	case partyCount == 2:
		levels++
	}
	if levels > 10 {
		levels = 10
	}
	return levels
}

func stampCombatPreparation(f *Failure, obs Observation) {
	if f == nil {
		return
	}
	baseline := partyCombatReadiness(obs)
	if baseline <= 0 {
		f.ReadinessBaseline, f.ReadinessTarget = 0, 0
		return
	}
	partyCount := len(obs.Party)
	if partyCount == 0 {
		partyCount = obs.PartyCount
	}
	target := baseline + combatPreparationLevelGain(f.Times, partyCount)*4
	if target > maxCombatReadiness {
		target = maxCombatReadiness
	}
	f.ReadinessBaseline, f.ReadinessTarget = baseline, target
}

func combatPreparationFor(k *Knowledge, obs Observation) combatPreparationState {
	state := combatPreparationState{Current: partyCombatReadiness(obs)}
	if k == nil {
		return state
	}
	for storage, failure := range k.Failures {
		_, mode, ok := parseFailureStorageKey(storage)
		if !ok {
			continue
		}
		switch mode {
		case failureModeCombatLoss, legacyFailureModeTrainerLoss, legacyFailureModeGymLoss:
			state.Active = true
			if failure.Times > state.Losses {
				state.Losses = failure.Times
			}
			if failure.ReadinessTarget > state.Target {
				state.Target = failure.ReadinessTarget
			}
		}
	}
	return state
}

// hasCombatLossEvidence reports any typed combat loss not yet cleared by a win,
// including losses already promoted to retry-ready. Done() clears both modes.
func (k *Knowledge) hasCombatLossEvidence() bool {
	if k == nil {
		return false
	}
	for storage := range k.Failures {
		_, mode, ok := parseFailureStorageKey(storage)
		if !ok {
			continue
		}
		switch mode {
		case failureModeCombatLoss, failureModeCombatRetry,
			legacyFailureModeTrainerLoss, legacyFailureModeGymLoss, legacyFailureModeGymRetry:
			return true
		}
	}
	return false
}

func combatPreparationNote(k *Knowledge, obs Observation) string {
	state := combatPreparationFor(k, obs)
	if !state.Active {
		return ""
	}
	if state.Target > 0 {
		return fmt.Sprintf("(combat preparation after %d loss(es): readiness %d/%d; retry remains locked until the target is reached)",
			state.Losses, state.Current, state.Target)
	}
	return "(combat preparation after a loss: make material party progress before retrying)"
}

// annotateCombatPreparation makes the campaign visible to the chooser. When
// local grinding is not viable it annotates journeys instead of unlocking the
// failed fight, so the planner can deliberately seek a stronger encounter area.
func annotateCombatPreparation(obs Observation, known *Knowledge, out []Objective) []Objective {
	note := combatPreparationNote(known, obs)
	if note == "" {
		return out
	}
	hasTrain := false
	for i := range out {
		switch out[i].Kind {
		case KindTrain, KindTrainer:
			out[i] = appendObjectiveNote(out[i], note)
			if out[i].Kind == KindTrain {
				hasTrain = true
			}
		}
	}
	if hasTrain {
		return out
	}
	for i := range out {
		if out[i].Kind == KindGoTo {
			out[i] = appendObjectiveNote(out[i], "(combat preparation active; no viable local training here, seek stronger encounters before retrying)")
		}
	}
	return out
}

// combatPreparationObjective keeps an active recovery campaign deterministic
// while useful local work exists: heal first when needed, otherwise keep
// training. If the current area cannot train efficiently, return no forced
// choice and let the planner pick a journey using the annotations above.
func combatPreparationObjective(obs Observation, offered []Objective, known *Knowledge, readiness []ChallengeReadiness) (Objective, bool) {
	state := combatPreparationFor(known, obs)
	if !state.Active || (state.Target > 0 && state.Current >= state.Target) {
		return Objective{}, false
	}
	if partyHurt(obs) || leadOutOfPP(obs) {
		for _, o := range offered {
			if o.Kind == KindHeal {
				return o, true
			}
		}
	}
	// A loss with zero HP-healing stock is a logistics gap training cannot
	// fix: chained fights (no Center between them) can only heal from the bag.
	// Economy offers a bounded healing buy only while stock is below target.
	if emergencyHealStock(obs) == 0 {
		for _, o := range offered {
			if _, ok := hpHealingItems[string(o.Item)]; ok && o.Kind == KindBuy {
				return o, true
			}
		}
	}
	o, ok, slotOnly := preferredTrainingObjective(offered, readiness)
	if ok || slotOnly {
		return o, ok
	}
	if journey, _, ok := bestKnownTrainingJourney(obs, known, offered); ok {
		return journey, true
	}
	return Objective{}, false
}

// preferredTrainingObjective trains a readiness-flagged weak support member
// when that slot's training is offered, else the lead, else any training.
// slotOnly reports a support-slot request: leveling the carry cannot satisfy
// it, so callers leave the choice to the planner (fail open) instead of
// grinding the lead or forcing a journey that may not help.
func preferredTrainingObjective(offered []Objective, readiness []ChallengeReadiness) (Objective, bool, bool) {
	slotOnly := false
	for _, assessment := range readiness {
		if assessment.Action != ChallengeTrain || assessment.TrainSlot == 0 {
			continue
		}
		slotOnly = true
		for _, o := range offered {
			if o.Kind == KindTrain && o.Slot == assessment.TrainSlot {
				return o, true, true
			}
		}
	}
	if slotOnly {
		return Objective{}, false, true
	}
	for _, o := range offered {
		if o.Kind == KindTrain && o.Species == "" && o.Slot == 0 {
			return o, true, false
		}
	}
	for _, o := range offered {
		if o.Kind == KindTrain {
			return o, true, false
		}
	}
	return Objective{}, false, false
}

// promoteCombatLossesToRetryWhere is the only live loss->retry state
// transition. Quantified recovery calls it with a readiness predicate; legacy
// callers can still release their unquantified gates after one material party
// change. Existing retry-ready checkpoint modes are always migrated.
func (k *Knowledge) promoteCombatLossesToRetryWhere(ready func(Failure) bool) {
	if k == nil {
		return
	}
	retries := map[ObjectiveKey]Failure{}
	for storage, f := range k.Failures {
		if key, mode, ok := parseFailureStorageKey(storage); ok {
			switch mode {
			case failureModeCombatLoss, legacyFailureModeTrainerLoss, legacyFailureModeGymLoss:
				if ready != nil && !ready(f) {
					continue
				}
				key = combatRecoveryObjective(key.Objective()).Key()
				retries[key] = mergeCombatRetryFailure(retries[key], f)
				delete(k.Failures, storage)
			case legacyFailureModeGymRetry:
				// Legacy retry-ready state is migrated on sight.
				key = combatRecoveryObjective(key.Objective()).Key()
				retries[key] = mergeCombatRetryFailure(retries[key], f)
				delete(k.Failures, storage)
			}
			continue
		}
		if strings.HasPrefix(storage, legacyGymLossFailurePrefix) {
			if ready != nil && !ready(f) {
				continue
			}
			place := strings.ToLower(strings.TrimPrefix(storage, legacyGymLossFailurePrefix))
			if place != "" {
				key := combatRecoveryObjective(Objective{Kind: KindGym, Place: PlaceID(place)}).Key()
				retries[key] = mergeCombatRetryFailure(retries[key], f)
			}
			delete(k.Failures, storage)
			continue
		}
		if strings.HasPrefix(storage, legacyTrainerLossFailurePrefix) {
			if ready == nil || ready(f) {
				// v4 trainer-loss strings did not persist a structural key. Their
				// historical post-training behavior was simply to release the gate.
				delete(k.Failures, storage)
			}
			continue
		}
		// v4 checkpoints used the generic gym display sentence as the retry key
		// and kept the place only in Last.
		if storage == (Objective{Kind: KindGym}).String() {
			const prefix = "party trained after losing at "
			const suffix = "; retry is due"
			if strings.HasPrefix(f.Last, prefix) && strings.HasSuffix(f.Last, suffix) {
				place := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(f.Last, prefix), suffix))
				if place != "" {
					key := combatRecoveryObjective(Objective{Kind: KindGym, Place: PlaceID(place)}).Key()
					retries[key] = mergeCombatRetryFailure(retries[key], f)
					delete(k.Failures, storage)
				}
			}
		}
	}
	for key, retry := range retries {
		o := key.Objective()
		retry.Objective = o.String()
		retry.Last = "combat preparation target reached after defeat; retry is due"
		storage := combatRetryReadyKey(o)
		retry = mergeCombatRetryFailure(k.Failures[storage], retry)
		k.Failures[storage] = retry
	}
}

func (k *Knowledge) promoteCombatLossesToRetry() {
	k.promoteCombatLossesToRetryWhere(func(Failure) bool { return true })
}

// combatRetryKeys returns retry-ready objectives while honoring legacy
// checkpoint modes. A fresh loss for the same normalized key always wins over
// an older ready marker.
func combatRetryKeys(k *Knowledge) map[ObjectiveKey]bool {
	ready := map[ObjectiveKey]bool{}
	lost := map[ObjectiveKey]bool{}
	if k == nil {
		return ready
	}
	for storage, f := range k.Failures {
		if key, mode, ok := parseFailureStorageKey(storage); ok {
			key = combatRecoveryObjective(key.Objective()).Key()
			switch mode {
			case failureModeCombatRetry, legacyFailureModeGymRetry:
				ready[key] = true
			case failureModeCombatLoss, legacyFailureModeTrainerLoss, legacyFailureModeGymLoss:
				lost[key] = true
			}
			continue
		}
		if strings.HasPrefix(storage, legacyGymLossFailurePrefix) {
			place := strings.ToLower(strings.TrimPrefix(storage, legacyGymLossFailurePrefix))
			if place != "" {
				lost[combatRecoveryObjective(Objective{Kind: KindGym, Place: PlaceID(place)}).Key()] = true
			}
			continue
		}
		if storage == (Objective{Kind: KindGym}).String() {
			const prefix = "party trained after losing at "
			const suffix = "; retry is due"
			if strings.HasPrefix(f.Last, prefix) && strings.HasSuffix(f.Last, suffix) {
				place := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(f.Last, prefix), suffix))
				if place != "" {
					ready[combatRecoveryObjective(Objective{Kind: KindGym, Place: PlaceID(place)}).Key()] = true
				}
			}
		}
	}
	for key := range lost {
		delete(ready, key)
	}
	return ready
}

// notePartyCombatChange is retained for direct legacy callers/tests; live Run
// uses notePartyCombatResult. Quantified combat losses stay blocked until their
// readiness target is reached; old checkpoints without a target retain the
// historical one-material-change behavior.
func (k *Knowledge) notePartyCombatChange(before, after Observation, execErr error) {
	if k == nil {
		return
	}
	progress := partyCombatAdvanced(before, after) ||
		errors.Is(execErr, skill.ErrTrainProgress) ||
		errors.Is(execErr, ErrTrainingInefficient)
	k.releaseSatisfiedCombatLossGates(after, progress)
}

func (k *Knowledge) releaseSatisfiedCombatLossGates(after Observation, legacyProgress bool) {
	readiness := partyCombatReadiness(after)
	k.promoteCombatLossesToRetryWhere(func(f Failure) bool {
		if f.ReadinessTarget > 0 {
			return readiness >= f.ReadinessTarget
		}
		return legacyProgress
	})
}

func (k *Knowledge) releaseCombatLossGates() {
	k.promoteCombatLossesToRetry()
}

func partyCombatAdvanced(before, after Observation) bool {
	if after.PartyCount > before.PartyCount {
		return true
	}
	return partyMaxLevel(after) > partyMaxLevel(before)
}

func partyMaxLevel(obs Observation) uint8 {
	var max uint8
	for _, mon := range obs.Party {
		if mon.Level > max {
			max = mon.Level
		}
	}
	return max
}

func trainingUnviableHere(obs Observation) bool {
	return obs.Training != nil && obs.Training.Viability == TrainingOutsideBudget
}

// ppRecoveryDue reports whether Offer already proved that attacking PP needs
// recovery by constructing a recovery objective.
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

func combatRetryMatchesObjective(ready map[ObjectiveKey]bool, o Objective) bool {
	if ready[combatRecoveryObjective(o).Key()] {
		return true
	}
	// A gym retry may require an ordinary journey back to the gym before the
	// challenge itself is locally offerable.
	if o.Kind == KindGoTo && o.Place != "" {
		return ready[combatRecoveryObjective(Objective{Kind: KindGym, Place: o.Place}).Key()]
	}
	return false
}

// chainCombatLossRecorded extends combatLossRecorded to every member of o's
// adapter-declared challenge chain.
func chainCombatLossRecorded(known *Knowledge, catalog ObjectiveCatalog, o Objective) bool {
	for _, peer := range combatChainPeers(catalog, o) {
		if combatLossRecorded(known, peer) {
			return true
		}
	}
	return false
}

func chainCombatRetryMatches(ready map[ObjectiveKey]bool, catalog ObjectiveCatalog, o Objective) bool {
	for _, peer := range combatChainPeers(catalog, o) {
		if combatRetryMatchesObjective(ready, peer) {
			return true
		}
	}
	return false
}

func filterCombatRecoveryBlocked(out []Objective, known *Knowledge, catalog ObjectiveCatalog) []Objective {
	retryKeys := combatRetryKeys(known)
	// A retry withholds further training only while it is actually offered. A
	// marker whose objective is not on the menu cannot be tested now, and must
	// not starve a later combat-loss campaign of its only recovery path.
	retryDue := false
	for _, o := range out {
		if chainCombatRetryMatches(retryKeys, catalog, o) {
			retryDue = true
			break
		}
	}
	ppDue := ppRecoveryDue(out)
	filtered := make([]Objective, 0, len(out))
	for _, o := range out {
		if chainCombatLossRecorded(known, catalog, o) {
			continue
		}
		if ppDue && (o.Kind == KindTrain || o.Kind == KindGym) {
			continue
		}
		if retryDue && o.Kind == KindTrain {
			continue
		}
		if chainCombatRetryMatches(retryKeys, catalog, o) && !strings.Contains(o.Note, combatRetryDueNote) {
			o = appendObjectiveNote(o, combatRetryDueNote)
		}
		filtered = append(filtered, o)
	}
	return filtered
}

const combatRetryDueNote = "(retry due after combat-readiness progress; test the stronger party now)"
