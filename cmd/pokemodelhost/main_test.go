package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLifecycleRejectsSwitchWhileRunActive(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer health.Close()
	service, err := newLifecycleService(hostConfig{
		HostID: "gpu-4090", Compute: "RTX 4090", PollEvery: "1ms", LoadTimeout: "1s",
		Models: []hostModel{
			{DeploymentID: "qwen-4b", ModelID: "qwen3.5-4b", Endpoint: health.URL, HealthURL: health.URL, APIModel: "pokepilot-4090"},
			{DeploymentID: "qwen-9b", ModelID: "qwen3.5-9b", Endpoint: health.URL, HealthURL: health.URL, APIModel: "pokepilot-4090"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.handler())
	defer server.Close()

	post := func(path string, body any) (*http.Response, hostStatus) {
		t.Helper()
		data, _ := json.Marshal(body)
		resp, err := http.Post(server.URL+path, "application/json", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		var status hostStatus
		_ = json.NewDecoder(resp.Body).Decode(&status)
		resp.Body.Close()
		return resp, status
	}

	resp, _ := post("/v1/leases/acquire", leaseRequest{RunID: "run-a", DeploymentID: "qwen-4b"})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("acquire status = %d", resp.StatusCode)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if service.status().State == "ready" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := service.status(); got.State != "ready" || got.ActiveLeases != 1 {
		t.Fatalf("status = %#v", got)
	}

	resp, _ = post("/v1/load", loadRequest{DeploymentID: "qwen-9b"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("switch status = %d, want 409", resp.StatusCode)
	}

	resp, _ = post("/v1/leases/release", leaseRequest{RunID: "run-a", DeploymentID: "qwen-4b"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("release status = %d", resp.StatusCode)
	}
	resp, _ = post("/v1/load", loadRequest{DeploymentID: "qwen-9b"})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("second load status = %d", resp.StatusCode)
	}
}

func TestLifecycleBearerAuth(t *testing.T) {
	t.Setenv("MODELHOST_TEST_TOKEN", "secret")
	service, err := newLifecycleService(hostConfig{HostID: "gpu", Compute: "test", TokenEnv: "MODELHOST_TEST_TOKEN", Models: []hostModel{{DeploymentID: "m", ModelID: "m", Endpoint: "http://127.0.0.1:1/v1", APIModel: "m"}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.handler())
	defer server.Close()
	resp, err := http.Get(server.URL + "/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authorized status = %d", resp.StatusCode)
	}
}
