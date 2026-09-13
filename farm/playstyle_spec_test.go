package farm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSpecPlayStyleRoundTripsAsOptionalWireField(t *testing.T) {
	runID := "playstyle-roundtrip"
	RememberPlayStyle(runID, "adventure")
	b, err := json.Marshal(Spec{RunID: runID, Planner: "llm", Goal: "badges:1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"play_style":"adventure"`) {
		t.Fatalf("encoded spec = %s, want play_style", b)
	}

	var got Spec
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if style := PlayStyleForSpec(got); style != "adventure" {
		t.Fatalf("play style = %q, want adventure", style)
	}
}

func TestLegacySpecHasNoPlayStyleAndStaysCompatible(t *testing.T) {
	const raw = `{"run_id":"legacy-style","planner":"llm","goal":"badges:1"}`
	var got Spec
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if style := PlayStyleForSpec(got); style != "" {
		t.Fatalf("legacy play style = %q, want empty compatibility default", style)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "play_style") {
		t.Fatalf("legacy spec unexpectedly gained play_style: %s", b)
	}
}

func TestCopyPlayStyleCarriesEndlessPolicy(t *testing.T) {
	RememberPlayStyle("style-parent", "team_builder")
	CopyPlayStyle("style-parent", "style-child")
	if got := PlayStyleForRun("style-child"); got != "team_builder" {
		t.Fatalf("copied style = %q, want team_builder", got)
	}
}
