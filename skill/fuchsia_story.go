package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	eventGotHM04              state.Event = 0x238
	eventGaveGoldTeeth        state.Event = 0x239
	eventSafariGameOver       state.Event = 0x24E
	eventInSafariZone         state.Event = 0x24F
	eventBeatKoga             state.Event = 0x259
	eventFightRoute12Snorlax  state.Event = 0x48E
	eventBeatRoute12Snorlax   state.Event = 0x48F
	eventGotHM03              state.Event = 0x880
	fuchsiaStoryBudget                    = 12000
	fuchsiaTravelEngagements              = 80
	maxSafariSessions                     = 3
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

	// Establish Fuchsia as the recovery checkpoint before the Gym/Safari
	// phases. This also gives the trainer-heavy eastern route a clean heal.
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

	state.Snapshot(m, &mem)
	if !state.DecodeProgress(&mem).Has(state.BadgeSoul) {
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
			return fmt.Errorf("skill: FuchsiaProgression: Koga battle ended with outcome %d", outcome)
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
	if outcome != state.ResultWon && outcome != state.ResultCaught {
		return fmt.Errorf("skill: FuchsiaProgression: Snorlax battle ended with outcome %d", outcome)
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

	wantMax, itemIndex := startMenuShape(&mem)
	drawn := func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0 && int(m.Peek8(sym.MaxMenuItem)) == wantMax
	}
	for attempt := 0; attempt < 5 && !drawn(m); attempt++ {
		m.Tap(emu.Start, 3, 7)
		_, _ = m.StepUntil(startMenuDrawBudget, drawn)
	}
	if !drawn(m) {
		return fmt.Errorf("start menu did not draw")
	}
	if err := SelectMenuItem(m, itemIndex); err != nil {
		return fmt.Errorf("select ITEM: %w", err)
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
			teethStand, _ := Place("safari gold teeth")
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
			secret, _ := Place("safari secret house")
			if _, err := TravelFlee(m, romData, secret, policy, fuchsiaTravelEngagements); err != nil {
				state.Snapshot(m, &mem)
				if !state.HasEvent(&mem, eventInSafariZone) {
					continue
				}
				return fmt.Errorf("skill: FuchsiaProgression: reach Safari Secret House: %w", err)
			}
			if _, err := TalkAt(m, romData, 3, 3, policy); err != nil {
				return fmt.Errorf("skill: FuchsiaProgression: receive HM03: %w", err)
			}
			state.Snapshot(m, &mem)
			if !hasBagItem(&mem, hm03SurfItem) || !state.HasEvent(&mem, eventGotHM03) {
				return fmt.Errorf("skill: FuchsiaProgression: HM03 was not positively awarded (bag may be full)")
			}
		}
	}
	return fmt.Errorf("skill: FuchsiaProgression: Safari rewards still incomplete after %d bounded sessions", maxSafariSessions)
}

func enterSafariZone(m *emu.Emu, romData []byte, policy MovePolicy) error {
	gate, ok := Place("safari zone gate")
	if !ok {
		return fmt.Errorf("safari zone gate place missing")
	}
	if _, err := TravelFlee(m, romData, gate, policy, fuchsiaTravelEngagements); err != nil {
		return err
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

func leaveSafariZoneIfNeeded(m *emu.Emu, romData []byte, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, eventInSafariZone) {
		return nil
	}
	gateInside, ok := Place("safari gate inside")
	if !ok {
		return fmt.Errorf("safari gate inside place missing")
	}
	if _, err := TravelFlee(m, romData, gateInside, policy, fuchsiaTravelEngagements); err != nil {
		state.Snapshot(m, &mem)
		if !state.HasEvent(&mem, eventInSafariZone) {
			return nil
		}
		return err
	}
	return driveStoryUntil(m, fuchsiaStoryBudget, func(mm *state.Mem) bool {
		return !state.HasEvent(mm, eventInSafariZone) && state.Controllable(mm)
	})
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
		return fmt.Errorf("skill: FuchsiaProgression: reach Warden: %w", err)
	}
	if _, err := TalkAt(m, romData, 2, 3, policy); err != nil {
		return fmt.Errorf("skill: FuchsiaProgression: give Gold Teeth to Warden: %w", err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !hasBagItem(&mem, hm04StrengthItem) || !state.HasEvent(&mem, eventGotHM04) {
		return fmt.Errorf("skill: FuchsiaProgression: HM04 was not positively awarded (Gold Teeth missing or bag full)")
	}
	return nil
}
