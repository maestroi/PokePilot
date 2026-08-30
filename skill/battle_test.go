package skill_test

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestFirstUsableMove is pure unit coverage: no emulator, no ROM.
func TestFirstUsableMove(t *testing.T) {
	t.Run("all four have PP picks slot 0", func(t *testing.T) {
		b := state.BattleState{
			Moves: [4]state.Move{
				{ID: 1, PP: 10},
				{ID: 2, PP: 10},
				{ID: 3, PP: 10},
				{ID: 4, PP: 10},
			},
		}
		if got := skill.FirstUsableMove(b); got != 0 {
			t.Errorf("FirstUsableMove = %d, want 0", got)
		}
	})

	t.Run("slots 0 and 1 out of PP picks slot 2", func(t *testing.T) {
		b := state.BattleState{
			Moves: [4]state.Move{
				{ID: 1, PP: 0},
				{ID: 2, PP: 0},
				{ID: 3, PP: 10},
				{ID: 4, PP: 10},
			},
		}
		if got := skill.FirstUsableMove(b); got != 2 {
			t.Errorf("FirstUsableMove = %d, want 2", got)
		}
	})

	t.Run("empty slots are skipped", func(t *testing.T) {
		b := state.BattleState{
			Moves: [4]state.Move{
				{ID: 0, PP: 10},
				{ID: 0, PP: 10},
				{ID: 3, PP: 5},
				{ID: 0, PP: 0},
			},
		}
		if got := skill.FirstUsableMove(b); got != 2 {
			t.Errorf("FirstUsableMove = %d, want 2", got)
		}
	})

	t.Run("nothing usable returns -1", func(t *testing.T) {
		b := state.BattleState{}
		if got := skill.FirstUsableMove(b); got != -1 {
			t.Errorf("FirstUsableMove = %d, want -1", got)
		}
	})
}

// TestBattleNoBattleInProgress is ROM-gated: without POKEMON_RED_ROM the
// fixture loader skips. A failure assertion must also assert nothing
// changed (docs/DESIGN.md 3.2b).
func TestBattleNoBattleInProgress(t *testing.T) {
	e := loadFixture(t)

	before := playerAt(t, e)
	if before.X != 3 || before.Y != 6 {
		t.Fatalf("fixture start = (%d,%d), want (3,6)", before.X, before.Y)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	if !state.Controllable(&mem) {
		t.Fatal("fixture not controllable at start")
	}

	result, err := skill.Battle(e, skill.FirstUsableMove)
	if err == nil {
		t.Fatalf("Battle = %v, nil error; want error (no battle in progress)", result)
	}

	after := playerAt(t, e)
	if before.MapID != after.MapID || before.X != after.X || before.Y != after.Y {
		t.Errorf("player changed: before %+v, after %+v", before, after)
	}
	state.Snapshot(e, &mem)
	if !state.Controllable(&mem) {
		t.Error("player not controllable after failed Battle")
	}
}

// speciesPidgey / speciesRattata are the ROM pokemon indexes (not dex
// numbers, pokered/constants/pokemon_constants.asm): PIDGEY = $24,
// RATTATA = $A5. They are Route 1's two wild entries, so either one is a
// valid reserve for the forced switch. speciesSquirtle / speciesWartortle
// (train_test.go) name the fixture's lead and its level-16 evolution; the
// success assertion accepts either in slot 1.
const (
	speciesPidgey  uint8 = 0x24
	speciesRattata uint8 = 0xA5
)

// TestBattleForcedSwitchAfterFaint is S6-5b: a two-mon party driven into a
// REAL wild faint with a live reserve. The screen under test is the one that
// appeared the first time a second party member existed and the lead lost:
// "Use next #MON?" (core.asm DoUseNextMonDialogue), then the battle party
// menu (ChooseNextMon). Before this task Battle handled neither — both fall
// into the default A-tap branch and the frame cap trips — so the proof is a
// journey that used to fail loudly now ending with the fainted lead in slot
// 0 at HP 0 and SQUIRTLE alive in slot 1.
//
// The shape: the post_pokeballs fixture carries a level-15 SQUIRTLE and five
// balls. Catch a PIDGEY or RATTATA on Route 1 (S6-3), promote it to lead (it
// is the only way a caught mon fights), then fight through Route 1's grass
// with the weak catch up front. It cannot hold that line against level 2-5
// wilds with cumulative damage; when it faints, Battle must answer YES and
// send out SQUIRTLE instead of tripping the frame cap.
//
// Every leg is one the fixture builders already walk (Pallet <-> Route 1
// <-> Viridian City): measured on the first two runs, any leg that starts in
// Viridian Forest is planned through the city interior, and the planner cuts
// the MUSEUM_1F there — whose scientist asks "Would you like to come in?",
// a choice Travel does not answer. The Route 1 legs avoid the city interior
// entirely.
//
// Slow and stochastic (a hunt plus a KO), so it is guarded out of -short and
// proven under its own command:
//
//	POKEMON_RED_ROM=roms/pokemon_red.gb go test ./skill -run TestBattleForcedSwitchAfterFaint -v
func TestBattleForcedSwitchAfterFaint(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey (catch plus wild KOs); run without -short, see the test docs")
	}

	const attempts = 3
	for attempt := 1; attempt <= attempts; attempt++ {
		if forcedSwitchAttempt(t, attempt) {
			return
		}
	}
	t.Fatalf("no real faint with a live reserve in %d attempts — read the failure state before changing anything", attempts)
}

// forcedSwitchAttempt runs one full try from a fresh fixture and reports
// whether it observed the forced switch. A missed catch or a lead that
// never fainted is a game outcome, not a bug: log it and let the caller
// retry from a fresh fixture state (a fresh five-ball bag and RNG phase).
func forcedSwitchAttempt(t *testing.T, attempt int) bool {
	t.Helper()
	m := fixture.Load(t, "post_pokeballs")
	// Re-phase the encounter rolls so attempts are independent (measured in
	// TestCatchCaterpie: unshifted attempts replay one identical run).
	m.StepFrames((attempt - 1) * 97)
	romData := m.ROM()
	policy := skill.StatAwareMove(romData)

	var mem state.Mem
	state.Snapshot(m, &mem)
	party0 := state.DecodeParty(&mem)
	if party0.Count != 1 {
		t.Fatalf("attempt %d: fixture precondition: party has %d members, want 1", attempt, party0.Count)
	}
	leadSpecies := party0.Mons[0].Species
	if leadSpecies != speciesSquirtle {
		t.Fatalf("attempt %d: fixture precondition: lead is species %#02x, want SQUIRTLE (%#02x)", attempt, leadSpecies, speciesSquirtle)
	}
	if q := bagQty(t, m, skill.ItemPokeBall); q != 5 {
		t.Fatalf("attempt %d: fixture precondition: expected five POKE BALLS, got %d", attempt, q)
	}

	// 1. A live reserve: catch a PIDGEY or RATTATA on Route 1.
	route1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal(`Place "route 1" not found`)
	}
	if _, err := skill.Travel(m, romData, route1, policy, 20); err != nil {
		t.Fatalf("attempt %d: travel to Route 1: %v", attempt, err)
	}
	res, err := skill.Catch(m, romData, []uint8{speciesPidgey, speciesRattata}, policy, 5)
	if err != nil || res.Outcome != skill.OutcomeCaught {
		t.Logf("attempt %d: no catch (outcome=%d balls=%d encounters=%d err=%v); retrying", attempt, res.Outcome, res.BallsThrown, res.Encounters, err)
		return false
	}
	state.Snapshot(m, &mem)
	party1 := state.DecodeParty(&mem)
	if party1.Count != 2 || (party1.Mons[1].Species != speciesPidgey && party1.Mons[1].Species != speciesRattata) {
		t.Fatalf("attempt %d: post-catch party = %+v, want SQUIRTLE + a Route 1 wild", attempt, party1.Mons)
	}

	// 2. The weak catch up front: it is the mon that will faint.
	if err := skill.PromoteToLead(m, 1); err != nil {
		t.Fatalf("attempt %d: PromoteToLead: %v", attempt, err)
	}
	state.Snapshot(m, &mem)
	leadAfterSwap := state.DecodeParty(&mem).Mons[0].Species
	if leadAfterSwap == leadSpecies {
		t.Fatalf("attempt %d: lead after swap is still SQUIRTLE (%#02x)", attempt, leadAfterSwap)
	}

	// 3. Fight through Route 1's grass until the lead faints. The positive
	// fact that the switch happened: Travel returned nil (a frame-cap trip
	// would be an error) and the weak catch is now in slot 0 at HP 0.
	// No heal: its damage is cumulative, which is what brings the faint.
	pallet, ok := skill.Place("pallet town")
	if !ok {
		t.Fatal(`Place "pallet town" not found`)
	}
	viridian, ok := skill.Place("viridian city")
	if !ok {
		t.Fatal(`Place "viridian city" not found`)
	}
	for trip := 0; trip < 6; trip++ {
		dest := pallet
		if trip%2 == 1 {
			dest = viridian
		}
		if _, err := skill.Travel(m, romData, dest, policy, 20); err != nil {
			if errors.Is(err, skill.ErrBlackedOut) {
				// Both mons went down in the same battle: no switch was
				// observed. The respawn is fully healed; keep fighting.
				t.Logf("attempt %d trip %d: blackout (no switch observed); continuing", attempt, trip)
				continue
			}
			t.Fatalf("attempt %d trip %d: Travel to %v: %v", attempt, trip, dest, err)
		}
		state.Snapshot(m, &mem)
		party := state.DecodeParty(&mem)
		if !party.Mons[0].Fainted() {
			continue
		}

		// The forced switch happened: the fainted catch was replaced by a
		// SPECIFIC live species — the fixture's lead. It may have evolved:
		// the fixture's SQUIRTLE is level 15, one battle XP away from 16,
		// where it becomes WARTORTLE (measured on the first green run: the
		// replacement read species 0xB3 at level 16).
		if party.Mons[1].Species != leadSpecies && party.Mons[1].Species != speciesWartortle {
			t.Fatalf("attempt %d trip %d: slot 1 is species %#02x, want SQUIRTLE (%#02x) or WARTORTLE (%#02x): %+v",
				attempt, trip, party.Mons[1].Species, leadSpecies, speciesWartortle, party.Mons)
		}
		if party.Mons[1].Fainted() {
			t.Fatalf("attempt %d trip %d: the replacement SQUIRTLE is fainted: %+v", attempt, trip, party.Mons)
		}
		if !state.Controllable(&mem) {
			t.Fatalf("attempt %d trip %d: player not controllable after the forced switch: %+v",
				attempt, trip, state.DecodePlayer(&mem))
		}
		t.Logf("attempt %d trip %d: lead (species %#02x) fainted; Battle answered UseNextMon and sent out the fixture's lead (species %#02x) — party now %+v",
			attempt, trip, leadAfterSwap, party.Mons[1].Species, party.Mons)
		return true
	}
	t.Logf("attempt %d: the lead never fainted in 6 trips; retrying", attempt)
	return false
}

// TestBattleAnswersForgetMovePrompt is S8-2: a level-up that offers a move
// while all four slots are full prints "<NAME> is trying to learn <MOVE>?"
// and, on YES, a "Which move should be forgotten?" list. Before this task
// Battle's default A-tap branch answered that prompt by accident — the run
// survives but the move is dropped. Now Battle answers it on purpose: YES,
// then the move in the lowest slot that is not the mon's only damaging
// option (forgetSlot) gives way to the new one.
//
// It follows TestTrainSurvivesEvolution's setup: the post_pokeballs
// fixture's level-15 SQUIRTLE already carries four moves, so the BITE offer
// is a prompt, not a plain box. The target is level 24, NOT 22: the mon
// evolves into WARTORTLE at 16, and LearnMoveFromLevelUp reads the CURRENT
// species' learnset (wPokedexNum = wCurSpecies), where BITE sits at 24 —
// Squirtle's table says 22, but that table stopped applying at level 16.
// (Measured: a grind to 22 shows "grew to level 22!" and the stats box and
// then the battle ends with no prompt at all.) The assertion is POSITIVE
// and read from RAM: after the grind, the move set is exactly what the
// stated policy says it should be — BITE in the computed slot, the other
// three moves untouched.
//
// A full journey (Route 1 grind), guarded out of -short:
//
//	POKEMON_RED_ROM=roms/pokemon_red.gb go test ./skill -run TestBattleAnswersForgetMovePrompt -v
func TestBattleAnswersForgetMovePrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey (Route 1 grind); run without -short, see the test docs")
	}
	e := fixture.Load(t, "post_pokeballs")
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)

	// Precondition: one mon, a level-15 SQUIRTLE with all four slots full.
	// An empty slot would make the level-24 BITE offer a plain text box and
	// the prompt under test would never fire.
	var mem state.Mem
	state.Snapshot(e, &mem)
	party := state.DecodeParty(&mem)
	if party.Count != 1 {
		t.Fatalf("fixture precondition: party has %d mons, want exactly one", party.Count)
	}
	lead := party.Mons[0]
	if lead.Species != speciesSquirtle || lead.Level < 15 {
		t.Fatalf("fixture precondition: lead is species %#02x lv%d, want SQUIRTLE (%#02x) lv>=15", lead.Species, lead.Level, speciesSquirtle)
	}
	before := lead.Moves
	for i, mv := range before {
		if mv == 0 {
			t.Fatalf("fixture precondition: move slot %d is empty — the prompt only fires with four moves held", i)
		}
	}

	// Expected outcome per the stated policy: BITE replaces the lowest slot
	// that is not the mon's only damaging option; the other three moves stay.
	wantSlot := expectedForgetSlot(t, romData, before)
	want := before
	want[wantSlot] = moveBite

	dest := route1Grass(t, romData)
	// Level 24: WARTORTLE's BITE (WartortleEvosMoves db 24), the first
	// offered move after the level-16 evolution.
	const target = 24
	// Same blackout handling as TestTrainSurvivesEvolution: a one-mon party
	// that faints is fully healed and respawned on a grassless town, so each
	// segment re-Travel's to the grass and resumes from the level reached.
	// Four levels at roughly 35-45 battles a level (measured), plus
	// blackouts, so the cap is generous.
	const totalCap = 700
	totalBattles := 0
	for segment := 1; ; segment++ {
		if _, err := skill.Travel(e, romData, dest, policy, 6); err != nil {
			t.Fatalf("Travel to Route 1 (segment %d): %v", segment, err)
		}
		r, err := skill.Train(e, romData, target, policy, 150)
		if err != nil {
			t.Fatalf("Train (segment %d): %v (battles=%d)", segment, err, totalBattles+r.Battles)
		}
		totalBattles += r.Battles
		if r.Reached {
			break
		}
		if totalBattles >= totalCap {
			t.Fatalf("did not reach level %d in %d battles (endLevel=%d) — Train stopped short of or hung at an interruption", target, totalBattles, r.EndLevel)
		}
		t.Logf("segment %d: %d battle(s), level %d, blackedOut=%v; resuming the grind", segment, r.Battles, r.EndLevel, r.BlackedOut)
	}

	state.Snapshot(e, &mem)
	after := state.DecodeParty(&mem).Mons[0]
	if after.Species != speciesWartortle {
		t.Fatalf("lead is species %#02x lv%d after training to %d, want WARTORTLE (%#02x)", after.Species, after.Level, target, speciesWartortle)
	}
	if state.DecodeBattle(&mem) != nil || !state.Controllable(&mem) {
		t.Fatalf("a sequence was left in progress after Train: battle=%v controllable=%v", state.DecodeBattle(&mem) != nil, state.Controllable(&mem))
	}
	if after.Moves != want {
		t.Fatalf("move set after the level-24 prompt is %v, want %v — BITE (%#02x) replacing slot %d, the lowest slot that is not the mon's only damaging option", after.Moves, want, moveBite, wantSlot)
	}
	t.Logf("level-24 prompt answered on purpose: moves %v -> %v (BITE %#02x in slot %d), %d battle(s)", before, after.Moves, moveBite, wantSlot, totalBattles)
}

// expectedForgetSlot mirrors the policy Battle states in forgetSlot: the
// lowest slot that is not the mon's only damaging option. If exactly one of
// the four moves deals damage (power > 0 in the ROM move table), it stays
// and the lowest of the rest is returned; otherwise the lowest slot is.
func expectedForgetSlot(t *testing.T, romData []byte, moves [4]uint8) int {
	t.Helper()
	damagers := 0
	damages := [4]bool{}
	for i, id := range moves {
		mv, err := rom.LookupMove(romData, id)
		if err != nil {
			t.Fatalf("look up move %#02x: %v", id, err)
		}
		damages[i] = mv.Power > 0
		if damages[i] {
			damagers++
		}
	}
	for i := range moves {
		if !(damagers == 1 && damages[i]) {
			return i
		}
	}
	t.Fatal("no slot is acceptable")
	return 0
}
