package profile

import "testing"

func TestRocketB2FEventsRequireLanceFinalClear(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{}
		if got := p.DecodeRocketB2FStory(mem); got.Cleared || got.ExecutiveFBeaten || got.TransmitterDoorOpened {
			t.Fatalf("fresh B2F story already complete: %+v", got)
		}
		setGSEvent(mem, eventRocketTransmitterDoorOpened)
		setGSEvent(mem, eventRocketExecutiveFBeaten)
		setGSEvent(mem, eventRocketElectrode1)
		setGSEvent(mem, eventRocketElectrode3)
		got := p.DecodeRocketB2FStory(mem)
		if !got.TransmitterDoorOpened || !got.ExecutiveFBeaten ||
			!got.Electrodes[0] || got.Electrodes[1] || !got.Electrodes[2] || got.Cleared {
			t.Fatalf("B2F flags incorrectly merged: %+v", got)
		}
		story := p.DecodeFirstBadgeProgress(mem)
		if !story.Has(ProgressRocketTransmitterDoorOpened) ||
			!story.Has(ProgressRocketExecutiveFBeaten) || story.Has(ProgressRocketHideoutCleared) {
			t.Fatalf("B2F progress projection did not preserve separate events: %+v", story)
		}
		setGSEvent(mem, eventRocketElectrode2)
		if p.DecodeRocketB2FStory(mem).Cleared {
			t.Fatal("Electrodes alone must not imply clearance")
		}
		setGSEvent(mem, eventClearedRocketHideout)
		if !p.DecodeRocketB2FStory(mem).Cleared || !p.DecodeFirstBadgeProgress(mem).Has(ProgressRocketHideoutCleared) {
			t.Fatal("post-reward Rocket-clear event not observed")
		}
	}
}

func TestRocketB2FEventFlagsArePinned(t *testing.T) {
	if eventRocketTransmitterDoorOpened != 768 || eventRocketExecutiveFBeaten != 1392 ||
		eventRocketElectrode1 != 1760 || eventRocketElectrode2 != 1761 ||
		eventRocketElectrode3 != 1762 || eventClearedRocketHideout != 34 {
		t.Fatal("B2F event indices drifted from pokegold")
	}
}
