package main

import (
	"errors"
	"os/exec"
	"testing"
)

func TestConsoleBehaviorJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if errors.Is(err, exec.ErrNotFound) {
		t.Skip("node unavailable; skipping executable console behavior tests")
	}
	if err != nil {
		t.Fatalf("locate node: %v", err)
	}
	cmd := exec.Command(node, "--test", "ui/behavior_test.js", "ui/run_cleanup_test.js")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("console behavior tests: %v\n%s", err, output)
	}
}
