package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	agentMovesOffset             = 0x0E * 0x4000
	agentTechnicalMachinesOffset = 0x13773
	agentPokedexOrderOffset      = 0x41024
	agentBaseStatsOffset         = 0x383DE
	agentBaseStatsEntryLen       = 28
	agentBaseStatsTMHMOffset     = 20
)

func plannerTMHMROM(t *testing.T, item, machineMove, species, dex uint8, moves ...rom.Move) []byte {
	t.Helper()
	romData := make([]byte, agentPokedexOrderOffset+190)
	for _, mv := range moves {
		off := agentMovesOffset + (int(mv.ID)-1)*6
		accuracy := mv.Accuracy
		if accuracy == 0 {
			accuracy = 255
		}
		pp := mv.PP
		if pp == 0 {
			pp = 20
		}
		romData[off] = mv.ID
		romData[off+1] = mv.Effect
		romData[off+2] = mv.Power
		romData[off+3] = mv.Type
		romData[off+4] = accuracy
		romData[off+5] = pp
	}
	number, _, err := rom.MachineNumber(item)
	if err != nil {
		t.Fatal(err)
	}
	romData[agentTechnicalMachinesOffset+number-1] = machineMove
	romData[agentPokedexOrderOffset+int(species)-1] = dex
	flag := number - 1
	compat := agentBaseStatsOffset + (int(dex)-1)*agentBaseStatsEntryLen + agentBaseStatsTMHMOffset + flag/8
	romData[compat] |= 1 << uint(flag%8)
	return romData
}

func TestAppendTMHMObjectivesOffersMaterialTMUpgrade(t *testing.T) {
	const (
		typeNormal   = 0x00
		typeGrass    = 0x16
		typeElectric = 0x17
	)
	tackle := rom.Move{ID: 33, Power: 35, Type: typeNormal, Accuracy: 255, PP: 35}
	growl := rom.Move{ID: 45, Effect: rom.AttackDown1Effect, Accuracy: 255, PP: 40}
	leechSeed := rom.Move{ID: 73, Effect: 0x54, Accuracy: 229, PP: 10}
	vineWhip := rom.Move{ID: 22, Power: 35, Type: typeGrass, Accuracy: 255, PP: 10}
	thunderbolt := rom.Move{ID: 85, Power: 95, Type: typeElectric, Accuracy: 255, PP: 15}
	romData := plannerTMHMROM(t, rom.TM01Item, thunderbolt.ID, 0x99, 1, tackle, growl, leechSeed, vineWhip, thunderbolt)
	party := state.PartyState{Count: 1, Mons: []state.Mon{{
		Species: 0x99, Type1: typeGrass, Type2: 0x03,
		Moves: [4]uint8{tackle.ID, growl.ID, leechSeed.ID, vineWhip.ID},
	}}}
	inv := state.InventoryState{Items: []state.BagItem{{ID: rom.TM01Item, Quantity: 1}}}

	got := appendTMHMObjectives(romData, party, inv, nil)
	if len(got) != 1 {
		t.Fatalf("offered %d machine objectives, want 1: %+v", len(got), got)
	}
	o := got[0]
	if o.Kind != KindUseItem || o.Item != rom.TM01Item || o.Slot != 0 {
		t.Fatalf("machine objective = %+v, want KindUseItem TM01 party slot 0", o)
	}
	for _, want := range []string{"TM01", "consumable/finite", "score", "replace move slot"} {
		if !strings.Contains(o.Note, want) {
			t.Errorf("machine note %q does not contain %q", o.Note, want)
		}
	}
}

func TestAppendTMHMObjectivesDoesNotOfferBadConsumableTM(t *testing.T) {
	const typeNormal = 0x00
	tackle := rom.Move{ID: 33, Power: 35, Type: typeNormal, Accuracy: 255, PP: 35}
	bodySlam := rom.Move{ID: 34, Power: 85, Type: typeNormal, Accuracy: 255, PP: 15}
	splash := rom.Move{ID: 150, Effect: 0x55, Accuracy: 255, PP: 40}
	romData := plannerTMHMROM(t, rom.TM01Item, splash.ID, 0x99, 1, tackle, bodySlam, splash)
	party := state.PartyState{Count: 1, Mons: []state.Mon{{
		Species: 0x99, Type1: typeNormal, Type2: typeNormal,
		Moves: [4]uint8{tackle.ID, bodySlam.ID},
	}}}
	inv := state.InventoryState{Items: []state.BagItem{{ID: rom.TM01Item, Quantity: 1}}}

	if got := appendTMHMObjectives(romData, party, inv, nil); len(got) != 0 {
		t.Fatalf("bad consumable TM was offered: %+v", got)
	}
}

func TestAppendTMHMObjectivesExplainsHMPermanence(t *testing.T) {
	const typeNormal = 0x00
	tackle := rom.Move{ID: 33, Power: 35, Type: typeNormal, Accuracy: 255, PP: 35}
	strength := rom.Move{ID: 70, Power: 80, Type: typeNormal, Accuracy: 255, PP: 15}
	romData := plannerTMHMROM(t, rom.HM01Item+3, strength.ID, 0x99, 1, tackle, strength)
	party := state.PartyState{Count: 1, Mons: []state.Mon{{
		Species: 0x99, Type1: typeNormal, Type2: typeNormal,
		Moves: [4]uint8{tackle.ID},
	}}}
	inv := state.InventoryState{Items: []state.BagItem{{ID: rom.HM01Item + 3, Quantity: 1}}}

	got := appendTMHMObjectives(romData, party, inv, nil)
	if len(got) != 1 {
		t.Fatalf("offered %d HM objectives, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Note, "HM04") || !strings.Contains(got[0].Note, "cannot be forgotten") {
		t.Fatalf("HM permanence missing from note: %q", got[0].Note)
	}
}
