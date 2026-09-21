package main

import (
	"bytes"
	"testing"
)

func TestNewRunUIOffersQualificationBenchmarks(t *testing.T) {
	for _, want := range []string{
		`name="qualification_target"`,
		`value="hall-of-fame"`,
		`name="qualification_runs"`,
	} {
		if !bytes.Contains(indexHTML, []byte(want)) {
			t.Fatalf("index missing %q", want)
		}
	}
	js := string(uiJS)
	for _, want := range []string{
		"qualificationGoals",
		"newQualificationGroupId",
		"experiment_id",
		"experiment_case",
		"baseSeed + i",
		`fetch("/v1/specs"`,
	} {
		if !bytes.Contains(uiJS, []byte(want)) {
			t.Fatalf("ui.js missing %q\n%s", want, js)
		}
	}
}
