package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeDebugName(t *testing.T) {
	if got := safeDebugName("run/a b"); got != "run_a_b" {
		t.Fatalf("safeDebugName = %q", got)
	}
}

func TestParseGrepLine(t *testing.T) {
	path, line, ok := parseGrepLine("skill/travel.go:42:return fmt.Errorf(\"text box interrupted movement\")")
	if !ok || path != "skill/travel.go" || line != 42 {
		t.Fatalf("parseGrepLine = %q %d %v", path, line, ok)
	}
	if _, _, ok := parseGrepLine("not-a-match"); ok {
		t.Fatal("invalid grep line parsed successfully")
	}
}

func TestSourceSnippetIsBoundedAroundMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, "line-"+strings.Repeat("x", i%5))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := sourceSnippet(path, 15, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "13:") || !strings.Contains(got, "17:") || strings.Contains(got, "12:") || strings.Contains(got, "18:") {
		t.Fatalf("snippet = %q", got)
	}
}

func TestPortableFileHashesBytes(t *testing.T) {
	got := portableFile("checkpoint.state", []byte("state"))
	if got.Name != "checkpoint.state" || got.Size != 5 || len(got.SHA256) != 64 {
		t.Fatalf("portableFile = %+v", got)
	}
}
