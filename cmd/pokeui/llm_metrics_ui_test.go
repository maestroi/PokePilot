package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLLMMetricsAssetsAreWiredIntoOperator(t *testing.T) {
	page := string(operatorIndexPage())
	for _, want := range []string{`href="/llm_metrics.css"`, `src="/llm_metrics.js"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("operator page missing %q", want)
		}
	}

	js := string(llmMetricsJS)
	for _, want := range []string{"Timing", "Usage", "Reliability", "latencyBreakdown", "served by", "llm-metric-columns"} {
		if !strings.Contains(js, want) {
			t.Errorf("llm_metrics.js missing %q", want)
		}
	}
	analytics := string(statsJS)
	for _, want := range []string{"LLM workload", "Routing profiles", "Latest serving models", "successful avg", "repeat picks"} {
		if !strings.Contains(analytics, want) {
			t.Errorf("stats.js missing %q", want)
		}
	}
}

func TestLLMMetricsAssetsAreNoStore(t *testing.T) {
	h := handler("http://unused")
	for _, path := range []string{"/llm_metrics.js", "/llm_metrics.css"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, res.Code)
		}
		if got := res.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("%s Cache-Control = %q", path, got)
		}
		if res.Body.Len() == 0 {
			t.Fatalf("%s served an empty body", path)
		}
	}
}
