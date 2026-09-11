package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLLMHTTPTransportCapturesLlamaTimingsWithoutChangingBody(t *testing.T) {
	body := `{"model":"qwen3.8-27b","choices":[{"message":{"role":"assistant","content":"{\"choice\":1}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":800,"completion_tokens":60,"total_tokens":860},"timings":{"cache_n":120,"prompt_n":800,"prompt_ms":1000,"prompt_per_second":800,"predicted_n":60,"predicted_ms":1000,"predicted_per_second":60}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	before := currentLLMTelemetrySeq()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", strings.NewReader(`{"model":"qwen3.8-27b"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	gotBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if string(gotBody) != body {
		t.Fatalf("body changed:\n%s", gotBody)
	}

	got, ok := latestLLMTelemetryAfter(before)
	if !ok {
		t.Fatal("no telemetry captured")
	}
	if got.Endpoint != srv.URL || got.ResponseModel != "qwen3.8-27b" {
		t.Fatalf("route = %q model = %q", got.Endpoint, got.ResponseModel)
	}
	if got.PromptTokens != 800 || got.CompletionTokens != 60 || got.CachedPromptTokens != 120 {
		t.Fatalf("tokens = %+v", got)
	}
	if got.PrefillMS != 1000 || got.PrefillTPS != 800 || got.DecodeMS != 1000 || got.DecodeTPS != 60 {
		t.Fatalf("timings = %+v", got)
	}
	if got.TimingSource != "llama.cpp" {
		t.Fatalf("timing source = %q", got.TimingSource)
	}
}

func TestLLMHTTPTransportKeepsGenericOpenAIUsage(t *testing.T) {
	body := `{"model":"generic-model","choices":[{"message":{"role":"assistant","content":"1"},"finish_reason":"stop"}],"usage":{"prompt_tokens":321,"completion_tokens":12,"total_tokens":333}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	before := currentLLMTelemetrySeq()
	resp, err := http.Post(srv.URL+"/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	got, ok := latestLLMTelemetryAfter(before)
	if !ok {
		t.Fatal("no telemetry captured")
	}
	if got.PromptTokens != 321 || got.CompletionTokens != 12 {
		t.Fatalf("usage = %+v", got)
	}
	if got.TimingSource != "" || got.PrefillTPS != 0 || got.DecodeTPS != 0 {
		t.Fatalf("generic OpenAI response invented llama timings: %+v", got)
	}
}
