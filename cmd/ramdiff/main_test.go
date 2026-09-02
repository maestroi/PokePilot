package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunReportsChangedAddressesAndLimit(t *testing.T) {
	dir := t.TempDir()
	before := make([]byte, addressSpaceSize)
	after := make([]byte, addressSpaceSize)
	after[0xC000] = 0x12
	after[0xC001] = 0x34
	beforePath := filepath.Join(dir, "before.ram")
	afterPath := filepath.Join(dir, "after.ram")
	if err := os.WriteFile(beforePath, before, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(afterPath, after, 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := run([]string{"-start", "0xC000", "-end", "0xC010", "-limit", "1", beforePath, afterPath}, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"0xC000  00 -> 12",
		"... 1 more changed address(es) not shown",
		"2 changed address(es) in 0xC000..0xC010",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "0xC001") {
		t.Fatalf("limit did not suppress second address: %q", got)
	}
}

func TestRunRejectsWrongSizedSnapshot(t *testing.T) {
	dir := t.TempDir()
	short := filepath.Join(dir, "short.ram")
	full := filepath.Join(dir, "full.ram")
	if err := os.WriteFile(short, []byte{1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, make([]byte, addressSpaceSize), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{short, full}, bytes.NewBuffer(nil)); err == nil || !strings.Contains(err.Error(), "want exactly 65536") {
		t.Fatalf("wrong-size error = %v", err)
	}
}
