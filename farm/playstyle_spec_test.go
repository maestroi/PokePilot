package farm

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// TestSpecCarriesRunPolicyAsOwnFields pins that the gameplay policy is part of
// the Spec itself rather than a side channel keyed by run id.
func TestSpecCarriesRunPolicyAsOwnFields(t *testing.T) {
	spec := Spec{
		RunID:          "playstyle-roundtrip",
		Planner:        "llm",
		Goal:           GoalFrom("badges:1"),
		PlayStyle:      "adventure",
		RiskTolerance:  "balanced",
		WildEncounters: "fight",
	}
	b, err := json.Marshal(spec)
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
	if got.PlayStyle != "adventure" {
		t.Fatalf("play style = %q, want adventure", got.PlayStyle)
	}
	if got.RiskTolerance != "balanced" {
		t.Fatalf("risk tolerance = %q, want balanced", got.RiskTolerance)
	}
	if got.WildEncounters != "fight" {
		t.Fatalf("wild encounters = %q, want fight", got.WildEncounters)
	}
	if got.Goal.String() != "badges:1" {
		t.Fatalf("goal = %q, want badges:1", got.Goal.String())
	}
}

// TestLegacySpecHasNoRunPolicyAndStaysCompatible covers a queued spec written
// before the policy became first-class: it must decode to the documented empty
// compatibility defaults and must not acquire policy keys it never carried.
func TestLegacySpecHasNoRunPolicyAndStaysCompatible(t *testing.T) {
	const raw = `{"run_id":"legacy-style","planner":"llm","goal":"badges:1"}`
	var got Spec
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if got.PlayStyle != "" {
		t.Fatalf("legacy play style = %q, want empty compatibility default", got.PlayStyle)
	}
	if got.RiskTolerance != "" {
		t.Fatalf("legacy risk tolerance = %q, want empty compatibility default", got.RiskTolerance)
	}
	if got.WildEncounters != "" {
		t.Fatalf("legacy wild encounters = %q, want empty compatibility default", got.WildEncounters)
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

// TestLegacySpecOmitsUnsetGoalWhileFreePlayKeepsIt pins the three-state goal
// contract on the wire: absent stays absent across a round trip, while an
// explicit empty Free play goal survives.
func TestLegacySpecOmitsUnsetGoalWhileFreePlayKeepsIt(t *testing.T) {
	var unset Spec
	if err := json.Unmarshal([]byte(`{"run_id":"no-goal","planner":"llm"}`), &unset); err != nil {
		t.Fatal(err)
	}
	if unset.Goal.Provided() {
		t.Fatalf("absent goal decoded as provided: %q", unset.Goal.String())
	}
	wire, err := json.Marshal(unset)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), `"goal"`) {
		t.Fatalf("unset goal must stay off the wire: %s", wire)
	}

	var freePlay Spec
	if err := json.Unmarshal([]byte(`{"run_id":"free-play","planner":"llm","goal":""}`), &freePlay); err != nil {
		t.Fatal(err)
	}
	if !freePlay.Goal.Provided() || freePlay.Goal.String() != "" {
		t.Fatalf("free play goal = %q provided=%v, want provided empty", freePlay.Goal.String(), freePlay.Goal.Provided())
	}
}

// TestSpecsWithDifferentPolicyDoNotCrossTalk is the regression for the
// process-global policy side channel: two Specs alive at once must each keep
// their own behavior, in either construction order.
func TestSpecsWithDifferentPolicyDoNotCrossTalk(t *testing.T) {
	first := Spec{
		RunID:          "isolated-a",
		Planner:        "llm",
		Goal:           GoalFrom("badges:1"),
		PlayStyle:      "adventure",
		RiskTolerance:  "balanced",
		WildEncounters: "fight",
	}
	second := Spec{
		RunID:          "isolated-b",
		Planner:        "llm",
		Goal:           GoalFrom("dex"),
		PlayStyle:      "completionist",
		RiskTolerance:  "cautious",
		WildEncounters: "planner",
	}

	// Encode and decode both, interleaved, the way a wall with two in-flight
	// runs and a runner leasing them in sequence would.
	firstWire, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondWire, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	var firstBack, secondBack Spec
	if err := json.Unmarshal(secondWire, &secondBack); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(firstWire, &firstBack); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		got  Spec
		want Spec
	}{
		"first":  {got: firstBack, want: first},
		"second": {got: secondBack, want: second},
	} {
		if tc.got.PlayStyle != tc.want.PlayStyle {
			t.Fatalf("%s play style = %q, want %q", name, tc.got.PlayStyle, tc.want.PlayStyle)
		}
		if tc.got.RiskTolerance != tc.want.RiskTolerance {
			t.Fatalf("%s risk tolerance = %q, want %q", name, tc.got.RiskTolerance, tc.want.RiskTolerance)
		}
		if tc.got.WildEncounters != tc.want.WildEncounters {
			t.Fatalf("%s wild encounters = %q, want %q", name, tc.got.WildEncounters, tc.want.WildEncounters)
		}
		if tc.got.Goal.String() != tc.want.Goal.String() {
			t.Fatalf("%s goal = %q, want %q", name, tc.got.Goal.String(), tc.want.Goal.String())
		}
	}
}

// TestSpecPolicyIsRaceFreeUnderConcurrentDecode exercises the same guarantee
// under concurrency, so a reintroduced shared map would trip -race.
func TestSpecPolicyIsRaceFreeUnderConcurrentDecode(t *testing.T) {
	const runs, iterations = 8, 32
	var wg sync.WaitGroup
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := Spec{
				RunID:          "concurrent-" + string(rune('a'+i)),
				Planner:        "llm",
				Goal:           GoalFrom("badges:1"),
				PlayStyle:      "adventure",
				RiskTolerance:  "balanced",
				WildEncounters: "fight",
			}
			wire, err := json.Marshal(want)
			if err != nil {
				t.Errorf("marshal: %v", err)
				return
			}
			for n := 0; n < iterations; n++ {
				var got Spec
				if err := json.Unmarshal(wire, &got); err != nil {
					t.Errorf("unmarshal: %v", err)
					return
				}
				if got.PlayStyle != want.PlayStyle || got.RiskTolerance != want.RiskTolerance || got.WildEncounters != want.WildEncounters {
					t.Errorf("run %d policy = %q/%q/%q", i, got.PlayStyle, got.RiskTolerance, got.WildEncounters)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

// TestAdoptModelUpdatesOnlyThatIdentity pins the scoped replacement for the
// old process-global adopted-inference lease.
func TestAdoptModelUpdatesOnlyThatIdentity(t *testing.T) {
	var spec Spec
	if err := json.Unmarshal([]byte(`{
		"run_id":"adopt-model",
		"inference":{
			"deployment_id":"7900-primary",
			"model_id":"qwen3.5-9b",
			"api_model":"qwen3.5-9b",
			"endpoint":"http://gpu.example/v1",
			"compute":"RX 7900 XTX",
			"revision":"stale",
			"artifact":"/old.gguf",
			"quantization":"Q4"
		}
	}`), &spec); err != nil {
		t.Fatal(err)
	}
	other := &InferenceIdentity{ModelID: "untouched", APIModel: "untouched"}

	spec.Inference.AdoptModel("qwen3.8-27b")

	if spec.Inference.ModelID != "qwen3.8-27b" || spec.Inference.APIModel != "qwen3.8-27b" {
		t.Fatalf("adopted inference = %#v", spec.Inference)
	}
	if spec.Inference.Revision != "" || spec.Inference.Artifact != "" || spec.Inference.Quantization != "" {
		t.Fatalf("adopt retained stale artifact fields: %#v", spec.Inference)
	}
	if other.ModelID != "untouched" || other.APIModel != "untouched" {
		t.Fatalf("adoption leaked into another identity: %#v", other)
	}
}
