package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPlannerContractUsesSemanticIDs(t *testing.T) {
	objective := reflect.TypeOf(Objective{})
	for field, want := range map[string]reflect.Type{
		"Place":   reflect.TypeOf(PlaceID("")),
		"Species": reflect.TypeOf(SpeciesID("")),
		"Item":    reflect.TypeOf(ItemID("")),
	} {
		got, ok := objective.FieldByName(field)
		if !ok || got.Type != want {
			t.Fatalf("Objective.%s type = %v, want %v", field, got.Type, want)
		}
	}

	observation := reflect.TypeOf(Observation{})
	story, _ := observation.FieldByName("Story")
	if story.Type != reflect.TypeOf(ProgressState{}) {
		t.Fatalf("Observation.Story type = %v, want ProgressState", story.Type)
	}
	party := reflect.TypeOf(PartyMon{})
	species, _ := party.FieldByName("Species")
	if species.Type != reflect.TypeOf(SpeciesID("")) {
		t.Fatalf("PartyMon.Species type = %v, want SpeciesID", species.Type)
	}
}

func TestPlannerJSONHidesRawRedMapID(t *testing.T) {
	obs := Observation{
		Map:      0x24,
		Location: "pallet town",
		Party:    []PartyMon{{Species: "pidgey", Level: 5}},
		Story:    ProgressState{{ID: ProgressSaffronGateOpen, Complete: true}},
	}
	b, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if strings.Contains(text, `"Map":`) {
		t.Fatalf("planner JSON exposes raw map id: %s", text)
	}
	for _, want := range []string{"pallet town", "pidgey", string(ProgressSaffronGateOpen)} {
		if !strings.Contains(text, want) {
			t.Fatalf("planner JSON %s does not contain semantic value %q", text, want)
		}
	}
}

func TestRedSemanticTranslation(t *testing.T) {
	if got, ok := semanticSpecies(" PIDGEY "); !ok || got != SpeciesID("pidgey") {
		t.Fatalf("semanticSpecies = %q,%v", got, ok)
	}
	if got, ok := redSpeciesID("pidgey"); !ok || got != 0x24 {
		t.Fatalf("redSpeciesID = %#02x,%v, want 0x24,true", got, ok)
	}
	if got, ok := semanticItem(" POTION "); !ok || got != ItemID("potion") {
		t.Fatalf("semanticItem = %q,%v", got, ok)
	}
	if got, ok := redItemID("potion"); !ok || got != 0x14 {
		t.Fatalf("redItemID = %#02x,%v, want 0x14,true", got, ok)
	}
}
