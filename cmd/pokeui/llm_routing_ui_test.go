package main

import (
	"strings"
	"testing"
)

func TestOperatorExposesDirectLLMRoutingPolicies(t *testing.T) {
	page := string(operatorIndexPage())
	for _, want := range []string{
		`id="operations-llm"`,
		`7900 XTX · default · CPU after 120s`,
		`RTX 4090 · manual`,
		`CPU only · manual`,
		`pokefarm-llm-default-profile`,
		`auto: { label: "7900 XTX"`,
		`gpu: { label: "RTX 4090"`,
		`default: { label: "CPU only"`,
		`data-llm-default=`,
		`Active or already-leased runs keep their current route.`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("operator page missing LLM routing UI %q", want)
		}
	}
	for _, stale := range []string{
		`Auto · 7900 XTX → 4090 → LAN`,
		`Reserve 4090 · 7900 XTX only`,
		`Reserve all GPUs · LAN only`,
	} {
		if strings.Contains(page, stale) {
			t.Errorf("operator page still contains stale LLM route %q", stale)
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
	for _, want := range []string{
		`case "gpu": return "RTX 4090";`,
		`case "auto": return "7900 XTX → CPU";`,
		`case "default": return "CPU only";`,
	} {
		if !strings.Contains(string(uiJS), want) {
			t.Errorf("ui.js missing route label %q", want)
		}
	}
}
