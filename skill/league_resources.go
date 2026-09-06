package skill

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

var ErrLeagueResourcesInsufficient = errors.New("skill: league resources are insufficient for the configured viability floor")

// LeagueResourceActionKind names one between-battle preparation action.
type LeagueResourceActionKind uint8

const (
	LeagueUseCenter LeagueResourceActionKind = iota
	LeagueRevive
	LeagueHealHP
	LeagueCureStatus
	LeagueRestorePP
)

func (k LeagueResourceActionKind) String() string {
	switch k {
	case LeagueUseCenter:
		return "center heal"
	case LeagueRevive:
		return "revive"
	case LeagueHealHP:
		return "heal HP"
	case LeagueCureStatus:
		return "cure status"
	case LeagueRestorePP:
		return "restore PP"
	default:
		return fmt.Sprintf("league resource action %d", uint8(k))
	}
}

// LeagueResourceAction is an explainable, executable preparation step. Item is
// zero for the free Center action. MoveSlot is advisory for PP recovery: ETHER
// and MAX ETHER use the same deterministic emptiest-move rule as UseFieldItem,
// and this field records which move the planner was trying to recover.
type LeagueResourceAction struct {
	Kind     LeagueResourceActionKind
	Item     uint8
	Slot     int
	MoveSlot int
	Reason   string
}

// LeagueResourcePolicy is intentionally party-agnostic. It defines a viability
// floor and how many future encounters must share finite items; it does not
// prescribe a canonical team or exact inventory.
type LeagueResourcePolicy struct {
	EncountersRemaining int
	MinimumLiveMons     int
	MinimumHPPercent    int
	MinimumPPRatio      int
	MaxActions          int
	FreeCenterAvailable bool
}

// DefaultLeagueResourcePolicy returns the conservative League policy used by
// progression code. Two live party members are preferred when the party has
// at least two; otherwise the only member must remain live. Half HP and 35% of
// base move PP are the between-battle floor. Finite items are shared across
// EncountersRemaining rather than spent greedily after the first fight.
func DefaultLeagueResourcePolicy(encountersRemaining, partyCount int) LeagueResourcePolicy {
	if encountersRemaining <= 0 {
		encountersRemaining = 1
	}
	minimumLive := 1
	if partyCount >= 2 {
		minimumLive = 2
	}
	return LeagueResourcePolicy{
		EncountersRemaining: encountersRemaining,
		MinimumLiveMons:     minimumLive,
		MinimumHPPercent:    50,
		MinimumPPRatio:      35,
		MaxActions:          12,
	}
}

// LeagueResourceAssessment is the observed or projected sequence state used
// for preparation and retry diagnostics.
type LeagueResourceAssessment struct {
	PartyCount     int
	LiveMons       int
	FaintedMons    int
	StatusedMons   int
	BelowHPFloor   int
	CurrentPP      int
	BaseMaxPP      int
	PPRatioPercent int
	FullyRecovered bool
	Viable         bool
	Summary        string
}

// LeagueResourcePlan contains a bounded fair-share preparation plan. Reserved
// is the item inventory left after the projected actions, making the resource
// preservation decision inspectable by callers/tests.
type LeagueResourcePlan struct {
	Before   LeagueResourceAssessment
	After    LeagueResourceAssessment
	Actions  []LeagueResourceAction
	Reserved map[uint8]int
}

// LeagueResourceResult records actions actually executed plus the final live
// assessment. A failed preparation still returns the actions that succeeded.
type LeagueResourceResult struct {
	Actions []LeagueResourceAction
	Final   LeagueResourceAssessment
}

func normalizedLeaguePolicy(policy LeagueResourcePolicy, partyCount int) LeagueResourcePolicy {
	defaults := DefaultLeagueResourcePolicy(policy.EncountersRemaining, partyCount)
	if policy.EncountersRemaining <= 0 {
		policy.EncountersRemaining = defaults.EncountersRemaining
	}
	if policy.MinimumLiveMons <= 0 {
		policy.MinimumLiveMons = defaults.MinimumLiveMons
	}
	if policy.MinimumLiveMons > partyCount && partyCount > 0 {
		policy.MinimumLiveMons = partyCount
	}
	if policy.MinimumHPPercent <= 0 || policy.MinimumHPPercent > 100 {
		policy.MinimumHPPercent = defaults.MinimumHPPercent
	}
	if policy.MinimumPPRatio <= 0 || policy.MinimumPPRatio > 100 {
		policy.MinimumPPRatio = defaults.MinimumPPRatio
	}
	if policy.MaxActions <= 0 {
		policy.MaxActions = defaults.MaxActions
	}
	return policy
}

func leagueItemCounts(inv state.InventoryState) map[uint8]int {
	out := make(map[uint8]int, len(inv.Items))
	for _, item := range inv.Items {
		out[item.ID] += int(item.Quantity)
	}
	return out
}

func copyLeagueCounts(in map[uint8]int) map[uint8]int {
	out := make(map[uint8]int, len(in))
	for item, qty := range in {
		out[item] = qty
	}
	return out
}

func copyLeagueParty(in state.PartyState) state.PartyState {
	out := state.PartyState{Count: in.Count, Mons: make([]state.Mon, len(in.Mons))}
	copy(out.Mons, in.Mons)
	return out
}

func fairShare(total, encounters int) int {
	if total <= 0 {
		return 0
	}
	if encounters <= 1 {
		return total
	}
	return (total + encounters - 1) / encounters
}

func leagueBasePP(romData []byte, mon state.Mon) (current, maximum int, perMove [4]int, err error) {
	for i, moveID := range mon.Moves {
		if moveID == 0 {
			continue
		}
		move, lookupErr := rom.LookupMove(romData, moveID)
		if lookupErr != nil {
			return 0, 0, perMove, lookupErr
		}
		current += int(mon.PP[i])
		maximum += int(move.PP)
		perMove[i] = int(move.PP)
	}
	return current, maximum, perMove, nil
}

func assessLeagueResources(romData []byte, party state.PartyState, policy LeagueResourcePolicy) (LeagueResourceAssessment, error) {
	policy = normalizedLeaguePolicy(policy, len(party.Mons))
	assessment := LeagueResourceAssessment{PartyCount: len(party.Mons), FullyRecovered: len(party.Mons) > 0}

	for _, mon := range party.Mons {
		curPP, maxPP, _, err := leagueBasePP(romData, mon)
		if err != nil {
			return LeagueResourceAssessment{}, err
		}
		if mon.Fainted() {
			assessment.FaintedMons++
			assessment.FullyRecovered = false
			continue
		}
		assessment.LiveMons++
		assessment.CurrentPP += curPP
		assessment.BaseMaxPP += maxPP
		if mon.Status != 0 {
			assessment.StatusedMons++
			assessment.FullyRecovered = false
		}
		if mon.MaxHP == 0 || int(mon.HP)*100 < int(mon.MaxHP)*policy.MinimumHPPercent {
			assessment.BelowHPFloor++
		}
		if mon.HP != mon.MaxHP {
			assessment.FullyRecovered = false
		}
		if curPP < maxPP {
			assessment.FullyRecovered = false
		}
	}
	if assessment.BaseMaxPP > 0 {
		assessment.PPRatioPercent = assessment.CurrentPP * 100 / assessment.BaseMaxPP
	} else if assessment.LiveMons > 0 {
		assessment.PPRatioPercent = 100
	}
	assessment.Viable = assessment.PartyCount > 0 &&
		assessment.LiveMons >= policy.MinimumLiveMons &&
		assessment.StatusedMons == 0 &&
		assessment.BelowHPFloor == 0 &&
		assessment.PPRatioPercent >= policy.MinimumPPRatio

	var reasons []string
	if assessment.PartyCount == 0 {
		reasons = append(reasons, "no party")
	}
	if assessment.LiveMons < policy.MinimumLiveMons {
		reasons = append(reasons, fmt.Sprintf("%d live < %d required", assessment.LiveMons, policy.MinimumLiveMons))
	}
	if assessment.StatusedMons > 0 {
		reasons = append(reasons, fmt.Sprintf("%d statused", assessment.StatusedMons))
	}
	if assessment.BelowHPFloor > 0 {
		reasons = append(reasons, fmt.Sprintf("%d below %d%% HP", assessment.BelowHPFloor, policy.MinimumHPPercent))
	}
	if assessment.PPRatioPercent < policy.MinimumPPRatio {
		reasons = append(reasons, fmt.Sprintf("PP %d%% < %d%%", assessment.PPRatioPercent, policy.MinimumPPRatio))
	}
	if len(reasons) == 0 {
		assessment.Summary = "viable"
	} else {
		assessment.Summary = strings.Join(reasons, ", ")
	}
	return assessment, nil
}

// AssessLeagueResources models live party health/status/PP against the current
// sequence policy. Move maximum PP comes from the ROM move table; PP Up bonus
// capacity is deliberately not invented, so the base maximum is a conservative
// denominator and current PP is never treated as worse because PP Ups exist.
func AssessLeagueResources(romData []byte, party state.PartyState, inv state.InventoryState, policy LeagueResourcePolicy) (LeagueResourceAssessment, error) {
	_ = inv // inventory affects planning; viability itself is party state.
	return assessLeagueResources(romData, party, policy)
}

func needsFreeCenter(party state.PartyState, romData []byte) (bool, error) {
	if len(party.Mons) == 0 {
		return false, nil
	}
	for _, mon := range party.Mons {
		if mon.HP != mon.MaxHP || mon.Status != 0 {
			return true, nil
		}
		cur, max, _, err := leagueBasePP(romData, mon)
		if err != nil {
			return false, err
		}
		if cur < max {
			return true, nil
		}
	}
	return false, nil
}

func centerRecoveredProjection(romData []byte, party state.PartyState) (state.PartyState, error) {
	out := copyLeagueParty(party)
	for i := range out.Mons {
		mon := &out.Mons[i]
		mon.HP = mon.MaxHP
		mon.Status = 0
		_, _, perMove, err := leagueBasePP(romData, *mon)
		if err != nil {
			return state.PartyState{}, err
		}
		for slot, moveID := range mon.Moves {
			if moveID != 0 && int(mon.PP[slot]) < perMove[slot] {
				mon.PP[slot] = uint8(perMove[slot])
			}
		}
	}
	return out, nil
}

func totalCount(counts map[uint8]int, items ...uint8) int {
	total := 0
	for _, item := range items {
		total += counts[item]
	}
	return total
}

func chooseFaintedForRevive(party state.PartyState) int {
	best, bestScore := -1, -1
	for slot, mon := range party.Mons {
		if !mon.Fainted() || mon.MaxHP == 0 {
			continue
		}
		score := int(mon.MaxHP) + int(mon.Level)*4
		for _, pp := range mon.PP {
			score += int(pp)
		}
		if score > bestScore {
			best, bestScore = slot, score
		}
	}
	return best
}

func statusMedicineForCounts(counts map[uint8]int, status string) uint8 {
	var specific uint8
	switch status {
	case "poisoned":
		specific = itemAntidote
	case "burned":
		specific = itemBurnHeal
	case "frozen":
		specific = itemIceHeal
	case "asleep":
		specific = itemAwakening
	case "paralyzed":
		specific = itemParlyzHeal
	}
	for _, item := range []uint8{specific, itemFullHeal, itemFullRestore} {
		if item != 0 && counts[item] > 0 {
			return item
		}
	}
	return 0
}

func hpMedicineForCounts(counts map[uint8]int, missing int) (uint8, int) {
	var strongest uint8
	strongestHeal := 0
	for _, med := range hpMedicines {
		if counts[med.item] <= 0 {
			continue
		}
		strongest, strongestHeal = med.item, med.heal
		if med.heal >= missing {
			return med.item, med.heal
		}
	}
	return strongest, strongestHeal
}

func sortedLowHPSlots(party state.PartyState, floor int) []int {
	var slots []int
	for slot, mon := range party.Mons {
		if mon.Fainted() || mon.MaxHP == 0 || int(mon.HP)*100 >= int(mon.MaxHP)*floor {
			continue
		}
		slots = append(slots, slot)
	}
	sort.SliceStable(slots, func(i, j int) bool {
		a, b := party.Mons[slots[i]], party.Mons[slots[j]]
		return int(a.HP)*int(b.MaxHP) < int(b.HP)*int(a.MaxHP)
	})
	return slots
}

func lowestPPRatioMove(mon state.Mon, maxPP [4]int) (slot int, ok bool) {
	bestNum, bestDen := 0, 1
	best := -1
	for i, moveID := range mon.Moves {
		if moveID == 0 || maxPP[i] <= 0 || int(mon.PP[i]) >= maxPP[i] {
			continue
		}
		if best < 0 || int(mon.PP[i])*bestDen < bestNum*maxPP[i] {
			best = i
			bestNum, bestDen = int(mon.PP[i]), maxPP[i]
		}
	}
	return best, best >= 0
}

func lowPPMoveCount(mon state.Mon, maxPP [4]int, floor int) int {
	count := 0
	for i, moveID := range mon.Moves {
		if moveID != 0 && maxPP[i] > 0 && int(mon.PP[i])*100 < maxPP[i]*floor {
			count++
		}
	}
	return count
}

func choosePPRestoreItem(counts map[uint8]int, multi bool) uint8 {
	if multi {
		if counts[itemElixer] > 0 {
			return itemElixer
		}
		if counts[itemMaxElixer] > 0 {
			return itemMaxElixer
		}
	}
	if counts[itemEther] > 0 {
		return itemEther
	}
	if counts[itemMaxEther] > 0 {
		return itemMaxEther
	}
	if counts[itemElixer] > 0 {
		return itemElixer
	}
	if counts[itemMaxElixer] > 0 {
		return itemMaxElixer
	}
	return 0
}

func simulatePPRestore(mon *state.Mon, item uint8, moveSlot int, maxPP [4]int) {
	if item == itemEther || item == itemMaxEther {
		if moveSlot < 0 || moveSlot >= len(mon.PP) {
			return
		}
		if item == itemMaxEther {
			mon.PP[moveSlot] = uint8(maxPP[moveSlot])
			return
		}
		next := int(mon.PP[moveSlot]) + 10
		if next > maxPP[moveSlot] {
			next = maxPP[moveSlot]
		}
		mon.PP[moveSlot] = uint8(next)
		return
	}
	for i, moveID := range mon.Moves {
		if moveID == 0 {
			continue
		}
		if item == itemMaxElixer {
			mon.PP[i] = uint8(maxPP[i])
			continue
		}
		next := int(mon.PP[i]) + 10
		if next > maxPP[i] {
			next = maxPP[i]
		}
		mon.PP[i] = uint8(next)
	}
}

func appendLeagueAction(actions *[]LeagueResourceAction, action LeagueResourceAction, max int) bool {
	if len(*actions) >= max {
		return false
	}
	*actions = append(*actions, action)
	return true
}

// PlanLeagueResources creates one bounded between-battle plan. Each resource
// category can spend at most ceil(stock / EncountersRemaining) during this
// preparation window, so five Ethers with four fights left cannot all vanish
// after the next battle. The projected state is assessed after those actions.
//
// If FreeCenterAvailable is true and any party resource is below its Center
// maximum, the plan contains exactly one free Center heal and consumes zero
// finite items. That is the pre-League behavior at Indigo Plateau.
func PlanLeagueResources(romData []byte, party state.PartyState, inv state.InventoryState, policy LeagueResourcePolicy) (LeagueResourcePlan, error) {
	policy = normalizedLeaguePolicy(policy, len(party.Mons))
	before, err := assessLeagueResources(romData, party, policy)
	if err != nil {
		return LeagueResourcePlan{}, err
	}
	counts := leagueItemCounts(inv)
	plan := LeagueResourcePlan{Before: before, After: before, Reserved: copyLeagueCounts(counts)}

	if policy.FreeCenterAvailable {
		needCenter, err := needsFreeCenter(party, romData)
		if err != nil {
			return LeagueResourcePlan{}, err
		}
		if needCenter {
			projected, err := centerRecoveredProjection(romData, party)
			if err != nil {
				return LeagueResourcePlan{}, err
			}
			plan.Actions = []LeagueResourceAction{{
				Kind: LeagueUseCenter, Slot: -1, MoveSlot: -1,
				Reason: "free Center recovery is available before committing finite League resources",
			}}
			plan.After, err = assessLeagueResources(romData, projected, policy)
			return plan, err
		}
	}

	projected := copyLeagueParty(party)
	encounters := policy.EncountersRemaining
	reviveBudget := fairShare(totalCount(counts, itemRevive, itemMaxRevive), encounters)
	healBudget := fairShare(totalCount(counts,
		itemPotion, itemSuperPotion, itemFreshWater, itemSodaPop, itemLemonade,
		itemHyperPotion, itemMaxPotion, itemFullRestore), encounters)
	statusBudget := fairShare(totalCount(counts,
		itemAntidote, itemBurnHeal, itemIceHeal, itemAwakening, itemParlyzHeal,
		itemFullHeal, itemFullRestore), encounters)
	ppBudget := fairShare(totalCount(counts, itemEther, itemMaxEther, itemElixer, itemMaxElixer), encounters)

	live := before.LiveMons
	for reviveBudget > 0 && live < policy.MinimumLiveMons && len(plan.Actions) < policy.MaxActions {
		slot := chooseFaintedForRevive(projected)
		if slot < 0 {
			break
		}
		item := itemRevive
		if counts[item] <= 0 {
			item = itemMaxRevive
		}
		if counts[item] <= 0 {
			break
		}
		mon := &projected.Mons[slot]
		if item == itemMaxRevive {
			mon.HP = mon.MaxHP
		} else {
			mon.HP = mon.MaxHP / 2
			if mon.HP == 0 && mon.MaxHP > 0 {
				mon.HP = 1
			}
		}
		counts[item]--
		reviveBudget--
		live++
		appendLeagueAction(&plan.Actions, LeagueResourceAction{
			Kind: LeagueRevive, Item: item, Slot: slot, MoveSlot: -1,
			Reason: fmt.Sprintf("restore live-party floor %d/%d", live, policy.MinimumLiveMons),
		}, policy.MaxActions)
	}

	for slot := range projected.Mons {
		if statusBudget <= 0 || len(plan.Actions) >= policy.MaxActions {
			break
		}
		mon := &projected.Mons[slot]
		if mon.Fainted() || mon.Status == 0 {
			continue
		}
		item := statusMedicineForCounts(counts, mon.StatusName())
		if item == 0 {
			continue
		}
		statusName := mon.StatusName()
		mon.Status = 0
		if item == itemFullRestore {
			mon.HP = mon.MaxHP
		}
		counts[item]--
		statusBudget--
		appendLeagueAction(&plan.Actions, LeagueResourceAction{
			Kind: LeagueCureStatus, Item: item, Slot: slot, MoveSlot: -1,
			Reason: "clear " + statusName + " before the next chained fight",
		}, policy.MaxActions)
	}

	for healBudget > 0 && len(plan.Actions) < policy.MaxActions {
		slots := sortedLowHPSlots(projected, policy.MinimumHPPercent)
		if len(slots) == 0 {
			break
		}
		slot := slots[0]
		mon := &projected.Mons[slot]
		targetHP := (int(mon.MaxHP)*policy.MinimumHPPercent + 99) / 100
		missing := targetHP - int(mon.HP)
		item, heal := hpMedicineForCounts(counts, missing)
		if item == 0 {
			break
		}
		if heal >= 1<<29 {
			mon.HP = mon.MaxHP
			if item == itemFullRestore {
				mon.Status = 0
			}
		} else {
			next := int(mon.HP) + heal
			if next > int(mon.MaxHP) {
				next = int(mon.MaxHP)
			}
			mon.HP = uint16(next)
		}
		counts[item]--
		healBudget--
		appendLeagueAction(&plan.Actions, LeagueResourceAction{
			Kind: LeagueHealHP, Item: item, Slot: slot, MoveSlot: -1,
			Reason: fmt.Sprintf("raise slot %d to at least %d%% HP", slot, policy.MinimumHPPercent),
		}, policy.MaxActions)
	}

	for ppBudget > 0 && len(plan.Actions) < policy.MaxActions {
		assessment, err := assessLeagueResources(romData, projected, policy)
		if err != nil {
			return LeagueResourcePlan{}, err
		}
		if assessment.PPRatioPercent >= policy.MinimumPPRatio {
			break
		}

		bestSlot, bestRatio := -1, 101
		var bestMax [4]int
		for slot, mon := range projected.Mons {
			if mon.Fainted() {
				continue
			}
			cur, max, perMove, err := leagueBasePP(romData, mon)
			if err != nil {
				return LeagueResourcePlan{}, err
			}
			if max == 0 || cur >= max {
				continue
			}
			ratio := cur * 100 / max
			if ratio < bestRatio {
				bestSlot, bestRatio, bestMax = slot, ratio, perMove
			}
		}
		if bestSlot < 0 {
			break
		}
		mon := &projected.Mons[bestSlot]
		moveSlot, ok := lowestPPRatioMove(*mon, bestMax)
		if !ok {
			break
		}
		multi := lowPPMoveCount(*mon, bestMax, policy.MinimumPPRatio) >= 2
		item := choosePPRestoreItem(counts, multi)
		if item == 0 {
			break
		}
		simulatePPRestore(mon, item, moveSlot, bestMax)
		counts[item]--
		ppBudget--
		appendLeagueAction(&plan.Actions, LeagueResourceAction{
			Kind: LeagueRestorePP, Item: item, Slot: bestSlot, MoveSlot: moveSlot,
			Reason: fmt.Sprintf("raise aggregate PP toward %d%% floor; slot %d was at %d%%", policy.MinimumPPRatio, bestSlot, bestRatio),
		}, policy.MaxActions)
	}

	plan.After, err = assessLeagueResources(romData, projected, policy)
	if err != nil {
		return LeagueResourcePlan{}, err
	}
	plan.Reserved = copyLeagueCounts(counts)
	return plan, nil
}

// ExecuteLeagueResourceAction executes exactly one planned action through the
// normal game UI. Every finite-item path has a positive RAM effect + bag-count
// postcondition; Center healing has Heal's full-party recovery postcondition.
func ExecuteLeagueResourceAction(m *emu.Emu, action LeagueResourceAction) error {
	switch action.Kind {
	case LeagueUseCenter:
		return Heal(m)
	case LeagueRevive:
		return RevivePartyMember(m, action.Slot, action.Item == itemMaxRevive)
	case LeagueHealHP, LeagueCureStatus:
		return UseFieldItem(m, action.Item, action.Slot)
	case LeagueRestorePP:
		return RestorePartyPP(m, action.Item, action.Slot)
	default:
		return fmt.Errorf("skill: ExecuteLeagueResourceAction: unknown action kind %d", action.Kind)
	}
}

// PrepareLeagueResources observes once, creates one fair-share plan, executes
// that bounded plan, then re-observes. It deliberately does NOT repeatedly
// re-plan and spend another fair share in the same between-battle window: that
// would defeat the future-fight reserve invariant. If the projected floor
// cannot be reached with this encounter's allocation, the caller gets a typed
// recoverable error plus the successful actions and final state.
func PrepareLeagueResources(m *emu.Emu, romData []byte, policy LeagueResourcePolicy) (LeagueResourceResult, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	inv := state.DecodeInventory(&mem)
	policy = normalizedLeaguePolicy(policy, len(party.Mons))

	plan, err := PlanLeagueResources(romData, party, inv, policy)
	if err != nil {
		return LeagueResourceResult{}, err
	}
	result := LeagueResourceResult{}
	for _, action := range plan.Actions {
		if len(result.Actions) >= policy.MaxActions {
			break
		}
		if err := ExecuteLeagueResourceAction(m, action); err != nil {
			state.Snapshot(m, &mem)
			result.Final, _ = assessLeagueResources(romData, state.DecodeParty(&mem), policy)
			return result, fmt.Errorf("skill: PrepareLeagueResources: %s for slot %d: %w", action.Kind, action.Slot, err)
		}
		result.Actions = append(result.Actions, action)
	}

	state.Snapshot(m, &mem)
	result.Final, err = assessLeagueResources(romData, state.DecodeParty(&mem), policy)
	if err != nil {
		return result, err
	}
	if !result.Final.Viable {
		return result, fmt.Errorf("%w: %s after %d bounded action(s), encounters remaining=%d",
			ErrLeagueResourcesInsufficient, result.Final.Summary, len(result.Actions), policy.EncountersRemaining)
	}
	return result, nil
}

const LeagueBattleCount = 5

// LeagueSequenceProgress is the resumable, non-success checkpoint for the
// Lorelei -> Bruno -> Agatha -> Lance -> Champion chain. Wins can advance
// internally, but Complete is true only when the final Champion event is
// positively observed. An intermediate Battle() ResultWon can therefore never
// masquerade as overall League completion.
type LeagueSequenceProgress struct {
	Wins              int
	ChampionConfirmed bool
}

func (p LeagueSequenceProgress) Complete() bool {
	return p.Wins >= LeagueBattleCount && p.ChampionConfirmed
}

func (p LeagueSequenceProgress) EncountersRemaining() int {
	remaining := LeagueBattleCount - p.Wins
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (p LeagueSequenceProgress) WithWin() LeagueSequenceProgress {
	if p.Wins < LeagueBattleCount {
		p.Wins++
	}
	return p
}

// ConfirmLeagueChampion applies the deterministic run-ending game fact to an
// existing intermediate progress checkpoint.
func ConfirmLeagueChampion(mem *state.Mem, p LeagueSequenceProgress) LeagueSequenceProgress {
	p.ChampionConfirmed = state.HasEvent(mem, state.EventBeatChampionRival)
	return p
}
