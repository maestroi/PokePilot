package gen1rom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedROMPackageHasNoConcreteGameDependencies(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"/red/",
			"/blue/",
			"/yellow/",
		} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s depends on concrete game package %q", entry.Name(), forbidden)
			}
		}
	}
}
