package main

import (
	"strings"
	"testing"
)

func TestOperatorPageIncludesTriageKeyDecorator(t *testing.T) {
	page := string(operatorIndexPage())
	for _, want := range []string{`id="triage-key-script"`, `data-triage-key`, `Triage key for cleanup`} {
		if !strings.Contains(page, want) {
			t.Fatalf("operator page missing %q", want)
		}
	}
}
