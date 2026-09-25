package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

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
				t.Fatalf("captureNeedsBoxSwitch(%d, %d) = %t, want %t", tc.partyCount, tc.boxCount, got, tc.want)
			}
		})
	}
}

// TestCaptureStorageSkipsDepositWhenFieldRosterBlocksIt pins the Route 13 farm
// failure (fingerprint 580bb626 / field_roster_no_recovery): a full party that
// cannot deposit any member without dropping owned Cut/Surf must still be
// allowed to capture into a non-full box. Gen I sends the catch to the active
// box; forcing EnsurePartySlot first is what turned a valid wild catch into
// ErrFieldRosterNoRecovery.
func TestCaptureStorageSkipsDepositWhenFieldRosterBlocksIt(t *testing.T) {
	romData := fakeCoreFieldROM(t)
	cut, _ := FieldMoveSpecFor(FieldCut)
	const dittoSpecies = 0x4c
	// Valid species mapping, no HM compatibility bits — matching Gen I Ditto.
	romData[testPokedexOrderOffset+dittoSpecies-1] = 132

	party := state.PartyState{Count: gen1PartyCapacity, Mons: []state.Mon{
		{Species: 0x10, Level: 45, Moves: [4]uint8{33}},
		{Species: 0x11, Level: 13, Moves: [4]uint8{33}},
		{Species: 0x12, Level: 7, Moves: [4]uint8{33}},
		{Species: 0x13, Level: 8, Moves: [4]uint8{33}},
		{Species: 0x14, Level: 3, Moves: [4]uint8{33}},
		{Species: 0x15, Level: 14, Moves: [4]uint8{cut.MoveID}}, // sole Cut user; nobody Surf-compatible
	}}
	box := state.BoxState{Number: 0, Count: 0}
	required := []FieldMove{FieldCut, FieldSurf}
	incoming := state.Mon{Species: dittoSpecies}

	_, err := planPartySlot(romData, party, box, incoming, required)
	if !errors.Is(err, ErrFieldRosterNoRecovery) {
		t.Fatalf("deposit preflight err=%v, want ErrFieldRosterNoRecovery", err)
	}
	if captureNeedsBoxSwitch(party.Count, box.Count) {
		t.Fatalf("captureNeedsBoxSwitch(%d, %d)=true; wild catch must use box room without depositing", party.Count, box.Count)
	}
}
