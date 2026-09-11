package farm

import (
	"encoding/json"
	"testing"
)

func TestLLMStatsTelemetryJSONRoundTrip(t *testing.T) {
	want := LLMStats{
		Endpoint:               "http://192.168.50.130:8002",
		ResponseModel:          "qwen3.8-27b",
		LastPromptTokens:       800,
		LastCompletionTokens:   60,
		LastCachedPromptTokens: 120,
		PrefillMS:              1000,
		PrefillTPS:             800,
		DecodeMS:               1000,
		DecodeTPS:              60,
		OverheadMS:             25,
		TimingSource:           "llama.cpp",
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got LLMStats
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Endpoint != want.Endpoint || got.ResponseModel != want.ResponseModel ||
		got.LastPromptTokens != want.LastPromptTokens || got.LastCompletionTokens != want.LastCompletionTokens ||
		got.LastCachedPromptTokens != want.LastCachedPromptTokens || got.PrefillTPS != want.PrefillTPS ||
		got.DecodeTPS != want.DecodeTPS || got.TimingSource != want.TimingSource {
		t.Fatalf("telemetry roundtrip = %+v, want %+v", got, want)
	}
}
