package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	reddata "github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	staticRetryCount  = 6
	staticBallBudget  = 12
	staticStartBudget = 1200
)

// StaticCaptureSite remains an alias for compatibility with existing skill
// callers/tests, but the facts themselves are owned by the Red adapter data.
type StaticCaptureSite = reddata.StaticCaptureSite

var (
	ErrStaticCaptureUnavailable = errors.New("skill: one-time static source unavailable or already consumed")
	ErrStaticCaptureExhausted   = errors.New("skill: static capture attempts exhausted")
)

func init() {
	for _, site := range reddata.StaticCaptureSites() {
		interactionPlaces[site.Place] = Destination{Map: site.Map, X: site.StandX, Y: site.StandY}
	}
}

// StaticCaptureSites is the compatibility surface for existing skill callers.
// New Red-owned planning code should read red/data directly.
func StaticCaptureSites() []StaticCaptureSite {
	return reddata.StaticCaptureSites()
}

func staticCaptureSite(species uint8) (StaticCaptureSite, bool) {
	return reddata.StaticCaptureSiteForSpecies(species)
}

func initiateStaticBattle(m *emu.Emu, romData []byte, site StaticCaptureSite, policy MovePolicy) error {
	if site.WakeItem != 0 {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if !bagHasItem(&mem, site.WakeItem) {
			return fmt.Errorf("skill: static %s: required item %q is not in the bag", site.Name, site.Requirement)
		}
		if state.HasEvent(&mem, eventBeatRoute16Snorlax) {
			return fmt.Errorf("%w: %s event is already consumed", ErrStaticCaptureUnavailable, site.Name)
		}
		if err := Face(m, site.X, site.Y); err != nil {
			return fmt.Errorf("skill: static %s: face encounter: %w", site.Name, err)
		}
		if err := useOverworldKeyItem(m, site.WakeItem, func(mm *state.Mem) bool {
			return state.HasEvent(mm, eventFightRoute16Snorlax) || state.DecodeBattle(mm) != nil
		}); err != nil {
			return fmt.Errorf("%w: %s did not wake: %v", ErrStaticCaptureUnavailable, site.Name, err)
		}
	} else {
		_, err := TalkAt(m, romData, site.X, site.Y, policy)
		if err != nil && !errors.Is(err, ErrTalkStartedBattle) {
			return fmt.Errorf("%w: %s interaction: %v", ErrStaticCaptureUnavailable, site.Name, err)
		}
	}

	if _, err := m.StepUntil(staticStartBudget, func(mm *emu.Emu) bool {
		return mm.Peek8(sym.IsInBattle) != 0
	}); err != nil {
		return fmt.Errorf("%w: %s did not start a battle", ErrStaticCaptureUnavailable, site.Name)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	battle := state.DecodeBattle(&mem)
	if battle == nil || battle.EnemySpecies != site.Species {
		return fmt.Errorf("skill: static %s: expected species %#02x, battle=%+v", site.Name, site.Species, battle)
	}
	return nil
}

func staticBall(mem *state.Mem, site StaticCaptureSite) (uint8, bool) {
	for _, item := range reddata.StaticCaptureBallOrder(site) {
		if bagHasItem(mem, item) {
			return item, true
		}
	}
	return 0, false
}

// catchStaticBattle owns only ball throws. Unlike ordinary Catch it never
// attacks after its ball budget is exhausted; the caller restores the
// pre-encounter emulator checkpoint instead, so a one-time encounter cannot be
// consumed by a failed bounded attempt.
func catchStaticBattle(m *emu.Emu, romData []byte, site StaticCaptureSite, maxBalls int) (CatchResult, error) {
	var before state.Mem
	state.Snapshot(m, &before)
	partyBefore := int(state.DecodeParty(&before).Count)
	boxBefore := int(state.DecodeBox(&before).Count)
	ownedBefore := append([]uint8(nil), state.DecodePokedex(&before).Owned...)
	want := []uint8{site.Species}
	wantDex := wantedDexNumbers(romData, want)
	res := CatchResult{Encounters: 1}

	for res.BallsThrown < maxBalls && battleInFlight(m) {
		var mem state.Mem
		state.Snapshot(m, &mem)
		ball, ok := staticBall(&mem, site)
		if !ok {
			res.Outcome = OutcomeOutOfBalls
			return res, nil
		}
		if err := UseItem(m, ball); err != nil {
			if errors.Is(err, ErrNotInBag) {
				continue
			}
			return res, fmt.Errorf("skill: static catch: throw item %#02x: %w", ball, err)
		}
		res.BallsThrown++
		ended, err := waitThrowResult(m)
		if err != nil {
			return res, err
		}
		if ended {
			break
		}
	}

	if battleInFlight(m) {
		res.Outcome = OutcomeOutOfBalls
		return res, nil
	}
	if err := waitForBattleEnd(m); err != nil {
		return res, err
	}
	var after state.Mem
	state.Snapshot(m, &after)
	if got, ok := catchAcquiredWanted(partyBefore, state.DecodeParty(&after), boxBefore, state.DecodeBox(&after), ownedBefore, state.DecodePokedex(&after).Owned, want, wantDex); ok {
		res.Outcome = OutcomeCaught
		res.Species = got
		return res, nil
	}
	res.Outcome = OutcomeFled
	return res, nil
}

// CaptureStatic captures one of Red's finite overworld encounters. Every failed
// attempt is rolled back to the exact pre-interaction state. Retry phases are
// deliberately different so deterministic RNG does not reproduce the same ball
// results forever; the whole skill is still bounded.
func CaptureStatic(m *emu.Emu, romData []byte, species uint8, policy MovePolicy) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: CaptureStatic: nil policy")
	}
	site, ok := staticCaptureSite(species)
	if !ok {
		return CatchResult{}, fmt.Errorf("skill: CaptureStatic: species %#02x has no Red static site", species)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if giftPokemonAlreadyOwned(&mem, romData, species) {
		return CatchResult{Outcome: OutcomeCaught, Species: species}, nil
	}
	dest, ok := Place(site.Place)
	if !ok {
		return CatchResult{}, fmt.Errorf("skill: static %s: destination is not registered", site.Name)
	}
	if _, err := TravelFlee(m, romData, dest, policy, 120); err != nil {
		return CatchResult{}, fmt.Errorf("skill: static %s: reach encounter: %w", site.Name, err)
	}
	checkpoint, err := m.SaveState()
	if err != nil {
		return CatchResult{}, fmt.Errorf("skill: static %s: checkpoint: %w", site.Name, err)
	}

	phases := [...]int{0, 17, 37, 61, 89, 127}
	var last CatchResult
	for attempt := 0; attempt < staticRetryCount; attempt++ {
		if attempt > 0 {
			if err := m.LoadState(checkpoint); err != nil {
				return last, fmt.Errorf("skill: static %s: restore attempt %d: %w", site.Name, attempt+1, err)
			}
			m.StepFrames(phases[attempt])
		}
		if err := initiateStaticBattle(m, romData, site, policy); err != nil {
			_ = m.LoadState(checkpoint)
			return last, err
		}
		result, err := catchStaticBattle(m, romData, site, staticBallBudget)
		last = result
		if err == nil && result.Outcome == OutcomeCaught {
			return result, nil
		}
		if loadErr := m.LoadState(checkpoint); loadErr != nil {
			return last, fmt.Errorf("skill: static %s: rollback failed after attempt %d: %w", site.Name, attempt+1, loadErr)
		}
		if err != nil {
			continue
		}
	}
	return last, fmt.Errorf("%w: %s after %d rollback-safe phases", ErrStaticCaptureExhausted, site.Name, staticRetryCount)
}
