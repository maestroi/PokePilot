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

func TestDecisionEngineDeploymentIdentity(t *testing.T) {
	// Only the wall's registry binds a deployment.
	if _, err := (&DecisionEngineSpec{Backend: "jev", Deployment: "typesafe-jev"}).Normalized(); err == nil || !strings.Contains(err.Error(), "decision_engine.deployment") {
		t.Fatalf("unbound deployment err = %v", err)
	}
	src := &DecisionEngineSpec{Backend: "jev", Deployment: "typesafe-jev", Inference: &InferenceIdentity{DeploymentID: "typesafe-jev", Endpoint: "https://api.typesafe.ai/v1"}}
	got, err := src.Normalized()
	if err != nil || got.Inference == src.Inference || *got.Inference != *src.Inference {
		t.Fatalf("normalized = %+v, %v; want an equal, independent identity", got, err)
	}
	clone := src.Clone()
	clone.Inference.Endpoint = "changed"
	if src.Inference.Endpoint == "changed" {
		t.Fatal("clone shares the identity pointer")
	}
}

func TestModelDeploymentProtocol(t *testing.T) {
	base := ModelDeployment{ID: "d", ModelID: "m", Compute: "c", Endpoint: "http://x/v1", APIModel: "m"}
	if !base.ServesStrategist() || base.DecisionBackend() != DecisionBackendSystemOne || base.Identity().Protocol != ProtocolOpenAI {
		t.Fatalf("legacy row = strategist %v backend %q", base.ServesStrategist(), base.DecisionBackend())
	}
	jev := base
	jev.Protocol = "TypeSafe-Choice"
	if jev.ServesStrategist() || jev.DecisionBackend() != DecisionBackendJev || jev.Identity().Protocol != ProtocolTypeSafeChoice {
		t.Fatalf("jev row = strategist %v backend %q", jev.ServesStrategist(), jev.DecisionBackend())
	}
	bad := base
	bad.Protocol = "grpc"
	if err := (ModelRegistry{Deployments: []ModelDeployment{bad}}).Validate(); err == nil || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("invalid protocol err = %v", err)
	}
}
