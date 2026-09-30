package main

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestBattleActiveROMRejectsIllegalOutputWithoutGameplayChange is the
// ROM-backed #1460 safety proof. Both runs restore exactly the same Route 1
// checkpoint and execute the same deterministic battle policy. The active run
// additionally asks a high-confidence backend that always returns an
// undeclared move. Every answer must be rejected and the resulting emulator
// state must be identical to the control.
func TestBattleActiveROMRejectsIllegalOutputWithoutGameplayChange(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed active battle legality proof")
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
	run := func(active bool) (runResult, *statsPlanner, *countingDecisionEngine) {
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
		var engine *countingDecisionEngine
		if active {
			engine = &countingDecisionEngine{resp: agent.DecisionResponse{
				Choice:      "move:99",
				Confidence:  0.99,
				Backend:     "test-active",
				Model:       "illegal-choice",
				Probabilities: map[string]float64{"move:99": 1},
			}}
			planner = battleActivePlanner(engine)
			romData := m.ROM()
			restoreSelector := skill.WithMoveSelector(m, func(b game.BattleState, deterministic int) int {
				state, err := skill.MoveOnlyBattleDecisionState(romData, b)
				if err != nil {
					return deterministic
				}
				fallback := game.BattleAction{Kind: game.BattleActionMove, Slot: deterministic}
				action := planner.ControlBattleMove(state, fallback)
				if action.Kind != game.BattleActionMove {
					return deterministic
				}
				return action.Slot
			})
			defer restoreSelector()
			restoreResult := skill.WithBattleResultObserver(m, planner.ObserveBattleResult)
			defer restoreResult()
		}

		for i := 0; i < 18; i++ {
			m.Tap(emu.Right, 3, 7)
		}
		m.StepFrames(300)

		result, err := skill.Battle(m, skill.StatAwareMove(m.ROM()))
		if err != nil {
			t.Fatalf("Battle(active=%t): %v", active, err)
		}
		return runResult{battle: result, frame: m.FrameCount(), digest: battleShadowDigest(m)}, planner, engine
	}

	control, _, _ := run(false)
	guarded, planner, engine := run(true)

	if control.battle != guarded.battle || control.frame != guarded.frame || control.digest != guarded.digest {
		t.Fatalf("illegal active output changed gameplay: control=(result=%d frame=%d digest=%x) active=(result=%d frame=%d digest=%x)",
			control.battle, control.frame, control.digest, guarded.battle, guarded.frame, guarded.digest)
	}
	if engine == nil || engine.calls == 0 {
		t.Fatal("active safety run never consulted the backend")
	}
	if planner == nil || len(planner.stats.DecisionRecords) == 0 {
		t.Fatal("active safety run recorded no decisions")
	}
	for i, record := range planner.stats.DecisionRecords {
		if record.Kind != agent.DecisionKindBattleTurn {
			continue
		}
		if record.Controlled || !record.Fallback || record.Error == "" {
			t.Fatalf("decision %d escaped fallback gate: %+v", i, record)
		}
	}
}
