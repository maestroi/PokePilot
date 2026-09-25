package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

const (
	fakeEscapeVisible uint16 = 60 + iota
	fakeEscapeKind
)

type fakeGen2BattleEscapeDecoder struct{}

func (fakeGen2BattleEscapeDecoder) DecodeBattleEscapeMenu(r game.MemoryReader) game.BattleEscapeMenuState {
	if r.Peek8(fakeEscapeVisible) == 0 {
		return game.BattleEscapeMenuState{}
	}
	kind := game.BattleEscapeMenuOrdinary
	if r.Peek8(fakeEscapeKind) != 0 {
		kind = game.BattleEscapeMenuSafari
	}
	return game.BattleEscapeMenuState{
		Visible: true,
		Kind:    kind,
		Cursor: game.BattleMenuPosition{
			Column: int(r.Peek8(fakeBattleColumn)),
			Row:    int(r.Peek8(fakeBattleRow)),
		},
	}
}

func (fakeGen2BattleEscapeDecoder) BattleEscapeRunPosition(kind game.BattleEscapeMenuKind) (game.BattleMenuPosition, bool) {
	switch kind {
	case game.BattleEscapeMenuOrdinary:
		// Deliberately unlike Gen I to prove the driver follows the profile.
		return game.BattleMenuPosition{Column: 2, Row: 2}, true
	case game.BattleEscapeMenuSafari:
		return game.BattleMenuPosition{Column: 0, Row: 2}, true
	default:
		return game.BattleMenuPosition{}, false
	}
}

func TestGenericBattleEscapeMenuUsesProfileLayout(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind byte
	}{
		{name: "ordinary", kind: 0},
		{name: "alternate", kind: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &fakeBattleMenuMachine{}
			m.mem[fakeEscapeVisible] = 1
			m.mem[fakeEscapeKind] = tc.kind
			m.mem[fakeBattleColumn] = 1
			m.mem[fakeBattleRow] = 0

			decoder := fakeGen2BattleEscapeDecoder{}
			if err := selectBattleEscapeRunWithDecoder(m, decoder); err != nil {
				t.Fatalf("select RUN: %v", err)
			}
			live := decoder.DecodeBattleEscapeMenu(m)
			want, _ := decoder.BattleEscapeRunPosition(live.Kind)
			if live.Cursor != want {
				t.Fatalf("cursor = %+v, want %+v", live.Cursor, want)
			}
		})
	}
}

func TestGenericBattleEscapeMenuRequiresVisibleMenu(t *testing.T) {
	m := &fakeBattleMenuMachine{}
	if err := selectBattleEscapeRunWithDecoder(m, fakeGen2BattleEscapeDecoder{}); err == nil {
		t.Fatal("hidden escape menu was accepted")
	}
}
