package main

import "testing"

// A scripted spec may leave the starter empty: that asks for the loaded
// game's own opening (Yellow's Pikachu), resolved against the cartridge at
// run time. A named starter must still be one the runner knows.
func TestValidateSpecScriptedStarter(t *testing.T) {
	if err := validateSpec("scripted", "", "viridian pokemon center"); err != nil {
		t.Fatalf("empty scripted starter rejected: %v", err)
	}
	if err := validateSpec("scripted", "squirtle", "viridian pokemon center"); err != nil {
		t.Fatalf("squirtle rejected: %v", err)
	}
	if err := validateSpec("scripted", "not-a-pokemon", "viridian pokemon center"); err == nil {
		t.Fatal("unknown scripted starter validated")
	}
}
