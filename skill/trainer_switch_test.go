package skill_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestBattleDeclinesTrainerSwitchWithReserve reproduces the live LLM-run
// wedge: RUN is refused in Pewter Gym while the party has a reserve, then the
// trainer's second Pokémon triggers "Will <PLAYER> change Pokémon?" Battle
// must answer NO and finish instead of selecting the already-active lead in
// the party menu forever.
func TestBattleDeclinesTrainerSwitchWithReserve(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey (catch plus Pewter trainer); run under its own gate")
	}

	const attempts = 3
	for attempt := 1; attempt <= attempts; attempt++ {
		m := fixture.Load(t, "post_pokeballs")
		m.StepFrames((attempt - 1) * 97)
		romData := m.ROM()
		policy := skill.StatAwareMove(romData)

		route1, ok := skill.Place("route 1")
		if !ok {
			t.Fatal(`Place "route 1" not found`)
		}
		if _, err := skill.Travel(m, romData, route1, policy, 20); err != nil {
			t.Fatalf("attempt %d: travel to Route 1: %v", attempt, err)
		}
		caught, err := skill.Catch(m, romData, []uint8{speciesPidgey, speciesRattata}, policy, 5)
		if err != nil || caught.Outcome != skill.OutcomeCaught {
			t.Logf("attempt %d: no reserve caught (outcome=%d err=%v); retrying", attempt, caught.Outcome, err)
			continue
		}

		center, ok := skill.Place("pewter pokemon center")
		if !ok {
			t.Fatal(`Place "pewter pokemon center" not found`)
		}
		if _, err := skill.Travel(m, romData, center, policy, 20); err != nil {
			t.Fatalf("attempt %d: travel to Pewter Center: %v", attempt, err)
		}
		if err := skill.Heal(m); err != nil {
			t.Fatalf("attempt %d: heal: %v", attempt, err)
		}

		for _, dest := range []skill.Destination{{Map: 0x36, X: 1, Y: 8}, {Map: 0x36, X: 1, Y: 4}} {
			if _, err := skill.Travel(m, romData, dest, policy, 10); err != nil {
				t.Fatalf("attempt %d: approach Pewter trainer: %v", attempt, err)
			}
		}
		err = skill.GoTo(m, romData, skill.Destination{Map: 0x36, X: 5, Y: 6})
		if !errors.Is(err, skill.ErrBattle) && !errors.Is(err, skill.ErrDialogueInterrupted) {
			t.Fatalf("attempt %d: engage trainer: %v", attempt, err)
		}
		if errors.Is(err, skill.ErrDialogueInterrupted) {
			skill.RecoverDialogue(m, 10000)
		}
		if err := waitForBattleStart(t, m); err != nil {
			t.Fatal(err)
		}

		if err := skill.Flee(m, 3); !errors.Is(err, skill.ErrTrainerBattle) {
			t.Fatalf("attempt %d: Flee: want ErrTrainerBattle, got %v", attempt, err)
		}
		sawTrainerSwitchPrompt := false
		m.OnFrame(func(m *emu.Emu) {
			var mem state.Mem
			state.Snapshot(m, &mem)
			if strings.Contains(state.ScreenText(&mem), "change POK") && state.DecodeTwoOptionMenu(&mem) != nil {
				sawTrainerSwitchPrompt = true
			}
		})
		outcome, err := skill.Battle(m, policy)
		if err != nil {
			t.Fatalf("attempt %d: Battle after refused RUN with reserve: %v", attempt, err)
		}
		if outcome != state.ResultWon {
			t.Fatalf("attempt %d: outcome = %v, want won", attempt, outcome)
		}
		if !sawTrainerSwitchPrompt {
			t.Fatal("battle never presented the live trainer-switch prompt; regression path was not exercised")
		}
		return
	}
	t.Fatalf("no reserve caught in %d phase-shifted attempts", attempts)
}
