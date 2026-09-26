package skill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	eventGotHM04             state.Event = 0x238
	eventGaveGoldTeeth       state.Event = 0x239
	eventInSafariZone        state.Event = 0x24F
	eventFightRoute12Snorlax state.Event = 0x48E
	eventBeatRoute12Snorlax  state.Event = 0x48F
	eventGotHM03             state.Event = 0x880
	fuchsiaStoryBudget                   = 12000
	fuchsiaTravelEngagements             = 80
	maxSafariSessions                    = 3
)

// FuchsiaProgression executes issue #33 as one resumable story verb. Its
// positive postcondition is intentionally delegated to
// FuchsiaProgressionComplete: Soul Badge + HM03 + HM04 must all be present.
func FuchsiaProgression(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: FuchsiaProgression: nil policy")
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if FuchsiaProgressionComplete(&mem) {
		return nil
	}
	if !FuchsiaProgressionReady(&mem) {
		return fmt.Errorf("skill: FuchsiaProgression: POKE FLUTE is required")
	}
	if !FuchsiaProgressionAvailable(mem.U8(sym.CurMap)) {
		return fmt.Errorf("skill: FuchsiaProgression: map %#04x is outside the supported Lavender/Fuchsia slice", mem.U8(sym.CurMap))
	}

	if !state.HasEvent(&mem, eventBeatRoute12Snorlax) {
		if err := clearRoute12Snorlax(m, romData, policy); err != nil {
			return err
		}
	}

	// A resumed save may already be inside the Safari Zone. Consume that
	// finite 502-step session before routing back to a Center/Gym; otherwise
	// resuming this objective would throw away the remaining Safari budget.
	state.Snapshot(m, &mem)
	if state.HasEvent(&mem, eventInSafariZone) && needsSafariRewards(&mem) {
		if err := collectSafariRewards(m, romData, policy); err != nil {
			return err
		}
	}

	state.Snapshot(m, &mem)
	if !state.DecodeProgress(&mem).Has(state.BadgeSoul) {
		// Establish Fuchsia as the recovery checkpoint before Koga. This also
		// gives the trainer-heavy eastern route a clean heal.
		center, ok := Place("fuchsia pokemon center")
		if !ok {
			return fmt.Errorf("skill: FuchsiaProgression: fuchsia pokemon center place missing")
		}
		if _, err := TravelFlee(m, romData, center, policy, fuchsiaTravelEngagements); err != nil {
			return fmt.Errorf("skill: FuchsiaProgression: reach Fuchsia Pokemon Center: %w", err)
		}
		if err := Heal(m); err != nil {
			return fmt.Errorf("skill: FuchsiaProgression: heal in Fuchsia: %w", err)
		}

		gym, ok := Place("fuchsia gym")
		if !ok {
			return fmt.Errorf("skill: FuchsiaProgression: fuchsia gym place missing")
		}
		if _, err := Travel(m, romData, gym, policy, 30); err != nil {
			return fmt.Errorf("skill: FuchsiaProgression: enter Fuchsia Gym: %w", err)
		}
		outcome, err := Gym(m, romData, policy)
		if err != nil {
			return fmt.Errorf("skill: FuchsiaProgression: Koga: %w", err)
		}
		if outcome != state.ResultWon {
			return fuchsiaKogaOutcomeErr(outcome)
		}
	}

	state.Snapshot(m, &mem)
	if needsSafariRewards(&mem) {
		if err := collectSafariRewards(m, romData, policy); err != nil {
			return err
		}
	}

	state.Snapshot(m, &mem)
	if !hasBagItem(&mem, hm04StrengthItem) {
		if err := receiveStrengthFromWarden(m, romData, policy); err != nil {
			return err
		}
	}

	state.Snapshot(m, &mem)
	if !FuchsiaProgressionComplete(&mem) {
		return fmt.Errorf("skill: FuchsiaProgression: incomplete after execution: soul=%v surf=%v strength=%v",
			state.DecodeProgress(&mem).Has(state.BadgeSoul), hasBagItem(&mem, hm03SurfItem), hasBagItem(&mem, hm04StrengthItem))
	}
	return nil
}

// fuchsiaKogaOutcomeErr maps the Koga battle outcome to the skill's error. A
// loss keeps the typed trainer-blackout signal so the planner can train or
// grow the party and retry the slice, instead of a raw outcome that classifies
// as an unknown, unrecoverable failure.
func fuchsiaKogaOutcomeErr(outcome state.BattleResult) error {
	if err := RequireTrainerBattleWin("gym:koga", outcome); err != nil {
		return fmt.Errorf("skill: FuchsiaProgression: %w", err)
	}
	return nil
}

func hasBagItem(mem *state.Mem, item uint8) bool {
	_, n := bagEntry(mem, item)
	return n > 0
}

func needsSafariRewards(mem *state.Mem) bool {
	needSurf := !hasBagItem(mem, hm03SurfItem)
	needTeeth := !hasBagItem(mem, goldTeethItem) && !state.HasEvent(mem, eventGaveGoldTeeth)
	return needSurf || needTeeth
}

func clearRoute12Snorlax(m *emu.Emu, romData []byte, policy MovePolicy) error {
	dest, ok := Place("route 12 snorlax")
	if !ok {
		return fmt.Errorf("skill: FuchsiaProgression: route 12 snorlax place missing")
	}
	if _, err := TravelFlee(m, romData, dest, policy, fuchsiaTravelEngagements); err != nil {
		return fmt.Errorf("skill: FuchsiaProgression: reach Route 12 Snorlax: %w", err)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.HasEvent(&mem, eventBeatRoute12Snorlax) {
		return nil
	}
	if err := useOverworldKeyItem(m, pokeFluteItemFuchsia, func(mm *state.Mem) bool {
		return state.HasEvent(mm, eventFightRoute12Snorlax) || state.DecodeBattle(mm) != nil
	}); err != nil {
		return fmt.Errorf("skill: FuchsiaProgression: wake Route 12 Snorlax: %w", err)
	}

	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) == nil {
		if err := driveStoryUntil(m, fuchsiaStoryBudget, func(mm *state.Mem) bool {
			return state.DecodeBattle(mm) != nil
		}); err != nil {
			return fmt.Errorf("skill: FuchsiaProgression: Snorlax battle did not start: %w", err)
		}
	}
	outcome, err := Battle(m, policy)
	if err != nil {
		return fmt.Errorf("skill: FuchsiaProgression: Snorlax battle: %w", err)
	}
	if err := RequireBattleWin("static:route12_snorlax", outcome); err != nil {
		return fmt.Errorf("skill: FuchsiaProgression: Snorlax battle: %w", err)
	}
	if err := Cutscene(m, fuchsiaStoryBudget, func(mm *state.Mem) bool {
		return state.HasEvent(mm, eventBeatRoute12Snorlax)
	}); err != nil {
		return fmt.Errorf("skill: FuchsiaProgression: settle Snorlax story: %w", err)
	}
	return nil
}

// useOverworldKeyItem performs START -> ITEM -> bag entry -> USE for key
// items whose effect does not open a party selector. It reuses the verified
// menu primitives used by UseFieldItem rather than relying on press counts.
func useOverworldKeyItem(m *emu.Emu, item uint8, started func(*state.Mem) bool) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return fmt.Errorf("player not controllable")
	}
	idx, _ := bagEntry(&mem, item)
	if idx < 0 {
		return fmt.Errorf("%w (id %#02x)", ErrNotInBag, item)
	}

	if err := openStartMenuEntry(m, startMenuItems); err != nil {
		return fmt.Errorf("open ITEM: %w", err)
	}
	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.ListMenuID) == itemListMenuID
	}); err != nil {
		return fmt.Errorf("bag list did not open: %w", err)
	}
	if err := selectBagEntry(m, idx); err != nil {
		return err
	}
	if _, err := m.StepUntil(useTossBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return useTossPrompt(&mem) != nil
	}); err != nil {
		return fmt.Errorf("USE/TOSS prompt did not open: %w", err)
	}
	state.Snapshot(m, &mem)
	if p := useTossPrompt(&mem); p == nil || p.Index != 0 {
		return fmt.Errorf("USE/TOSS cursor is not on USE")
	}
	m.Tap(emu.A, 3, 7)
	return driveStoryUntil(m, fuchsiaStoryBudget, started)
}

func driveStoryUntil(m *emu.Emu, budget int, done func(*state.Mem) bool) error {
	var mem state.Mem
	for spent := 0; spent < budget; spent += 10 {
		state.Snapshot(m, &mem)
		if done(&mem) {
			return nil
		}
		// A advances ordinary text and accepts the default YES choice used
		// by the Safari gate. During scripted walking or animations it is
		// ignored. Predicates are checked before every tap, so we never tap
		// into a battle/menu after the desired transition has happened.
		m.Tap(emu.A, 3, 7)
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("story transition exceeded %d frames on map %#04x at (%d,%d)", budget,
		mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
}

func collectSafariRewards(m *emu.Emu, romData []byte, policy MovePolicy) error {
	for session := 1; session <= maxSafariSessions; session++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if !needsSafariRewards(&mem) {
			return leaveSafariZoneIfNeeded(m, romData, policy)
		}
		if !state.HasEvent(&mem, eventInSafariZone) {
			if err := enterSafariZone(m, romData, policy); err != nil {
				return fmt.Errorf("skill: FuchsiaProgression: enter Safari Zone session %d: %w", session, err)
			}
		}

		state.Snapshot(m, &mem)
		if !hasBagItem(&mem, goldTeethItem) && !state.HasEvent(&mem, eventGaveGoldTeeth) {
			teethStand, ok := Place("safari gold teeth")
			if !ok {
				return fmt.Errorf("skill: FuchsiaProgression: safari gold teeth place missing")
			}
			if _, err := TravelFlee(m, romData, teethStand, policy, fuchsiaTravelEngagements); err != nil {
				state.Snapshot(m, &mem)
				if !state.HasEvent(&mem, eventInSafariZone) {
					continue
				}
				return fmt.Errorf("skill: FuchsiaProgression: reach Gold Teeth: %w", err)
			}
			if err := Pickup(m, romData, 19, 7, goldTeethItem, policy); err != nil {
				return fmt.Errorf("skill: FuchsiaProgression: collect Gold Teeth: %w", err)
			}
		}

		state.Snapshot(m, &mem)
		if !hasBagItem(&mem, hm03SurfItem) {
			secret, ok := Place("safari secret house")
			if !ok {
				return fmt.Errorf("skill: FuchsiaProgression: safari secret house place missing")
			}
			if _, err := TravelFlee(m, romData, secret, policy, fuchsiaTravelEngagements); err != nil {
				state.Snapshot(m, &mem)
				if !state.HasEvent(&mem, eventInSafariZone) {
					continue
				}
				return fmt.Errorf("skill: FuchsiaProgression: reach Safari Secret House: %w", err)
			}
			if err := EnsureBagSpaceFor(m, hm03SurfItem); err != nil {
				return fmt.Errorf("skill: FuchsiaProgression: make room for HM03: %w", err)
			}
			if _, err := TalkAt(m, romData, 3, 3, policy); err != nil {
				return fmt.Errorf("skill: FuchsiaProgression: receive HM03: %w", err)
			}
			state.Snapshot(m, &mem)
			if !hasBagItem(&mem, hm03SurfItem) || !state.HasEvent(&mem, eventGotHM03) {
				return fmt.Errorf("skill: FuchsiaProgression: HM03 was not positively awarded after bag-capacity preflight")
			}
		}
	}
	return fmt.Errorf("skill: FuchsiaProgression: Safari rewards still incomplete after %d bounded sessions", maxSafariSessions)
}

// safariGateJoinChoiceIndex recognizes only the paid Safari entrance prompt.
// FuchsiaProgression owns this transaction, so it may answer it deliberately:
// YES when entering for rewards, NO when a resumed/finished session is trying
// to continue south toward Fuchsia and the Warden. Keeping the map and text
// checks here prevents generic travel from guessing at unrelated choices.
func safariGateJoinChoiceIndex(mapID uint8, text string, join bool) (int, bool) {
	if mapID != safariZoneGateMap {
		return 0, false
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	if !strings.Contains(normalized, "would you like to join the hunt?") {
		return 0, false
	}
	if join {
		return 0, true // YES
	}
	return 1, true // NO
}

func answerSafariGateJoinChoice(m *emu.Emu, text string, join bool) (bool, error) {
	index, ok := safariGateJoinChoiceIndex(m.Peek8(sym.CurMap), text, join)
	if !ok {
		return false, nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeTwoOptionMenu(&mem) == nil {
		return false, nil
	}
	if err := selectTwoOption(m, index); err != nil {
		return true, err
	}
	return true, nil
}

func recoverAfterSafariGateChoice(m *emu.Emu) error {
	rec := RecoverDialogue(m, dialogueRecoveryBudget)
	switch rec.Stop {
	case DialogueRecovered:
		return nil
	case DialogueChoiceRequired, DialogueMenuOpen:
		return fmt.Errorf("Safari gate answer led to another unanswered choice/menu: %q", rec.Text)
	case DialogueBudgetExhausted:
		return fmt.Errorf("Safari gate answer text did not clear: %q", rec.Text)
	case DialogueUnexpectedMode:
		return fmt.Errorf("Safari gate answer unexpectedly entered battle")
	default:
		return fmt.Errorf("Safari gate answer recovery stopped with %d", rec.Stop)
	}
}

func enterSafariZone(m *emu.Emu, romData []byte, policy MovePolicy) error {
	gate, ok := Place("safari zone gate")
	if !ok {
		return fmt.Errorf("safari zone gate place missing")
	}
	if _, err := TravelFlee(m, romData, gate, policy, fuchsiaTravelEngagements); err != nil {
		var choice *ErrDialogueChoice
		if !errors.As(err, &choice) {
			return err
		}
		handled, answerErr := answerSafariGateJoinChoice(m, choice.Result.Text, true)
		if answerErr != nil {
			return fmt.Errorf("answer Safari entry prompt: %w", answerErr)
		}
		if !handled {
			return err
		}
		return driveStoryUntil(m, fuchsiaStoryBudget, func(mm *state.Mem) bool {
			return state.HasEvent(mm, eventInSafariZone) && mm.U8(sym.CurMap) == safariZoneCenterMap && state.Controllable(mm)
		})
	}
	if m.Peek8(sym.CurMap) != safariZoneGateMap {
		return fmt.Errorf("expected Safari gate, on %#04x", m.Peek8(sym.CurMap))
	}
	// (3,2) is the script trigger immediately above our stable (3,3) gate
	// target. The gate's YES/NO prompt defaults to YES; driveStoryUntil
	// advances that script until EVENT_IN_SAFARI_ZONE and the center warp.
	m.Tap(emu.Up, 3, 7)
	if err := driveStoryUntil(m, fuchsiaStoryBudget, func(mm *state.Mem) bool {
		return state.HasEvent(mm, eventInSafariZone) && mm.U8(sym.CurMap) == safariZoneCenterMap && state.Controllable(mm)
	}); err != nil {
		return err
	}
	return nil
}

const (
	safariExitTaps         = 4
	safariExitSettleFrames = 40

	safariRejoinPromptSettleFrames = 240
)

func leaveSafariZoneIfNeeded(m *emu.Emu, romData []byte, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, eventInSafariZone) {
		return nil
	}
	exit, ok := Place("safari exit approach")
	if !ok {
		return fmt.Errorf("safari exit approach place missing")
	}
	if _, err := TravelFlee(m, romData, exit, policy, fuchsiaTravelEngagements); err != nil {
		state.Snapshot(m, &mem)
		if !state.HasEvent(&mem, eventInSafariZone) {
			return declineSafariRejoinPrompt(m)
		}
		return err
	}
	// Center (14,25) is the south gate warp. One Down enters the gate, whose
	// early-leave prompt defaults to YES. If the Safari timer already expired,
	// the same predicate simply observes the automatic ejection.
	//
	// A short tap toward a new direction only turns the player, and a tap
	// during a step is swallowed, so arriving at the approach tile from the
	// warp side (facing up) needs more than one Down. Settle after each tap
	// and stop as soon as the warp has left the center map.
	for tap := 0; tap < safariExitTaps; tap++ {
		state.Snapshot(m, &mem)
		if mem.U8(sym.CurMap) != safariZoneCenterMap || !state.Controllable(&mem) {
			break
		}
		m.Tap(emu.Down, 3, 7)
		m.StepFrames(safariExitSettleFrames)
	}
	if err := driveStoryUntil(m, fuchsiaStoryBudget, func(mm *state.Mem) bool {
		return !state.HasEvent(mm, eventInSafariZone) && state.Controllable(mm)
	}); err != nil {
		return err
	}
	return declineSafariRejoinPrompt(m)
}

// declineSafariRejoinPrompt finishes a leave at a stable boundary. Leaving
// early can stop the player on the gate's join trigger, and the gate script
// opens "Would you like to join the hunt?" a few frames after control returns.
// Leaving owns that prompt: a caller that wants another session re-enters
// through enterSafariZone, and one that cannot afford it must not inherit an
// open YES/NO (run-s6v9q3t2w5rl, triage:e3a271f89ba8e843).
func declineSafariRejoinPrompt(m *emu.Emu) error {
	var mem state.Mem
	if _, err := m.StepUntil(safariRejoinPromptSettleFrames, func(e *emu.Emu) bool {
		state.Snapshot(e, &mem)
		return state.DecodeTwoOptionMenu(&mem) != nil
	}); err != nil {
		return nil // no prompt: the leave already ended controllable
	}
	handled, err := answerSafariGateJoinChoice(m, state.ScreenText(&mem), false)
	if err != nil {
		return fmt.Errorf("decline Safari re-entry after leaving: %w", err)
	}
	if !handled {
		return fmt.Errorf("unexpected choice after leaving Safari Zone: %q", state.ScreenText(&mem))
	}
	return recoverAfterSafariGateChoice(m)
}

func receiveStrengthFromWarden(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if err := leaveSafariZoneIfNeeded(m, romData, policy); err != nil {
		return fmt.Errorf("skill: FuchsiaProgression: leave Safari Zone: %w", err)
	}
	warden, ok := Place("warden")
	if !ok {
		return fmt.Errorf("skill: FuchsiaProgression: warden place missing")
	}
	if _, err := TravelFlee(m, romData, warden, policy, fuchsiaTravelEngagements); err != nil {
		var choice *ErrDialogueChoice
		if !errors.As(err, &choice) {
			return fmt.Errorf("skill: FuchsiaProgression: reach Warden: %w", err)
		}
		handled, answerErr := answerSafariGateJoinChoice(m, choice.Result.Text, false)
		if answerErr != nil {
			return fmt.Errorf("skill: FuchsiaProgression: decline Safari re-entry while reaching Warden: %w", answerErr)
		}
		if !handled {
			return fmt.Errorf("skill: FuchsiaProgression: reach Warden: %w", err)
		}
		if err := recoverAfterSafariGateChoice(m); err != nil {
			return fmt.Errorf("skill: FuchsiaProgression: settle Safari re-entry decline: %w", err)
		}
		if _, err := TravelFlee(m, romData, warden, policy, fuchsiaTravelEngagements); err != nil {
			return fmt.Errorf("skill: FuchsiaProgression: reach Warden after declining Safari re-entry: %w", err)
		}
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	// When GOLD TEETH are still in the bag, the Warden removes them before
	// GiveItem(HM04), so that same interaction creates the required slot. A
	// resumed save with EVENT_GAVE_GOLD_TEETH already set has no such removal
	// and must reserve a slot before asking for HM04 again.
	if !hasBagItem(&mem, goldTeethItem) && state.HasEvent(&mem, eventGaveGoldTeeth) {
		if err := EnsureBagSpaceFor(m, hm04StrengthItem); err != nil {
			return fmt.Errorf("skill: FuchsiaProgression: make room for HM04: %w", err)
		}
	}
	if _, err := TalkAt(m, romData, 2, 3, policy); err != nil {
		return fmt.Errorf("skill: FuchsiaProgression: give Gold Teeth to Warden: %w", err)
	}
	state.Snapshot(m, &mem)
	if !hasBagItem(&mem, hm04StrengthItem) || !state.HasEvent(&mem, eventGotHM04) {
		return fmt.Errorf("skill: FuchsiaProgression: HM04 was not positively awarded (Gold Teeth missing or capacity preflight failed)")
	}
	return nil
}
