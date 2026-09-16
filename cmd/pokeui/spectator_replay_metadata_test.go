package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSpectatorPublicMetadataIncludesAllowlistedModelProfile(t *testing.T) {
	var source spectatorSourceRun
	if err := json.Unmarshal([]byte(`{
		"run_id":"replay-model-metadata",
		"status":"done",
		"llm_profile":"qwen-9b",
		"play_style":"speedrunner",
		"detail":"private failure detail"
	}`), &source); err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(source.spectatorRun)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"llm_profile":"qwen-9b"`)) {
		t.Fatalf("public run missing allowlisted model profile: %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(`"play_style":"speedrunner"`)) {
		t.Fatalf("public run missing play style: %s", encoded)
	}
	if bytes.Contains(encoded, []byte("private failure detail")) || bytes.Contains(encoded, []byte(`"detail"`)) {
		t.Fatalf("public run leaked private detail: %s", encoded)
	}
}
