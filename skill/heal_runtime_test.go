package skill

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestHealRuntimeUsesFakeGen2OverworldState(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldMap] = 0x58
	m.mem[fakeOverworldX] = 12
	m.mem[fakeOverworldY] = 7
	m.mem[fakeOverworldFlags] = fakeOverworldControllable | fakeOverworldIdle

	got, err := healRuntimeStateWithDecoder(m, fakeGen2OverworldDecoder{})
	if err != nil {
		t.Fatalf("heal runtime state: %v", err)
	}
	want := (healRuntimeState{Map: 0x58, X: 12, Y: 7, Controllable: true})
	if got != want {
		t.Fatalf("heal runtime state = %+v, want %+v", got, want)
	}
}

func TestHealRuntimeRejectsWideNativeMap(t *testing.T) {
	decoder := fixedOverworldDecoder{state: game.OverworldState{NativeMapID: 0x231, X: 2, Y: 3, Controllable: true}}
	_, err := healRuntimeStateWithDecoder(&fakeOverworldMachine{}, decoder)
	if err == nil || !strings.Contains(err.Error(), "exceeds current routing range") {
		t.Fatalf("error = %v, want routing-range error", err)
	}
}
