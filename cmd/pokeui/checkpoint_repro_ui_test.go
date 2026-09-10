package main

import (
	"strings"
	"testing"
)

func TestRunInspectorOffersCheckpointRepro(t *testing.T) {
	js := string(inspectorJS)
	for _, want := range []string{
		"Start a new run from here",
		"/checkpoints",
		"/repro`",
		"repro source",
		"paired agent knowledge",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("inspector.js missing %q", want)
		}
	}
}
