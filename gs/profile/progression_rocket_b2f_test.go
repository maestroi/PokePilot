package profile

import "testing"

// Event indexes are the zero-based positions in pret/pokegold's event_flags.asm,
// including const_next gaps. Every progress fact must be independently durable.
func TestRocketB2FTransmitterAndElectrodeFactsAreNotSyntheticBadges(t *testing.T) {
	if eventOpenedTransmitterDoor != 765 ||
		eventBeatRocketExecutiveF2 != 1390 ||
		eventRocketElectrode1 != 1760 ||
		eventRocketElectrode2 != 1761 ||
		eventRocketElectrode3 != 1762 ||
		eventClearedRocketHideout != 33 ||
		eventGotHM06Whirlpool != 21 {
		t.Fatal("Rocket B2F event indices diverged from pinned retail pokegold")
	}
	mem := fakeGSReader{}
	setGSEvent(mem, eventLearnedMurkrowPassword)
	story := projectEarlyStory(mem)
	if story.Has(ProgressRocketTransmitterDoor) || story.Has(ProgressRocketExecutiveF) {
		t.Fatal("Murkrow password does not open the B2F door or defeat the executive")
	}
	setGSEvent(mem, eventOpenedTransmitterDoor)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressRocketTransmitterDoor) || story.Has(ProgressRocketExecutiveF) {
		t.Fatal("opening the door must not fake an executive victory")
	}
	setGSEvent(mem, eventBeatRocketExecutiveF2)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressRocketExecutiveF) || story.Has(ProgressRocketHideoutCleared) {
		t.Fatal("executive victory must not fake an Electrode victory")
	}
	for _, flag := range []uint16{eventRocketElectrode1, eventRocketElectrode2, eventRocketElectrode3} {
		setGSEvent(mem, flag)
		story = projectEarlyStory(mem)
		if story.Has(ProgressRocketHideoutCleared) {
			t.Fatal("electrodes alone must not prove Lance's Whirlpool handoff")
		}
	}
	if !story.Has(ProgressRocketElectrode1) || !story.Has(ProgressRocketElectrode2) ||
		!story.Has(ProgressRocketElectrode3) {
		t.Fatal("three individually settled Electrode must have three separate flags")
	}
	setGSEvent(mem, eventGotHM06Whirlpool)
	if projectEarlyStory(mem).Has(ProgressRocketHideoutCleared) {
		t.Fatal("HM06 alone without clearing hideout is not campaign progress")
	}
	setGSEvent(mem, eventClearedRocketHideout)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressRocketHideoutCleared) {
		t.Fatal("all three Electrode, the hideout clear and HM06 must complete B2F")
	}
	if story.Has(ProgressGlacierBadgeEarned) || story.Has(ProgressRisingBadgeEarned) ||
		story.Has(ProgressSupportedFrontier) {
		t.Fatal("Rocket B2F must never award a Johto badge or advance the qualified frontier")
	}
}
