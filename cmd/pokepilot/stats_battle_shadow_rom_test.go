package main

import (
	"context"
	"crypto/sha256"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

type disagreeingBattleShadowEngine struct {
	choice string
	calls  int
}

func (e *disagreeingBattleShadowEngine) Decide(_ context.Context, req agent.DecisionRequest) (agent.DecisionResponse, error) {
	e.calls++
	probabilities := make(map[string]float64, len(req.Choices))
	for _, choice := range req.Choices {
		probabilities[choice.ID] = 0.01
	}
	probabilities[e.choice] = 0.99
	return agent.DecisionResponse{
		Choice:        e.choice,
		Confidence:    0.99,
		Probabilities: probabilities,
		Backend:       "test-shadow",
		Model:         "deliberate-disagreement",
	}, nil
}

// TestBattleShadowROMDoesNotChangeGameplay is the ROM-backed #1456 proof.
// The same restored Route 1 state is replayed twice. The shadow run asks a
// backend that is deliberately configured to choose a different legal move
// from the deterministic policy on every turn where an alternative exists.
// The backend answer is recorded, but never returned to Battle. Final frame
// count and emulator memory must therefore remain identical to the control.
func TestBattleShadowROMDoesNotChangeGameplay(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed shadow equivalence proof")
	}
	rom := os.Getenv("POKEMON_RED_ROM")
	if rom == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}

	base, err := fixture.LoadState("route1")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := base.SaveState()
	if err != nil {
		base.Close()
		t.Fatal(err)
	}
	base.Close()

	type runResult struct {
		battle game.BattleResult
		frame  uint64
		digest [32]byte
	}
	run := func(shadow bool) (runResult, *statsPlanner, *disagreeingBattleShadowEngine) {
		t.Helper()
		m, err := emu.Open(rom)
		if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		if err := m.LoadState(snapshot); err != nil {
			t.Fatal(err)
		}

		var planner *statsPlanner
		var engine *disagreeingBattleShadowEngine
		if shadow {
			engine = &disagreeingBattleShadowEngine{}
			planner = battleShadowPlanner(engine)
			romData := m.ROM()
			restoreMove := skill.WithMoveObserver(m, func(b game.BattleState, executed int) {
				state, err := skill.MoveOnlyBattleDecisionState(romData, b)
				if err != nil {
					t.Fatalf("build shadow turn: %v", err)
				}
				executedID := (game.BattleAction{Kind: game.BattleActionMove, Slot: executed}).ID()
				engine.choice = ""
				for _, candidate := range state.Actions() {
					if candidate.ID() != executedID {
						engine.choice = candidate.ID()
						break
					}
				}
				if engine.choice == "" {
					// A one-move turn cannot deliberately disagree. Do not ask
					// the fake backend; later turns still prove disagreement.
					return
				}
				planner.ObserveBattleTurn(state, game.BattleAction{Kind: game.BattleActionMove, Slot: executed})
			})
			defer restoreMove()
			restoreResult := skill.WithBattleResultObserver(m, planner.ObserveBattleResult)
			defer restoreResult()
		}

		// route1 stands at (5,14). This is the same deterministic battle trigger
		// used by emu/determinism_test.go: walking east reaches tall grass.
		for i := 0; i < 18; i++ {
			m.Tap(emu.Right, 3, 7)
		}
		m.StepFrames(300)

		result, err := skill.Battle(m, skill.StatAwareMove(m.ROM()))
		if err != nil {
			t.Fatalf("Battle(shadow=%t): %v", shadow, err)
		}
		return runResult{battle: result, frame: m.FrameCount(), digest: battleShadowDigest(m)}, planner, engine
	}

	control, _, _ := run(false)
	observed, planner, engine := run(true)

	if control.battle != observed.battle || control.frame != observed.frame || control.digest != observed.digest {
		t.Fatalf("shadow changed gameplay: control=(result=%d frame=%d digest=%x) shadow=(result=%d frame=%d digest=%x)",
			control.battle, control.frame, control.digest, observed.battle, observed.frame, observed.digest)
	}
	if engine == nil || engine.calls == 0 || planner.stats.DecisionDisagreements == 0 {
		t.Fatalf("shadow backend did not produce a measured disagreement: engine=%+v stats=%+v", engine, planner.stats)
	}
	if len(planner.battleShadowSamples) == 0 {
		t.Fatal("shadow run recorded no battle samples")
	}
	for i, sample := range planner.battleShadowSamples {
		if sample.DecisionIndex == 0 || sample.StateFingerprint == "" || sample.Outcome == nil {
			t.Fatalf("sample %d missing #1456 evidence: %+v", i, sample)
		}
	}
	last := planner.battleShadowSamples[len(planner.battleShadowSamples)-1]
	if last.Outcome.Kind != "battle_result" {
		t.Fatalf("final sample outcome = %+v, want terminal battle result", last.Outcome)
	}
}

// TestBattleActiveROMRejectsIllegalControllerOutput is the checkpoint-backed
// #1460 safety proof. A malicious active controller returns an impossible move
// slot on every turn. skill.Battle must clamp that output back to the already
// validated deterministic slot, producing exactly the same result, frame and
// emulator-state digest as the control replay.
func TestBattleActiveROMRejectsIllegalControllerOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed active legality proof")
	}
	rom := os.Getenv("POKEMON_RED_ROM")
	if rom == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	base, err := fixture.LoadState("route1")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := base.SaveState()
	if err != nil {
		base.Close()
		t.Fatal(err)
	}
	base.Close()

	type runResult struct {
		battle game.BattleResult
		frame  uint64
		digest [32]byte
	}
	run := func(active bool) runResult {
		t.Helper()
		m, err := emu.Open(rom)
		if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		if err := m.LoadState(snapshot); err != nil {
			t.Fatal(err)
		}
		if active {
			restore := skill.WithBattleMoveController(m, func(_ game.BattleState, _ int) int { return 99 })
			defer restore()
		}
		for i := 0; i < 18; i++ {
			m.Tap(emu.Right, 3, 7)
		}
		m.StepFrames(300)
		result, err := skill.Battle(m, skill.StatAwareMove(m.ROM()))
		if err != nil {
			t.Fatalf("Battle(active=%t): %v", active, err)
		}
		return runResult{battle: result, frame: m.FrameCount(), digest: battleShadowDigest(m)}
	}

	control := run(false)
	malicious := run(true)
	if control != malicious {
		t.Fatalf("illegal active output escaped the legal gate: control=%+v malicious=%+v", control, malicious)
	}
}

func battleShadowDigest(m *emu.Emu) [32]byte {
	h := sha256.New()
	buf := make([]byte, 0x2000)
	m.PeekInto(0xC000, buf[:0x1000])
	_, _ = h.Write(buf[:0x1000])
	m.PeekInto(0x8000, buf)
	_, _ = h.Write(buf)
	m.PeekInto(0xFE00, buf[:0x100])
	_, _ = h.Write(buf[:0x100])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}
