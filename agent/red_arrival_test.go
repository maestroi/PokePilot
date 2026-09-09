package agent

import (
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"testing"
)

func TestRedOccupiedDestinationArrival(t *testing.T) {
	dest, ok := skill.Place("cerulean gym")
	if !ok {
		t.Fatal("missing destination")
	}
	adjacent := Observation{Map: dest.Map, X: dest.X + 1, Y: dest.Y, Controllable: true}
	occupied := []state.SpriteState{{X: int(dest.X), Y: int(dest.Y)}}
	for _, tc := range []struct {
		name    string
		obs     Observation
		sprites []state.SpriteState
		want    bool
	}{
		{"occupied adjacent", adjacent, occupied, true},
		{"empty destination", adjacent, nil, false},
		{"sprite elsewhere", adjacent, []state.SpriteState{{X: int(dest.X) + 2, Y: int(dest.Y)}}, false},
		{"battle", func() Observation { o := adjacent; o.InBattle = true; return o }(), occupied, false},
		{"uncontrollable", func() Observation { o := adjacent; o.Controllable = false; return o }(), occupied, false},
		{"wrong map", func() Observation { o := adjacent; o.Map++; return o }(), occupied, false},
		{"diagonal", func() Observation { o := adjacent; o.Y++; return o }(), occupied, false},
		{"too far", func() Observation { o := adjacent; o.X++; return o }(), occupied, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := redOccupiedDestinationArrival(tc.obs, dest, tc.sprites); got != tc.want {
				t.Fatalf("arrival=%v want %v", got, tc.want)
			}
		})
	}
}
