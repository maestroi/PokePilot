package controller

import "testing"

func TestOpeningPhaseForResumableStates(t *testing.T) {
	tests := []struct {
		name     string
		state    openingState
		nickname bool
		want     openingPhase
	}{
		{
			name: "completed boundary requires durable starter evidence",
			state: openingState{
				mapID: mapOaksLab, controllable: true, partyCount: 1,
				hasPikachu: true, starter: true, labRival: true,
			},
			want: openingPhaseDone,
		},
		{
			name: "rival flag cannot hide missing Pikachu",
			state: openingState{
				mapID: mapOaksLab, controllable: true, partyCount: 1,
				starter: true, labRival: true,
			},
			want: openingPhaseUnexpected,
		},
		{
			name:  "resume inside battle",
			state: openingState{mapID: mapOaksLab, inBattle: true},
			want:  openingPhaseBattle,
		},
		{
			name:     "resume at nickname choice",
			state:    openingState{mapID: mapOaksLab, controllable: true, partyCount: 1},
			nickname: true,
			want:     openingPhaseNickname,
		},
		{
			name:  "resume while script owns input",
			state: openingState{mapID: mapPalletTown},
			want:  openingPhaseScript,
		},
		{
			name:  "fresh bedroom upstairs",
			state: openingState{mapID: mapRedsHouse2F, controllable: true},
			want:  openingPhaseBedroomUpstairs,
		},
		{
			name:  "fresh bedroom downstairs",
			state: openingState{mapID: mapRedsHouse1F, controllable: true},
			want:  openingPhaseBedroomDownstairs,
		},
		{
			name:  "resume before Oak intercept",
			state: openingState{mapID: mapPalletTown, controllable: true},
			want:  openingPhaseOakGate,
		},
		{
			name:  "resume before Eevee ball interaction",
			state: openingState{mapID: mapOaksLab, controllable: true},
			want:  openingPhaseEeveeBall,
		},
		{
			name: "resume after Pikachu enters party before event settles",
			state: openingState{
				mapID: mapOaksLab, controllable: true, partyCount: 1, hasPikachu: true,
			},
			want: openingPhaseAwaitStarter,
		},
		{
			name: "resume after starter before rival trigger",
			state: openingState{
				mapID: mapOaksLab, controllable: true, partyCount: 1,
				hasPikachu: true, starter: true,
			},
			want: openingPhaseRivalTrigger,
		},
		{
			name: "starter outside lab fails closed",
			state: openingState{
				mapID: mapPalletTown, controllable: true, partyCount: 1,
				hasPikachu: true, starter: true,
			},
			want: openingPhaseUnexpected,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := openingPhaseFor(tc.state, tc.nickname); got != tc.want {
				t.Fatalf("openingPhaseFor(%+v, nickname=%v) = %q, want %q",
					tc.state, tc.nickname, got, tc.want)
			}
		})
	}
}
