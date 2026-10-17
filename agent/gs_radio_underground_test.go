package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSRadioWarehouseRetailOrderAndBoundaries(t *testing.T) {
	want := []struct {
		name      string
		x, standY uint8
	}{
		{"3", 2, 2}, {"2", 10, 2}, {"1", 16, 2},
	}
	if len(gsRadioWarehouseSwitchOrder) != len(want) {
		t.Fatalf("puzzle steps = %d", len(gsRadioWarehouseSwitchOrder))
	}
	s := gsprofile.RadioTowerSwitches{}
	for i, step := range gsRadioWarehouseSwitchOrder {
		if step.name != want[i].name || step.x != want[i].x ||
			step.y != 1 || step.standX != step.x || step.standY != want[i].standY {
			t.Fatalf("switch %d wrong retail event position: %+v", i, step)
		}
		if step.enabled(s) {
			t.Fatalf("step %s was on before its turn", step.name)
		}
		switch i {
		case 0:
			s.Switch3 = true
		case 1:
			s.Switch2 = true
		case 2:
			s.Switch1 = true
		}
		if !step.enabled(s) {
			t.Fatalf("step %s did not reflect observed switch state", step.name)
		}
	}
	if s.FinalDoorOpen {
		t.Fatal("test switch inputs must not fabricate final-door event")
	}
}

func TestGSRadioUndergroundOwnedWithoutUnlockingBadgeEight(t *testing.T) {
	for _, name := range []string{"GOLDENROD_UNDERGROUND", "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES",
		"GOLDENROD_UNDERGROUND_WAREHOUSE", "RADIO_TOWER_3F"} {
		id, err := gsOpeningMapID(name)
		if err != nil || !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("owned Radio Tower map %s: %v", name, err)
		}
	}
	for _, progress := range []ProgressID{
		gsprofile.ProgressRadioTowerFakeDirectorBeaten,
		gsprofile.ProgressRadioTowerBasementUnlocked,
		gsprofile.ProgressRadioTowerCardKeyReceived,
		gsprofile.ProgressRadioTowerShutterOpened,
		gsprofile.ProgressRadioTowerCleared,
		gsprofile.ProgressRisingBadgeEarned,
	} {
		if gsSupportedProgress(progress) {
			t.Fatalf("unqualified Gen2 milestone offered to production: %s", progress)
		}
	}
}
