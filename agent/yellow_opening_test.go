package agent

import (
	"errors"
	"testing"

	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestYellowOpeningPhaseWaitsForOakChooseSpeech(t *testing.T) {
	tests := []struct {
		name  string
		facts yellowprofile.OpeningFacts
		want  yellowOpeningPhase
		err   error
	}{
		{
			name:  "fresh bedroom routes toward gate",
			facts: yellowprofile.OpeningFacts{Map: 0x26, Controllable: true},
			want:  yellowOpeningWalkToGate,
		},
		{
			name:  "Oak gate script owns input",
			facts: yellowprofile.OpeningFacts{Map: yellowprofile.PalletTownMap, OakAppeared: true},
			want:  yellowOpeningScript,
		},
		{
			name: "transient lab control before choose speech still waits",
			facts: yellowprofile.OpeningFacts{
				Map: yellowprofile.OaksLabMap, Controllable: true,
				OakAppeared: true, FollowedOak: true, OakAskedToChoose: false,
			},
			want: yellowOpeningScript,
		},
		{
			name: "ball only after choose speech",
			facts: yellowprofile.OpeningFacts{
				Map: yellowprofile.OaksLabMap, Controllable: true,
				OakAppeared: true, FollowedOak: true, OakAskedToChoose: true,
			},
			want: yellowOpeningTakeBall,
		},
		{
			name:  "scripted Oak capture battle is not player battle",
			facts: yellowprofile.OpeningFacts{Map: yellowprofile.PalletTownMap, InBattle: true},
			want:  yellowOpeningScript,
		},
		{
			name: "lab rival battle is player owned",
			facts: yellowprofile.OpeningFacts{
				Map: yellowprofile.OaksLabMap, InBattle: true, GotStarter: true, PartyCount: 1,
			},
			want: yellowOpeningFightRival,
		},
		{
			name: "Pikachu received walks to rival",
			facts: yellowprofile.OpeningFacts{
				Map: yellowprofile.OaksLabMap, Controllable: true, GotStarter: true, PartyCount: 1,
				OakAppeared: true, FollowedOak: true, OakAskedToChoose: true,
			},
			want: yellowOpeningWalkToRival,
		},
		{
			name: "completed opening",
			facts: yellowprofile.OpeningFacts{
				Map: yellowprofile.OaksLabMap, Controllable: true, GotStarter: true, PartyCount: 1, BattledRival: true,
			},
			want: yellowOpeningDone,
		},
		{
			name:  "unexpected choice fails closed",
			facts: yellowprofile.OpeningFacts{ChoicePrompt: true},
			err:   errYellowOpeningChoiceRequired,
		},
		{
			name: "starter outside lab fails closed",
			facts: yellowprofile.OpeningFacts{
				Map: yellowprofile.PalletTownMap, Controllable: true, GotStarter: true, PartyCount: 1,
				OakAppeared: true, FollowedOak: true, OakAskedToChoose: true,
			},
			err: errYellowOpeningUnexpectedState,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := yellowOpeningPhaseFor(tc.facts)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("yellowOpeningPhaseFor(%+v) error=%v, want %v", tc.facts, err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("yellowOpeningPhaseFor(%+v): %v", tc.facts, err)
			}
			if got != tc.want {
				t.Fatalf("yellowOpeningPhaseFor(%+v)=%q, want %q", tc.facts, got, tc.want)
			}
		})
	}
}

func TestYellowOpeningReachedRequiresStableStarterAndRivalFacts(t *testing.T) {
	if yellowOpeningReached(yellowprofile.OpeningFacts{GotStarter: true, BattledRival: true, Controllable: true}) {
		t.Fatal("opening completed without a party member")
	}
	if yellowOpeningReached(yellowprofile.OpeningFacts{GotStarter: true, PartyCount: 1, BattledRival: true, InBattle: true}) {
		t.Fatal("opening completed during battle")
	}
	if !yellowOpeningReached(yellowprofile.OpeningFacts{GotStarter: true, PartyCount: 1, BattledRival: true, Controllable: true}) {
		t.Fatal("stable completed opening was not recognized")
	}
}

func TestYellowOpeningOakCaptureRemainsScriptOwned(t *testing.T) {
	facts := yellowprofile.OpeningFacts{
		Map:          yellowprofile.PalletTownMap,
		InBattle:     true,
		TextOpen:     true,
		OakAppeared:  true,
		Controllable: false,
	}
	phase, err := yellowOpeningPhaseFor(facts)
	if err != nil {
		t.Fatal(err)
	}
	if phase != yellowOpeningScript {
		t.Fatalf("phase = %q, want %q for Oak's simulated Pikachu capture", phase, yellowOpeningScript)
	}
}
