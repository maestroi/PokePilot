package controller

import "testing"

func TestDialoguePhaseForFailsClosedOnChoices(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		maxMenu      uint8
		fontLoaded   uint8
		controllable bool
		want         dialoguePhase
	}{
		{name: "choice", text: "YES NO", maxMenu: 1, fontLoaded: 1, want: dialoguePhaseChoice},
		{name: "page text", text: "OAK: Come with me!", fontLoaded: 1, want: dialoguePhasePage},
		{name: "script wait", text: "", controllable: false, want: dialoguePhaseWait},
		{name: "stable boundary", text: "", controllable: true, want: dialoguePhaseDone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := dialoguePhaseFor(tc.text, tc.maxMenu, tc.fontLoaded, tc.controllable); got != tc.want {
				t.Fatalf("dialoguePhaseFor(%q, %d, %d, %v) = %d, want %d",
					tc.text, tc.maxMenu, tc.fontLoaded, tc.controllable, got, tc.want)
			}
		})
	}
}
