package skill

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

func TestInteractionRuntimeUsesFakeGen2OverworldState(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldMap] = 0x61
	m.mem[fakeOverworldX] = 18
	m.mem[fakeOverworldY] = 9
	m.mem[fakeOverworldFlags] = fakeOverworldBattle

	got, err := interactionRuntimeStateWithDecoder(m, fakeGen2OverworldDecoder{})
	if err != nil {
		t.Fatalf("interaction runtime: %v", err)
	}
	want := (interactionRuntimeState{Map: 0x61, X: 18, Y: 9, InBattle: true})
	if got != want {
		t.Fatalf("interaction runtime = %+v, want %+v", got, want)
	}
}

func TestInteractionStepUsesSemanticPosition(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldMap] = 0x12
	m.mem[fakeOverworldX] = 7
	m.mem[fakeOverworldY] = 4

	step, live, err := interactionStepWithDecoder(m, fakeGen2OverworldDecoder{}, 7, 3)
	if err != nil {
		t.Fatalf("interaction step: %v", err)
	}
	if step != world.StepUp {
		t.Fatalf("step = %v, want %v", step, world.StepUp)
	}
	if live.Map != 0x12 || live.X != 7 || live.Y != 4 {
		t.Fatalf("live = %+v", live)
	}
}

func TestInteractionStepRejectsNonAdjacentTarget(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldX] = 7
	m.mem[fakeOverworldY] = 4

	_, _, err := interactionStepWithDecoder(m, fakeGen2OverworldDecoder{}, 9, 4)
	if err == nil || !strings.Contains(err.Error(), "not orthogonally adjacent") {
		t.Fatalf("error = %v, want adjacency error", err)
	}
}

func TestInteractionRuntimeRejectsWideNativeMap(t *testing.T) {
	decoder := fixedOverworldDecoder{state: game.OverworldState{NativeMapID: 0x321, X: 1, Y: 2}}
	_, err := interactionRuntimeStateWithDecoder(&fakeOverworldMachine{}, decoder)
	if err == nil || !strings.Contains(err.Error(), "exceeds current routing range") {
		t.Fatalf("error = %v, want routing-range error", err)
	}
}
