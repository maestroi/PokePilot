package farm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecisionEngineSpecNormalized(t *testing.T) {
	var nilSpec *DecisionEngineSpec
	if got, err := nilSpec.Normalized(); got != nil || err != nil {
		t.Fatalf("nil = %+v, %v", got, err)
	}
	for in, want := range map[string]string{
		"TypeSafe": DecisionBackendJev, " jev ": DecisionBackendJev, "local": DecisionBackendSystemOne,
		"system_one": DecisionBackendSystemOne, "": DecisionBackendOff, "disabled": DecisionBackendOff,
	} {
		got, err := (&DecisionEngineSpec{Backend: in, Objectives: true, MinConfidence: 0.5}).Normalized()
		if err != nil || got.Backend != want {
			t.Errorf("Normalized(%q) = %+v, %v; want %q", in, got, err, want)
		}
		if want == DecisionBackendOff && (got.Objectives || got.MinConfidence != 0 || got.Enabled()) {
			t.Errorf("off selection kept features: %+v", got)
		}
	}
	for _, bad := range []DecisionEngineSpec{{Backend: "gpt-4"}, {Backend: "jev", MinConfidence: -0.1}, {Backend: "jev", MinConfidence: 1.01}} {
		if _, err := bad.Normalized(); err == nil || !strings.Contains(err.Error(), "decision_engine") {
			t.Errorf("Normalized(%+v) err = %v", bad, err)
		}
	}
}

func TestDecisionEngineSpecMode(t *testing.T) {
	got, err := (&DecisionEngineSpec{Backend: "jev", Objectives: true}).Normalized()
	if err != nil || got.Mode != DecisionModeActive || got.Shadow() {
		t.Fatalf("legacy selection = %+v, %v; want active", got, err)
	}
	got, err = (&DecisionEngineSpec{Backend: "jev", Mode: " Shadow ", Battles: true}).Normalized()
	if err != nil || got.Mode != DecisionModeShadow || !got.Battles || !got.Shadow() {
		t.Fatalf("shadow = %+v, %v", got, err)
	}
	got, err = (&DecisionEngineSpec{Backend: "jev", Mode: "off", Objectives: true, Battles: true, MinConfidence: 0.8}).Normalized()
	if err != nil || *got != (DecisionEngineSpec{Backend: DecisionBackendOff, Mode: DecisionModeOff}) || got.Enabled() {
		t.Fatalf("mode off = %+v, %v", got, err)
	}
	for _, bad := range []DecisionEngineSpec{{Backend: "jev", Mode: "yolo"}, {Backend: "jev", Mode: "active", Battles: true}, {Backend: "jev", Battles: true}} {
		if _, err := bad.Normalized(); err == nil || !strings.Contains(err.Error(), "decision_engine") {
			t.Errorf("Normalized(%+v) err = %v", bad, err)
		}
	}
}

func TestSpecDecisionEngineIsOptionalOnTheWire(t *testing.T) {
	b, err := json.Marshal(Spec{RunID: "old"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "decision_engine") {
		t.Fatalf("unset selection serialized: %s", b)
	}
	var old Spec
	if err := json.Unmarshal([]byte(`{"run_id":"old","planner":"llm"}`), &old); err != nil || old.DecisionEngine != nil {
		t.Fatalf("old spec decoded selection %+v, %v", old.DecisionEngine, err)
	}
}
