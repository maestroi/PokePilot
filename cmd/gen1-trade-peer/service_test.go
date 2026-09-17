package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTradeServiceCapabilities(t *testing.T) {
	svc := newTradeService(tradeServiceConfig{})
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	res := httptest.NewRecorder()
	svc.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	var got struct {
		Available bool     `json:"available"`
		Kind      string   `json:"kind"`
		Policies  []string `json:"policies"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Available || got.Kind != "gen1_virtual_trader" {
		t.Fatalf("capabilities = %+v", got)
	}
	if len(got.Policies) != len(virtualTraderPolicies) {
		t.Fatalf("policies = %v, want %v", got.Policies, virtualTraderPolicies)
	}
}

func TestTradeServiceRejectsBadSessionBeforeStartingBroker(t *testing.T) {
	svc := newTradeService(tradeServiceConfig{})
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"session":"bad session"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	svc.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", res.Code, res.Body.String())
	}
}

func TestTradeServiceHealth(t *testing.T) {
	svc := newTradeService(tradeServiceConfig{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()
	svc.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"available":true`) {
		t.Fatalf("health = %d %s", res.Code, res.Body.String())
	}
}
