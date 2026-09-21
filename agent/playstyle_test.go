package agent

import (
	"strings"
	"testing"
)

func TestAnnotatePlayStyleSpeedrunIsExactNoOp(t *testing.T) {
	obs := Observation{PartyCount: 1}
	offered := []Objective{
		{Kind: KindProgress, Progress: ProgressID("test_progress"), Note: "(existing note)"},
		{Kind: KindTalk, X: 3, Y: 4},
	}

	got := AnnotatePlayStyle(obs, offered, PlayStyle("speedrun"))
	if len(got) != len(offered) {
		t.Fatalf("len = %d, want %d", len(got), len(offered))
	}
	for i := range offered {
		if got[i] != offered[i] {
			t.Fatalf("speedrun changed offered[%d]:\ngot:  %#v\nwant: %#v", i, got[i], offered[i])
		}
	}
	if &got[0] == &offered[0] {
		t.Fatal("AnnotatePlayStyle must return its own slice")
	}
}

func TestAdventureAnnotatesOptionalObjectivesWithExplicitDrives(t *testing.T) {
	obs := Observation{PartyCount: 2}
	offered := []Objective{
		{Kind: KindProgress, Progress: ProgressID("test_progress")},
		{Kind: KindTalk, X: 3, Y: 4},
		{Kind: KindPickup, Item: ItemID("potion"), X: 5, Y: 6},
		{Kind: KindGoTo, Place: PlaceID("route 2"), Note: "(unvisited adjacent map)"},
	}

	got := AnnotatePlayStyle(obs, offered, PlayStyle("adventure"))
	for i, o := range got {
		if !strings.Contains(o.Note, "[adventure ") {
			t.Fatalf("objective %d has no Adventure annotation: %#v", i, o)
		}
	}
	if !strings.Contains(got[1].Note, "interaction") {
		t.Fatalf("talk annotation = %q, want interaction drive", got[1].Note)
	}
	if !strings.Contains(got[2].Note, "items") {
		t.Fatalf("pickup annotation = %q, want item drive", got[2].Note)
	}
	if !strings.Contains(got[3].Note, "exploration") {
		t.Fatalf("unvisited journey annotation = %q, want exploration drive", got[3].Note)
	}
}

func TestAdventureFailureEvidenceRaisesExplorationUrgencyWithoutSuppressingProgression(t *testing.T) {
	profile := PlayStyle("adventure")
	goTo := Objective{Kind: KindGoTo, Place: PlaceID("route 2"), Note: "(unvisited adjacent map)"}
	progress := Objective{Kind: KindProgress, Progress: ProgressID("test_progress")}

	base := Observation{PartyCount: 2}
	stalled := base
	stalled.Failures = []Failure{{Objective: "go to route 2", Times: 4}}

	baseExplore := ScoreObjective(base, goTo, profile)
	stalledExplore := ScoreObjective(stalled, goTo, profile)
	if stalledExplore.Weighted[DriveExploration] <= baseExplore.Weighted[DriveExploration] {
		t.Fatalf("exploration did not rise after failures: base %.3f stalled %.3f",
			baseExplore.Weighted[DriveExploration], stalledExplore.Weighted[DriveExploration])
	}

	baseProgress := ScoreObjective(base, progress, profile)
	stalledProgress := ScoreObjective(stalled, progress, profile)
	if stalledProgress.Weighted[DriveProgression] != baseProgress.Weighted[DriveProgression] {
		t.Fatalf("progression weight changed during mild fallback: base %.3f stalled %.3f",
			baseProgress.Weighted[DriveProgression], stalledProgress.Weighted[DriveProgression])
	}
}

func TestAdventureRaisesSafetyWhenPartyNeedsRecovery(t *testing.T) {
	profile := PlayStyle("adventure")
	heal := Objective{Kind: KindHeal}

	healthy := Observation{
		PartyCount: 1,
		Party:      []PartyMon{{HP: 30, MaxHP: 30}},
	}
	hurt := Observation{
		PartyCount: 1,
		Party:      []PartyMon{{HP: 10, MaxHP: 30}},
	}

	healthyScore := ScoreObjective(healthy, heal, profile)
	hurtScore := ScoreObjective(hurt, heal, profile)
	if hurtScore.Weighted[DriveSafety] <= healthyScore.Weighted[DriveSafety] {
		t.Fatalf("hurt safety %.3f <= healthy safety %.3f",
			hurtScore.Weighted[DriveSafety], healthyScore.Weighted[DriveSafety])
	}
}

func TestPlayStyleUnknownFallsBackToSpeedrun(t *testing.T) {
	got := PlayStyle("definitely-not-a-profile")
	if got.Name != "speedrun" {
		t.Fatalf("unknown profile name = %q, want speedrun", got.Name)
	}
}


func TestAnnotatePlayStyleSpeedrunPrioritizesFlySetup(t *testing.T) {
	offered := []Objective{
		{Kind: KindProgress, Progress: redProgressSilphScopeAcquired},
		{Kind: KindProgress, Progress: redProgressFlyReady},
		{Kind: KindProgress, Progress: redProgressRainbowBadge},
	}

	got := AnnotatePlayStyle(Observation{}, offered, PlayStyle(PlayStyleSpeedrun))
	if len(got) != 1 || got[0].Kind != KindProgress || got[0].Progress != redProgressFlyReady {
		t.Fatalf("speedrun Fly priority = %#v, want only %q", got, redProgressFlyReady)
	}

	adventure := AnnotatePlayStyle(Observation{}, offered, PlayStyle(PlayStyleAdventure))
	if len(adventure) != len(offered) {
		t.Fatalf("adventure Fly menu len = %d, want %d", len(adventure), len(offered))
	}
}
