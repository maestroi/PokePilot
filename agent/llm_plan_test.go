package agent

import "testing"

func TestPlanSchemaIsSeparateFromChoiceSchema(t *testing.T) {
	choice := choiceSchema["properties"].(map[string]any)
	if _, ok := choice["steps"]; ok {
		t.Fatal("strategist fields leaked into the chooser schema")
	}
	props := planSchema["properties"].(map[string]any)
	if _, ok := props["goal"]; !ok {
		t.Fatal("plan schema missing goal")
	}
	if _, ok := props["steps"]; !ok {
		t.Fatal("plan schema missing steps")
	}
}

func TestStrategicPlanRejectsUnavailableFutureAction(t *testing.T) {
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	_, err := validateStrategicPlan(Plan{Goal: "reach pewter", Steps: []string{"go to route 1", "go to pewter city"}}, offered, 1)
	if err == nil {
		t.Fatal("strategist invented an unavailable future action")
	}
}
