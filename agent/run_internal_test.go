package agent

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
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

// TestDialogueTapeSeenMapsDrains is the check behind the Pallet-loop fix:
// maps walked through inside one Execute must reach the caller once, and
// then be gone, so Knowledge holds them and the tape does not re-report a
// map the player has left.
func TestDialogueTapeSeenMapsDrains(t *testing.T) {
	d := &dialogueTape{}
	d.noteMap(0x00)
	d.noteMap(0x0c)
	d.noteMap(0x00)

	got := d.seenMaps()
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if want := []uint8{0x00, 0x0c}; !reflect.DeepEqual(got, want) {
		t.Fatalf("seenMaps = %v, want %v", got, want)
	}
	if got := d.seenMaps(); len(got) != 0 {
		t.Fatalf("second seenMaps = %v, want empty", got)
	}
}

// TestObserveAfterClosesALeftoverTextBox is the check behind the mart
// stall: an objective that ends with a box still up made every later round
// fail on "not controllable", because no objective's job is to close one.
func TestObserveAfterClosesALeftoverTextBox(t *testing.T) {
	e := fixture.Load(t, "post_starter")
	// Stand on the lab's approach tile (5,3), directly below Oak.
	dest, ok := skill.Place("oak's lab")
	if !ok {
		t.Fatal(`Place "oak's lab" not found`)
	}
	if _, err := fixture.Travel(e, dest, skill.StatAwareMove(e.ROM()), 1); err != nil {
		t.Fatalf("travel to the lab tile: %v", err)
	}
	if err := skill.Face(e, 5, 2); err != nil {
		t.Fatalf("face Oak: %v", err)
	}
	e.Tap(emu.A, 3, 7)
	e.StepFrames(30)

	var mem state.Mem
	state.Snapshot(e, &mem)
	if state.Controllable(&mem) {
		t.Fatal("setup did not leave a text box open; the test proves nothing")
	}

	if obs := observeAfter(e, e.ROM(), nil); !obs.Controllable {
		t.Fatal("observeAfter left the box open; every objective after this one refuses to start")
	}
}

// TestDialogueTapeKeepsAWholeUtterance is the Viridian old man, as the game
// actually renders him: two pages of one box. Split, only the first page
// carries a requirement shape and the rest of the sentence is filed as
// unrelated chatter and dropped. Joined, the run keeps what he said.
func TestDialogueTapeKeepsAWholeUtterance(t *testing.T) {
	d := &dialogueTape{}
	for _, text := range []string{
		"You can't go", "You can't go",
		"You can't go\nthrough here!", "You can't go\nthrough here!",
		"This is private\nproperty!", "This is private\nproperty!",
	} {
		d.observeText(text)
	}

	got := d.recent()
	if len(got) != 1 {
		t.Fatalf("recent() = %q, want one utterance, not one entry per page", got)
	}
	if !strings.Contains(got[0], "through here!") || !strings.Contains(got[0], "private") {
		t.Fatalf("utterance = %q, want both pages of the box", got[0])
	}

	k := NewKnowledge(nil)
	k.SawDialogue(got, "VIRIDIAN_CITY", 19, 10)
	if len(k.Requirements) != 1 {
		t.Fatalf("Requirements = %v, want the wall harvested once", k.Requirements)
	}
	if !strings.Contains(k.Requirements[0].Text, "private") {
		t.Fatalf("harvested %q, want the whole sentence the game said", k.Requirements[0].Text)
	}
	if r := k.Requirements[0]; r.Place != "VIRIDIAN_CITY" || r.X != 19 || r.Y != 10 || r.Times != 1 {
		t.Fatalf("wall = %+v, want it located where the player stood", r)
	}
}

// TestFailureTallyOutlivesHistory is the round-19 retry: the failures that
// mattered had scrolled out of History, so an objective tried twice looked
// untried. The tally must still hold it, and must let go the moment the
// objective works.
func TestFailureTallyOutlivesHistory(t *testing.T) {
	k := NewKnowledge(nil)
	route2 := Objective{Kind: KindGoTo, Place: "route 2"}
	k.Failed(route2, errors.New("still interrupted by a text box"))
	k.Failed(route2, errors.New("still interrupted by a text box"))

	// Push historyCap rounds of unrelated work through the history window:
	// the failure is long gone from there.
	var history []RoundRecord
	for i := 0; i < historyCap+2; i++ {
		history = appendHistory(history, RoundRecord{Objective: "talk", Outcome: "done"})
	}
	for _, r := range history {
		if strings.Contains(r.Objective, "route 2") {
			t.Fatal("setup failed: the failure is still inside the history window")
		}
	}

	got := k.FailureList()
	if len(got) != 1 || got[0].Objective != route2.String() || got[0].Times != 2 {
		t.Fatalf("FailureList = %+v, want route 2 failed twice", got)
	}
	if !strings.Contains(got[0].Last, "text box") {
		t.Fatalf("last error = %q, want the error verbatim", got[0].Last)
	}

	// It worked: a wall that opened is not a wall.
	k.Done(route2)
	if got := k.FailureList(); len(got) != 0 {
		t.Fatalf("FailureList = %+v after success, want empty", got)
	}
}
