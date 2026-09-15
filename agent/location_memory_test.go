package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnowledgeTopologySupportsNonByteSemanticLocations(t *testing.T) {
	here := LocationID("johto/bank-03/goldenrod-city")
	next := LocationID("johto/bank-03/goldenrod-pokemon-center")
	known := NewKnowledge(KnowledgeTopology{
		Adjacency: map[LocationID][]LocationID{
			here: {next},
		},
	})
	known.SawLocation(here)

	if !known.Visited[here] {
		t.Fatalf("visited = %v; semantic location %q was not retained", known.Visited, here)
	}
	hops := mapHops(known.Adjacency, here)
	if got := hops[next]; got != 1 {
		t.Fatalf("hops[%q] = %d, want 1", next, got)
	}
}

func TestMemoryV6SerializationIsDeterministic(t *testing.T) {
	topology := KnowledgeTopology{Adjacency: map[LocationID][]LocationID{
		"region/bank-2/map-b": {"region/bank-1/map-a"},
		"region/bank-1/map-a": {"region/bank-2/map-b"},
	}}
	build := func(reverse bool) *Knowledge {
		k := NewKnowledge(topology)
		locations := []LocationID{"region/bank-1/map-a", "region/bank-2/map-b"}
		places := []string{"beta place", "alpha place"}
		if reverse {
			locations[0], locations[1] = locations[1], locations[0]
			places[0], places[1] = places[1], places[0]
		}
		for _, location := range locations {
			k.SawLocation(location)
		}
		for _, place := range places {
			k.Places[place] = true
		}
		k.TalkedAt(locations[0], 8, 4)
		k.TalkedAt(locations[1], 2, 9)
		k.Completed["z-objective"] = 2
		k.Completed["a-objective"] = 1
		k.Failures["z-failure"] = Failure{Objective: "z", Times: 2, Last: "later"}
		k.Failures["a-failure"] = Failure{Objective: "a", Times: 1, Last: "earlier"}
		return k
	}

	first, err := encodeMemoryFile(build(false), "semantic migration", 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := encodeMemoryFile(build(true), "semantic migration", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("v6 serialization depends on map insertion order:\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestMemoryMigratesV5NativeGeographyToSemanticLocations(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-004-frame-0000001234-go-to-route.state")
	if err := os.WriteFile(statePath, []byte("state-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"version":5,"visited":[1,40],"places":["pallet town"],"completed":[],"talked":[{"map":40,"x":6,"y":3}],"intent":"continue","intent_age":1}`)
	if err := os.WriteFile(knowledgePathForStateVersion(statePath, 5), legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	pallet := LocationID("kanto/overworld/pallet-town")
	lab := LocationID("kanto/interior/oaks-lab")
	topology := KnowledgeTopology{
		Adjacency: map[LocationID][]LocationID{pallet: {lab}},
		NativeLocations: map[uint8]LocationID{
			0x01: pallet,
			0x28: lab,
		},
	}
	var log bytes.Buffer
	got := LoadCheckpointMemory(statePath, topology, &log)

	if !got.Knowledge.Visited[pallet] || !got.Knowledge.Visited[lab] {
		t.Fatalf("visited = %v; want migrated semantic Pallet and Oak's Lab", got.Knowledge.Visited)
	}
	if !got.Knowledge.Talked[lab][[2]uint8{6, 3}] {
		t.Fatalf("talked = %v; want v5 map 0x28 migrated to %q at (6,3)", got.Knowledge.Talked, lab)
	}
	if got.Intent != "continue" || got.IntentAge != 1 {
		t.Fatalf("intent = (%q,%d), want (continue,1)", got.Intent, got.IntentAge)
	}
	if !strings.Contains(log.String(), "migrating v5 checkpoint geography to semantic locations") {
		t.Fatalf("migration log = %q", log.String())
	}
}
