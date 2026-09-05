package main

import (
	"bytes"
	"testing"
)

func TestObjectiveFailureTelemetryMarksRepeatedUnrecoveredFailureBlocking(t *testing.T) {
	resetObjectiveFailureTelemetry()
	t.Cleanup(resetObjectiveFailureTelemetry)

	var out bytes.Buffer
	log := &agentTraceLog{w: &out} // no TraceNote: telemetry must still observe it
	for _, line := range []string{
		"round 14: go to mt moon b1f, fleeing wild battles -> failed: agent: go to mt moon b1f: skill: step left blocked at (10,22), map 3b at (10,22)\n",
		"round 15: go to mt moon b1f, fleeing wild battles -> failed: agent: go to mt moon b1f: skill: step left blocked at (10,22), map 3b at (10,22)\n",
	} {
		if _, err := log.Write([]byte(line)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	got := drainObjectiveFailureTelemetry("failed")
	if len(got) != 1 {
		t.Fatalf("failures = %+v, want one group", got)
	}
	f := got[0]
	if f.Count != 2 || f.FirstRound != 14 || f.LastRound != 15 {
		t.Fatalf("round/count summary = %+v", f)
	}
	if f.Map != 0x3b || f.X != 10 || f.Y != 22 {
		t.Fatalf("location = %+v", f)
	}
	if f.Recovered || !f.Blocking {
		t.Fatalf("classification = recovered=%t blocking=%t, want false/true", f.Recovered, f.Blocking)
	}
	if f.ObservedAt.IsZero() {
		t.Fatal("blocking failure has no stable observation timestamp")
	}
}

func TestObjectiveFailureTelemetryMarksLaterProgressRecovered(t *testing.T) {
	resetObjectiveFailureTelemetry()
	t.Cleanup(resetObjectiveFailureTelemetry)

	observeAgentLogLine("round 4: heal the party -> failed: skill: A did not open a text box, map 3a at (11,6)")
	observeAgentLogLine("round 7: major progress -> 1 badge(s), 9 event(s), 14 map(s)")

	got := drainObjectiveFailureTelemetry("budget")
	if len(got) != 1 {
		t.Fatalf("failures = %+v, want one group", got)
	}
	if !got[0].Recovered || got[0].Blocking {
		t.Fatalf("classification = recovered=%t blocking=%t, want true/false", got[0].Recovered, got[0].Blocking)
	}
}

func TestObjectiveFailureTelemetryExcludesExpectedGameOutcome(t *testing.T) {
	resetObjectiveFailureTelemetry()
	t.Cleanup(resetObjectiveFailureTelemetry)

	observeAgentLogLine("round 9: go to route 3 -> failed: agent: skill: Travel: blacked out after 2 battles, map 3a at (3,3)")
	if got := drainObjectiveFailureTelemetry("budget"); len(got) != 0 {
		t.Fatalf("blackout produced engineering failure telemetry: %+v", got)
	}
}
