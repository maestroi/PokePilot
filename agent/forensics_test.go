package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureObjectiveFailureDisabledDoesNotTouchEmulator(t *testing.T) {
	t.Setenv(RAMForensicsDirEnv, "")
	if err := captureObjectiveFailure(nil, Objective{}, errors.New("boom")); err != nil {
		t.Fatalf("disabled capture = %v, want nil", err)
	}
}

func TestRAMKeep(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        int
	}{
		{name: "unset", value: "", want: defaultRAMKeep},
		{name: "invalid", value: "wat", want: defaultRAMKeep},
		{name: "zero", value: "0", want: defaultRAMKeep},
		{name: "positive", value: "7", want: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(RAMForensicsKeepEnv, tc.value)
			if got := ramKeep(); got != tc.want {
				t.Fatalf("ramKeep() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestUniqueFailureBaseDoesNotOverwriteSameFrame(t *testing.T) {
	dir := t.TempDir()
	base := "failure-frame-0000000012-go-to-pallet"
	if err := os.WriteFile(filepath.Join(dir, base+".ram"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := uniqueFailureBase(dir, base)
	if err != nil {
		t.Fatal(err)
	}
	if want := base + "-02"; got != want {
		t.Fatalf("uniqueFailureBase = %q, want %q", got, want)
	}
}

func TestEvictRAMCapturesKeepsNewestPairs(t *testing.T) {
	dir := t.TempDir()
	for _, frame := range []string{"0000000001", "0000000002", "0000000003"} {
		base := "failure-frame-" + frame + "-x"
		if err := os.WriteFile(filepath.Join(dir, base+".ram"), []byte(frame), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, base+".json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := evictRAMCaptures(dir, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "failure-frame-0000000001-x.ram")); !os.IsNotExist(err) {
		t.Fatalf("oldest RAM still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "failure-frame-0000000001-x.json")); !os.IsNotExist(err) {
		t.Fatalf("oldest metadata still exists: %v", err)
	}
	for _, name := range []string{
		"failure-frame-0000000002-x.ram",
		"failure-frame-0000000002-x.json",
		"failure-frame-0000000003-x.ram",
		"failure-frame-0000000003-x.json",
		"notes.txt",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s was evicted: %v", name, err)
		}
	}
}
