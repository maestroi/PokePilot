package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	staticMasterBall uint8 = 0x01
	staticUltraBall  uint8 = 0x02
	staticGreatBall  uint8 = 0x03
	staticPokeBall   uint8 = 0x04

	staticSnorlax    uint8 = 0x84
	staticArticuno   uint8 = 0x4A
	staticZapdos     uint8 = 0x4B
	staticMoltres    uint8 = 0x49
	staticMewtwo     uint8 = 0x83
	staticRetryCount       = 6
	staticBallBudget       = 12
	staticStartBudget      = 1200
)

// StaticCaptureSite describes a one-time Red encounter. Place is an
// interaction-owned standing tile; X/Y is the immutable ROM object home.
type StaticCaptureSite struct {
	Name        string
	Place       string
	Map         uint8
	X, Y        uint8
	StandX      uint8
	StandY      uint8
	Species     uint8
	Requirement string
}

var staticCaptureSites = []StaticCaptureSite{
	{Name: "Route 16 Snorlax", Place: "route 16 snorlax capture", Map: 0x1B, X: 26, Y: 10, StandX: 27, StandY: 10, Species: staticSnorlax, Requirement: "poke_flute"},
	{Name: "Articuno", Place: "seafoam articuno", Map: 0xA2, X: 6, Y: 1, StandX: 6, StandY: 2, Species: staticArticuno},
	{Name: "Zapdos", Place: "power plant zapdos", Map: 0x53, X: 4, Y: 9, StandX: 4, StandY: 10, Species: staticZapdos},
	{Name: "Moltres", Place: "victory road moltres", Map: 0xC2, X: 11, Y: 5, StandX: 11, StandY: 6, Species: staticMoltres},
	{Name: "Mewtwo", Place: "cerulean cave mewtwo", Map: 0xE3, X: 27, Y: 13, StandX: 27, StandY: 14, Species: staticMewtwo},
}

var (
	ErrStaticCaptureUnavailable = errors.New("skill: one-time static source unavailable or already consumed")
	ErrStaticCaptureExhausted   = errors.New("skill: static capture attempts exhausted")
)

func init() {
	for _, site := range staticCaptureSites {
		interactionPlaces[site.Place] = Destination{Map: site.Map, X: site.StandX, Y: site.StandY}
	}
}

// StaticCaptureSites returns a copy for Red-owned planning/tests.
func StaticCaptureSites() []StaticCaptureSite {
	out := make([]StaticCaptureSite, len(staticCaptureSites))
	copy(out, staticCaptureSites)
	return out
}

func staticCaptureSite(species uint8) (StaticCaptureSite, bool) {
	for _, site := range staticCaptureSites {
		if site.Species == species {
			return site, true
		}
	}
	return StaticCaptureSite{}, false
}

func initiateStaticBattle(m *emu.Emu, romData []byte, site StaticCaptureSite, policy MovePolicy) error {
	if site.Species == staticSnorlax {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if !bagHasItem(&mem, pokeFluteItemFuchsia) {
			return fmt.Errorf("skill: static %s: POKE FLUTE is required", site.Name)
		}
		if state.HasEvent(&mem, eventBeatRoute16Snorlax) {
			return fmt.Errorf("%w: %s event is already consumed", ErrStaticCaptureUnavailable, site.Name)
		}
		if err := Face(m, site.X, site.Y); err != nil {
			return fmt.Errorf("skill: static %s: face encounter: %w", site.Name, err)
		}
		if err := useOverworldKeyItem(m, pokeFluteItemFuchsia, func(mm *state.Mem) bool {
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

func staticBall(mem *state.Mem, species uint8) (uint8, bool) {
	// Mewtwo is the highest-value deterministic Master Ball target. Other
	// statics preserve it while an ordinary ball remains available.
	if species == staticMewtwo && bagHasItem(mem, staticMasterBall) {
		return staticMasterBall, true
	}
	for _, item := range []uint8{staticUltraBall, staticGreatBall, staticPokeBall} {
		if bagHasItem(mem, item) {
			return item, true
		}
	}
	if bagHasItem(mem, staticMasterBall) {
		return staticMasterBall, true
	}
	return 0, false
}

// catchStaticBattle owns only ball throws. Unlike ordinary Catch it never
// attacks after its ball budget is exhausted; the caller restores the
// pre-encounter emulator checkpoint instead, so a one-time encounter cannot be
// consumed by a failed bounded attempt.
func catchStaticBattle(m *emu.Emu, romData []byte, species uint8, maxBalls int) (CatchResult, error) {
	var before state.Mem
	state.Snapshot(m, &before)
	partyBefore := int(state.DecodeParty(&before).Count)
	boxBefore := int(state.DecodeBox(&before).Count)
	ownedBefore := append([]uint8(nil), state.DecodePokedex(&before).Owned...)
	want := []uint8{species}
	wantDex := wantedDexNumbers(romData, want)
	res := CatchResult{Encounters: 1}

	for res.BallsThrown < maxBalls && battleInFlight(m) {
		var mem state.Mem
		state.Snapshot(m, &mem)
		ball, ok := staticBall(&mem, species)
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
		result, err := catchStaticBattle(m, romData, species, staticBallBudget)
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
