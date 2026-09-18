package farm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeOpenAIEndpointDiscoversSingleModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "qwen3.5-9b"}},
		})
	}))
	defer server.Close()

	got, err := ProbeOpenAIEndpoint(context.Background(), server.Client(), ModelDeployment{
		ID: "farm-gpu", Compute: "RX 7900 XTX", Endpoint: server.URL + "/v1",
		DiscoverModel: true, APIModel: "qwen3.8-27b", ModelID: "qwen3.8-27b",
		Revision: "old", Artifact: "old.gguf", Quantization: "Q4_K_S",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.APIModel != "qwen3.5-9b" || got.ModelID != "qwen3.5-9b" {
		t.Fatalf("discovered model = %#v", got)
	}
	if got.Revision != "" || got.Artifact != "" || got.Quantization != "" {
		t.Fatalf("stale artifact identity survived model change: %#v", got)
	}
}

func TestProbeOpenAIEndpointValidatesConfiguredModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "a"}, {"id": "b"}},
		})
	}))
	defer server.Close()

	_, err := ProbeOpenAIEndpoint(context.Background(), server.Client(), ModelDeployment{
		ID: "cloud", ModelID: "c", Compute: "cloud", Endpoint: server.URL + "/v1", APIModel: "c",
	})
	if err == nil {
		t.Fatal("expected configured model mismatch")
	}
}

func TestProbeOpenAIEndpointRequiresSelectionForMultiModelDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "a"}, {"id": "b"}},
		})
	}))
	defer server.Close()

	_, err := ProbeOpenAIEndpoint(context.Background(), server.Client(), ModelDeployment{
		ID: "shared", Compute: "cloud", Endpoint: server.URL + "/v1", DiscoverModel: true,
	})
	if err == nil {
		t.Fatal("expected ambiguous model discovery error")
	}
}


func TestProbeOpenAIEndpointPreservesLogicalModelBehindAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "stable-cloud-alias"}},
		})
	}))
	defer server.Close()

	got, err := ProbeOpenAIEndpoint(context.Background(), server.Client(), ModelDeployment{
		ID: "cloud", ModelID: "provider-model-v2", Revision: "provider-revision",
		Artifact: "provider://model-v2", Quantization: "fp8",
		Compute: "cloud", Endpoint: server.URL + "/v1",
		APIModel: "stable-cloud-alias", Enabled: true, DiscoverModel: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.APIModel != "stable-cloud-alias" || got.ModelID != "provider-model-v2" {
		t.Fatalf("alias discovery changed logical identity: %#v", got)
	}
	if got.Revision != "provider-revision" || got.Artifact != "provider://model-v2" || got.Quantization != "fp8" {
		t.Fatalf("alias discovery cleared valid metadata: %#v", got)
	}
}
