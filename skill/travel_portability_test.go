package skill

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

type fakeTravelMemory struct{}

func (fakeTravelMemory) Peek8(uint16) byte       { return 0 }
func (fakeTravelMemory) PeekInto(uint16, []byte) {}

type fakeGen2TravelRuntime struct {
	world    game.OverworldState
	battle   game.BattleState
	inBattle bool
	result   game.BattleResult
	blackout game.OverworldBlackoutState
}

func (f *fakeGen2TravelRuntime) DecodeOverworld(game.MemoryReader) game.OverworldState {
	return f.world
}

func (f *fakeGen2TravelRuntime) DecodeBattleState(game.MemoryReader) (game.BattleState, bool) {
	if !f.inBattle {
		return game.BattleState{}, false
	}
	return f.battle, true
}

func (f *fakeGen2TravelRuntime) DecodeBattleResult(game.MemoryReader) game.BattleResult {
	return f.result
}

func (f *fakeGen2TravelRuntime) DecodeOverworldBlackout(game.MemoryReader) game.OverworldBlackoutState {
	return f.blackout
}

func fakeGen2TravelResolvers(
	mem game.MemoryReader,
	runtime *fakeGen2TravelRuntime,
	resolve resolveBattle,
) interruptionResolvers {
	return interruptionResolvers{
		recoverBox: func() DialogueRecoveryResult {
			return DialogueRecoveryResult{Stop: DialogueRecovered}
		},
		blackout: func() bool {
			return blackoutInProgress(mem, runtime)
		},
		resolveBattle: resolve,
		observe: func() (Replan, error) {
			return currentWorldWithDecoder(mem, runtime)
		},
		settle: func(Replan, bool) (Replan, error) {
			return currentWorldWithDecoder(mem, runtime)
		},
	}
}

func TestPortableTravelFakeGen2WildFleeReplansFromPostEngagementWorld(t *testing.T) {
	mem := fakeTravelMemory{}
	runtime := &fakeGen2TravelRuntime{
		world:    game.OverworldState{NativeMapID: 0x41, X: 8, Y: 12, Controllable: false, InBattle: true},
		battle:   game.BattleState{Kind: game.BattleWild},
		inBattle: true,
	}

	fightCalls := 0
	resolve := fleeThenFightWith(
		func(attempts int) error {
			if attempts != guaranteedWildFleeAttempts {
				t.Fatalf("flee attempts = %d, want %d", attempts, guaranteedWildFleeAttempts)
			}
			runtime.inBattle = false
			runtime.world = game.OverworldState{NativeMapID: 0x41, X: 9, Y: 12, Controllable: true}
			return nil
		},
		func() (game.BattleResult, error) {
			fightCalls++
			return game.BattleWon, nil
		},
		func() game.BattleKind { return runtime.battle.Kind },
		guaranteedWildFleeAttempts,
	)

	calls := 0
	res, err := runInterruptions(nil, 3, func() error {
		calls++
		if calls == 1 {
			return ErrBattle
		}
		return nil
	}, fakeGen2TravelResolvers(mem, runtime, resolve))
	if err != nil {
		t.Fatalf("runInterruptions: %v", err)
	}
	if fightCalls != 0 {
		t.Fatalf("fight called %d time(s), want 0 after successful wild flee", fightCalls)
	}
	if res.Flees != 1 || res.Battles != 0 {
		t.Fatalf("result = %+v, want one flee and no fights", res)
	}
	want := Replan{Map: 0x41, X: 9, Y: 12}
	if len(res.Replans) != 1 || res.Replans[0] != want {
		t.Fatalf("replans = %+v, want post-flee world %+v", res.Replans, want)
	}
}

func TestPortableTravelFakeGen2TrainerFleeRefusalFallsBackToFight(t *testing.T) {
	mem := fakeTravelMemory{}
	runtime := &fakeGen2TravelRuntime{
		world:    game.OverworldState{NativeMapID: 0x52, X: 3, Y: 4, InBattle: true},
		battle:   game.BattleState{Kind: game.BattleTrainer},
		inBattle: true,
	}

	fleeCalls, fightCalls := 0, 0
	resolve := fleeThenFightWith(
		func(int) error {
			fleeCalls++
			return ErrTrainerBattle
		},
		func() (game.BattleResult, error) {
			fightCalls++
			runtime.inBattle = false
			runtime.world = game.OverworldState{NativeMapID: 0x52, X: 4, Y: 4, Controllable: true}
			return game.BattleWon, nil
		},
		func() game.BattleKind { return runtime.battle.Kind },
		guaranteedWildFleeAttempts,
	)

	calls := 0
	res, err := runInterruptions(nil, 3, func() error {
		calls++
		if calls == 1 {
			return ErrBattle
		}
		return nil
	}, fakeGen2TravelResolvers(mem, runtime, resolve))
	if err != nil {
		t.Fatalf("runInterruptions: %v", err)
	}
	if fleeCalls != 1 || fightCalls != 1 {
		t.Fatalf("flee calls=%d fight calls=%d, want 1/1", fleeCalls, fightCalls)
	}
	if res.Flees != 0 || res.Battles != 1 {
		t.Fatalf("result = %+v, want one fought trainer interruption", res)
	}
	if len(res.Replans) != 1 || res.Replans[0] != (Replan{Map: 0x52, X: 4, Y: 4}) {
		t.Fatalf("replans = %+v, want post-fight world", res.Replans)
	}
}

func TestPortableTravelFleeMenuStallFallsBackToOwnedBattle(t *testing.T) {
	mem := fakeTravelMemory{}
	runtime := &fakeGen2TravelRuntime{
		world:    game.OverworldState{NativeMapID: 0x01, X: 19, Y: 9, InBattle: true},
		battle:   game.BattleState{Kind: game.BattleWild},
		inBattle: true,
	}

	fightCalls := 0
	resolve := fleeThenFightWith(
		func(int) error {
			return fmt.Errorf("RUN menu never became actionable: %w", ErrMenuStuck)
		},
		func() (game.BattleResult, error) {
			fightCalls++
			runtime.inBattle = false
			runtime.world = game.OverworldState{NativeMapID: 0x01, X: 19, Y: 9, Controllable: true}
			return game.BattleWon, nil
		},
		func() game.BattleKind { return runtime.battle.Kind },
		guaranteedWildFleeAttempts,
	)

	calls := 0
	res, err := runInterruptions(nil, 3, func() error {
		calls++
		if calls == 1 {
			return ErrBattle
		}
		return nil
	}, fakeGen2TravelResolvers(mem, runtime, resolve))
	if err != nil {
		t.Fatalf("runInterruptions: %v", err)
	}
	if fightCalls != 1 {
		t.Fatalf("fight called %d time(s), want 1 after RUN-menu stall", fightCalls)
	}
	if res.Flees != 0 || res.Battles != 1 {
		t.Fatalf("result = %+v, want one owned fallback fight", res)
	}
	if len(res.Replans) != 1 || res.Replans[0] != (Replan{Map: 0x01, X: 19, Y: 9}) {
		t.Fatalf("replans = %+v, want stable post-fight Viridian world", res.Replans)
	}
}

func TestPortableTravelFleeExhaustionFallsBackToFightAndKeepsTrainerKind(t *testing.T) {
	fightCalls := 0
	resolve := fleeThenFightWith(
		func(int) error { return fmt.Errorf("bounded RUN attempts spent: %w", ErrFleeExhausted) },
		func() (game.BattleResult, error) {
			fightCalls++
			return game.BattleLost, nil
		},
		func() game.BattleKind { return game.BattleTrainer },
		guaranteedWildFleeAttempts,
	)

	got, err := resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if fightCalls != 1 || got.outcome != game.BattleLost || !got.trainer || got.fled {
		t.Fatalf("resolution = %+v, fight calls=%d; want lost trainer fallback fight", got, fightCalls)
	}
}

func TestPortableTravelFleeSettlementRestartFallsBackToOwnedTrainerBattle(t *testing.T) {
	// A battle that restarts while settling (scripted back-to-back trainers
	// clear wIsInBattle between the two) leaves a live battle behind. The
	// journey owns that encounter and must hand it to the fight loop, not
	// fail closed into a dirty objective boundary (triage:f3a1128235845bdd).
	fightCalls := 0
	resolve := fleeThenFightWith(
		func(int) error { return errBattleRestarted },
		func() (game.BattleResult, error) {
			fightCalls++
			return game.BattleWon, nil
		},
		func() game.BattleKind { return game.BattleWild },
		guaranteedWildFleeAttempts,
	)

	got, err := resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if fightCalls != 1 || got.outcome != game.BattleWon || !got.trainer || got.fled {
		t.Fatalf("resolution = %+v, fight calls=%d; want the restarted battle fought as a trainer battle", got, fightCalls)
	}
}

func TestPortableTravelFleeUnexpectedErrorStillFailsClosed(t *testing.T) {
	fightCalls := 0
	want := errors.New("decoder capability missing")
	resolve := fleeThenFightWith(
		func(int) error { return want },
		func() (game.BattleResult, error) {
			fightCalls++
			return game.BattleWon, nil
		},
		func() game.BattleKind { return game.BattleWild },
		guaranteedWildFleeAttempts,
	)

	_, err := resolve()
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want original unexpected failure", err)
	}
	if fightCalls != 0 {
		t.Fatalf("fight called %d time(s), want fail-closed without fallback", fightCalls)
	}
}

func TestPortableTravelFakeGen2BattleLossUsesSemanticTrainerKind(t *testing.T) {
	mem := fakeTravelMemory{}
	runtime := &fakeGen2TravelRuntime{
		world:    game.OverworldState{NativeMapID: 0x63, X: 10, Y: 10, InBattle: true},
		battle:   game.BattleState{Kind: game.BattleTrainer},
		inBattle: true,
	}

	resolve := fightOnlyWithDecoder(mem, runtime, func() (game.BattleResult, error) {
		runtime.inBattle = false
		runtime.world = game.OverworldState{NativeMapID: 0x22, X: 5, Y: 6, Controllable: true}
		return game.BattleLost, nil
	})

	res, err := runInterruptions(nil, 3, func() error { return ErrBattle }, fakeGen2TravelResolvers(mem, runtime, resolve))
	if !errors.Is(err, ErrTrainerBlackedOut) || !errors.Is(err, ErrBlackedOut) {
		t.Fatalf("err = %v, want trainer blackout classification", err)
	}
	if !res.BlackedOut || !res.TrainerDefeat || res.Battles != 1 {
		t.Fatalf("result = %+v, want one trainer loss/blackout", res)
	}
	want := Replan{Map: 0x22, X: 5, Y: 6}
	if len(res.Replans) != 1 || res.Replans[0] != want {
		t.Fatalf("replans = %+v, want settled respawn %+v", res.Replans, want)
	}
}

func TestPortableTravelFakeGen2DialogueBlackoutUsesRuntimeSemantics(t *testing.T) {
	mem := fakeTravelMemory{}
	runtime := &fakeGen2TravelRuntime{
		world: game.OverworldState{NativeMapID: 0x71, X: 2, Y: 2, InDialogue: true},
		blackout: game.OverworldBlackoutState{
			BlackoutInProgress: true,
			PartyAllFainted:    true,
			RespawnNativeMapID: 0x10,
		},
	}
	resolve := func() (battleResolution, error) {
		return battleResolution{}, errors.New("unexpected battle resolution")
	}
	res, err := runInterruptions(nil, 3, func() error {
		return ErrDialogueInterrupted
	}, fakeGen2TravelResolvers(mem, runtime, resolve))
	if !errors.Is(err, ErrBlackedOut) {
		t.Fatalf("err = %v, want ErrBlackedOut", err)
	}
	if !res.BlackedOut || res.Battles != 0 || res.Flees != 0 {
		t.Fatalf("result = %+v, want out-of-battle blackout", res)
	}
}
