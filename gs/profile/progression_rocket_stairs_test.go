package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func TestMahoganyStaircaseRequiresPermanentRetailEvent(t *testing.T) {
	// Source: pokegold/constants/event_flags.asm with const_next gaps.
	if eventUncoveredMahoganyMartStaircase != 738 {
		t.Fatalf("Mahogany staircase event index = %d, want 738", eventUncoveredMahoganyMartStaircase)
	}
	mem := fakeGSReader{}
	setGSEvent(mem, eventLakeOfRageRedGyarados)
	setGSEvent(mem, eventDecidedToHelpLance)
	story := projectEarlyStory(mem)
	if story.Has(ProgressMahoganyStairsOpened) {
		t.Fatal("Lance agreement must not mark the Mart staircase uncovered")
	}
	setGSEvent(mem, eventUncoveredMahoganyMartStaircase)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressMahoganyStairsOpened) {
		t.Fatal("the cartridge's revealed stair event must be durable")
	}
	if story.Has(ProgressGlacierBadgeEarned) || story.Has(ProgressRisingBadgeEarned) {
		t.Fatal("Mahogany stairs are not a gym badge")
	}
	mem[sym.JohtoBadges] = 0
	if story.Has(ProgressSupportedFrontier) {
		t.Fatal("opening the Rocket Base cannot advance the supported badge frontier")
	}
}
