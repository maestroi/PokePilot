package skill

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestNavigationStateUsesFakeGen2OverworldDecoder(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldMap] = 0x7a
	m.mem[fakeOverworldX] = 33
	m.mem[fakeOverworldY] = 14
	m.mem[fakeOverworldFlags] = fakeOverworldIdle | fakeOverworldControllable

	got, err := navigationStateWithDecoder(m, fakeGen2OverworldDecoder{})
	if err != nil {
		t.Fatalf("navigation state: %v", err)
	}
	want := (navigationState{Map: 0x7a, X: 33, Y: 14})
	if got != want {
		t.Fatalf("navigation state = %+v, want %+v", got, want)
	}
}

func TestNavigationStateRejectsWideNativeMapInsteadOfTruncating(t *testing.T) {
	decoder := fixedOverworldDecoder{state: game.OverworldState{NativeMapID: 0x123, X: 4, Y: 5}}
	_, err := navigationStateWithDecoder(&fakeOverworldMachine{}, decoder)
	if err == nil || !strings.Contains(err.Error(), "exceeds current routing range") {
		t.Fatalf("error = %v, want routing-range error", err)
	}
}

func TestGoToBattleAbortUsesSemanticOverworldState(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldMap] = 0x32
	m.mem[fakeOverworldX] = 8
	m.mem[fakeOverworldY] = 19
	m.mem[fakeOverworldFlags] = fakeOverworldBattle

	err := abortIfBattleWithDecoder(m, fakeGen2OverworldDecoder{})
	if !errors.Is(err, ErrBattle) {
		t.Fatalf("error = %v, want ErrBattle", err)
	}
	if !strings.Contains(err.Error(), "map 32 at (8,19)") {
		t.Fatalf("diagnostic = %q, want semantic map/position", err)
	}
}

type fixedOverworldDecoder struct {
	state game.OverworldState
}

func (d fixedOverworldDecoder) DecodeOverworld(game.MemoryReader) game.OverworldState {
	return d.state
}
