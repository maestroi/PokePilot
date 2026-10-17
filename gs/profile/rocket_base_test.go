package profile

import "testing"

func TestRocketBaseEventsAreIndependentAndResumable(t *testing.T) {
	mem := fakeGSReader{}
	p := NewGold()
	if got := p.DecodeRocketBaseStory(mem); got.Cleared || got.SlowpokeTail || got.Electrodes[0] {
		t.Fatalf("fresh Rocket Base already completed: %+v", got)
	}
	setGSEvent(mem, rocketSlowpokeTailEvent)
	setGSEvent(mem, rocketRaticateTailEvent)
	setGSEvent(mem, rocketGiovanniDoorEvent)
	setGSEvent(mem, rocketExecutiveMEvent)
	setGSEvent(mem, rocketHailGiovanniEvent)
	setGSEvent(mem, rocketTransmitterDoorEvent)
	setGSEvent(mem, rocketExecutiveFEvent)
	setGSEvent(mem, rocketElectrode1Event)
	setGSEvent(mem, rocketElectrode3Event)
	got := p.DecodeRocketBaseStory(mem)
	if !got.SlowpokeTail || !got.RaticateTail || !got.GiovanniDoor || !got.ExecutiveM ||
		!got.HailGiovanni || !got.TransmitterDoor || !got.ExecutiveF ||
		!got.Electrodes[0] || got.Electrodes[1] || !got.Electrodes[2] || got.Cleared {
		t.Fatalf("Rocket stage facts incorrectly aliased: %+v", got)
	}
	setGSEvent(mem, rocketElectrode2Event)
	if p.DecodeRocketBaseStory(mem).Cleared {
		t.Fatal("defeating three Electrodes must not imply the Lance reward script finished")
	}
	setGSEvent(mem, eventClearedRocketHideout)
	if !p.DecodeRocketBaseStory(mem).Cleared || !projectEarlyStory(mem).Has(ProgressRocketHideoutCleared) {
		t.Fatal("durable Rocket-clear flag is not projected")
	}
}

func TestRocketBasePinnedEventIndices(t *testing.T) {
	if rocketSlowpokeTailEvent != 769 || rocketRaticateTailEvent != 770 ||
		rocketGiovanniDoorEvent != 771 || rocketExecutiveMEvent != 1396 ||
		rocketHailGiovanniEvent != 767 || rocketTransmitterDoorEvent != 768 ||
		rocketExecutiveFEvent != 1392 ||
		rocketElectrode1Event != 1760 || rocketElectrode2Event != 1761 || rocketElectrode3Event != 1762 ||
		eventClearedRocketHideout != 34 {
		t.Fatal("Rocket stage event constants no longer match the vendored Gold/Silver flags")
	}
}
