package skill

import "testing"

func TestCaptureNeedsBoxSwitchOnlyWhenFullPartyWouldOverflowFullBox(t *testing.T) {
	for _, tc := range []struct {
		name       string
		partyCount uint8
		boxCount   uint8
		want       bool
	}{
		{name: "open party ignores full box", partyCount: 5, boxCount: gen1BoxCapacity, want: false},
		{name: "full party with box room", partyCount: gen1PartyCapacity, boxCount: gen1BoxCapacity - 1, want: false},
		{name: "full party and full box", partyCount: gen1PartyCapacity, boxCount: gen1BoxCapacity, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := captureNeedsBoxSwitch(tc.partyCount, tc.boxCount); got != tc.want {
				t.Fatalf("captureNeedsBoxSwitch(%d, %d, redWram()) = %t, want %t", tc.partyCount, tc.boxCount, got, tc.want)
			}
		})
	}
}
