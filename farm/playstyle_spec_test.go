package farm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSpecPlayStyleRoundTripsAsOptionalWireField(t *testing.T) {
	runID := "playstyle-roundtrip"
	RememberPlayStyle(runID, "adventure")
	RememberRiskTolerance(runID, "balanced")
	RememberWildEncounters(runID, "fight")
	b, err := json.Marshal(Spec{RunID: runID, Planner: "llm", Goal: "badges:1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"play_style":"adventure"`,
		`"risk_tolerance":"balanced"`,
		`"wild_encounters":"fight"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("encoded spec = %s, want %s", b, want)
		}
	}

	var got Spec
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if style := PlayStyleForSpec(got); style != "adventure" {
		t.Fatalf("play style = %q, want adventure", style)
	}
	if risk := RiskToleranceForSpec(got); risk != "balanced" {
		t.Fatalf("risk tolerance = %q, want balanced", risk)
	}
	if wild := WildEncountersForSpec(got); wild != "fight" {
		t.Fatalf("wild encounters = %q, want fight", wild)
	}
	if CurrentRiskTolerance() != "balanced" || CurrentWildEncounters() != "fight" {
		t.Fatalf("current run policy = risk %q wild %q", CurrentRiskTolerance(), CurrentWildEncounters())
	}
}

func TestLegacySpecHasNoRunPolicyAndStaysCompatible(t *testing.T) {
	const raw = `{"run_id":"legacy-style","planner":"llm","goal":"badges:1"}`
	var got Spec
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if style := PlayStyleForSpec(got); style != "" {
		t.Fatalf("legacy play style = %q, want empty compatibility default", style)
	}
	if risk := RiskToleranceForSpec(got); risk != "" {
		t.Fatalf("legacy risk tolerance = %q, want empty compatibility default", risk)
	}
	if wild := WildEncountersForSpec(got); wild != "" {
		t.Fatalf("legacy wild encounters = %q, want empty compatibility default", wild)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"play_style", "risk_tolerance", "wild_encounters"} {
		if strings.Contains(string(b), field) {
			t.Fatalf("legacy spec unexpectedly gained %s: %s", field, b)
		}
	}
}

func TestCopyPlayStyleCarriesEndlessPolicy(t *testing.T) {
	RememberPlayStyle("style-parent", "team_builder")
	CopyPlayStyle("style-parent", "style-child")
	if got := PlayStyleForRun("style-child"); got != "team_builder" {
		t.Fatalf("copied style = %q, want team_builder", got)
	}
}

func TestCopyRunPolicyCarriesOrthogonalSettings(t *testing.T) {
	RememberPlayStyle("policy-parent", "completionist")
	RememberRiskTolerance("policy-parent", "cautious")
	RememberWildEncounters("policy-parent", "fight")
	CopyRunPolicy("policy-parent", "policy-child")
	if got := PlayStyleForRun("policy-child"); got != "completionist" {
		t.Fatalf("copied style = %q", got)
	}
	if got := RiskToleranceForRun("policy-child"); got != "cautious" {
		t.Fatalf("copied risk = %q", got)
	}
	if got := WildEncountersForRun("policy-child"); got != "fight" {
		t.Fatalf("copied wild policy = %q", got)
	}
}
