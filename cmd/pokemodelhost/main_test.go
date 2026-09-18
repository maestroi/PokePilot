package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

func TestProductionHostConfigLoads(t *testing.T) {
	cfg, err := loadConfig(filepath.Join("..", "..", "deploy", "modelhost-4090.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HostID != "inference-4090" || cfg.Listen != "192.168.50.81:8091" {
		t.Fatalf("host config = %#v", cfg)
	}
	seen := map[string]bool{}
	for _, model := range cfg.Models {
		seen[model.DeploymentID] = true
		if model.APIModel != "pokepilot-4090" {
			t.Fatalf("%s api_model = %q", model.DeploymentID, model.APIModel)
		}
		if model.Command == "" || len(model.Args) == 0 {
			t.Fatalf("%s missing launch command", model.DeploymentID)
		}
	}
	if !seen["qwen35-4b-4090"] {
		t.Fatal("missing 4B deployment")
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

func TestLifecycleEnforcesParallelWorkerLimit(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer health.Close()
	service, err := newLifecycleService(hostConfig{
		HostID: "gpu", Compute: "test", PollEvery: "1ms", LoadTimeout: "1s",
		Models: []hostModel{{DeploymentID: "m", ModelID: "m", Endpoint: health.URL, HealthURL: health.URL, APIModel: "m", MaxParallelWorkers: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.handler())
	defer server.Close()
	acquire := func(run string) int {
		data, _ := json.Marshal(leaseRequest{RunID: run, DeploymentID: "m", MaxParallelWorkers: 2})
		resp, err := http.Post(server.URL+"/v1/leases/acquire", "application/json", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := acquire("one"); code != http.StatusAccepted {
		t.Fatalf("first acquire = %d", code)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && service.status().State != "ready" {
		time.Sleep(time.Millisecond)
	}
	if code := acquire("two"); code != http.StatusOK {
		t.Fatalf("second acquire = %d", code)
	}
	if code := acquire("three"); code != http.StatusTooManyRequests {
		t.Fatalf("third acquire = %d, want 429", code)
	}
}

func TestLifecycleFollowsRequestedLeaseLimitNotHostJSON(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer health.Close()
	service, err := newLifecycleService(hostConfig{
		HostID: "gpu", Compute: "test", PollEvery: "1ms", LoadTimeout: "1s",
		Models: []hostModel{{DeploymentID: "m", ModelID: "m", Endpoint: health.URL, HealthURL: health.URL, APIModel: "m", MaxParallelWorkers: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.handler())
	defer server.Close()
	acquire := func(run string, limit int) int {
		data, _ := json.Marshal(leaseRequest{RunID: run, DeploymentID: "m", MaxParallelWorkers: limit})
		resp, err := http.Post(server.URL+"/v1/leases/acquire", "application/json", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := acquire("one", 2); code != http.StatusAccepted {
		t.Fatalf("first acquire = %d", code)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && service.status().State != "ready" {
		time.Sleep(time.Millisecond)
	}
	if code := acquire("two", 2); code != http.StatusOK {
		t.Fatalf("second acquire at wall ceiling 2 = %d, host JSON cap must not block interleaved farm leases", code)
	}
}


func TestEndpointReadyRejectsWrongAdvertisedModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "other-model"}},
		})
	}))
	defer server.Close()

	service, err := newLifecycleService(hostConfig{
		HostID: "gpu", Compute: "test",
		Models: []hostModel{{
			DeploymentID: "wanted", ModelID: "wanted", Endpoint: server.URL + "/v1",
			HealthURL: server.URL, APIModel: "wanted-alias",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.endpointReady(service.models["wanted"]) {
		t.Fatal("endpoint with wrong advertised model must not be ready")
	}
}
