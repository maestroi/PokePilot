package agent

import (
	"os"
	"testing"

	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

// Yellow offers through its own adapter, so the Gen-I service-talk filter must
// still drop the Pokécenter nurse (farm run-1jc1gst1w5tv2f, #2183).
func TestYellowOfferDropsServiceTalk(t *testing.T) {
	path := os.Getenv("POKEMON_YELLOW_ROM")
	if path == "" {
		t.Skip("POKEMON_YELLOW_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	obs := Observation{GameID: yellowprofile.GameID, Map: 41} // Viridian Pokécenter
	nurse := Objective{Kind: KindTalk, X: 3, Y: 1}
	person := Objective{Kind: KindTalk, X: 10, Y: 5}
	got := filterGen1EngineServiceTalk(romData, obs, ObjectiveOffer{Candidates: []Objective{nurse, person}}).Candidates
	if len(got) != 1 || got[0] != person {
		t.Fatalf("candidates = %v, want only %v", got, person)
	}
}
