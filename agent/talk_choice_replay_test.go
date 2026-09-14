package agent_test

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// TestTalkChoiceReplay pins the generic-Talk YES/NO decline repair (#399) end
// to end. It replays the round-033 objective-start state from
// run-206056rm7csb113ymnzmli3zuc (Pewter City, "Did you check out the MUSEUM?"),
// where a generic KindTalk objective reached an unclassified YES/NO prompt.
// Before #399 the objective failed with "unanswered choice remains open"; the
// repair declines the prompt with NO and returns to a controllable boundary.
//
// The state is private (not committed), so the test is gated on an env var,
// mirroring TestTrainerArrivalReplay.
func TestTalkChoiceReplay(t *testing.T) {
	path := talkChoiceStatePath(t)
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.LoadState(b); err != nil {
		t.Fatal(err)
	}

	result, err := agent.Execute(m, m.ROM(), agent.Objective{Kind: agent.KindTalk, X: 27, Y: 17})
	if err != nil {
		t.Fatalf("objective failed: outcome=%q err=%v", result.Outcome, err)
	}
	if result.Outcome != agent.OutcomeCompleted {
		t.Fatalf("outcome=%q, want completed", result.Outcome)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		t.Fatal("player not controllable after objective")
	}
	if state.MenuUp(&mem) {
		t.Fatal("menu still up after objective")
	}
	if state.DecodeTwoOptionMenu(&mem) != nil {
		t.Fatal("two-option choice still open after objective")
	}
}

func talkChoiceStatePath(t *testing.T) string {
	t.Helper()
	path := os.Getenv("POKEPILOT_TALK_CHOICE_STATE")
	if path == "" {
		t.Skip("POKEPILOT_TALK_CHOICE_STATE not set")
	}
	return path
}
