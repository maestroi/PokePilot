package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsExposesTelegramCountersAndBuildInfo(t *testing.T) {
	b := &bot{confirmations: map[string]confirmation{"tok": {}}}
	b.m.updates.Store(3)
	b.m.wallHealthy.Store(1)

	rec := httptest.NewRecorder()
	b.httpHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"poketelegram_updates_total 3",
		"poketelegram_pending_confirmations 1",
		"poketelegram_wall_healthy 1",
		`service="telegram"`,
		`revision="dev"`,
		"pokepilot_build_info{",
		"pokepilot_up{",
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body missing %q\n%s", want, body)
		}
	}
}
