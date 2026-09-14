package agent

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// TestRuntimePolicyDependencies protects the policy half of agent from
// accidentally reaching back into the Red/controller implementation. Adapter
// and execution files may import these packages; run/recovery policy may not.
func TestRuntimePolicyDependencies(t *testing.T) {
	files := []string{
		"run_engine_policy.go",
		"run_engine_boundary.go",
		"recovery.go",
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
					t.Errorf("%s imports concrete game/controller package %q; keep runtime policy portable", name, path)
				}
			}
		}
	}
}
