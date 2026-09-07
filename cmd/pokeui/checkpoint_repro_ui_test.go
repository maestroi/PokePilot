package main

import (
	"strings"
	"testing"
)

func TestRunInspectorOffersCheckpointRepro(t *testing.T) {
	js := string(inspectorJS)
	for _, want := range []string{
		"Run from checkpoint",
		"/checkpoints",
		"/repro`",
		"repro source",
		"currently deployed runner build",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("inspector.js missing %q", want)
		}
	}
}
