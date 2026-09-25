package skill

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

func TestFieldPathActionTargetUsesFakeGen2OverworldState(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldMap] = 0x52
	m.mem[fakeOverworldX] = 14
	m.mem[fakeOverworldY] = 9

	live, tx, ty, err := fieldPathActionTargetWithDecoder(m, fakeGen2OverworldDecoder{}, fieldPathStep{
		Move:   world.StepLeft,
		Action: fieldPathCut,
	})
	if err != nil {
		t.Fatalf("field action target: %v", err)
	}
	if live.Map != 0x52 || live.X != 14 || live.Y != 9 {
		t.Fatalf("live = %+v", live)
	}
	if tx != 13 || ty != 9 {
		t.Fatalf("target = (%d,%d), want (13,9)", tx, ty)
	}
}

func TestFieldPathActionTargetRejectsNonAdjacentStep(t *testing.T) {
	m := &fakeOverworldMachine{}
	_, _, _, err := fieldPathActionTargetWithDecoder(m, fakeGen2OverworldDecoder{}, fieldPathStep{
		Move: world.Step{DX: 2},
	})
	if err == nil || !strings.Contains(err.Error(), "non-adjacent") {
		t.Fatalf("error = %v, want non-adjacent error", err)
	}
}

func TestFieldPathRuntimeRejectsWideNativeMap(t *testing.T) {
	decoder := fixedOverworldDecoder{state: game.OverworldState{NativeMapID: 0x220, X: 4, Y: 5}}
	_, err := fieldPathRuntimeStateWithDecoder(&fakeOverworldMachine{}, decoder)
	if err == nil || !strings.Contains(err.Error(), "exceeds current routing range") {
		t.Fatalf("error = %v, want routing-range error", err)
	}
}

func TestSemanticPositionStabilityUsesFakeGen2Coordinates(t *testing.T) {
	m := &fakeTravelWorldMachine{changeAt: 3, mapAfter: 0x44, xAfter: 8, yAfter: 12}
	m.mem[fakeOverworldMap] = 0x44
	m.mem[fakeOverworldX] = 4
	m.mem[fakeOverworldY] = 4

	if err := waitForPositionStableWithDecoder(m, fakeGen2OverworldDecoder{}, 100, 5); err != nil {
		t.Fatalf("wait for semantic position stability: %v", err)
	}
	if m.frames < 3+5 {
		t.Fatalf("frames = %d, want movement plus stability window", m.frames)
	}
}
