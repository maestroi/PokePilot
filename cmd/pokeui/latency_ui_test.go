package main

import (
	"strings"
	"testing"
)

func TestUILabelsPlannerLatencyAsCallTime(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{
		`row("call latency"`,
		`row("latency avg"`,
		`s.successful_avg_seconds`,
		`s.rejected_avg_seconds`,
		`s.strategic_avg_seconds`,
		`all avg`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("planner latency UI missing %q", want)
		}
	}
	if strings.Contains(js, `row("think"`) {
		t.Error(`planner latency UI still labels endpoint duration as "think"`)
	}
}
