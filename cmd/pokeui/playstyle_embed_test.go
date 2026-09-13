package main

import (
	"bytes"
	"testing"
)

func TestOperatorIndexIncludesClassicRunPolicyControls(t *testing.T) {
	page := operatorIndexPage()
	checks := [][]byte{
		[]byte(`id="play-style-script"`),
		[]byte(`"play_style"`),
		[]byte(`Adventure · natural play`),
		[]byte(`Speedrun · progression first`),
		[]byte(`Completionist · explore and collect`),
		[]byte(`Team Builder · catches and training`),
		[]byte(`"risk_tolerance"`),
		[]byte(`Balanced · protect progress`),
		[]byte(`Cautious · heal early and often`),
		[]byte(`"wild_encounters"`),
		[]byte(`Fight every encounter · never flee`),
		[]byte(`Max · uncapped`),
		[]byte(`spec.play_style = playStyle.value || "adventure"`),
		[]byte(`spec.risk_tolerance = riskTolerance.value || "balanced"`),
		[]byte(`spec.wild_encounters = wildEncounters.value || "planner"`),
	}
	for _, want := range checks {
		if !bytes.Contains(page, want) {
			t.Fatalf("operator index missing %q", want)
		}
	}
}
