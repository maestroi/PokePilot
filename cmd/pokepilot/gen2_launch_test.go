package main

import "testing"

func TestGen2LaunchIdentity(t *testing.T) {
	for _, gameID := range []string{"pokemon-gold", "pokemon-silver"} {
		if !isGen2GameID(gameID) {
			t.Fatalf("%s not recognized as Gen2", gameID)
		}
	}
	if isGen2GameID("pokemon-yellow") {
		t.Fatal("Yellow unexpectedly recognized as Gen2")
	}
	for _, starter := range []string{"", "chikorita", "cyndaquil", "totodile"} {
		if !isGen2StarterRequest(starter) {
			t.Fatalf("starter %q rejected", starter)
		}
	}
	for _, starter := range []string{"squirtle", "pikachu", "mewtwo", "random:any"} {
		if isGen2StarterRequest(starter) {
			t.Fatalf("starter %q unexpectedly accepted", starter)
		}
	}
}
