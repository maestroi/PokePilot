package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvictRAMCapturesRemovesCheckedStateWithBundle(t *testing.T) {
	dir := t.TempDir()
	writeBundle := func(base string) {
		t.Helper()
		for _, ext := range []string{".ram", ".state", ".json"} {
			if err := os.WriteFile(filepath.Join(dir, base+ext), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	oldBase := "failure-frame-0000000001-x"
	newBase := "failure-frame-0000000002-x"
	writeBundle(oldBase)
	writeBundle(newBase)

	if err := evictRAMCaptures(dir, failurePrefix, 1); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".ram", ".state", ".json"} {
		if _, err := os.Stat(filepath.Join(dir, oldBase+ext)); !os.IsNotExist(err) {
			t.Fatalf("old bundle %s survived eviction: %v", ext, err)
		}
		if _, err := os.Stat(filepath.Join(dir, newBase+ext)); err != nil {
			t.Fatalf("new bundle %s was evicted: %v", ext, err)
		}
	}
}
