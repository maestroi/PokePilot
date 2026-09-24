package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func trainingAreaTestCatalog(destinations ...CatalogDestination) ObjectiveCatalog {
	return ObjectiveCatalog{Destinations: destinations}
}

func TestRememberTrainingAreaLearnsObservedBand(t *testing.T) {
	location := LocationID("kanto/route/route-10")
	obs := Observation{
		Location: PlaceID(location),
		HasGrass: true,
		WildGrass: []WildSpecies{
			{Name: "voltorb", MinLevel: 14, MaxLevel: 16, Slots: 5},
			{Name: "spearow", MinLevel: 13, MaxLevel: 17, Slots: 5},
		},
		Catalog: trainingAreaTestCatalog(CatalogDestination{
			Place: "route 10", Location: location,
		}),
	}
	known := NewKnowledge(KnowledgeTopology{Adjacency: map[LocationID][]LocationID{location: nil}})

	noteObservation(known, obs)

	got, ok := known.TrainingAreas[location]
	if !ok {
		t.Fatalf("training area was not learned: %+v", known.TrainingAreas)
	}
	if got.Place != "route 10" || got.MinLevel != 13 || got.MaxLevel != 17 {
		t.Fatalf("training area = %+v, want route 10 L13-L17", got)
	}
}

func TestBestKnownTrainingPlaceRejectsUnsafeHighLevelArea(t *testing.T) {
	current := LocationID("kanto/route/route-1")
	safe := LocationID("kanto/route/route-10")
	danger := LocationID("kanto/dungeon/victory-road")
	known := NewKnowledge(KnowledgeTopology{Adjacency: map[LocationID][]LocationID{
		current: {safe, danger},
		safe:    {current},
		danger:  {current},
	}})
	known.TrainingAreas[safe] = TrainingAreaKnowledge{Location: safe, Place: "route 10", MinLevel: 13, MaxLevel: 15}
	known.TrainingAreas[danger] = TrainingAreaKnowledge{Location: danger, Place: "victory road", MinLevel: 38, MaxLevel: 42}

	obs := Observation{
		Location: PlaceID(current),
		Party: []PartyMon{
			{Species: "pikachu", Level: 12, HP: 30, MaxHP: 30},
			{Species: "wartortle", Level: 20, HP: 55, MaxHP: 60},
		},
		HasGrass:  true,
		WildGrass: []WildSpecies{{Name: "pidgey", MinLevel: 3, MaxLevel: 5, Slots: 10}},
		Training:  &TrainingEstimate{Viability: TrainingOutsideBudget},
	}
	catalog := trainingAreaTestCatalog(
		CatalogDestination{Place: "route 10", Location: safe},
		CatalogDestination{Place: "victory road", Location: danger},
	)
	got, ok := bestKnownTrainingPlace(obs, known, []string{"route 10", "victory road"}, catalog)
	if !ok {
		t.Fatal("no training area selected")
	}
	if got.Area.Place != "route 10" || got.Method != TrainingDirect {
		t.Fatalf("choice = %+v, want safe direct training on route 10", got)
	}
}

func TestBestKnownTrainingPlaceCanUseHighLevelAreaWithCarry(t *testing.T) {
	current := LocationID("kanto/route/route-1")
	mid := LocationID("kanto/route/route-10")
	high := LocationID("kanto/dungeon/victory-road")
	known := NewKnowledge(KnowledgeTopology{Adjacency: map[LocationID][]LocationID{
		current: {mid, high},
		mid:     {current},
		high:    {current},
	}})
	known.TrainingAreas[mid] = TrainingAreaKnowledge{Location: mid, Place: "route 10", MinLevel: 13, MaxLevel: 15}
	known.TrainingAreas[high] = TrainingAreaKnowledge{Location: high, Place: "victory road", MinLevel: 38, MaxLevel: 40}

	obs := Observation{
		Location: PlaceID(current),
		Party: []PartyMon{
			{Species: "pikachu", Level: 12, HP: 30, MaxHP: 30},
			{Species: "blastoise", Level: 45, HP: 120, MaxHP: 120},
		},
		HasGrass:  true,
		WildGrass: []WildSpecies{{Name: "pidgey", MinLevel: 3, MaxLevel: 5, Slots: 10}},
		Training:  &TrainingEstimate{Viability: TrainingOutsideBudget},
	}
	got, ok := bestKnownTrainingPlace(obs, known, []string{"route 10", "victory road"}, trainingAreaTestCatalog())
	if !ok {
		t.Fatal("no training area selected")
	}
	if got.Area.Place != "victory road" || got.Method != TrainingSwitch || got.Carry != 45 {
		t.Fatalf("choice = %+v, want Victory Road switch training via L45 carry", got)
	}
}

func TestTravelProviderPreservesBestTrainingAreaThroughJourneyCap(t *testing.T) {
	current := LocationID("region/current")
	training := LocationID("region/zz-training")
	adjacency := map[LocationID][]LocationID{current: {}}
	destinations := make([]CatalogDestination, 0, journeyPlaceLimit+3)
	known := NewKnowledge(KnowledgeTopology{Adjacency: adjacency})
	for i := 0; i < journeyPlaceLimit+2; i++ {
		name := PlaceID("area " + string(rune('a'+i)))
		location := LocationID("region/" + string(rune('a'+i)))
		known.Adjacency[current] = append(known.Adjacency[current], location)
		known.Adjacency[location] = []LocationID{current}
		known.Visited[location] = true
		destinations = append(destinations, CatalogDestination{Place: name, Location: location})
	}
	known.Adjacency[current] = append(known.Adjacency[current], training)
	known.Adjacency[training] = []LocationID{current}
	known.Visited[current], known.Visited[training] = true, true
	destinations = append(destinations, CatalogDestination{Place: "zz training", Location: training})
	known.TrainingAreas[training] = TrainingAreaKnowledge{Location: training, Place: "zz training", MinLevel: 20, MaxLevel: 24}

	obs := Observation{
		Location:   PlaceID(current),
		PartyCount: 1,
		Party:      []PartyMon{{Species: "ivysaur", Level: 22, HP: 60, MaxHP: 60}},
		HasGrass:   true,
		WildGrass:  []WildSpecies{{Name: "pidgey", MinLevel: 3, MaxLevel: 5}},
		Training:   &TrainingEstimate{Viability: TrainingOutsideBudget},
		Catalog:    ObjectiveCatalog{Destinations: destinations},
	}
	offer := OfferWithEvidence(obs, known)
	found := false
	for _, objective := range offer.Candidates {
		if objective.Kind == KindGoTo && objective.Place == "zz training" {
			found = true
			if !strings.Contains(objective.Note, "best known training area") {
				t.Fatalf("training journey note = %q", objective.Note)
			}
		}
	}
	if !found {
		t.Fatalf("best training destination was lost to journey cap: %+v", offer.Candidates)
	}
}

func TestCombatPreparationTravelsToBestKnownTrainingArea(t *testing.T) {
	current := LocationID("kanto/route/route-1")
	training := LocationID("kanto/route/route-10")
	known := NewKnowledge(KnowledgeTopology{Adjacency: map[LocationID][]LocationID{
		current:  {training},
		training: {current},
	}})
	known.TrainingAreas[training] = TrainingAreaKnowledge{Location: training, Place: "route 10", MinLevel: 14, MaxLevel: 16}
	lost := Objective{Kind: KindGym, Place: "pewter gym"}
	known.Failures[combatLossFailureKey(lost)] = Failure{
		Objective: lost.String(), Times: 1, ReadinessBaseline: 48, ReadinessTarget: 68,
	}

	obs := Observation{
		Location:   PlaceID(current),
		Party:      []PartyMon{{Species: "pikachu", Level: 12, HP: 30, MaxHP: 30}},
		PartyCount: 1,
		HasGrass:   true,
		WildGrass:  []WildSpecies{{Name: "pidgey", MinLevel: 3, MaxLevel: 5}},
		Training:   &TrainingEstimate{Viability: TrainingOutsideBudget},
		Catalog:    trainingAreaTestCatalog(CatalogDestination{Place: "route 10", Location: training}),
	}
	offered := []Objective{
		{Kind: KindGoTo, Place: "route 10"},
		{Kind: KindGoTo, Place: "route 10", Flee: true},
	}
	got, ok := combatPreparationObjective(obs, offered, known)
	if !ok {
		t.Fatal("combat preparation did not choose a training journey")
	}
	if got.Kind != KindGoTo || got.Place != "route 10" || !got.Flee {
		t.Fatalf("combat preparation choice = %+v, want fleeing journey to route 10", got)
	}
	if !strings.Contains(got.Note, "best known training area") {
		t.Fatalf("choice note = %q, want training-area evidence", got.Note)
	}
}

func TestTrainingAreaKnowledgePersistsInMemory(t *testing.T) {
	location := LocationID("kanto/route/route-10")
	known := NewKnowledge(nil)
	known.TrainingAreas[location] = TrainingAreaKnowledge{
		Location: location, Place: "route 10", MinLevel: 13, MaxLevel: 17,
	}
	data, err := encodeMemoryFile(known, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var mem memoryFile
	if err := json.Unmarshal(data, &mem); err != nil {
		t.Fatal(err)
	}
	restored := NewKnowledge(nil)
	restored.restore(mem)
	got, ok := restored.TrainingAreas[location]
	if !ok || got.Place != "route 10" || got.MinLevel != 13 || got.MaxLevel != 17 {
		t.Fatalf("restored training areas = %+v", restored.TrainingAreas)
	}
}
