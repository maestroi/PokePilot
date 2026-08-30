package skill_test

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestFleeWildBattle is the S8-4 postcondition: from a wild battle, Flee(5)
// ends the battle and the player is controllable. Both halves are asserted
// from RAM — "the menu went away" is not the postcondition. The attempt
// count is sampled per frame: wNumRunAttempts increments once per RUN roll
// in a wild battle and is ZEROED when the battle ends, so the max observed
// during the flee is exactly how many attempts it took — and it must lie in
// [1, 5]: at least one roll happened (RUN was really selected) and no more
// than `attempts` (the retry bound).
func TestFleeWildBattle(t *testing.T) {
	if testing.Short() {
		t.Skip("wild-battle journey test; run without -short")
	}
	m := fixture.Load(t, "post_pokeballs")

	travelToRoute1(t, m)
	if err := skill.EnterWildBattle(m, 3); err != nil {
		t.Fatalf("enter wild battle: %v", err)
	}

	var maxAttempts byte
	m.OnFrame(func(m *emu.Emu) {
		if a := m.Peek8(sym.NumRunAttempts); a > maxAttempts {
			maxAttempts = a
		}
	})

	const attempts = 5
	if err := skill.Flee(m, attempts); err != nil {
		t.Fatalf("Flee(%d): %v", attempts, err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if bs := state.DecodeBattle(&mem); bs != nil {
		t.Fatalf("postcondition: battle still in progress after Flee: %+v", bs)
	}
	if !state.Controllable(&mem) {
		t.Fatal("postcondition: player not controllable after Flee")
	}
	if maxAttempts == 0 {
		t.Fatal("no RUN roll was made: wNumRunAttempts never rose, so Flee never selected RUN")
	}
	if int(maxAttempts) > attempts {
		t.Fatalf("bound: %d RUN attempts, want at most %d", maxAttempts, attempts)
	}
	t.Logf("fled the wild battle in %d attempt(s)", maxAttempts)
}

// TestFleeTrainerBattle is the trapped-opponent case: a trainer battle
// refuses the RUN option (engine/battle/core.asm TryRunningFromBattle
// .trainerBattle prints "No! There's no running from a trainer battle!" and
// never rolls an escape). Flee must return the typed ErrTrainerBattle after
// the first refusal rather than looping `attempts` times against a wall,
// leaving the battle in progress for a caller to fight.
//
// The attempt count is asserted POSITIVELY from RAM: wNumRunAttempts
// increments only on the WILD roll path, after the trainer check, so it must
// still read 0 — no wild rolls were made, and Flee did not burn its
// attempts.
//
// Route to the Pewter Gym Cool Trainer (PEWTERGYM_COOLTRAINER_M at (3,6)
// facing right, sight line row 6 x=4..8): the same x=1 side corridor
// TestGymJourneyAffordances proved keeps every leg off his sight line, then
// one GoTo onto the line at (5,6), which engages him and aborts with
// ErrBattle. A full journey — minutes of emulation — so it runs only
// outside -short.
func TestFleeTrainerBattle(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey; proven by its own -run gate, not the per-task gate")
	}
	m := fixture.Load(t, "post_pokeballs")
	romData := m.ROM()
	policy := skill.StatAwareMove(romData)
	leg := func(dest skill.Destination, what string) {
		for attempt := 0; ; attempt++ {
			res, err := skill.Travel(m, romData, dest, policy, 10)
			if err == nil {
				t.Logf("%s: arrived (battles=%d)", what, res.Battles)
				return
			}
			t.Logf("%s: attempt %d failed after %d battle(s): %v", what, attempt, res.Battles, err)
			if !errors.Is(err, skill.ErrBlackedOut) || attempt >= 2 {
				diagFatalf(t, m, err, "%s: %v", what, err)
			}
			settleBlackout(t, m)
		}
	}

	// Heal first: the Cool Trainer sends out two L11 mons and his defeat
	// flag is set only by Brock's victory script, so there is no discount on
	// this fight.
	center, ok := skill.Place("pewter pokemon center")
	if !ok {
		t.Fatal(`Place "pewter pokemon center" not found`)
	}
	leg(center, "Travel to the Pewter Center")
	if err := skill.Heal(m); err != nil {
		t.Fatalf("Heal at the Pewter Center: %v", err)
	}

	// The side corridor: every leg's shortest path stays off row 6 x=4..8
	// (probed on map 0x36 by TestGymJourneyAffordances).
	leg(skill.Destination{Map: 0x36, X: 1, Y: 8}, "Enter the gym through the side corridor")
	leg(skill.Destination{Map: 0x36, X: 1, Y: 4}, "Up the side corridor to row 4")

	// Onto his sight line: the last step engages him. The first signal is
	// his pre-battle text box (ErrDialogueInterrupted), with ErrBattle on a
	// later walk; either way page the box closed — RecoverDialogue stops the
	// frame the battle starts — and wait for the battle to be in progress.
	err := skill.GoTo(m, romData, skill.Destination{Map: 0x36, X: 5, Y: 6})
	if !errors.Is(err, skill.ErrBattle) && !errors.Is(err, skill.ErrDialogueInterrupted) {
		t.Fatalf("GoTo onto the Cool Trainer's sight line: want ErrBattle or ErrDialogueInterrupted, got %v", err)
	}
	if errors.Is(err, skill.ErrDialogueInterrupted) {
		skill.RecoverDialogue(m, 10000)
	}
	if err := waitForBattleStart(t, m); err != nil {
		t.Fatal(err)
	}

	const attempts = 3
	err = skill.Flee(m, attempts)
	if !errors.Is(err, skill.ErrTrainerBattle) {
		t.Fatalf("Flee(%d) in a trainer battle: want ErrTrainerBattle, got %v", attempts, err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if bs := state.DecodeBattle(&mem); bs == nil {
		t.Fatal("the trainer battle is over after a refused flee; the refusal must leave it in progress")
	}
	if got := int(m.Peek8(sym.NumRunAttempts)); got != 0 {
		t.Fatalf("wNumRunAttempts = %d after a refused flee: the trainer path never rolls an escape, so Flee burned attempts it should not have", got)
	}
	t.Log("trainer battle refused RUN; Flee returned ErrTrainerBattle with zero wild rolls and the battle still in progress")
}

// waitForBattleStart steps until a battle is in progress and reports an
// error if one does not start within budget frames.
func waitForBattleStart(t *testing.T, m *emu.Emu) error {
	t.Helper()
	for i := 0; i < 3000; i++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) != nil {
			return nil
		}
		m.StepFrame()
	}
	return errors.New("no battle in progress 3000 frames after the trainer engaged")
}
