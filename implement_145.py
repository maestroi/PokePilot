from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    s = p.read_text()
    if old not in s:
        raise SystemExit(f"missing anchor in {path}: {old[:160]!r}")
    if s.count(old) != 1:
        raise SystemExit(f"non-unique anchor in {path}: {s.count(old)}")
    p.write_text(s.replace(old, new, 1))


def replace_between(path, start, end, new):
    p = Path(path)
    s = p.read_text()
    i = s.find(start)
    if i < 0:
        raise SystemExit(f"missing start in {path}: {start!r}")
    j = s.find(end, i + len(start))
    if j < 0:
        raise SystemExit(f"missing end in {path}: {end!r}")
    p.write_text(s[:i] + new + s[j:])


# ---------------------------------------------------------------------------
# skill/shop.go: type bounded mart/controller failures and only return the
# recoverable type after Buy itself has established a stable overworld.
# ---------------------------------------------------------------------------
replace_once("skill/shop.go",
'''var ErrNotInStock = errors.New("skill: the clerk does not stock the requested item")
''',
'''var ErrNotInStock = errors.New("skill: the clerk does not stock the requested item")

// ErrShopMenuTimeout is a bounded mart transition timeout. It is recoverable
// only when Buy successfully backs out to the overworld before returning it.
var ErrShopMenuTimeout = errors.New("skill: shop menu transition timed out")

// ErrShopControllerStalled is a bounded cursor/menu controller failure inside
// Buy. Like ErrShopMenuTimeout, callers may recover only after a clean boundary.
var ErrShopControllerStalled = errors.New("skill: shop controller stalled")

// ErrShopStabilization means Buy could not prove that it returned to a safe
// overworld boundary. It is deliberately terminal even if the original mart
// error would otherwise be recoverable.
var ErrShopStabilization = errors.New("skill: shop stabilization failed")
''')

replace_once("skill/shop.go",
'''\tif err := martAdvance(m, buySellQuitUp, "the BUY/SELL/QUIT menu"); err != nil {
\t\treturn err
\t}
''',
'''\tif err := martAdvance(m, buySellQuitUp, "the BUY/SELL/QUIT menu"); err != nil {
\t\treturn recoverShopFailure(m, err)
\t}
''')
replace_once("skill/shop.go",
'''\tif err := SelectMenuItem(m, 0); err != nil {
\t\treturn fmt.Errorf("skill: Buy: select BUY: %w", err)
\t}
''',
'''\tif err := SelectMenuItem(m, 0); err != nil {
\t\treturn recoverShopFailure(m, shopControllerFailure("select BUY", err))
\t}
''')
replace_once("skill/shop.go",
'''\tif err := martAdvance(m, itemListUp, "the item list"); err != nil {
\t\treturn err
\t}
''',
'''\tif err := martAdvance(m, itemListUp, "the item list"); err != nil {
\t\treturn recoverShopFailure(m, err)
\t}
''')
replace_once("skill/shop.go",
'''\t\tif err := exitToOverworld(m); err != nil {
\t\t\treturn fmt.Errorf("skill: Buy: item %#02x is not stocked and the shop did not close: %w", item, err)
\t\t}
\t\treturn fmt.Errorf("skill: Buy: %w: item %#02x", ErrNotInStock, item)
''',
'''\t\tprimary := fmt.Errorf("skill: Buy: %w: item %#02x", ErrNotInStock, item)
\t\tif err := exitToOverworld(m); err != nil {
\t\t\treturn shopStabilizationFailure(primary, err)
\t\t}
\t\treturn primary
''')
replace_once("skill/shop.go",
'''\tif err := selectListEntry(m, pos); err != nil {
\t\t// Same rule as the ErrNotInStock backout above: a cursor that never
\t\t// reached its target leaves the item list up, and returning here
\t\t// without closing it wedges every later objective on the same
\t\t// unanswered shop menu.
\t\tif exitErr := exitToOverworld(m); exitErr != nil {
\t\t\treturn fmt.Errorf("skill: Buy: select item %#02x: %v (shop did not close: %w)", item, err, exitErr)
\t\t}
\t\treturn fmt.Errorf("skill: Buy: select item %#02x: %w", item, err)
\t}
''',
'''\tif err := selectListEntry(m, pos); err != nil {
\t\t// Same rule as the ErrNotInStock backout above: a cursor that never
\t\t// reached its target leaves the item list up. The typed controller
\t\t// failure becomes recoverable only after recoverShopFailure proves the
\t\t// shop is gone.
\t\treturn recoverShopFailure(m, shopControllerFailure(fmt.Sprintf("select item %#02x", item), err))
\t}
''')
replace_once("skill/shop.go",
'''\tif err := martWait(m, qtyUp, "the choose-quantity box"); err != nil {
\t\t// Same rule as the ErrNotInStock backout above: returning here
\t\t// leaves the item list (or whatever selectListEntry's A landed on)
\t\t// up, and every later objective then fails on a shop menu nothing
\t\t// closed. MEASURED 2026-09-02: "buy 3 POKEBALL" timed out here and
\t\t// parked the run on "MONEY BUY... Is there anything else I can
\t\t// do?" for the rest of the run.
\t\tif exitErr := exitToOverworld(m); exitErr != nil {
\t\t\treturn fmt.Errorf("%w (shop did not close: %v)", err, exitErr)
\t\t}
\t\treturn err
\t}
''',
'''\tif err := martWait(m, qtyUp, "the choose-quantity box"); err != nil {
\t\t// A timeout is still an engineering failure. The campaign may survive
\t\t// it only after the owning skill proves the shop has been closed.
\t\treturn recoverShopFailure(m, err)
\t}
''')
replace_once("skill/shop.go",
'''\tif err := setQuantity(m, qty); err != nil {
\t\tif exitErr := exitToOverworld(m); exitErr != nil {
\t\t\treturn fmt.Errorf("%w (shop did not close: %v)", err, exitErr)
\t\t}
\t\treturn err
\t}
''',
'''\tif err := setQuantity(m, qty); err != nil {
\t\treturn recoverShopFailure(m, err)
\t}
''')
replace_once("skill/shop.go",
'''\t\tif err := backOutOfShop(m); err != nil {
\t\t\t// NOT wrapped in ErrCantAfford. A caller that sees ErrCantAfford
\t\t\t// is told "the game said no, the world is fine, pick something
\t\t\t// else" — agent.Execute treats it as a benign outcome and
\t\t\t// reports the round DONE. A backout that failed leaves the shop
\t\t\t// menus on screen, which is the opposite of fine: every later
\t\t\t// objective refuses to start on it. MEASURED 2026-08-31: "buy 3
\t\t\t// POTION -> done" with 793 in hand against a 900 total, and the
\t\t\t// run dead four rounds later on an item list nobody closed.
\t\t\treturn fmt.Errorf("skill: Buy: cannot afford %d (have %d) and the shop did not close: %w", total, moneyBefore, err)
\t\t}
\t\treturn fmt.Errorf("skill: Buy: %w: have %d, need %d", ErrCantAfford, moneyBefore, total)
''',
'''\t\tprimary := fmt.Errorf("skill: Buy: %w: have %d, need %d", ErrCantAfford, moneyBefore, total)
\t\tif err := backOutOfShop(m); err != nil {
\t\t\t// Do not leak the benign ErrCantAfford classification when cleanup
\t\t\t// itself failed: the campaign no longer has a trustworthy boundary.
\t\t\treturn shopStabilizationFailure(primary, err)
\t\t}
\t\treturn primary
''')
replace_once("skill/shop.go",
'''\tif err := martAdvance(m, twoOptionUp, "the purchase-confirmation prompt"); err != nil {
\t\tif exitErr := exitToOverworld(m); exitErr != nil {
\t\t\treturn fmt.Errorf("%w (shop did not close: %v)", err, exitErr)
\t\t}
\t\treturn err
\t}
''',
'''\tif err := martAdvance(m, twoOptionUp, "the purchase-confirmation prompt"); err != nil {
\t\treturn recoverShopFailure(m, err)
\t}
''')
replace_once("skill/shop.go",
'''\tif err := SelectMenuItem(m, 0); err != nil {
\t\tif exitErr := exitToOverworld(m); exitErr != nil {
\t\t\treturn fmt.Errorf("skill: Buy: answer YES: %v (shop did not close: %w)", err, exitErr)
\t\t}
\t\treturn fmt.Errorf("skill: Buy: answer YES: %w", err)
\t}
''',
'''\tif err := SelectMenuItem(m, 0); err != nil {
\t\treturn recoverShopFailure(m, shopControllerFailure("answer YES", err))
\t}
''')
replace_once("skill/shop.go",
'''\tif err := exitShop(m); err != nil {
\t\treturn err
\t}
''',
'''\tif err := exitShop(m); err != nil {
\t\treturn recoverShopFailure(m, err)
\t}
''')
replace_once("skill/shop.go",
'''func martTimeout(what string, mem *state.Mem) error {
\treturn fmt.Errorf("skill: Buy: %s did not appear (wFontLoaded=%#04x wCurMenuItem=%d wMaxMenuItem=%d wItemQuantity=%d wMoney=%d)",
\t\twhat, mem.U8(sym.FontLoaded), mem.U8(sym.CurrentMenuItem), mem.U8(sym.MaxMenuItem),
\t\tmem.U8(sym.ItemQuantity), bcdMoney(mem))
}
''',
'''func martTimeout(what string, mem *state.Mem) error {
\treturn fmt.Errorf("%w: skill: Buy: %s did not appear (wFontLoaded=%#04x wCurMenuItem=%d wMaxMenuItem=%d wItemQuantity=%d wMoney=%d)",
\t\tErrShopMenuTimeout, what, mem.U8(sym.FontLoaded), mem.U8(sym.CurrentMenuItem), mem.U8(sym.MaxMenuItem),
\t\tmem.U8(sym.ItemQuantity), bcdMoney(mem))
}

func shopControllerFailure(context string, err error) error {
\treturn fmt.Errorf("skill: Buy: %s: %w", context, errors.Join(ErrShopControllerStalled, err))
}

func shopStabilizationFailure(primary, cleanup error) error {
\treturn errors.Join(primary, fmt.Errorf("%w: %v", ErrShopStabilization, cleanup))
}

// recoverShopFailure is the ownership proof for a recoverable mart fault. A
// timeout/controller error remains fully visible to callers, but it is only
// eligible for runtime re-planning if this cleanup succeeds. Cleanup failure
// adds ErrShopStabilization, which the agent treats as terminal.
func recoverShopFailure(m *emu.Emu, err error) error {
\tif err == nil {
\t\treturn nil
\t}
\tif cleanup := exitToOverworld(m); cleanup != nil {
\t\treturn shopStabilizationFailure(err, cleanup)
\t}
\treturn err
}
''')
replace_once("skill/shop.go",
'''\t\t\tif stuck >= stuckLimit {
\t\t\t\treturn fmt.Errorf("cursor stuck at list entry %d, wanted %d, %d consecutive taps without movement", pos, index, stuck)
\t\t\t}
''',
'''\t\t\tif stuck >= stuckLimit {
\t\t\t\treturn fmt.Errorf("%w: cursor stuck at list entry %d, wanted %d, %d consecutive taps without movement", ErrShopControllerStalled, pos, index, stuck)
\t\t\t}
''')
replace_once("skill/shop.go",
'''\t\tif cur > qty {
\t\t\treturn fmt.Errorf("quantity overshot %d (wItemQuantity=%d)", qty, cur)
\t\t}
''',
'''\t\tif cur > qty {
\t\t\treturn fmt.Errorf("%w: quantity overshot %d (wItemQuantity=%d)", ErrShopControllerStalled, qty, cur)
\t\t}
''')
replace_once("skill/shop.go",
'''\treturn fmt.Errorf("quantity did not reach %d (wItemQuantity=%d)", qty, mem.U8(sym.ItemQuantity))
''',
'''\treturn fmt.Errorf("%w: quantity did not reach %d (wItemQuantity=%d)", ErrShopControllerStalled, qty, mem.U8(sym.ItemQuantity))
''')


# ---------------------------------------------------------------------------
# Objective outcome policy: known bounded controller faults become blockage
# only when the transaction finished at a trustworthy stable boundary.
# ---------------------------------------------------------------------------
replace_once("agent/objective_result.go",
'''\tGymOutcome   *state.BattleResult `json:"gym_outcome,omitempty"`
''',
'''\tGymOutcome   *state.BattleResult `json:"gym_outcome,omitempty"`
\tRecovered    bool                `json:"recovered,omitempty"`
\tTerminal     bool                `json:"terminal,omitempty"`
''')
replace_once("agent/objective_result.go",
'''\tcase OutcomeBlocked:
\t\treturn actionReplan
''',
'''\tcase OutcomeBlocked, OutcomePostconditionFailed:
\t\treturn actionReplan
''')
replace_between("agent/objective_result.go",
"func classifyObjectiveOutcome(_ Objective, err error, final Observation) Outcome {\n",
"func (r ObjectiveResult) HistoryText() string {\n",
'''func classifyObjectiveOutcome(_ Objective, err error, final Observation) Outcome {
\tif err == nil {
\t\treturn OutcomeCompleted
\t}

\tif errors.Is(err, ErrObjectiveBoundaryChoice) {
\t\treturn OutcomeChoiceRequired
\t}
\tvar choice *skill.ErrDialogueChoice
\tif errors.As(err, &choice) || errors.Is(err, skill.ErrFieldItemPrompt) {
\t\treturn OutcomeChoiceRequired
\t}

\t// A dirty finish dominates the error that caused it. errors.Join keeps the
\t// original typed controller fault too, so this check must precede every
\t// recoverable class or an unsafe boundary could be mislabeled blocked.
\tif errors.Is(err, ErrObjectiveBoundaryDirty) || errors.Is(err, skill.ErrShopStabilization) {
\t\treturn OutcomeStabilizationFailed
\t}

\tif errors.Is(err, ErrObjectivePostconditionFailed) {
\t\treturn OutcomePostconditionFailed
\t}
\tif errors.Is(err, ErrObjectivePostconditionUnavailable) {
\t\treturn OutcomePostconditionUnavailable
\t}

\tif recoverableControllerFault(err) {
\t\tif stableObjectiveBoundary(final) {
\t\t\treturn OutcomeBlocked
\t\t}
\t\treturn OutcomeControllerUncertain
\t}

\tif errors.Is(err, skill.ErrBattle) || errors.Is(err, skill.ErrBattleInterrupted) {
\t\treturn OutcomeOwnershipFailure
\t}

\tif errors.Is(err, skill.ErrFieldItemNoEffect) {
\t\treturn OutcomePostconditionFailed
\t}

\tif errors.Is(err, skill.ErrBlackedOut) ||
\t\terrors.Is(err, skill.ErrCatchBlackout) ||
\t\terrors.Is(err, skill.ErrCatchHuntExhausted) ||
\t\terrors.Is(err, skill.ErrTrainRetreat) ||
\t\terrors.Is(err, skill.ErrTrainProgress) ||
\t\terrors.Is(err, skill.ErrCantAfford) ||
\t\terrors.Is(err, skill.ErrNotInStock) ||
\t\terrors.Is(err, skill.ErrBagNotRisen) {
\t\treturn OutcomeBlocked
\t}

\tvar blocked *skill.ErrBlocked
\tknownBlockage := errors.Is(err, world.ErrNoPath) ||
\t\terrors.Is(err, world.ErrNoRoute) ||
\t\terrors.Is(err, skill.ErrLegUnwalkable) ||
\t\terrors.Is(err, skill.ErrNoDialogue) ||
\t\terrors.Is(err, skill.ErrDialogueInterrupted) ||
\t\terrors.As(err, &blocked)
\tif knownBlockage {
\t\tif stableObjectiveBoundary(final) {
\t\t\treturn OutcomeBlocked
\t\t}
\t\treturn OutcomeStabilizationFailed
\t}

\treturn OutcomeUnknownFailure
}

func recoverableControllerFault(err error) bool {
\treturn errors.Is(err, emu.ErrFrameDeadline) ||
\t\terrors.Is(err, skill.ErrNavigationStalled) ||
\t\terrors.Is(err, skill.ErrReplanExhausted) ||
\t\terrors.Is(err, skill.ErrMenuStuck) ||
\t\terrors.Is(err, skill.ErrCutsceneTimeout) ||
\t\terrors.Is(err, skill.ErrForcedChoiceStuck) ||
\t\terrors.Is(err, skill.ErrPickupMenu) ||
\t\terrors.Is(err, skill.ErrShopMenuTimeout) ||
\t\terrors.Is(err, skill.ErrShopControllerStalled)
}

func stableObjectiveBoundary(final Observation) bool {
\treturn final.Controllable && !final.InBattle
}

func (r ObjectiveResult) HistoryText() string {
''')

# Typed fingerprint causes. Unsafe stabilization must win over the original
# recoverable shop/controller cause when errors.Join contains both.
replace_once("agent/failure_identity.go",
'''\tif errors.Is(err, ErrObjectivePostconditionUnavailable) {
\t\treturn "objective_postcondition_unavailable", nil
\t}
\tif errors.Is(err, emu.ErrFrameDeadline) {
\t\treturn "frame_deadline", nil
\t}
''',
'''\tif errors.Is(err, ErrObjectivePostconditionUnavailable) {
\t\treturn "objective_postcondition_unavailable", nil
\t}
\tif errors.Is(err, ErrObjectiveBoundaryDirty) {
\t\treturn "objective_boundary_dirty", nil
\t}
\tif errors.Is(err, skill.ErrShopStabilization) {
\t\treturn "shop_stabilization_failed", nil
\t}
\tif errors.Is(err, emu.ErrFrameDeadline) {
\t\treturn "frame_deadline", nil
\t}
\tif errors.Is(err, skill.ErrNavigationStalled) {
\t\treturn "navigation_stalled", nil
\t}
\tif errors.Is(err, skill.ErrShopMenuTimeout) {
\t\treturn "shop_menu_timeout", nil
\t}
\tif errors.Is(err, skill.ErrShopControllerStalled) {
\t\treturn "shop_controller_stalled", nil
\t}
''')
replace_once("agent/failure_identity.go",
'''\tif errors.Is(err, ErrObjectiveBoundaryDirty) {
\t\treturn "objective_boundary_dirty", nil
\t}
''',
''' ''')

# ---------------------------------------------------------------------------
# Universal same-state quarantine and exact survival-impact bookkeeping.
# ---------------------------------------------------------------------------
Path("agent/recovery.go").write_text(r'''package agent

import (
    "crypto/sha256"
    "encoding/json"
    "fmt"
)

type failureQuarantineEntry struct {
    Cause    FailureCauseID
    StateKey string
}

type failureQuarantine map[string]failureQuarantineEntry

func newFailureQuarantine() failureQuarantine { return failureQuarantine{} }

// recoveryStateKey hashes only FailureState's semantic, planner-relevant
// fields. Raw RAM/map encodings and diagnostic prose never decide retry policy.
func recoveryStateKey(obs Observation) string {
    data, _ := json.Marshal(FailureStateFor(obs))
    sum := sha256.Sum256(data)
    return fmt.Sprintf("%x", sum[:8])
}

func recoverableFailureKey(obj Objective, result ObjectiveResult) string {
    cause := result.Cause
    if cause == "" {
        cause = FailureCauseID("outcome:" + string(result.Outcome))
    }
    return obj.String() + "|" + string(cause) + "|" + recoveryStateKey(result.Final)
}

func (q failureQuarantine) record(result ObjectiveResult) {
    if q == nil || actionFor(result.Outcome) != actionReplan {
        return
    }
    q[result.Objective.String()] = failureQuarantineEntry{
        Cause: result.Cause, StateKey: recoveryStateKey(result.Final),
    }
}

// filter suppresses an exact failed objective while the relevant semantic
// state is unchanged and alternatives exist. A material state change expires
// the quarantine automatically. If every option is quarantined it fails open;
// the repeated-failure budget then provides the hard loop ceiling.
func (q failureQuarantine) filter(obs Observation, offered []Objective) []Objective {
    if q == nil || len(q) == 0 || len(offered) <= 1 {
        return offered
    }
    stateKey := recoveryStateKey(obs)
    out := make([]Objective, 0, len(offered))
    for _, o := range offered {
        entry, ok := q[o.String()]
        if !ok {
            out = append(out, o)
            continue
        }
        if entry.StateKey != stateKey {
            delete(q, o.String())
            out = append(out, o)
            continue
        }
    }
    if len(out) == 0 {
        return offered
    }
    return out
}

func (q failureQuarantine) clear(o Objective) {
    if q != nil {
        delete(q, o.String())
    }
}

func markLastOutcomeRecovered(res *Result) {
    if res == nil || len(res.Outcomes) == 0 {
        return
    }
    i := len(res.Outcomes) - 1
    res.Outcomes[i].Recovered = true
    res.Outcomes[i].Terminal = false
}

func markLastOutcomeTerminal(res *Result) {
    if res == nil || len(res.Outcomes) == 0 {
        return
    }
    i := len(res.Outcomes) - 1
    res.Outcomes[i].Recovered = false
    res.Outcomes[i].Terminal = true
}
''')

replace_once("agent/plan.go",
'''func recoverableFailureReplan(seen map[string]bool, obj Objective, result ObjectiveResult, blackedOut, retreated bool, consecutive, maxConsecutive int) (reason, key string, terminal bool) {
\tcause := string(result.Cause)
\tif cause == "" {
\t\tcause = string(result.Outcome)
\t}
\tkey = obj.String() + "|" + cause
''',
'''func recoverableFailureReplan(seen map[string]bool, obj Objective, result ObjectiveResult, blackedOut, retreated bool, consecutive, maxConsecutive int) (reason, key string, terminal bool) {
\tkey = recoverableFailureKey(obj, result)
''')

replace_once("agent/run.go",
'''\tplanning := newRunPlanning(resumedPlan)
\tnotifyPlanning(p, planning.snapshot())
''',
'''\tplanning := newRunPlanning(resumedPlan)
\tquarantine := newFailureQuarantine()
\tnotifyPlanning(p, planning.snapshot())
''')
replace_once("agent/run.go",
'''\tlastFailObj, lastFailErr := "", ""
''',
'''\tlastFailKey := ""
''')
replace_once("agent/run.go",
'''\t\tnow := offerWithTMHM(m, romData, last, known)
\t\tif len(now) == 0 {
''',
'''\t\tnow := offerWithTMHM(m, romData, last, known)
\t\tnow = quarantine.filter(last, now)
\t\tif len(now) == 0 {
''')

new_error_block = r'''\t\tif execErr != nil {
\t\t\t// The normalized result, rather than the raw error string, decides
\t\t\t// whether this is ordinary gameplay blockage or a terminal
\t\t\t// ownership/controller invariant. The raw typed error remains for
\t\t\t// diagnostics, Knowledge failure tallies, and errors.Is checks.
\t\t\tblackedOut := errors.Is(execErr, skill.ErrBlackedOut)
\t\t\tretreated := errors.Is(execErr, skill.ErrTrainRetreat)
\t\t\ttrainProgressed := errors.Is(execErr, skill.ErrTrainProgress)
\t\t\tif blackedOut {
\t\t\t\tlast.BlackedOut = true
\t\t\t\tobjectiveResult.Final = last
\t\t\t\tobjectiveResult.Summary += fmt.Sprintf(" (respawned in %s, money %d -> %d)",
\t\t\t\t\tlast.RespawnPlace, before.Money, last.Money)
\t\t\t}
\t\t\tres.Outcomes = append(res.Outcomes, objectiveResult)
\t\t\toutcome := objectiveResult.HistoryText()

\t\t\tknown.Failed(obj, execErr)
\t\t\tif trainProgressed {
\t\t\t\tknown.clearGymLossFailures()
\t\t\t}
\t\t\thistory = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: outcome})
\t\t\tlast.History = history
\t\t\tlast.RecentDialogue = tape.recent()
\t\t\tlogRound(budget.Log, round, obj, outcome, last)

\t\t\t// Observable goal completion wins over fault policy. The failure still
\t\t\t// remains in Outcomes/telemetry, but it did not terminate the campaign.
\t\t\tif deterministicGoal {
\t\t\t\tstatus := evaluateRunGoal(p, runGoal, last, round, budget.MaxRounds, intent, intentAge)
\t\t\t\tres.GoalStatus = &status
\t\t\t\tif status.Complete {
\t\t\t\t\tmarkLastOutcomeRecovered(&res)
\t\t\t\t\tres.Stop = StopDone
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t}

\t\t\taction := actionFor(objectiveResult.Outcome)
\t\t\tswitch action {
\t\t\tcase actionChoice, actionStop:
\t\t\t\tmarkLastOutcomeTerminal(&res)
\t\t\t\tres.Stop, res.Err = StopError, execErr
\t\t\t\tbreak
\t\t\tcase actionContinue:
\t\t\t\tmarkLastOutcomeTerminal(&res)
\t\t\t\tres.Stop = StopError
\t\t\t\tres.Err = fmt.Errorf("agent: objective %s returned error with completed outcome: %w", obj, execErr)
\t\t\t\tbreak
\t\t\tcase actionReplan:
\t\t\t\t// The final transaction boundary is trustworthy. Keep the fault in
\t\t\t\t// telemetry, quarantine this exact objective/state, and hand the
\t\t\t\t// fresh observation back to policy instead of killing the run.
\t\t\t\tquarantine.record(objectiveResult)
\t\t\t}
\t\t\tif res.Stop != StopUnset {
\t\t\t\tbreak
\t\t\t}

\t\t\tfailureKey := recoverableFailureKey(obj, objectiveResult)
\t\t\tif planning.hasStrategist(p) {
\t\t\t\tconsecFailures++
\t\t\t\treason, key, terminal := recoverableFailureReplan(
\t\t\t\t\tfailureEscalated, obj, objectiveResult, blackedOut, retreated, consecFailures, maxConsecFailures,
\t\t\t\t)
\t\t\t\tif terminal {
\t\t\t\t\tmarkLastOutcomeTerminal(&res)
\t\t\t\t\tres.Stop, res.Err = StopFailed, execErr
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t\tfailureEscalated[key] = true
\t\t\t\tplanning.request(reason)
\t\t\t\tnotifyPlanning(p, planning.snapshot())
\t\t\t\tmarkLastOutcomeRecovered(&res)
\t\t\t\tlastFailKey = failureKey
\t\t\t\tif m.FrameCount()-startFrame >= uint64(budget.MaxFrames) {
\t\t\t\t\tres.Stop = StopBudget
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t\tcontinue
\t\t\t}
\t\t\tif blackedOut || retreated {
\t\t\t\tretreatLevel := uint8(0)
\t\t\t\tif len(last.Party) > 0 {
\t\t\t\t\tretreatLevel = last.Party[0].Level
\t\t\t\t}
\t\t\t\tif retreated && retreatLevel != 0 && retreatLevel == lastRetreatLevel {
\t\t\t\t\tretreatStreak++
\t\t\t\t} else if retreated {
\t\t\t\t\tretreatStreak = 1
\t\t\t\t\tlastRetreatLevel = retreatLevel
\t\t\t\t} else {
\t\t\t\t\tretreatStreak, lastRetreatLevel = 0, 0
\t\t\t\t}
\t\t\t\tif retreated && retreatStreak >= maxConsecFailures {
\t\t\t\t\tmarkLastOutcomeTerminal(&res)
\t\t\t\t\tres.Stop, res.Err = StopFailed, execErr
\t\t\t\t} else {
\t\t\t\t\tmarkLastOutcomeRecovered(&res)
\t\t\t\t}
\t\t\t\tlastFailKey = ""
\t\t\t\tif m.FrameCount()-startFrame >= uint64(budget.MaxFrames) && res.Stop == StopUnset {
\t\t\t\t\tres.Stop = StopBudget
\t\t\t\t}
\t\t\t\tif res.Stop != StopUnset {
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t\tcontinue
\t\t\t}
\t\t\tretreatStreak, lastRetreatLevel = 0, 0

\t\t\tconsecFailures++
\t\t\tfaultTerminal := false
\t\t\tswitch {
\t\t\tcase lastFailKey != "" && failureKey == lastFailKey:
\t\t\t\tfaultTerminal = true
\t\t\t\tres.Stop, res.Err = StopFailed, execErr
\t\t\tcase consecFailures >= maxConsecFailures:
\t\t\t\tfaultTerminal = true
\t\t\t\tres.Stop, res.Err = StopFailed, execErr
\t\t\tcase m.FrameCount()-startFrame >= uint64(budget.MaxFrames):
\t\t\t\tres.Stop = StopBudget
\t\t\t}
\t\t\tif faultTerminal {
\t\t\t\tmarkLastOutcomeTerminal(&res)
\t\t\t} else {
\t\t\t\tmarkLastOutcomeRecovered(&res)
\t\t\t}
\t\t\tif res.Stop != StopUnset {
\t\t\t\tbreak
\t\t\t}
\t\t\tlastFailKey = failureKey
\t\t\tcontinue
\t\t}
'''
replace_between("agent/run.go", "\t\tif execErr != nil {\n", "\n\t\tres.Outcomes = append(res.Outcomes, objectiveResult)\n", new_error_block)
replace_once("agent/run.go",
'''\t\tres.Completed = append(res.Completed, obj)
\t\tplanning.success(fromPlan)
''',
'''\t\tres.Completed = append(res.Completed, obj)
\t\tquarantine.clear(obj)
\t\tplanning.success(fromPlan)
''')
replace_once("agent/run.go",
'''\t\tlastFailObj, lastFailErr = "", ""
''',
'''\t\tlastFailKey = ""
''')


# ---------------------------------------------------------------------------
# Failure artifact v3: preserve per-occurrence recovery/terminal impact.
# ---------------------------------------------------------------------------
replace_once("farm/failure.go", "objectiveFailureVersion      = 2", "objectiveFailureVersion      = 3")
replace_once("farm/failure.go",
'''\tRecovered  bool      `json:"recovered"`
\tBlocking   bool      `json:"blocking,omitempty"`
''',
'''\tRecovered      bool      `json:"recovered"`
\tRecoveredCount int       `json:"recovered_count,omitempty"`
\tTerminalCount  int       `json:"terminal_count,omitempty"`
\tBlocking       bool      `json:"blocking,omitempty"`
''')
replace_once("farm/failure.go",
'''\tif env.Version != 1 && env.Version != objectiveFailureVersion {
\t\treturn nil, fmt.Errorf("farm: objective failure telemetry version %d, want 1 or %d", env.Version, objectiveFailureVersion)
\t}
\tif env.Version >= 2 {
''',
'''\tif env.Version != 1 && env.Version != 2 && env.Version != objectiveFailureVersion {
\t\treturn nil, fmt.Errorf("farm: objective failure telemetry version %d, want 1, 2 or %d", env.Version, objectiveFailureVersion)
\t}
\tif env.Version < 3 {
\t\tfor i := range env.Failures {
\t\t\tif env.Failures[i].Recovered {
\t\t\t\tenv.Failures[i].RecoveredCount = env.Failures[i].Count
\t\t\t} else if env.Failures[i].Blocking && env.Failures[i].Count > 0 {
\t\t\t\tenv.Failures[i].TerminalCount = 1
\t\t\t}
\t\t}
\t}
\tif env.Version >= 2 {
''')
replace_once("farm/failure.go",
'''func validateObjectiveFailure(f ObjectiveFailure) error {
\tif f.Identity == nil {
''',
'''func validateObjectiveFailure(f ObjectiveFailure) error {
\tif f.RecoveredCount < 0 || f.TerminalCount < 0 || f.RecoveredCount+f.TerminalCount > f.Count {
\t\treturn fmt.Errorf("invalid recovery impact counts recovered=%d terminal=%d count=%d", f.RecoveredCount, f.TerminalCount, f.Count)
\t}
\tif f.Identity == nil {
''')

# Replace telemetry aggregation with explicit impact accounting plus a fallback
# for older/in-memory results that predate the new booleans.
replace_between("cmd/pokepilot/failure_telemetry.go",
"func drainObjectiveFailureTelemetry(reason, build, checkpointDir string) ([]farm.ObjectiveFailure, *farm.FailureOccurrence) {\n",
"func farmIdentityFromAgent(result agent.ObjectiveResult) farm.FailureIdentity {\n",
r'''func drainObjectiveFailureTelemetry(reason, build, checkpointDir string) ([]farm.ObjectiveFailure, *farm.FailureOccurrence) {
\tobjectiveFailureTelemetry.Lock()
\tres := objectiveFailureTelemetry.result
\tobjectiveFailureTelemetry.result = nil
\tobjectiveFailureTelemetry.Unlock()
\tif res == nil {
\t\treturn nil, nil
\t}

\ttype grouped struct {
\t\tfailure        farm.ObjectiveFailure
\t\tlastIdx        int
\t\tlastOccurrence farm.FailureOccurrence
\t}
\tgroups := map[string]*grouped{}
\tobservedAt := time.Now().UTC()
\tvar terminal *farm.FailureOccurrence
\tlastFailureIdx := -1
\tlastFailureFingerprint := ""

\tfor i, result := range res.Outcomes {
\t\tif result.Outcome == agent.OutcomeCompleted || expectedStructuredGameOutcome(result) {
\t\t\tcontinue
\t\t}
\t\tidentity := farmIdentityFromAgent(result)
\t\tcheckpoint := checkpointForRound(checkpointDir, i+1)
\t\toccurrence, err := farm.NewFailureOccurrence(identity, build, i+1, checkpoint, result.Summary, observedAt)
\t\tif err != nil {
\t\t\tcontinue
\t\t}
\t\tg := groups[occurrence.Fingerprint]
\t\tif g == nil {
\t\t\tid := occurrence.Identity
\t\t\tg = &grouped{failure: farm.ObjectiveFailure{
\t\t\t\tObjective:    result.Objective.String(),
\t\t\t\tError:        result.Summary,
\t\t\t\tFirstRound:   i + 1,
\t\t\t\tMap:          result.Final.Map,
\t\t\t\tX:            result.Final.X,
\t\t\t\tY:            result.Final.Y,
\t\t\t\tObservedAt:   observedAt,
\t\t\t\tKey:          occurrence.Key,
\t\t\t\tFingerprint:  occurrence.Fingerprint,
\t\t\t\tIdentity:     &id,
\t\t\t\tBuild:        occurrence.Build,
\t\t\t\tOutcome:      string(result.Outcome),
\t\t\t\tCause:        string(result.Cause),
\t\t\t\tCauseContext: append([]string(nil), result.CauseContext...),
\t\t\t\tCheckpoint:   occurrence.Checkpoint,
\t\t\t}}
\t\t\tgroups[occurrence.Fingerprint] = g
\t\t}
\t\tg.failure.Count++
\t\tif result.Recovered {
\t\t\tg.failure.RecoveredCount++
\t\t}
\t\tif result.Terminal {
\t\t\tg.failure.TerminalCount++
\t\t\tocc := occurrence
\t\t\tterminal = &occ
\t\t}
\t\tg.failure.LastRound = i + 1
\t\tg.failure.Map = result.Final.Map
\t\tg.failure.X = result.Final.X
\t\tg.failure.Y = result.Final.Y
\t\tg.failure.Error = result.Summary
\t\tg.failure.Checkpoint = occurrence.Checkpoint
\t\tg.lastIdx = i
\t\tg.lastOccurrence = occurrence
\t\tlastFailureIdx = i
\t\tlastFailureFingerprint = occurrence.Fingerprint
\t}

\t// Backward compatibility for Results produced before per-occurrence impact
\t// flags existed. New Run paths mark every non-completed result explicitly.
\tfailureDrivenStop := reason == "failed" || reason == "stuck" || reason == "error"
\tif terminal == nil && failureDrivenStop && lastFailureIdx >= 0 {
\t\tif g := groups[lastFailureFingerprint]; g != nil && g.failure.TerminalCount == 0 {
\t\t\tg.failure.TerminalCount++
\t\t\tocc := g.lastOccurrence
\t\t\tterminal = &occ
\t\t}
\t}

\tout := make([]farm.ObjectiveFailure, 0, len(groups))
\tfor _, g := range groups {
\t\tunknown := g.failure.Count - g.failure.RecoveredCount - g.failure.TerminalCount
\t\tif unknown > 0 && (reason == "done" || majorProgressAfter(res.Outcomes, g.lastIdx)) {
\t\t\tg.failure.RecoveredCount += unknown
\t\t}
\t\tg.failure.Recovered = g.failure.RecoveredCount > 0 && g.failure.TerminalCount == 0
\t\tg.failure.Blocking = g.failure.TerminalCount > 0 && g.failure.Count >= 2 && failureDrivenStop
\t\tout = append(out, g.failure)
\t}
\tsort.Slice(out, func(i, j int) bool {
\t\tif out[i].Blocking != out[j].Blocking {
\t\t\treturn out[i].Blocking
\t\t}
\t\tif out[i].TerminalCount != out[j].TerminalCount {
\t\t\treturn out[i].TerminalCount > out[j].TerminalCount
\t\t}
\t\tif out[i].Count != out[j].Count {
\t\t\treturn out[i].Count > out[j].Count
\t\t}
\t\tif out[i].LastRound != out[j].LastRound {
\t\t\treturn out[i].LastRound > out[j].LastRound
\t\t}
\t\treturn out[i].Fingerprint < out[j].Fingerprint
\t})
\treturn out, terminal
}

func farmIdentityFromAgent(result agent.ObjectiveResult) farm.FailureIdentity {
''')

# Wall evidence and issue summary distinguish survival impact counts.
replace_once("cmd/pokewall/objective_failures.go",
'''\tseverity := "normal"
\tclassification := "observed"
\tif f.Recovered {
\t\tclassification = "recovered"
\t}
''',
'''\tseverity := "normal"
\tclassification := "observed"
\tif f.TerminalCount > 0 {
\t\tclassification = "terminal"
\t} else if f.Recovered {
\t\tclassification = "recovered"
\t}
''')
replace_once("cmd/pokewall/objective_failures.go",
'''\t\t"recovered":            f.Recovered,
\t\t"blocking":             f.Blocking,
''',
'''\t\t"recovered":            f.Recovered,
\t\t"recovered_count":      f.RecoveredCount,
\t\t"terminal_count":       f.TerminalCount,
\t\t"blocking":             f.Blocking,
''')
replace_once("cmd/pokewall/objective_failures.go",
'''\tsummary := fmt.Sprintf("%s failed %d time(s) on map 0x%02x; recovered=%t; run ended %s.",
\t\tf.Objective, f.Count, f.Map, f.Recovered, dump.Reason)
''',
'''\tsummary := fmt.Sprintf("%s failed %d time(s) on map 0x%02x; recovered=%d terminal=%d; run ended %s.",
\t\tf.Objective, f.Count, f.Map, f.RecoveredCount, f.TerminalCount, dump.Reason)
''')
replace_once("cmd/pokewall/objective_failures.go",
'''\t\tsummary = fmt.Sprintf("Progression blocker candidate: %s failed %d time(s) on map 0x%02x with no later major progress; run ended %s. Last error: %s",
\t\t\tf.Objective, f.Count, f.Map, dump.Reason, f.Error)
''',
'''\t\tsummary = fmt.Sprintf("Progression blocker candidate: %s failed %d time(s) on map 0x%02x (recovered=%d terminal=%d) with no later major progress; run ended %s. Last error: %s",
\t\t\tf.Objective, f.Count, f.Map, f.RecoveredCount, f.TerminalCount, dump.Reason, f.Error)
''')


# ---------------------------------------------------------------------------
# Focused tests for classification, quarantine, telemetry and artifact schema.
# ---------------------------------------------------------------------------
Path("agent/recovery_faults_test.go").write_text(r'''package agent

import (
    "errors"
    "fmt"
    "testing"

    "github.com/maestroi/pokepilot/skill"
)

func TestRecoverableControllerFaultRequiresStableBoundary(t *testing.T) {
    cases := []struct {
        name string
        err  error
    }{
        {"navigation stall", skill.ErrNavigationStalled},
        {"shop timeout", skill.ErrShopMenuTimeout},
        {"shop controller", skill.ErrShopControllerStalled},
        {"menu stuck", skill.ErrMenuStuck},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            wrapped := fmt.Errorf("owned action: %w", tc.err)
            if got := classifyObjectiveOutcome(Objective{}, wrapped, Observation{Controllable: true}); got != OutcomeBlocked {
                t.Fatalf("stable outcome = %q, want blocked", got)
            }
            if got := classifyObjectiveOutcome(Objective{}, wrapped, Observation{}); got != OutcomeControllerUncertain {
                t.Fatalf("unsafe outcome = %q, want controller_uncertain", got)
            }
        })
    }
}

func TestDirtyBoundaryDominatesRecoverableControllerCause(t *testing.T) {
    o := Objective{Kind: KindGoTo, Place: "route 1"}
    primary := fmt.Errorf("%w: repeated state", skill.ErrNavigationStalled)
    combined := objectiveBoundaryError(o, primary, fmt.Errorf("%w: cleanup failed", ErrObjectiveBoundaryDirty))
    if !errors.Is(combined, skill.ErrNavigationStalled) || !errors.Is(combined, ErrObjectiveBoundaryDirty) {
        t.Fatalf("combined error lost identity: %v", combined)
    }
    if got := classifyObjectiveOutcome(o, combined, Observation{Controllable: true}); got != OutcomeStabilizationFailed {
        t.Fatalf("outcome = %q, want stabilization_failed", got)
    }
    cause, _ := failureCauseFor(combined)
    if cause != "objective_boundary_dirty" {
        t.Fatalf("cause = %q, want objective_boundary_dirty", cause)
    }
}

func TestShopStabilizationIsTerminalEvenIfFinalSnapshotLooksControllable(t *testing.T) {
    err := errors.Join(skill.ErrShopMenuTimeout, skill.ErrShopStabilization)
    if got := classifyObjectiveOutcome(Objective{}, err, Observation{Controllable: true}); got != OutcomeStabilizationFailed {
        t.Fatalf("outcome = %q, want stabilization_failed", got)
    }
    cause, _ := failureCauseFor(err)
    if cause != "shop_stabilization_failed" {
        t.Fatalf("cause = %q, want shop_stabilization_failed", cause)
    }
}

func TestPostconditionFailureReplansButUnknownFailureStops(t *testing.T) {
    if got := actionFor(OutcomePostconditionFailed); got != actionReplan {
        t.Fatalf("postcondition action = %d, want replan", got)
    }
    if got := actionFor(OutcomeUnknownFailure); got != actionStop {
        t.Fatalf("unknown action = %d, want stop", got)
    }
}

func TestRecoverableFailureCauseVocabulary(t *testing.T) {
    for _, tc := range []struct {
        err  error
        want FailureCauseID
    }{
        {skill.ErrNavigationStalled, "navigation_stalled"},
        {skill.ErrShopMenuTimeout, "shop_menu_timeout"},
        {skill.ErrShopControllerStalled, "shop_controller_stalled"},
        {skill.ErrShopStabilization, "shop_stabilization_failed"},
    } {
        got, _ := failureCauseFor(tc.err)
        if got != tc.want {
            t.Errorf("cause(%v) = %q, want %q", tc.err, got, tc.want)
        }
    }
}

func TestFailureQuarantineSuppressesSameStateAndExpiresOnWorldChange(t *testing.T) {
    failed := Objective{Kind: KindGoTo, Place: "route 1"}
    other := Objective{Kind: KindGoTo, Place: "pallet town"}
    obs := Observation{Location: "viridian city", X: 10, Y: 12, Controllable: true, Money: 100}
    q := newFailureQuarantine()
    q.record(ObjectiveResult{Objective: failed, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: obs})

    got := q.filter(obs, []Objective{failed, other})
    if len(got) != 1 || got[0].String() != other.String() {
        t.Fatalf("same-state filter = %+v, want only %s", got, other)
    }

    changed := obs
    changed.Money++
    got = q.filter(changed, []Objective{failed, other})
    if len(got) != 2 {
        t.Fatalf("changed-state filter = %+v, want both objectives", got)
    }
}

func TestFailureQuarantineFailsOpenWhenNoAlternativeExists(t *testing.T) {
    failed := Objective{Kind: KindGoTo, Place: "route 1"}
    obs := Observation{Location: "viridian city", X: 10, Y: 12, Controllable: true}
    q := newFailureQuarantine()
    q.record(ObjectiveResult{Objective: failed, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: obs})
    got := q.filter(obs, []Objective{failed})
    if len(got) != 1 || got[0].String() != failed.String() {
        t.Fatalf("single-option filter = %+v, want fail-open", got)
    }
}

func TestRecoverableFailureFingerprintChangesWithRelevantState(t *testing.T) {
    obj := Objective{Kind: KindGoTo, Place: "route 1"}
    a := ObjectiveResult{Objective: obj, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: Observation{Location: "viridian city", X: 1, Y: 2, Controllable: true, Money: 100}}
    b := a
    b.Final.Money = 200
    if recoverableFailureKey(obj, a) == recoverableFailureKey(obj, b) {
        t.Fatal("money change did not change failure state fingerprint")
    }
}
''')

Path("skill/shop_error_test.go").write_text(r'''package skill

import (
    "errors"
    "testing"

    "github.com/maestroi/pokepilot/red/state"
)

func TestMartTimeoutIsTyped(t *testing.T) {
    var mem state.Mem
    err := martTimeout("the item list", &mem)
    if !errors.Is(err, ErrShopMenuTimeout) {
        t.Fatalf("martTimeout = %v, want ErrShopMenuTimeout", err)
    }
}

func TestShopStabilizationPreservesOriginalCause(t *testing.T) {
    err := shopStabilizationFailure(ErrShopMenuTimeout, errors.New("still in menu"))
    if !errors.Is(err, ErrShopMenuTimeout) || !errors.Is(err, ErrShopStabilization) {
        t.Fatalf("joined error lost identity: %v", err)
    }
}
''')

# Add explicit impact-count telemetry test.
p = Path("cmd/pokepilot/failure_telemetry_test.go")
s = p.read_text()
s += r'''

func TestObjectiveFailureTelemetryCountsRecoveredAndTerminalOccurrences(t *testing.T) {
    resetObjectiveFailureTelemetry()
    t.Cleanup(resetObjectiveFailureTelemetry)

    recovered1 := structuredFailureResult("navigation_stalled", 10)
    recovered1.Recovered = true
    recovered2 := recovered1
    terminalFailure := recovered1
    terminalFailure.Recovered = false
    terminalFailure.Terminal = true

    captureObjectiveFailureTelemetry(agent.Result{Outcomes: []agent.ObjectiveResult{recovered1, recovered2, terminalFailure}})
    got, terminal := drainObjectiveFailureTelemetry("failed", "build-a", "")
    if len(got) != 1 {
        t.Fatalf("failures = %+v, want one group", got)
    }
    f := got[0]
    if f.Count != 3 || f.RecoveredCount != 2 || f.TerminalCount != 1 {
        t.Fatalf("impact counts = %+v, want count=3 recovered=2 terminal=1", f)
    }
    if f.Recovered || !f.Blocking {
        t.Fatalf("mixed impact classification = recovered=%t blocking=%t", f.Recovered, f.Blocking)
    }
    if terminal == nil || terminal.Fingerprint != f.Fingerprint {
        t.Fatalf("terminal = %+v, want group fingerprint %q", terminal, f.Fingerprint)
    }
}
'''
p.write_text(s)

p = Path("farm/failure_test.go")
s = p.read_text()
s += r'''

func TestObjectiveFailureArtifactPreservesImpactCounts(t *testing.T) {
    want := []ObjectiveFailure{{Objective: "buy 3 pokeball", Error: "shop menu timeout", Count: 4, RecoveredCount: 3, TerminalCount: 1}}
    artifact, err := NewObjectiveFailureArtifact(want)
    if err != nil { t.Fatalf("artifact: %v", err) }
    got, err := DecodeObjectiveFailures(FinishReport{Artifacts: []Artifact{artifact}})
    if err != nil { t.Fatalf("decode: %v", err) }
    if !reflect.DeepEqual(got, want) { t.Fatalf("got %+v want %+v", got, want) }
}

func TestObjectiveFailureRejectsImpossibleImpactCounts(t *testing.T) {
    _, err := NewObjectiveFailureArtifact([]ObjectiveFailure{{Objective: "x", Count: 1, RecoveredCount: 1, TerminalCount: 1}})
    if err == nil { t.Fatal("accepted recovered+terminal counts greater than occurrence count") }
}
'''
p.write_text(s)
