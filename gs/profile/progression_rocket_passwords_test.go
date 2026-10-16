package profile

import "testing"

func TestRocketB3FPasswordFactsRequireBothRetailEvents(t *testing.T) {
	// From pokegold/constants/event_flags.asm with const_next index gaps.
	if eventLearnedSlowpokeTailPassword != 769 ||
		eventLearnedRaticateTailPassword != 770 ||
		eventOpenedRocketOfficeDoor != 771 ||
		eventBeatRocketExecutiveM4 != 1396 ||
		eventLearnedMurkrowPassword != 767 {
		t.Fatal("B3F password/office event index diverged from pinned pokegold")
	}
	mem := fakeGSReader{}
	setGSEvent(mem, eventLearnedRaticateTailPassword)
	story := projectEarlyStory(mem)
	if !story.Has(ProgressRocketRaticatePassword) || story.Has(ProgressRocketSlowpokePassword) ||
		story.Has(ProgressRocketPasswordsLearned) || story.Has(ProgressRocketOfficeOpened) {
		t.Fatalf("one password granted more than one fact: %+v", story)
	}
	setGSEvent(mem, eventLearnedSlowpokeTailPassword)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressRocketPasswordsLearned) || story.Has(ProgressRocketOfficeOpened) {
		t.Fatalf("two passwords did not stay separate from unlocking the office: %+v", story)
	}
	setGSEvent(mem, eventOpenedRocketOfficeDoor)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressRocketOfficeOpened) || story.Has(ProgressRocketExecutiveBeaten) {
		t.Fatalf("office door implied executive victory: %+v", story)
	}
	setGSEvent(mem, eventBeatRocketExecutiveM4)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressRocketExecutiveBeaten) || story.Has(ProgressRocketMurkrowPassword) {
		t.Fatalf("executive victory inferred the Murkrow password: %+v", story)
	}
	setGSEvent(mem, eventLearnedMurkrowPassword)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressRocketMurkrowPassword) {
		t.Fatalf("Murkrow password not projected: %+v", story)
	}
	if story.Has(ProgressGlacierBadgeEarned) || story.Has(ProgressRisingBadgeEarned) {
		t.Fatal("Rocket B3F story flags must never forge a Johto badge")
	}
}
