package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
	reddata "github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	staticRetryCount  = 6
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

// staticWeakenDivisor is the HP fraction weakening aims for. Gen I's capture
// HP factor, (MaxHP*255/12)/(HP/4) for a POKé BALL, already saturates at a
// third of max HP, so lower HP only adds turns the target can act in.
const staticWeakenDivisor = 3

// staticWeakening prepares a one-time target the way a player does: attacks
// that cannot knock it out, then a sleep or paralysis move. The profile's
// BattleDamageBound proves each attack's worst case (top roll, critical hit,
// every hit); a move without a bound is never used. BattleCaptureStatus is
// optional: without it the target is only weakened.
type staticWeakening struct {
	romData []byte
	bound   game.BattleDamageBound
	status  game.BattleCaptureStatus
}

func newStaticWeakening(romData []byte) (staticWeakening, bool) {
	profile, _, err := profiles.Detect(romData)
	if err != nil {
		return staticWeakening{}, false
	}
	bound, ok := profile.(game.BattleDamageBound)
	status, _ := profile.(game.BattleCaptureStatus)
	return staticWeakening{romData: romData, bound: bound, status: status}, ok
}

// hpWanted reports that the target is above the weakening goal.
func hpWanted(b game.BattleState) bool {
	return b.EnemyHP > 0 && uint32(b.EnemyHP)*staticWeakenDivisor > uint32(b.EnemyMaxHP)
}

// statusMove is a usable move that would put a healthy target to sleep or
// paralyze it, or -1.
func (w staticWeakening) statusMove(b game.BattleState) int {
	if w.status == nil || b.EnemyStatus != "" || b.EnemyHP == 0 {
		return -1
	}
	_, defender := battleCombatants(b)
	for _, slot := range b.Usable() {
		if w.status.CaptureStatusMove(w.romData, defender, uint16(b.Moves[slot].ID)) {
			return slot
		}
	}
	return -1
}

// move is the next preparation move: weaken first (a status move used
// first can wear off while the attacks land), then inflict status.
func (w staticWeakening) move(b game.BattleState) int {
	if hpWanted(b) {
		if slot := w.safeMove(b); slot >= 0 {
			return slot
		}
	}
	return w.statusMove(b)
}

// safeMove is the usable move with the largest worst case that still leaves
// the target alive, or -1 when no damaging move is safe.
func (w staticWeakening) safeMove(b game.BattleState) int {
	attacker, defender := battleCombatants(b)
	best, bestDamage := -1, uint16(0)
	for _, slot := range b.Usable() {
		damage, ok := w.bound.MaxCombatDamage(w.romData, attacker, defender, uint16(b.Moves[slot].ID))
		if !ok || damage == 0 || damage >= b.EnemyHP {
			continue
		}
		if best < 0 || damage > bestDamage {
			best, bestDamage = slot, damage
		}
	}
	return best
}

// weakener is the conscious bench slot whose safe move hits hardest, for a
// lead that is too strong to attack safely (an overleveled late-game lead
// against a level-50 legendary), or -1.
func (w staticWeakening) weakener(b game.BattleState, res game.BattleResourcesState) int {
	_, defender := battleCombatants(b)
	best, bestDamage := -1, uint16(0)
	for slot, mon := range res.Party {
		if slot == res.ActiveSlot || mon.Fainted() {
			continue
		}
		attacker := battlePartyCombatant(mon)
		for _, mv := range mon.Moves {
			if mv.NativeMoveID == 0 || mv.PP == 0 {
				continue
			}
			damage, ok := w.bound.MaxCombatDamage(w.romData, attacker, defender, mv.NativeMoveID)
			if ok && damage > 0 && damage < b.EnemyHP && damage > bestDamage {
				best, bestDamage = slot, damage
			}
		}
	}
	return best
}

// wanted reports that another preparation move still helps the catch and
// cannot faint the target.
func (w staticWeakening) wanted(b game.BattleState) bool {
	return w.move(b) >= 0
}

// catchStaticBattle weakens the target with attacks that cannot knock it
// out, then throws balls. A human's one-time encounter ends the same way: a
// catch, or a fainted/fled target or spent budget that the caller undoes by
// resetting to the in-game save made before the encounter.
func catchStaticBattle(m *emu.Emu, romData []byte, site StaticCaptureSite, maxBalls int) (CatchResult, error) {
	var before state.Mem
	state.Snapshot(m, &before)
	partyBefore := int(state.DecodeParty(&before).Count)
	boxBefore := int(state.DecodeBox(&before).Count)
	ownedBefore := append([]uint8(nil), state.DecodePokedex(&before).Owned...)
	want := []uint8{site.Species}
	wantDex := wantedDexNumbers(romData, want)
	res := CatchResult{Encounters: 1}
	weaken, canWeaken := newStaticWeakening(romData)
	battleDecoder, err := battleStateDecoderFor(m)
	if err != nil {
		canWeaken = false
	}
	resources, _ := battleResourcesDecoderFor(m)
	switched := false
	if ball, ok := staticBall(&before, site); ok && ball == masterBallItemID {
		canWeaken = false // the Master Ball never misses
	}

	for res.BallsThrown < maxBalls && battleInFlight(m) {
		var mem state.Mem
		state.Snapshot(m, &mem)
		var bs game.BattleState
		inBattle := false
		if canWeaken {
			bs, inBattle = battleDecoder.DecodeBattleState(m)
		}
		if inBattle && !switched && resources != nil && hpWanted(bs) && weaken.safeMove(bs) < 0 {
			switched = true // one deliberate switch per battle; never ping-pong
			if party := resources.DecodeBattleResources(m); !party.ActiveTrapped {
				if slot := weaken.weakener(bs, party); slot >= 0 {
					// SwitchActive needs the main menu; Battle pages the intro
					// text there and hands control straight back.
					if _, err := BattleWithOptions(m, FirstUsableMove, BattleOptions{YieldWhen: func(game.BattleState) bool { return true }}); !errors.Is(err, ErrBattleYielded) {
						return res, fmt.Errorf("skill: static %s: reach the battle menu: %v", site.Name, err)
					}
					if err := SwitchActive(m, slot); err != nil {
						return res, fmt.Errorf("skill: static %s: switch to a weakener: %w", site.Name, err)
					}
					continue
				}
			}
		}
		if inBattle && weaken.wanted(bs) {
			_, err := BattleWithOptions(m, func(b game.BattleState) int {
				if slot := weaken.move(b); slot >= 0 {
					return slot
				}
				return FirstUsableMove(b)
			}, BattleOptions{
				// The weakener was chosen for this target; a tactical swap to a
				// stronger member would end weakening without a safe move.
				HoldTrainingLead: true,
				YieldWhen:        func(b game.BattleState) bool { return !weaken.wanted(b) },
			})
			if err == nil || !errors.Is(err, ErrBattleYielded) {
				if err != nil {
					return res, fmt.Errorf("skill: static %s: weaken: %w", site.Name, err)
				}
				break // the battle ended while weakening: classify below
			}
			continue
		}
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

// staticBallBudget throws every ordinary ball the bag holds: a failed
// attempt is undone by the reset, which returns them, so only the catch
// itself spends any. The Master Ball stays last in the site's order and is
// only reached by a bag with no ordinary ball, as before.
func staticBallBudget(m *emu.Emu) int {
	var mem state.Mem
	state.Snapshot(m, &mem)
	total := 0
	for _, ball := range reddata.WildCaptureBallOrder() {
		total += itemCount(&mem, ball)
	}
	return max(total, 1)
}

// CaptureStatic captures one of Red's finite overworld encounters the way a
// player does: save the game in front of it, weaken it, throw balls, and on a
// failed attempt soft-reset to that save and try again. The reset is the
// cartridge's own mechanic, so the run never relies on emulator save states
// (docs/ARCHITECTURE.md §8). Waiting a different number of frames before each
// retry changes rDIV, and with it every roll of the next attempt. A catch is
// saved immediately so a later soft reset cannot undo it. Bounded by
// staticRetryCount attempts.
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
	if err := SaveGame(m); err != nil {
		return CatchResult{}, fmt.Errorf("skill: static %s: save before the encounter: %w", site.Name, err)
	}

	phases := [...]int{0, 17, 37, 61, 89, 127}
	var last CatchResult
	for attempt := 0; attempt < staticRetryCount; attempt++ {
		if attempt > 0 {
			if err := resetToStaticSave(m, site); err != nil {
				return last, fmt.Errorf("skill: static %s: reset for attempt %d: %w", site.Name, attempt+1, err)
			}
			m.StepFrames(phases[attempt])
		}
		if err := initiateStaticBattle(m, romData, site, policy); err != nil {
			if battleInFlight(m) {
				_ = resetToStaticSave(m, site)
			}
			return last, err
		}
		result, err := catchStaticBattle(m, romData, site, staticBallBudget(m))
		last = result
		if zbatDebug {
			fmt.Printf("zbat static %s attempt=%d outcome=%d balls=%d err=%v\n", site.Name, attempt+1, result.Outcome, result.BallsThrown, err)
		}
		if err == nil && result.Outcome == OutcomeCaught {
			if saveErr := SaveGame(m); saveErr != nil {
				return result, fmt.Errorf("skill: static %s: caught but could not save it: %w", site.Name, saveErr)
			}
			return result, nil
		}
	}
	if err := resetToStaticSave(m, site); err != nil {
		return last, fmt.Errorf("skill: static %s: reset after the last attempt: %w", site.Name, err)
	}
	return last, fmt.Errorf("%w: %s after %d save-and-reset attempts", ErrStaticCaptureExhausted, site.Name, staticRetryCount)
}

// resetToStaticSave soft-resets to the in-game save made at the encounter.
// Standing on the stand tile again is the positive proof that the save
// CaptureStatic wrote is the one CONTINUE loaded.
func resetToStaticSave(m *emu.Emu, site StaticCaptureSite) error {
	if err := softResetToSave(m); err != nil {
		return err
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if mem.U8(sym.CurMap) != site.Map || mem.U8(sym.XCoord) != site.StandX || mem.U8(sym.YCoord) != site.StandY {
		return fmt.Errorf("CONTINUE resumed at map %#02x (%d,%d), not the save at map %#02x (%d,%d)",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), site.Map, site.StandX, site.StandY)
	}
	return nil
}
