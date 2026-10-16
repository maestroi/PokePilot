package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func TestLakeOfRageFactsHaveIndependentRetailEventBoundaries(t *testing.T) {
	// EventFlags indices from pokegold/constants/event_flags.asm with all
	// const_next gaps counted, not line-based approximations.
	if eventLakeOfRageRedGyarados != 1873 || eventDecidedToHelpLance != 96 {
		t.Fatalf("Lake of Rage event indices = (%d,%d), want (1873,96)",
			eventLakeOfRageRedGyarados, eventDecidedToHelpLance)
	}
	for _, tc := range []struct {
		name     string
		gyarados bool
		lance    bool
	}{
		{"fresh", false, false},
		{"gyarados defeated", true, false},
		{"lance accepted", false, true},
		{"both complete", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem := fakeGSReader{}
			if tc.gyarados {
				setGSEvent(mem, eventLakeOfRageRedGyarados)
			}
			if tc.lance {
				setGSEvent(mem, eventDecidedToHelpLance)
			}
			story := projectEarlyStory(mem)
			if story.Has(ProgressRedGyaradosDefeated) != tc.gyarados ||
				story.Has(ProgressLanceHelpAccepted) != tc.lance {
				t.Fatalf("Lake of Rage story flags = %+v; want red=%v lance=%v", story, tc.gyarados, tc.lance)
			}
			if story.Has(ProgressGlacierBadgeEarned) || story.Has(ProgressSupportedFrontier) {
				t.Fatalf("Lake of Rage events falsely awarded a badge: %+v", story)
			}
		})
	}
}

func TestLakeOfRageStagesDoNotGrantGlacierOrRisingFromRocketEvents(t *testing.T) {
	mem := fakeGSReader{}
	setGSEvent(mem, eventLakeOfRageRedGyarados)
	setGSEvent(mem, eventDecidedToHelpLance)
	setGSEvent(mem, 34) // EVENT_CLEARED_ROCKET_HIDEOUT
	story := projectEarlyStory(mem)
	if !story.Has(ProgressRedGyaradosDefeated) || !story.Has(ProgressLanceHelpAccepted) {
		t.Fatalf("post-Rocket encounter flags missing: %+v", story)
	}
	if story.Has(ProgressGlacierBadgeEarned) || story.Has(ProgressRisingBadgeEarned) {
		t.Fatal("Rocket-related story events must not manufacture an engine badge")
	}
	mem[sym.JohtoBadges] = johtoBadgeMineralMask
	story = projectEarlyStory(mem)
	if !story.Has(ProgressSupportedFrontier) {
		t.Fatal("the six-badge completion frontier must remain Mineral until Pryce is qualified")
	}
}
