package agent

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestRuntimePolicyDependencies protects the generic objective/runtime half of
// agent from accidentally reaching back into the Red/controller implementation.
// Concrete bindings are isolated in red_* / gen1_* adapter files; planner,
// recovery, transaction lifecycle, and portable result verification stay clean.
func TestRuntimePolicyDependencies(t *testing.T) {
	files := []string{
		"game_adapter.go",
		"execute_structured.go",
		"objective_result.go",
		"run_engine_policy.go",
		"run_engine_boundary.go",
		"recovery.go",
		"prerequisite_recovery.go",
		"plan.go",
		"failure_identity.go",
		"normalized_failure.go",
	}
	forbidden := []string{
		"github.com/maestroi/pokepilot/red/",
		"github.com/maestroi/pokepilot/emu",
		"github.com/maestroi/pokepilot/skill",
		"github.com/maestroi/pokepilot/world",
	}
	for _, name := range files {
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", name, err)
			}
			for _, prefix := range forbidden {
				if strings.HasPrefix(path, prefix) {
					t.Errorf("%s imports concrete game/controller package %q; keep generic objective/runtime code adapter-only", name, path)
				}
			}
		}
	}
}

func TestGenericRunLoopDoesNotImportRedImplementation(t *testing.T) {
	for _, name := range []string{"run.go", "objective_adapter_registry.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", name, err)
			}
			if strings.HasPrefix(path, "github.com/maestroi/pokepilot/red/") ||
				strings.HasPrefix(path, "github.com/maestroi/pokepilot/skill") {
				t.Errorf("%s imports Gen-I implementation package %q; bind it through the registered objective adapter", name, path)
			}
		}
	}
}

func TestRedObjectiveDispatcherStaysInAdapterFile(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(data), "func executeRedOwned(") && !strings.HasPrefix(name, "red_") {
			t.Errorf("%s owns executeRedOwned; concrete objective dispatch must stay in a red_* adapter file", name)
		}
	}
}
