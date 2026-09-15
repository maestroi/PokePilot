package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/skill"
)

func TestStarterObjectiveForRequestPreservesPatchedSpecies(t *testing.T) {
	obj, err := starterObjectiveForRequest("mewtwo", 7)
	if err != nil {
		t.Fatal(err)
	}
	if obj.Kind != agent.KindStarter || obj.Starter != skill.StarterSquirtle || obj.Species != "mewtwo" {
		t.Fatalf("mewtwo objective = %+v, want middle ball with semantic mewtwo species", obj)
	}

	random, err := starterObjectiveForRequest("random:any", 123)
	if err != nil {
		t.Fatal(err)
	}
	if random.Starter != skill.StarterSquirtle || random.Species == "" {
		t.Fatalf("random objective = %+v, want patched middle ball plus resolved species", random)
	}
}

func TestScriptedObjectiveDetailUsesStructuredOutcome(t *testing.T) {
	result := agent.ObjectiveResult{
		Objective: agent.Objective{Kind: agent.KindGoTo, Place: "route 3"},
		Outcome:   agent.OutcomeBlocked,
		Cause:     agent.FailureCauseID("trainer_blacked_out"),
		CauseContext: []string{
			"trainer",
		},
		Summary: "at route 3: blacked out",
	}
	got := scriptedObjectiveDetail(result, errors.New("native wording should not define identity"))
	for _, want := range []string{"outcome=blocked", "cause=trainer_blacked_out", "context=trainer", "summary=at route 3: blacked out"} {
		if !strings.Contains(got, want) {
			t.Fatalf("detail = %q, want %q", got, want)
		}
	}
}
