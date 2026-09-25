package agent

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/world"
)

// run-jxh8lk19wv6on resumed from knowledge written before training areas were
// learned: Visited listed Routes 13-15 and Pokemon Mansion, TrainingAreas was
// empty.
// Combat preparation then had no area to route to and the planner flew between
// grassless towns forever ("seek stronger encounters" on every journey).
func TestBackfillVisitedTrainingAreasSeedsObservedHabitatsOnce(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatal(err)
	}
	adjacency := map[uint8][]uint8{}
	for from, edges := range graph.Edges {
		for _, e := range edges {
			adjacency[from] = append(adjacency[from], e.To)
		}
	}
	known := testKnowledge(adjacency)
	const palletTown, ceruleanCity, route14, mansion1F = 0x00, 0x03, 0x19, 0xa5
	for _, id := range []uint8{palletTown, ceruleanCity, route14, mansion1F} {
		known.SawLocation(known.locationForNative(id))
	}
	obs := Observation{GameID: testGameID}

	backfillVisitedTrainingAreas(romData, obs, known)

	if !known.TrainingAreasBackfilled {
		t.Fatal("backfill did not record that it ran")
	}
	for _, id := range []uint8{route14, mansion1F} {
		area, ok := known.TrainingAreas[known.locationForNative(id)]
		if !ok || area.Place == "" || area.MaxLevel < 25 {
			t.Fatalf("visited habitat %#04x = %+v (ok=%v), want seeded high-level area; all=%+v", id, area, ok, known.TrainingAreas)
		}
	}
	for _, id := range []uint8{palletTown, ceruleanCity} {
		if area, ok := known.TrainingAreas[known.locationForNative(id)]; ok {
			t.Fatalf("grassless town %#04x seeded as training area %+v", id, area)
		}
	}

	// A seeded area later forgotten as unreachable must stay forgotten.
	delete(known.TrainingAreas, known.locationForNative(route14))
	backfillVisitedTrainingAreas(romData, obs, known)
	if _, ok := known.TrainingAreas[known.locationForNative(route14)]; ok {
		t.Fatal("forgotten training area was re-seeded")
	}
}
