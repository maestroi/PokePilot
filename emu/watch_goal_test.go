package emu

import (
	"strings"
	"testing"
)

func TestWatchPageRendersStructuredGoalProgress(t *testing.T) {
	for _, want := range []string{`id="goal"`, `s.goal_summary`, `s.goal_complete`} {
		if !strings.Contains(watchPage, want) {
			t.Fatalf("watch page missing structured goal rendering %q", want)
		}
	}
}
