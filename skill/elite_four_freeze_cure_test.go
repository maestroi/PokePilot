package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestLeagueFreezeCureShortfallCountsEveryFreezeCure(t *testing.T) {
	inv := func(items ...state.BagItem) state.InventoryState { return state.InventoryState{Items: items} }
	cases := []struct {
		name string
		inv  state.InventoryState
		want int
	}{
		{"no medicine", inv(), leagueFreezeCureReserve},
		{"hp items only", inv(state.BagItem{ID: itemMaxPotion, Quantity: 9}), leagueFreezeCureReserve},
		{"one ice heal", inv(state.BagItem{ID: itemIceHeal, Quantity: 1}), leagueFreezeCureReserve - 1},
		{"full restores cover it", inv(state.BagItem{ID: itemFullRestore, Quantity: 5}), 0},
		{"mixed cures", inv(state.BagItem{ID: itemFullHeal, Quantity: 1}, state.BagItem{ID: itemIceHeal, Quantity: 1}), 0},
	}
	for _, tc := range cases {
		if got := leagueFreezeCureShortfall(tc.inv); got != tc.want {
			t.Errorf("%s: shortfall = %d, want %d", tc.name, got, tc.want)
		}
	}
}
