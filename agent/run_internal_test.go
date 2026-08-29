package agent

import (
	"reflect"
	"testing"
)

// TestDialogueTapeCollapsesGrowingPage catches the prompt-flood regression:
// Gen 1 can pause long enough mid-line for each growing prefix to settle.
// Those prefixes are one page, not separate dialogue lines. Closing the box
// resets that relationship, so the same text in a later page appends.
func TestDialogueTapeCollapsesGrowingPage(t *testing.T) {
	d := &dialogueTape{}
	for _, text := range []string{
		"PIKACHU is", "PIKACHU is",
		"PIKACHU is trying to learn", "PIKACHU is trying to learn",
		"PIKACHU is trying to learn THUNDER!", "PIKACHU is trying to learn THUNDER!",
	} {
		d.observeText(text)
	}

	want := []string{"PIKACHU is trying to learn THUNDER!"}
	if got := d.recent(); !reflect.DeepEqual(got, want) {
		t.Fatalf("growing <CONT> page = %q, want one completed page %q", got, want)
	}

	for _, text := range []string{"", "PIKACHU is", "PIKACHU is"} {
		d.observeText(text)
	}
	want = []string{"PIKACHU is trying to learn THUNDER!", "PIKACHU is"}
	if got := d.recent(); !reflect.DeepEqual(got, want) {
		t.Fatalf("text after box close = %q, want a separate page %q", got, want)
	}
}
