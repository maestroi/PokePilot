package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const gsFieldTestBaseData = 0x300

func gsFieldTestROM() []byte {
	const entrySize = 32
	rom := make([]byte, gsFieldTestBaseData+251*entrySize+0x100)
	for species := 1; species <= 251; species++ {
		rom[gsFieldTestBaseData+(species-1)*entrySize] = byte(species)
	}
	return rom
}

func gsFieldAllowMachine(rom []byte, species uint8, machineNumber int) {
	const (
		entrySize  = 32
		tmhmOffset = 24
	)
	flag := machineNumber - 1
	offset := gsFieldTestBaseData + (int(species)-1)*entrySize + tmhmOffset + flag/8
	rom[offset] |= 1 << uint(flag%8)
}

func TestGSFieldMoveNativeMappings(t *testing.T) {
	p := NewGold()
	tests := []struct {
		id      game.FieldMoveID
		item    uint16
		move    uint16
		badge   string
		machine int
	}{
		{game.FieldMoveCut, 0xf3, 0x0f, "Hive", 51},
		{game.FieldMoveFly, 0xf4, 0x13, "Storm", 52},
		{game.FieldMoveSurf, 0xf5, 0x39, "Fog", 53},
		{game.FieldMoveStrength, 0xf6, 0x46, "Plain", 54},
		{game.FieldMoveFlash, 0xf7, 0x94, "Zephyr", 55},
		{game.FieldMoveWhirlpool, 0xf8, 0xfa, "Glacier", 56},
		{game.FieldMoveWaterfall, 0xf9, 0x7f, "Rising", 57},
		{game.FieldMoveHeadbutt, 0xc0, 0x1d, "", 2},
	}
	for _, tc := range tests {
		t.Run(string(tc.id), func(t *testing.T) {
			native, ok := p.NativeFieldMove(tc.id)
			if !ok {
				t.Fatalf("NativeFieldMove(%s) unsupported", tc.id)
			}
			if native.MachineItemID != tc.item || native.MoveID != tc.move {
				t.Fatalf("NativeFieldMove(%s)=%+v, want item=%#x move=%#x", tc.id, native, tc.item, tc.move)
			}
			spec, ok := gsFieldMoveByID(tc.id)
			if !ok || spec.badgeName != tc.badge || spec.machineNumber != tc.machine {
				t.Fatalf("field spec %s=%+v ok=%v, want badge=%q machine=%d", tc.id, spec, ok, tc.badge, tc.machine)
			}
		})
	}
}

func TestGSFieldMoveBadgeMasksMatchEngineFlags(t *testing.T) {
	tests := map[game.FieldMoveID]byte{
		game.FieldMoveCut:       1 << 1, // ENGINE_HIVEBADGE
		game.FieldMoveFly:       1 << 5, // ENGINE_STORMBADGE
		game.FieldMoveSurf:      1 << 3, // ENGINE_FOGBADGE
		game.FieldMoveStrength:  1 << 2, // ENGINE_PLAINBADGE
		game.FieldMoveFlash:     1 << 0, // ENGINE_ZEPHYRBADGE
		game.FieldMoveWhirlpool: 1 << 6, // ENGINE_GLACIERBADGE
		game.FieldMoveWaterfall: 1 << 7, // ENGINE_RISINGBADGE
		game.FieldMoveHeadbutt:  0,
	}
	for id, want := range tests {
		spec, ok := gsFieldMoveByID(id)
		if !ok {
			t.Fatalf("%s missing field-move spec", id)
		}
		if spec.badgeMask != want {
			t.Fatalf("%s badge mask=%#02x, want %#02x", id, spec.badgeMask, want)
		}
	}
}

func TestGSFieldMoveCapabilityRecognizesLearnedCut(t *testing.T) {
	var mem fakeMemory
	mem[sym.JohtoBadges] = johtoBadgeHiveMask
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1] = 0x98 // Chikorita
	mem[sym.PartyMon1+gsPartyMovesOffset] = 0x0f
	mem[sym.TMsHMs+50] = 1 // HM01

	capability, supported, err := NewGold().DecodeFieldMoveCapability(&mem, nil, game.FieldMoveCut)
	if err != nil {
		t.Fatalf("DecodeFieldMoveCapability(Cut): %v", err)
	}
	if !supported {
		t.Fatal("Gold profile did not advertise Cut")
	}
	if capability.BadgeRequired != "Hive" || !capability.BadgeOwned || !capability.MachineOwned {
		t.Fatalf("Cut prerequisites=%+v", capability)
	}
	if !capability.Learned || capability.PartySlot != 0 || !capability.Usable || !capability.Preparable {
		t.Fatalf("Cut carrier=%+v, want learned/usable slot 0", capability)
	}
}

func TestGSFieldMoveCapabilityUsesROMCompatibilityForPreparation(t *testing.T) {
	rom := gsFieldTestROM()
	const chikorita = uint8(0x98)
	gsFieldAllowMachine(rom, chikorita, 51) // synthetic ROM says Chikorita can learn HM01

	var mem fakeMemory
	mem[sym.JohtoBadges] = johtoBadgeHiveMask
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1] = chikorita
	mem[sym.PartyMon1+gsPartyMovesOffset] = 0x21
	mem[sym.TMsHMs+50] = 1

	capability, supported, err := NewSilver().DecodeFieldMoveCapability(&mem, rom, game.FieldMoveCut)
	if err != nil {
		t.Fatalf("DecodeFieldMoveCapability(Cut): %v", err)
	}
	if !supported || capability.Learned || capability.Usable {
		t.Fatalf("unlearned Cut capability=%+v supported=%v", capability, supported)
	}
	if !capability.BadgeOwned || !capability.MachineOwned || !capability.Preparable {
		t.Fatalf("preparable Cut capability=%+v", capability)
	}
	if len(capability.CompatiblePartySlots) != 1 || capability.CompatiblePartySlots[0] != 0 {
		t.Fatalf("Cut compatible slots=%v, want [0]", capability.CompatiblePartySlots)
	}
}

func TestGSFieldMoveCapabilityKeepsBadgeGateAndMachinePocketSeparate(t *testing.T) {
	rom := gsFieldTestROM()
	const chikorita = uint8(0x98)
	gsFieldAllowMachine(rom, chikorita, 51)

	var mem fakeMemory
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1] = chikorita
	mem[sym.TMsHMs+50] = 1

	capability, _, err := NewGold().DecodeFieldMoveCapability(&mem, rom, game.FieldMoveCut)
	if err != nil {
		t.Fatalf("DecodeFieldMoveCapability(Cut): %v", err)
	}
	if capability.BadgeOwned || !capability.MachineOwned || capability.Preparable || capability.Usable {
		t.Fatalf("badge-gated Cut capability=%+v", capability)
	}
}

func TestGSFieldMoveCapabilityTreatsHeadbuttAsBadgeFreeTM(t *testing.T) {
	rom := gsFieldTestROM()
	const sentret = uint8(0xa1)
	gsFieldAllowMachine(rom, sentret, 2)

	var mem fakeMemory
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1] = sentret
	mem[sym.TMsHMs+1] = 1 // TM02

	capability, supported, err := NewGold().DecodeFieldMoveCapability(&mem, rom, game.FieldMoveHeadbutt)
	if err != nil {
		t.Fatalf("DecodeFieldMoveCapability(Headbutt): %v", err)
	}
	if !supported || capability.BadgeRequired != "" || !capability.BadgeOwned {
		t.Fatalf("Headbutt badge semantics=%+v supported=%v", capability, supported)
	}
	if !capability.MachineOwned || !capability.Preparable || capability.Usable {
		t.Fatalf("Headbutt preparation=%+v", capability)
	}
}

func TestGSFieldMoveCapabilityDoesNotReplaceFourHMs(t *testing.T) {
	rom := gsFieldTestROM()
	const species = uint8(0x98)
	gsFieldAllowMachine(rom, species, 2)

	var mem fakeMemory
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1] = species
	mem[sym.PartyMon1+gsPartyMovesOffset+0] = 0x0f
	mem[sym.PartyMon1+gsPartyMovesOffset+1] = 0x13
	mem[sym.PartyMon1+gsPartyMovesOffset+2] = 0x39
	mem[sym.PartyMon1+gsPartyMovesOffset+3] = 0x46
	mem[sym.TMsHMs+1] = 1

	capability, _, err := NewGold().DecodeFieldMoveCapability(&mem, rom, game.FieldMoveHeadbutt)
	if err != nil {
		t.Fatalf("DecodeFieldMoveCapability(Headbutt): %v", err)
	}
	if capability.Preparable || len(capability.CompatiblePartySlots) != 0 {
		t.Fatalf("four-HM carrier was considered replaceable: %+v", capability)
	}
}

func TestGSFieldMoveMenuFailsClosedUntilNativeMenuDecoderLands(t *testing.T) {
	menu := NewGold().DecodeFieldMoveMenu(&fakeMemory{})
	if len(menu.Entries) != 0 {
		t.Fatalf("field move menu=%v, want fail-closed empty projection", menu.Entries)
	}
}

func TestGoldFieldMoveMenuPreservesUnknownNativeRows(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 1
	mem[sym.CurPartyMon] = 0
	base := sym.PartyMon1 + gsPartyMovesOffset
	mem[base+0] = 0x5b // DIG: native field move, intentionally not portable here.
	mem[base+1] = 0x0f // CUT
	mem[base+2] = 0x1d // HEADBUTT
	mem[sym.TwoDMenuNumRows] = 7 // DIG, CUT, HEADBUTT, STATS, SWITCH, ITEM, CANCEL
	mem[sym.TwoDMenuNumCols] = 1
	mem[sym.MenuJoypadFilter] = gen2PadA | gen2PadB
	mem[sym.MenuCursorY] = 2
	mem[sym.MenuCursorX] = 1
	putGSText(&mem, "DIG CUT HEADBUTT STATS SWITCH ITEM CANCEL")

	got := NewGold().DecodeFieldMoveMenu(&mem)
	if len(got.Entries) != 7 {
		t.Fatalf("entries = %#v, want seven live menu rows", got.Entries)
	}
	if got.Entries[0] != "" || got.Entries[1] != game.FieldMoveCut || got.Entries[2] != game.FieldMoveHeadbutt {
		t.Fatalf("field rows = %#v, want unknown/Cut/Headbutt", got.Entries[:3])
	}
}

func TestGoldFieldMoveMenuFailsClosedWithoutRenderedMove(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 1
	mem[sym.CurPartyMon] = 0
	mem[sym.PartyMon1+gsPartyMovesOffset] = 0x0f
	mem[sym.TwoDMenuNumRows] = 5
	mem[sym.TwoDMenuNumCols] = 1
	mem[sym.MenuJoypadFilter] = gen2PadA | gen2PadB
	mem[sym.MenuCursorY] = 1
	mem[sym.MenuCursorX] = 1
	putGSText(&mem, "STATS SWITCH MOVE ITEM CANCEL")

	if got := NewGold().DecodeFieldMoveMenu(&mem); len(got.Entries) != 0 {
		t.Fatalf("stale/non-field submenu decoded as field menu: %#v", got.Entries)
	}
}
