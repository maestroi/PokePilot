package profile

import "testing"

func TestRocketBaseLanceHealingIsIndependentOfMartStairsAndBadges(t *testing.T) {
	if eventLanceHealedRocketBase != 76 {
		t.Fatalf("EVENT_LANCE_HEALED_YOU_IN_TEAM_ROCKET_BASE = %d, want 76", eventLanceHealedRocketBase)
	}
	mem := fakeGSReader{}
	setGSEvent(mem, eventUncoveredMahoganyMartStaircase)
	story := projectEarlyStory(mem)
	if !story.Has(ProgressMahoganyStairsOpened) || story.Has(ProgressRocketLanceHealing) {
		t.Fatalf("staircase reveal must not imply Lance B2F heal: %+v", story)
	}
	setGSEvent(mem, eventLanceHealedRocketBase)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressRocketLanceHealing) {
		t.Fatalf("B2F event was not projected: %+v", story)
	}
	if story.Has(ProgressGlacierBadgeEarned) || story.Has(ProgressRisingBadgeEarned) {
		t.Fatal("Lance healing the party must not award a gym badge")
	}
}
