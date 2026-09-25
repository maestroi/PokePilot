package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	safariCatchSessions       = 3
	safariCatchTravelAttempts = 8
	safariCatchLegsPerSession = 500
	safariCatchEntryFee       = 500
)

var ErrSafariCatchExhausted = errors.New("skill: SafariCatch: bounded Safari sessions exhausted without the wanted species")

// SafariCatch hunts one or more wanted species in a Safari grass habitat.
// The paid gate/session lifecycle is owned here, not by the planner. Unwanted
// encounters use Flee's already-verified Safari RUN path; wanted encounters
// select Safari BALL directly and prove acquisition through the same
// party/box/Pokedex postcondition as ordinary Catch.
func SafariCatch(m *emu.Emu, romData []byte, targetMap uint8, want []uint8, policy MovePolicy, maxBallsPerEncounter int) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: SafariCatch: nil policy")
	}
	if len(want) == 0 {
		return CatchResult{}, fmt.Errorf("skill: SafariCatch: want is empty")
	}
	if maxBallsPerEncounter <= 0 {
		return CatchResult{}, fmt.Errorf("skill: SafariCatch: maxBallsPerEncounter must be > 0, got %d", maxBallsPerEncounter)
	}
	if !isSafariHabitatMap(targetMap) {
		return CatchResult{}, fmt.Errorf("skill: SafariCatch: map %#04x is not a Safari habitat", targetMap)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return CatchResult{}, fmt.Errorf("skill: SafariCatch: player not controllable on map %#04x", m.Peek8(sym.CurMap))
	}
	partyBefore := int(state.DecodeParty(&mem).Count)
	boxBefore := int(state.DecodeBox(&mem).Count)
	ownedBefore := append([]uint8(nil), state.DecodePokedex(&mem).Owned...)
	wantDex := wantedDexNumbers(romData, want)
	res := CatchResult{}

	for session := 1; session <= safariCatchSessions; session++ {
		state.Snapshot(m, &mem)
		inZone := state.HasEvent(&mem, eventInSafariZone)
		money := state.DecodeInventory(&mem).Money
		if safariCatchReentryUnaffordable(session, inZone, money) {
			return res, fmt.Errorf(
				"%w: cannot fund Safari session %d after prior bounded session(s); money=%d fee=%d encounters=%d balls=%d",
				ErrSafariCatchExhausted, session, money, safariCatchEntryFee, res.Encounters, res.BallsThrown,
			)
		}
		if !inZone {
			if err := enterSafariZone(m, romData, policy); err != nil {
				return res, fmt.Errorf("skill: SafariCatch: enter session %d: %w", session, err)
			}
		}
		state.Snapshot(m, &mem)
		if mem.U8(sym.NumSafariBalls) == 0 {
			if err := leaveSafariZoneIfNeeded(m, romData, policy); err != nil {
				return res, fmt.Errorf("skill: SafariCatch: session %d starts without Safari Balls and cannot exit: %w", session, err)
			}
			continue
		}

		if err := travelToSafariGrass(m, romData, targetMap, policy); err != nil {
			state.Snapshot(m, &mem)
			if !state.HasEvent(&mem, eventInSafariZone) {
				continue // the 502-step session expired while routing; buy a fresh one
			}
			return res, fmt.Errorf("skill: SafariCatch: session %d reach habitat: %w", session, err)
		}

		caught, sessionEnded, err := huntSafariGrassSession(
			m, romData, targetMap, want, wantDex, policy,
			partyBefore, boxBefore, ownedBefore, &res, maxBallsPerEncounter,
		)
		if err != nil {
			return res, fmt.Errorf("skill: SafariCatch: session %d: %w", session, err)
		}
		if caught {
			return res, nil
		}
		if !sessionEnded {
			// The local leg/encounter bound can fire before the ROM's Safari
			// timer. Leave through the normal gate so the next bounded attempt
			// starts with a fresh 30-ball/502-step session.
			if err := leaveSafariZoneIfNeeded(m, romData, policy); err != nil {
				return res, fmt.Errorf("skill: SafariCatch: close session %d: %w", session, err)
			}
		}
	}

	return res, fmt.Errorf("%w: %d encounter(s), %d Safari Ball(s) thrown at wanted targets", ErrSafariCatchExhausted, res.Encounters, res.BallsThrown)
}

// safariCatchReentryUnaffordable distinguishes an exhausted paid hunt from an
// invalid initial request. The planner only promises enough money for one
// Safari entry; SafariCatch may opportunistically use more. Once at least one
// bounded session has been consumed, being outside the Zone without another
// entry fee is therefore an ordinary hunt exhaustion rather than a gate
// controller failure. An already-active session remains usable even at ¥0.
func safariCatchReentryUnaffordable(session int, inZone bool, money uint32) bool {
	return session > 1 && !inZone && money < safariCatchEntryFee
}

func isSafariHabitatMap(mapID uint8) bool {
	return mapID >= safariZoneEastMap && mapID <= safariZoneCenterMap
}

// isSafariSessionMap reports the maps a Safari Game session continues on: the
// four habitats and their rest houses. Anything else, the gate included, is
// reached only by walking out through the gate's "Leaving early?" prompt.
func isSafariSessionMap(mapID uint8) bool {
	return mapID >= safariZoneEastMap && mapID <= safariZoneNorthRestHouseMap
}

// leaveSafariSessionFor ends an active Safari Game session before travel to a
// destination outside it. Choosing a destination beyond the gate is choosing
// to leave, so the traveler answers the gate's early-leave prompt through the
// same owned exit SafariCatch and the Fuchsia story use. Leaving it to the
// ordinary route instead met that prompt as an unanswered choice, which every
// non-Safari objective started inside a session hit (triage 5a248584293ac9fc).
func leaveSafariSessionFor(m *emu.Emu, romData []byte, dest Destination, policy MovePolicy) error {
	if isSafariSessionMap(dest.Map) {
		return nil
	}
	if err := leaveSafariZoneIfNeeded(m, romData, policy); err != nil {
		return fmt.Errorf("leave Safari session before travel to map %#04x: %w", dest.Map, err)
	}
	return nil
}

// travelToSafariGrass chooses one representative grass cell from each static
// collision component and asks the ordinary semantic traveler to reach it.
// This avoids hard-coding a Safari coordinate while still handling maps whose
// first row-major grass patch is isolated by ledges/walls.
func travelToSafariGrass(m *emu.Emu, romData []byte, targetMap uint8, policy MovePolicy) error {
	grass, grid, err := grassCells(romData, targetMap)
	if err != nil {
		return err
	}
	if len(grass) == 0 || grid == nil {
		return fmt.Errorf("map %#04x has no encounter grass", targetMap)
	}
	components := world.Components(grid)
	seen := map[int]bool{}
	candidates := make([]cell, 0, safariCatchTravelAttempts)
	for _, c := range grass {
		component := 0
		if grid.InBounds(c.x, c.y) {
			component = components[c.y][c.x]
		}
		if component == 0 || seen[component] {
			continue
		}
		seen[component] = true
		candidates = append(candidates, c)
		if len(candidates) >= safariCatchTravelAttempts {
			break
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("map %#04x has no connected grass component", targetMap)
	}

	var lastErr error
	for _, c := range candidates {
		if _, err := TravelFlee(m, romData, Destination{Map: targetMap, X: uint8(c.x), Y: uint8(c.y)}, policy, fuchsiaTravelEngagements); err == nil {
			return nil
		} else {
			lastErr = err
			var mem state.Mem
			state.Snapshot(m, &mem)
			if !state.HasEvent(&mem, eventInSafariZone) {
				return err
			}
		}
	}
	return lastErr
}

func huntSafariGrassSession(m *emu.Emu, romData []byte, targetMap uint8, want, wantDex []uint8, policy MovePolicy, partyBefore, boxBefore int, ownedBefore []uint8, res *CatchResult, maxBallsPerEncounter int) (caught, sessionEnded bool, err error) {
	if m.Peek8(sym.CurMap) != targetMap {
		return false, false, fmt.Errorf("expected habitat map %#04x, on %#04x", targetMap, m.Peek8(sym.CurMap))
	}
	grass, grid, err := grassCells(romData, targetMap)
	if err != nil {
		return false, false, err
	}
	now, err := currentWorld(m)
	if err != nil {
		return false, false, fmt.Errorf("observe habitat world: %w", err)
	}
	grass = grassInPlayerComponent(grass, grid, int(now.X), int(now.Y))
	if len(grass) == 0 {
		return false, false, fmt.Errorf("habitat map %#04x has no reachable encounter grass from (%d,%d)", targetMap, now.X, now.Y)
	}
	a, b, ok := grindPair(grass, grid, int(now.X), int(now.Y), spriteBlockers(m))
	if !ok {
		return false, false, fmt.Errorf("habitat map %#04x has no usable grass pair", targetMap)
	}

	next := b
	legs := 0
	encountersAtStart := res.Encounters
	for res.Encounters-encountersAtStart < catchHuntCap && legs < safariCatchLegsPerSession {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if !state.HasEvent(&mem, eventInSafariZone) || mem.U8(sym.NumSafariBalls) == 0 {
			return false, true, nil
		}

		d := Destination{Map: targetMap, X: uint8(next.x), Y: uint8(next.y)}
		if err := GoTo(m, romData, d); err != nil && !errors.Is(err, ErrBattle) {
			state.Snapshot(m, &mem)
			if !state.HasEvent(&mem, eventInSafariZone) {
				return false, true, nil
			}
			if safariTimedEjectionInterrupted(targetMap, mem.U8(sym.CurMap), err) {
				// The Safari timer can expire while a same-map grass step is in
				// flight. The ROM warps the player to the gate and starts the
				// ejection dialogue before EVENT_IN_SAFARI_ZONE is cleared, so
				// checking only the event makes us repick a habitat grind pair
				// against the gate map and rethrow ErrDialogueInterrupted.
				// Own that bounded session-ending script here, then let the outer
				// loop buy a fresh session.
				if settleErr := driveStoryUntil(m, fuchsiaStoryBudget, func(mm *state.Mem) bool {
					return !state.HasEvent(mm, eventInSafariZone) && state.Controllable(mm)
				}); settleErr != nil {
					return false, false, fmt.Errorf("settle timed Safari ejection: %w", settleErr)
				}
				return false, true, nil
			}
			na, nb, ok := repickGrindPair(m, grass, grid, a, b)
			if !ok {
				return false, false, fmt.Errorf("Safari hunt leg %d: %w", legs+1, err)
			}
			a, b, next = na, nb, nb
			legs++
			continue
		}
		legs++
		next = flip(a, b, next)
		if !waitBattleStart(m, 1000) {
			continue
		}

		state.Snapshot(m, &mem)
		bs := state.DecodeBattle(&mem)
		if bs == nil {
			return false, false, fmt.Errorf("Safari hunt leg %d reported an encounter but no battle is in progress", legs)
		}
		res.Encounters++
		if !speciesIn(bs.EnemySpecies, want) {
			if err := Flee(m, 1); err != nil {
				return false, false, fmt.Errorf("flee unwanted Safari species %d: %w", bs.EnemySpecies, err)
			}
			continue
		}

		acquired, err := safariCatchWanted(m, want, wantDex, partyBefore, boxBefore, ownedBefore, res, maxBallsPerEncounter)
		if err != nil {
			return false, false, err
		}
		if acquired {
			return true, false, nil
		}
	}
	return false, false, nil
}

func safariTimedEjectionInterrupted(targetMap, currentMap uint8, err error) bool {
	return currentMap != targetMap && errors.Is(err, ErrDialogueInterrupted)
}

func safariBallCursor(mem *state.Mem) bool {
	return fleeMenuFromMem(mem) == fleeMenuSafari &&
		mem.U8(sym.TopMenuItemX) == safariBattleMenuLeftX &&
		mem.U8(sym.CurrentMenuItem) == 0
}

func safariBallNextInput(mem *state.Mem) (btn emu.Button, done bool) {
	if safariBallCursor(mem) {
		return 0, true
	}
	row := int(mem.U8(sym.CurrentMenuItem))
	x := mem.U8(sym.TopMenuItemX)
	switch {
	case row > 0:
		return emu.Up, false
	case x > safariBattleMenuLeftX:
		return emu.Left, false
	case x < safariBattleMenuLeftX:
		return emu.Right, false
	default:
		return 0, false
	}
}

func selectSafariBallEntry(m *emu.Emu) error {
	kind, err := waitFleeMenu(m)
	if err != nil {
		return err
	}
	if kind != fleeMenuSafari {
		return fmt.Errorf("wanted Safari BALL but battle menu kind is %d", kind)
	}
	for i := 0; i < 12; i++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		btn, done := safariBallNextInput(&mem)
		if done {
			return nil
		}
		if btn == 0 {
			m.StepFrame()
			continue
		}
		m.Tap(btn, 3, 7)
	}
	return fmt.Errorf("Safari BALL cursor did not converge")
}

func safariCatchWanted(m *emu.Emu, want, wantDex []uint8, partyBefore, boxBefore int, ownedBefore []uint8, res *CatchResult, maxBalls int) (bool, error) {
	for thrown := 0; thrown < maxBalls && battleInFlight(m); thrown++ {
		beforeBalls := m.Peek8(sym.NumSafariBalls)
		if beforeBalls == 0 {
			break
		}
		if err := selectSafariBallEntry(m); err != nil {
			return false, fmt.Errorf("select Safari BALL: %w", err)
		}
		m.Tap(emu.A, 3, 7)
		res.BallsThrown++

		ended, err := waitSafariBallResult(m, beforeBalls)
		if err != nil {
			return false, err
		}
		if !ended {
			continue
		}
		if err := waitForBattleEnd(m); err != nil {
			return false, err
		}
		var mem state.Mem
		state.Snapshot(m, &mem)
		if species, ok := catchAcquiredWanted(partyBefore, state.DecodeParty(&mem), boxBefore, state.DecodeBox(&mem), ownedBefore, state.DecodePokedex(&mem).Owned, want, wantDex); ok {
			res.Outcome = OutcomeCaught
			res.Species = species
			return true, nil
		}
		res.Outcome = OutcomeFled
		return false, nil
	}

	if battleInFlight(m) {
		if err := Flee(m, 1); err != nil {
			return false, fmt.Errorf("leave wanted Safari battle after bounded throws: %w", err)
		}
	}
	res.Outcome = OutcomeOutOfBalls
	return false, nil
}

// waitSafariBallResult is the Safari analogue of waitThrowResult. BALL can
// either return to the Safari menu (the Pokemon stayed), end the battle by a
// catch/run, or reach the nickname prompt on a successful catch. It never
// mistakes ordinary text for a menu and verifies that exactly one Safari Ball
// was consumed when the menu returns.
func waitSafariBallResult(m *emu.Emu, beforeBalls uint8) (bool, error) {
	for spent := 0; spent < battleEndSettle; spent += throwPollFrames {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) == nil {
			return true, nil
		}
		if state.DecodeTwoOptionMenu(&mem) != nil {
			if err := selectTwoOption(m, 1); err != nil {
				return false, fmt.Errorf("declining Safari catch nickname prompt: %w", err)
			}
			continue
		}
		if fleeMenuFromMem(&mem) == fleeMenuSafari {
			after := mem.U8(sym.NumSafariBalls)
			if beforeBalls == 0 || after+1 != beforeBalls {
				return false, fmt.Errorf("Safari BALL count changed %d -> %d, want exactly one consumed", beforeBalls, after)
			}
			return false, nil
		}
		m.Tap(emu.A, 3, 7)
		m.StepFrames(throwPollFrames)
	}
	return false, fmt.Errorf("Safari BALL result did not resolve within %d frames", battleEndSettle)
}
