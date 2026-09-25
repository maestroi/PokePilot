package agent

import (
	"testing"

	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestYellowKnowledgeTopologyUsesYellowMapVocabulary(t *testing.T) {
	topology := knowledgeTopologyFor(yellowprofile.GameID, map[uint8][]uint8{
		0x00: {0x0c},
		0xf8: {0x00},
	})
	if got := topology.NativeLocations[0x00]; got != "pallet town" {
		t.Fatalf("Pallet location = %q", got)
	}
	if got := topology.NativeLocations[0xf8]; got != "summer beach house" {
		t.Fatalf("Summer Beach House location = %q", got)
	}
	if got := topology.Adjacency["summer beach house"]; len(got) != 1 || got[0] != "pallet town" {
		t.Fatalf("Yellow adjacency = %+v", got)
	}
}
