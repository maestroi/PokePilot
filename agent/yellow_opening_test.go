package agent

import "testing"

func TestYellowOpeningPhaseForResumableStates(t *testing.T) {
	tests := []struct {
		name     string
		state    yellowOpeningState
		nickname bool
		want     yellowOpeningPhase
	}{
		{name: "completed boundary", state: yellowOpeningState{mapID: yellowOpeningOaksLab, controllable: true, partyCount: 1, hasPikachu: true, starter: true, labRival: true}, want: yellowOpeningDone},
		{name: "rival flag cannot hide missing Pikachu", state: yellowOpeningState{mapID: yellowOpeningOaksLab, controllable: true, partyCount: 1, starter: true, labRival: true}, want: yellowOpeningUnexpected},
		{name: "scripted capture battle", state: yellowOpeningState{mapID: yellowOpeningOaksLab, inBattle: true}, want: yellowOpeningBattle},
		{name: "nickname choice", state: yellowOpeningState{mapID: yellowOpeningOaksLab, controllable: true, partyCount: 1}, nickname: true, want: yellowOpeningNickname},
		{name: "script owns input", state: yellowOpeningState{mapID: yellowOpeningPalletTown}, want: yellowOpeningScript},
		{name: "fresh bedroom upstairs", state: yellowOpeningState{mapID: yellowOpeningRedsHouse2F, controllable: true}, want: yellowOpeningBedroomUpstairs},
		{name: "fresh bedroom downstairs", state: yellowOpeningState{mapID: yellowOpeningRedsHouse1F, controllable: true}, want: yellowOpeningBedroomDownstairs},
		{name: "before Oak intercept", state: yellowOpeningState{mapID: yellowOpeningPalletTown, controllable: true}, want: yellowOpeningOakGate},
		{name: "before Eevee ball", state: yellowOpeningState{mapID: yellowOpeningOaksLab, controllable: true}, want: yellowOpeningEeveeBall},
		{name: "Pikachu in party before starter event settles", state: yellowOpeningState{mapID: yellowOpeningOaksLab, controllable: true, partyCount: 1, hasPikachu: true}, want: yellowOpeningAwaitStarter},
		{name: "starter before rival trigger", state: yellowOpeningState{mapID: yellowOpeningOaksLab, controllable: true, partyCount: 1, hasPikachu: true, starter: true}, want: yellowOpeningRivalTrigger},
		{name: "starter outside lab fails closed", state: yellowOpeningState{mapID: yellowOpeningPalletTown, controllable: true, partyCount: 1, hasPikachu: true, starter: true}, want: yellowOpeningUnexpected},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := yellowOpeningPhaseFor(tc.state, tc.nickname); got != tc.want {
				t.Fatalf("yellowOpeningPhaseFor(%+v, nickname=%v) = %q, want %q",
					tc.state, tc.nickname, got, tc.want)
			}
		})
	}
}

func TestYellowOpeningFacingButton(t *testing.T) {
	tests := []struct {
		x, y, tx, ty uint8
		ok           bool
	}{
		{5, 5, 5, 6, true},
		{5, 5, 5, 4, true},
		{5, 5, 6, 5, true},
		{5, 5, 4, 5, true},
		{5, 5, 7, 5, false},
	}
	for _, tc := range tests {
		if _, ok := yellowOpeningFacingButton(tc.x, tc.y, tc.tx, tc.ty); ok != tc.ok {
			t.Fatalf("facing (%d,%d)->(%d,%d) ok=%v, want %v", tc.x, tc.y, tc.tx, tc.ty, ok, tc.ok)
		}
	}
}
