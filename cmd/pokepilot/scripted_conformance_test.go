package main

import (
	"os"
	"strings"
	"testing"
)

func functionSource(t *testing.T, path, name string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	start := strings.Index(text, "func "+name+"(")
	if start < 0 {
		t.Fatalf("%s has no %s", path, name)
	}
	chunk := text[start:]
	if next := strings.Index(chunk[1:], "\nfunc "); next >= 0 {
		chunk = chunk[:next+1]
	}
	return chunk
}

func TestProductionScriptedGameplayUsesObjectiveTransactions(t *testing.T) {
	for _, tc := range []struct {
		path string
		name string
	}{
		{"main.go", "runScripted"},
		{"farm.go", "runFarmScripted"},
		{"farm.go", "runFarmLLM"},
	} {
		src := functionSource(t, tc.path, tc.name)
		for _, bypass := range []string{"skill.GetStarter(", "skill.GoTo(", "skill.Travel("} {
			if strings.Contains(src, bypass) {
				t.Fatalf("%s.%s bypasses objective transaction via %s", tc.path, tc.name, bypass)
			}
		}
		if tc.name == "runFarmLLM" && !strings.Contains(src, "executeScriptedObjective") {
			t.Fatalf("%s.%s does not transactionize preselected starter setup", tc.path, tc.name)
		}
		if tc.name != "runFarmLLM" && !strings.Contains(src, "executeScriptedObjective") {
			t.Fatalf("%s.%s does not execute scripted objectives through the shared transaction helper", tc.path, tc.name)
		}
	}

	helper := functionSource(t, "scripted_objective.go", "executeScriptedObjective")
	if !strings.Contains(helper, "agent.Execute(") {
		t.Fatal("executeScriptedObjective no longer delegates to the common objective transaction API")
	}
}
