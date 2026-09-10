package main

import (
	"strings"
	"testing"
)

func TestOperatorExposesLLMRoutingPolicies(t *testing.T) {
	page := string(operatorIndexPage())
	for _, want := range []string{
		`id="operations-llm"`,
		`Auto · 7900 XTX → 4090 → LAN`,
		`Reserve 4090 · 7900 XTX only`,
		`Reserve all GPUs · LAN only`,
		`pokefarm-llm-default-profile`,
		`auto: { label: "Auto"`,
		`gpu: { label: "Reserve 4090"`,
		`default: { label: "Reserve all GPUs"`,
		`data-llm-default=`,
		`Active or already-leased runs keep their current route.`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("operator page missing LLM routing UI %q", want)
		}
	}
}

func TestOperatorLLMRoutingKeepsWireProfilesStable(t *testing.T) {
	page := string(operatorIndexPage())
	for _, value := range []string{`value="auto"`, `value="gpu"`, `value="default"`} {
		if !strings.Contains(page, value) {
			t.Errorf("operator page missing existing wire profile %q", value)
		}
	}
}
