package game

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestPortablePackageDependencies gives the architecture contract executable
// teeth. The game package is the portable objective runtime; importing Red,
// emulator, skill, or current world implementation packages here would turn a
// future game into a fork instead of an adapter.
func TestPortablePackageDependencies(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", entry.Name(), err)
			}
			for _, forbidden := range []string{
				"github.com/maestroi/pokepilot/red/",
				"github.com/maestroi/pokepilot/emu",
				"github.com/maestroi/pokepilot/skill",
				"github.com/maestroi/pokepilot/world",
			} {
				if strings.HasPrefix(path, forbidden) {
					t.Errorf("%s imports concrete game/runtime package %q; keep game/ portable and put that dependency in an adapter", entry.Name(), path)
				}
			}
		}
	}
}
