package main

import (
	"bytes"
	"testing"
)

func TestOperatorIndexIncludesClassicPlayStyleSelector(t *testing.T) {
	page := operatorIndexPage()
	checks := [][]byte{
		[]byte(`id="play-style-script"`),
		[]byte(`select.name = "play_style"`),
		[]byte(`Adventure · natural play`),
		[]byte(`Speedrun · progression first`),
		[]byte(`Completionist · explore and collect`),
		[]byte(`Team Builder · catches and training`),
		[]byte(`spec.play_style = select.value || "adventure"`),
	}
	for _, want := range checks {
		if !bytes.Contains(page, want) {
			t.Fatalf("operator index missing %q", want)
		}
	}
}
