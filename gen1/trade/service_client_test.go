package trade

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestServiceClientStartsAndWaitsForBrokerReadySession(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			var req SessionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if req.Session != "trade-1" || req.Policy != "tradeback" || req.Species != "pidgey" {
				t.Errorf("request = %+v", req)
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(SessionStatus{Session: req.Session, Policy: req.Policy, Species: req.Species, Status: "starting"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/trade-1":
			status := "starting"
			if polls.Add(1) >= 2 {
				status = "running"
			}
			_ = json.NewEncoder(w).Encode(SessionStatus{Session: "trade-1", Status: status})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sessions/trade-1":
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := ServiceClient{BaseURL: server.URL}
	ctx := t.Context()
	started, err := client.StartSession(ctx, SessionRequest{Session: "trade-1", Policy: "tradeback", Species: "pidgey"})
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != "starting" {
		t.Fatalf("start status = %q, want starting", started.Status)
	}
	ready, err := client.WaitReady(ctx, "trade-1")
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != "running" || polls.Load() < 2 {
		t.Fatalf("ready = %+v polls=%d", ready, polls.Load())
	}
	if err := client.DeleteSession(ctx, "trade-1"); err != nil {
		t.Fatal(err)
	}
}

func TestServiceClientWaitReadyReportsTerminalError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(SessionStatus{Session: "broken", Status: "error", Error: "broker unavailable"})
	}))
	defer server.Close()

	client := ServiceClient{BaseURL: server.URL, Client: &http.Client{Timeout: time.Second}}
	if _, err := client.WaitReady(t.Context(), "broken"); err == nil {
		t.Fatal("WaitReady error = nil, want terminal session error")
	}
}

func TestServiceClientSurfacesHTTPErrorEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "session already active"})
	}))
	defer server.Close()

	client := ServiceClient{BaseURL: server.URL}
	if _, err := client.StartSession(t.Context(), SessionRequest{Session: "dup"}); err == nil {
		t.Fatal("StartSession error = nil, want HTTP conflict")
	}
}
