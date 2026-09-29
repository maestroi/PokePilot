package profile

import (
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	testBaseDataOffset = 0x120
	testBaseDataSize   = 32
	testTMHMOffset     = 24
)

func fieldMoveTestROM() []byte {
	romData := make([]byte, testBaseDataOffset+251*testBaseDataSize+0x20)
	copy(romData[testBaseDataOffset:], []byte{0x01, 45, 49, 49, 45, 65, 65})
	copy(romData[testBaseDataOffset+testBaseDataSize:], []byte{0x02, 60, 62, 63, 60, 80, 80})
	copy(romData[testBaseDataOffset+2*testBaseDataSize:], []byte{0x03, 80, 82, 83, 80, 100, 100})
	return romData
}

func allowTestMachine(romData []byte, species uint8, machine int) {
	off := testBaseDataOffset + (int(species)-1)*testBaseDataSize + testTMHMOffset
	bit := machine - 1
	romData[off+bit/8] |= 1 << uint(bit%8)
}

func TestGen2NativeFieldMoveMappings(t *testing.T) {
	want := map[game.FieldMoveID]game.NativeFieldMove{
		game.FieldMoveCut:       {MachineItemID: 0xf3, MoveID: 15},
		game.FieldMoveFly:       {MachineItemID: 0xf4, MoveID: 19},
		game.FieldMoveSurf:      {MachineItemID: 0xf5, MoveID: 57},
		game.FieldMoveStrength:  {MachineItemID: 0xf6, MoveID: 70},
		game.FieldMoveFlash:     {MachineItemID: 0xf7, MoveID: 148},
		game.FieldMoveWhirlpool: {MachineItemID: 0xf8, MoveID: 250},
		game.FieldMoveWaterfall: {MachineItemID: 0xf9, MoveID: 127},
		game.FieldMoveHeadbutt:  {MachineItemID: 0xc0, MoveID: 29},
	}
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		for id, expected := range want {
			got, ok := p.NativeFieldMove(id)
			if !ok {
				t.Fatalf("%s: %s unsupported", p.ID(), id)
			}
			if got != expected {
				t.Fatalf("%s: %s mapping=%+v, want %+v", p.ID(), id, got, expected)
			}
		}
	}
}

func TestGen2CutCapabilityUsesBadgeMachineAndROMCompatibility(t *testing.T) {
	romData := fieldMoveTestROM()
	allowTestMachine(romData, 1, 51) // Bulbasaur can carry HM01 in this fixture.

	var mem fakeMemory
	mem[sym.JohtoBadges] = 1 << 1
	mem[sym.TMsHMs+50] = 1
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1] = 1

	for _, p := range []*Profile{NewGold(), NewSilver()} {
		capability, supported, err := p.DecodeFieldMoveCapability(&mem, romData, game.FieldMoveCut)
		if err != nil {
			t.Fatalf("%s: DecodeFieldMoveCapability(Cut): %v", p.ID(), err)
		}
		if !supported {
			t.Fatalf("%s: Cut reported unsupported", p.ID())
		}
		if capability.BadgeRequired != "Hive" || !capability.BadgeOwned || !capability.MachineOwned {
			t.Fatalf("%s: Cut prerequisites=%+v", p.ID(), capability)
		}
		if capability.Learned || capability.Usable || !capability.Preparable {
			t.Fatalf("%s: unlearned Cut capability=%+v", p.ID(), capability)
		}
		if !reflect.DeepEqual(capability.CompatiblePartySlots, []int{0}) {
			t.Fatalf("%s: Cut carriers=%v, want [0]", p.ID(), capability.CompatiblePartySlots)
		}
	}

	mem[sym.PartyMon1+2] = 15
	capability, _, err := NewGold().DecodeFieldMoveCapability(&mem, nil, game.FieldMoveCut)
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Learned || !capability.Usable || capability.PartySlot != 0 {
		t.Fatalf("learned Cut capability=%+v", capability)
	}
}

func TestGen2FieldMoveBadgeRequirements(t *testing.T) {
	tests := []struct {
		id    game.FieldMoveID
		badge string
		mask  uint8
	}{
		{game.FieldMoveCut, "Hive", 1 << 1},
		{game.FieldMoveFly, "Storm", 1 << 5},
		{game.FieldMoveSurf, "Fog", 1 << 3},
		{game.FieldMoveStrength, "Plain", 1 << 2},
		{game.FieldMoveFlash, "Zephyr", 1 << 0},
		{game.FieldMoveWhirlpool, "Glacier", 1 << 6},
		{game.FieldMoveWaterfall, "Rising", 1 << 7},
	}
	for _, tc := range tests {
		t.Run(string(tc.id), func(t *testing.T) {
			var mem fakeMemory
			p := NewGold()
			capability, supported, err := p.DecodeFieldMoveCapability(&mem, nil, tc.id)
			if err != nil || !supported {
				t.Fatalf("missing badge decode: supported=%v err=%v", supported, err)
			}
			if capability.BadgeRequired != tc.badge || capability.BadgeOwned {
				t.Fatalf("without badge: %+v", capability)
			}
			mem[sym.JohtoBadges] = tc.mask
			capability, _, err = p.DecodeFieldMoveCapability(&mem, nil, tc.id)
			if err != nil {
				t.Fatal(err)
			}
			if !capability.BadgeOwned {
				t.Fatalf("%s badge bit %#02x not recognized", tc.badge, tc.mask)
			}
		})
	}
}

func TestGen2HeadbuttIsBadgeFreeButStillNeedsTM02(t *testing.T) {
	romData := fieldMoveTestROM()
	allowTestMachine(romData, 1, 2)

	var mem fakeMemory
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1] = 1
	mem[sym.TMsHMs+1] = 1

	capability, supported, err := NewGold().DecodeFieldMoveCapability(&mem, romData, game.FieldMoveHeadbutt)
	if err != nil || !supported {
		t.Fatalf("Headbutt: supported=%v err=%v", supported, err)
	}
	if capability.BadgeRequired != "" || !capability.BadgeOwned || !capability.MachineOwned || !capability.Preparable {
		t.Fatalf("Headbutt capability=%+v", capability)
	}

	for i, move := range []uint8{15, 19, 57, 70} {
		mem[sym.PartyMon1+2+uint16(i)] = move
	}
	capability, _, err = NewGold().DecodeFieldMoveCapability(&mem, romData, game.FieldMoveHeadbutt)
	if err != nil {
		t.Fatal(err)
	}
	if capability.Preparable || len(capability.CompatiblePartySlots) != 0 {
		t.Fatalf("all-HM moveset should reject replacement: %+v", capability)
	}
}

func TestGen2FieldMoveMenuFollowsPartyMoveOrder(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 1
	mem[sym.CurPartyMon] = 0
	mem[sym.PartyMon1+2] = 29
	mem[sym.PartyMon1+3] = 15
	mem[sym.PartyMon1+4] = 33
	mem[sym.PartyMon1+5] = 57

	got := NewGold().DecodeFieldMoveMenu(&mem).Entries
	want := []game.FieldMoveID{game.FieldMoveHeadbutt, game.FieldMoveCut, game.FieldMoveSurf}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("field move menu=%v, want %v", got, want)
	}
}

func TestGen2FieldMoveProfileContract(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		if _, ok := any(p).(game.FieldMoveProfile); !ok {
			t.Fatalf("%s does not implement game.FieldMoveProfile", p.ID())
		}
	}
}
