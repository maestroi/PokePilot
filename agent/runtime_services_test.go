package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeServicesFromEnvironment(t *testing.T) {
	t.Setenv(virtualTraderURLEnv, "http://virtual-trader:8080")
	services := runtimeServicesFromEnvironment()
	if services == nil || !services.VirtualTrader {
		t.Fatalf("services = %+v, want virtual trader", services)
	}
}

func TestRuntimeServicesAbsentWithoutEndpoint(t *testing.T) {
	t.Setenv(virtualTraderURLEnv, "")
	if services := runtimeServicesFromEnvironment(); services != nil {
		t.Fatalf("services = %+v, want nil", services)
	}
}

func TestObservationJSONExposesVirtualTraderCapability(t *testing.T) {
	obs := Observation{Services: &RuntimeServices{VirtualTrader: true}}
	b, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"runtime_services":{"virtual_trader":true}`) {
		t.Fatalf("observation JSON = %s", b)
	}
}
