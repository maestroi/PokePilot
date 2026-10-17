package profile

import "testing"

func TestGSRadioTowerSwitchesRemainTransientAndIndependent(t *testing.T) {
	constants := []uint16{
		eventUndergroundSwitch1, eventUndergroundSwitch2, eventUndergroundSwitch3,
		eventUndergroundEmergencySwitch, eventUndergroundFinalDoorOpen,
	}
	for i, want := range []uint16{723, 724, 725, 726, 737} {
		if constants[i] != want {
			t.Fatalf("retail switch event index %d = %d, want %d", i, constants[i], want)
		}
	}
	for _, profile := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{}
		if s := profile.DecodeRadioTowerSwitches(mem); s != (RadioTowerSwitches{}) {
			t.Fatalf("fresh puzzle has unexpected state: %+v", s)
		}
		setGSEvent(mem, eventUndergroundSwitch3)
		setGSEvent(mem, eventUndergroundSwitch2)
		s := profile.DecodeRadioTowerSwitches(mem)
		if s.Switch1 || !s.Switch2 || !s.Switch3 || s.FinalDoorOpen {
			t.Fatalf("partial switches incorrectly imply open final door: %+v", s)
		}
		setGSEvent(mem, eventUndergroundSwitch1)
		s = profile.DecodeRadioTowerSwitches(mem)
		if !s.Switch1 || !s.Switch2 || !s.Switch3 || s.FinalDoorOpen {
			t.Fatalf("three pressed switches invented final shutter: %+v", s)
		}
		setGSEvent(mem, eventUndergroundFinalDoorOpen)
		s = profile.DecodeRadioTowerSwitches(mem)
		if !s.FinalDoorOpen {
			t.Fatalf("real final-door event not decoded: %+v", s)
		}
		story := profile.DecodeFirstBadgeProgress(mem)
		if story.Has(ProgressRadioTowerCardKeyReceived) ||
			story.Has(ProgressRadioTowerCleared) || story.Has(ProgressRisingBadgeEarned) {
			t.Fatal("volatile switch bits cannot produce durable Card Key, tower clear or Rising Badge")
		}
	}
}
