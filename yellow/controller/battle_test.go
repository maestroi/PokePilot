package controller

import "testing"

func TestBattlePhaseForYellowMenus(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		max    uint8
		forced bool
		want   battlePhase
	}{
		{name: "main", text: "FIGHT  PKMN  ITEM  RUN", want: battlePhaseMainMenu},
		{name: "moves", text: "THUNDERSHOCK TYPE/ELECTRIC", want: battlePhaseMoveMenu},
		{name: "use next", text: "Use next POKEMON?", max: 1, want: battlePhaseUseNext},
		{name: "trainer switch", text: "Will RED change POKEMON?", max: 1, want: battlePhaseTrainerSwitch},
		{name: "safari", text: "BALLx 30 BAIT THROW ROCK RUN", want: battlePhaseSafariMenu},
		{name: "learn", text: "PIKACHU is trying to learn QUICK ATTACK", want: battlePhaseLearnMove},
		{name: "abandon", text: "Abandon learning QUICK ATTACK?", max: 1, want: battlePhaseAbandonLearning},
		{name: "forced party", text: "Choose a POKEMON.", forced: true, want: battlePhaseForcedParty},
		{name: "unknown choice fails closed", text: "YES NO", max: 1, want: battlePhaseUnknownChoice},
		{name: "ordinary text", text: "Enemy PIDGEY used GUST!", want: battlePhaseText},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := battlePhaseFor(tc.text, tc.max, tc.forced); got != tc.want {
				t.Fatalf("battlePhaseFor(%q, max=%d, forced=%v) = %d, want %d",
					tc.text, tc.max, tc.forced, got, tc.want)
			}
		})
	}
}

func TestBattlePhaseForcedPartyRequiresChooseScreen(t *testing.T) {
	if got := battlePhaseFor("Enemy attack text", 0, true); got != battlePhaseText {
		t.Fatalf("forced flag without party screen = %d, want text phase", got)
	}
}
