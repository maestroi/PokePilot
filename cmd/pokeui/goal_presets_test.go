package main

import (
	"strings"
	"testing"
)

func TestOperatorIndexUsesDeterministicGoalPresets(t *testing.T) {
	page := string(operatorIndexPage())
	if strings.Contains(page, `<input name="goal"`) {
		t.Fatal("operator page still exposes the old unrestricted goal input")
	}
	for _, want := range []string{
		`<select name="goal">`,
		`value="Earn the Boulder Badge." selected`,
		`value="Earn all 8 badges."`,
		`value="Beat the Elite Four and Champion."`,
		`Free play (no automatic stop)`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("operator page missing goal preset %q", want)
		}
	}
}
