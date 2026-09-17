package main

import "testing"

func TestNormalizeGameFlag(t *testing.T) {
	tests := map[string]string{
		"":             "auto",
		"auto":         "auto",
		"red":          "pokemon-red",
		"pokemon-red":  "pokemon-red",
		"BLUE":         "pokemon-blue",
		"pokemon-blue": "pokemon-blue",
		"yellow":       "pokemon-yellow",
	}
	for input, want := range tests {
		got, err := normalizeGameFlag(input)
		if err != nil {
			t.Fatalf("normalizeGameFlag(%q): %v", input, err)
		}
		if got != want {
			t.Errorf("normalizeGameFlag(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := normalizeGameFlag("gold"); err == nil {
		t.Fatal("unsupported game flag unexpectedly accepted")
	}
}

func TestWorldAdapters(t *testing.T) {
	if !hasWorldAdapter("pokemon-red") || !hasWorldAdapter("pokemon-blue") {
		t.Fatal("Red and Blue must both have Gen-I world adapters")
	}
	if hasWorldAdapter("pokemon-yellow") {
		t.Fatal("Yellow must not be advertised until its world adapter is implemented")
	}
}
