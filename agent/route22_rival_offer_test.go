package agent

import "testing"

func TestFilterRedScriptedTalkObjectivesSuppressesRoute22Rival(t *testing.T) {
	objectives := []Objective{
		{Kind: KindTalk, X: route22RivalHomeX, Y: route22RivalHomeY},
		{Kind: KindTalk, X: 10, Y: 5},
		{Kind: KindGoTo, Place: PlaceID("route 22")},
	}

	got := filterRedScriptedTalkObjectives(Observation{Map: route22MapID}, objectives)
	if len(got) != 2 {
		t.Fatalf("filtered objective count = %d, want 2: %+v", len(got), got)
	}
	for _, o := range got {
		if o.Kind == KindTalk && o.X == route22RivalHomeX && o.Y == route22RivalHomeY {
			t.Fatalf("Route 22 rival was still offered as a generic talk objective: %+v", got)
		}
	}
	if got[0] != (Objective{Kind: KindTalk, X: 10, Y: 5}) {
		t.Fatalf("ordinary Route 22 person was filtered too: got[0]=%+v", got[0])
	}
}

func TestFilterRedScriptedTalkObjectivesLeavesSameCoordinateOnOtherMaps(t *testing.T) {
	want := []Objective{{Kind: KindTalk, X: route22RivalHomeX, Y: route22RivalHomeY}}
	got := filterRedScriptedTalkObjectives(Observation{Map: 0x20}, want)
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("non-Route-22 talk objective changed: got=%+v want=%+v", got, want)
	}
}
