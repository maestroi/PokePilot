package skill

// Pure tests for the dialogue recovery loop and Travel's dialogue branch.
// They drive the frameClock seam with a scripted fake, so they run without
// an emulator or a ROM and cannot be reseeded by a frame-budget edit.

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// noBattles is the battle resolver for these fake-driven travel tests: their
// fakes interrupt the walk with dialogue, never a battle, so if the resolver
// were ever called it is a bug in the test's fakes, not in travel.
func noBattles() (battleResolution, error) {
	return battleResolution{}, errors.New("test: travel's battle resolver was called but the fake never starts a battle")
}

// fakeClock is the scripted frameClock the recovery tests drive. It owns a
// RAM image and a one-knob model of the text box: when the box is open and
// A is tapped, it closes after talkSettle stepped frames, unless
// closesOnTap is false (a box that ignores A). battleIn flips the screen to
// a battle after that many stepped frames. Every call is logged in order so
// a test can assert WHEN input went out, not only what the loop decided.
type fakeClock struct {
	mem         *state.Mem
	ops         []string
	taps        int
	steps       int
	closing     int
	closesOnTap bool
	battleIn    int
	onClose     func(*state.Mem)
}

func (f *fakeClock) PeekInto(addr uint16, dst []byte) {
	f.ops = append(f.ops, "snapshot")
	// A full snapshot is 64 KiB, which overflows a uint16 bound, so slice
	// to the end of the array instead of addr+n.
	src := (*f.mem)[addr:]
	if len(dst) > len(src) {
		dst = dst[:len(src)]
	}
	copy(dst, src)
}

func (f *fakeClock) Tap(b emu.Button, holdFrames, gapFrames int) {
	f.ops = append(f.ops, "tap")
	f.taps++
	if f.closesOnTap && f.closing == 0 && f.mem.U8(sym.FontLoaded) != 0 {
		f.closing = talkSettle
	}
}

func (f *fakeClock) StepFrame() { f.step() }

func (f *fakeClock) StepFrames(n int) {
	for i := 0; i < n; i++ {
		f.step()
	}
}

func (f *fakeClock) step() {
	f.ops = append(f.ops, "step")
	f.steps++
	if f.closing > 0 {
		f.closing--
		if f.closing == 0 {
			closeBox(f.mem)
			if f.onClose != nil {
				f.onClose(f.mem)
			}
		}
	}
	if f.battleIn > 0 {
		f.battleIn--
		if f.battleIn == 0 {
			f.mem[sym.IsInBattle] = 1
			f.mem[sym.FontLoaded] = 0
			f.mem[sym.JoyIgnore] = 0
		}
	}
}

// closeBox mirrors the game: the font unloads and input comes back, but
// wTextBoxID is not cleared — it keeps its last value.
func closeBox(m *state.Mem) {
	m[sym.FontLoaded] = 0
	m[sym.JoyIgnore] = 0
	for i := 0; i < sym.TileMapLen; i++ {
		m[sym.TileMap+uint16(i)] = 0
	}
}

// newFakeRAM is an overworld RAM image: map dimensions set so
// state.Controllable can ever be true, everything else blank.
func newFakeRAM() *state.Mem {
	m := &state.Mem{}
	m[sym.CurMapWidth] = 20
	m[sym.CurMapHeight] = 18
	return m
}

// textTile is the font tile id that renders as c, per the charmap the
// decoder uses (textChars in red/state/text.go).
func textTile(c byte) byte {
	switch {
	case c >= 'A' && c <= 'Z':
		return 0x80 + (c - 'A')
	case c >= 'a' && c <= 'z':
		return 0xa0 + (c - 'a')
	}
	return 0x7f
}

// openTextBox draws an ordinary text box: font loaded, input ignored, the
// given text laid one char per tile on the tilemap's first row.
func openTextBox(m *state.Mem, text string) {
	m[sym.FontLoaded] = 1
	m[sym.JoyIgnore] = 1
	for i, c := range text {
		if c < 0x100 {
			m[sym.TileMap+uint16(i)] = textTile(byte(c))
		}
	}
}

// openChoice draws the two-option prompt shape state.DecodeTwoOptionMenu
// reads: the ordinary box plus wMaxMenuItem = 1 and the cursor glyph at
// (y, x).
func openChoice(m *state.Mem, y, x int, text string) {
	openTextBox(m, text)
	m[sym.MaxMenuItem] = 1
	m[sym.TopMenuItemY] = byte(y)
	m[sym.TopMenuItemX] = byte(x)
	m[sym.TileMap+uint16(y*20+x)] = 0xED
}

// TestRecoverDialoguePagesOrdinaryBox: a plain NPC line pages closed under
// A, and the loop's first act is the entry snapshot — the choice check —
// with every A press sent only on a fresh snapshot.
func TestRecoverDialoguePagesOrdinaryBox(t *testing.T) {
	mem := newFakeRAM()
	openTextBox(mem, "HI")
	f := &fakeClock{mem: mem, closesOnTap: true}

	res := recoverDialogue(f, 100)

	if res.Stop != DialogueRecovered {
		t.Fatalf("Stop = %d, want DialogueRecovered", res.Stop)
	}
	if res.Presses != 1 {
		t.Fatalf("Presses = %d, want 1 (one page)", res.Presses)
	}
	if len(f.ops) == 0 || f.ops[0] != "snapshot" {
		t.Fatalf("ops = %v, want the entry snapshot (and choice check) first", f.ops)
	}
	for i, op := range f.ops {
		if op != "tap" {
			continue
		}
		if i == 0 || f.ops[i-1] != "snapshot" {
			t.Fatalf("ops = %v: A at %d was not sent on a fresh snapshot", f.ops, i)
		}
	}
}

// TestRecoverDialogueRefusesChoiceOnEntry: a two-option prompt up on entry
// is a question, not a page. The loop checks the shape before its first A
// and sends zero input.
func TestRecoverDialogueRefusesChoiceOnEntry(t *testing.T) {
	mem := newFakeRAM()
	openChoice(mem, 8, 12, "HEAL")
	f := &fakeClock{mem: mem, closesOnTap: true}

	res := recoverDialogue(f, 100)

	if res.Stop != DialogueChoiceRequired {
		t.Fatalf("Stop = %d, want DialogueChoiceRequired", res.Stop)
	}
	if res.Presses != 0 || f.taps != 0 {
		t.Fatalf("Presses = %d taps = %d, want zero input on a choice", res.Presses, f.taps)
	}
	for _, op := range f.ops {
		if op == "tap" {
			t.Fatalf("ops = %v: input sent on a choice", f.ops)
		}
	}
	if len(f.ops) == 0 || f.ops[0] != "snapshot" {
		t.Fatalf("ops = %v, want the choice checked on the entry snapshot", f.ops)
	}
	if res.Text != "HEAL" {
		t.Fatalf("Text = %q, want the question on screen", res.Text)
	}
}

// TestRecoverDialogueStopsBeforePressingAOnLateChoice: the check runs
// before EVERY A, not only the first. The first page is ordinary text and
// takes one A; the prompt that appears when it closes takes none.
func TestRecoverDialogueStopsBeforePressingAOnLateChoice(t *testing.T) {
	mem := newFakeRAM()
	openTextBox(mem, "LET ME TAKE A LOOK")
	f := &fakeClock{mem: mem, closesOnTap: true}
	f.onClose = func(m *state.Mem) { openChoice(m, 8, 12, "HEAL") }

	res := recoverDialogue(f, 100)

	if res.Stop != DialogueChoiceRequired {
		t.Fatalf("Stop = %d, want DialogueChoiceRequired", res.Stop)
	}
	if res.Presses != 1 || f.taps != 1 {
		t.Fatalf("Presses = %d taps = %d, want exactly 1 (the ordinary page only)", res.Presses, f.taps)
	}
	if res.Text != "HEAL" {
		t.Fatalf("Text = %q, want the question on screen", res.Text)
	}
}

// TestRecoverDialogueIgnoresStaleTextBoxID: wTextBoxID is not a liveness
// bit — every catch leaves a stale 0x14 behind. With it set and ordinary
// text up, recovery still advances the box normally.
func TestRecoverDialogueIgnoresStaleTextBoxID(t *testing.T) {
	mem := newFakeRAM()
	openTextBox(mem, "STALE")
	mem[sym.TextBoxID] = 0x14
	f := &fakeClock{mem: mem, closesOnTap: true}

	res := recoverDialogue(f, 100)

	if res.Stop != DialogueRecovered {
		t.Fatalf("Stop = %d, want DialogueRecovered", res.Stop)
	}
	if res.Presses != 1 {
		t.Fatalf("Presses = %d, want 1", res.Presses)
	}
}

// TestRecoverDialogueBudgetExhausted: a box that ignores A runs the whole
// budget and comes back DialogueBudgetExhausted, still up, with its text.
func TestRecoverDialogueBudgetExhausted(t *testing.T) {
	mem := newFakeRAM()
	openTextBox(mem, "STUCK")
	f := &fakeClock{mem: mem, closesOnTap: false}

	res := recoverDialogue(f, 5)

	if res.Stop != DialogueBudgetExhausted {
		t.Fatalf("Stop = %d, want DialogueBudgetExhausted", res.Stop)
	}
	if res.Presses != 5 {
		t.Fatalf("Presses = %d, want 5 (one per iteration)", res.Presses)
	}
	if res.Text != "STUCK" {
		t.Fatalf("Text = %q, want the box that is still up", res.Text)
	}
}

// TestRecoverDialogueBattleAppears: a box that leads into a battle — the
// rival's "hey, wait up" — is not an ordinary text box. The loop stops the
// frame the battle starts and names it.
func TestRecoverDialogueBattleAppears(t *testing.T) {
	t.Run("mid recovery", func(t *testing.T) {
		mem := newFakeRAM()
		openTextBox(mem, "HEY WAIT UP")
		f := &fakeClock{mem: mem, closesOnTap: true, battleIn: 10}

		res := recoverDialogue(f, 100)

		if res.Stop != DialogueUnexpectedMode {
			t.Fatalf("Stop = %d, want DialogueUnexpectedMode", res.Stop)
		}
		if res.Final.Battle == nil {
			t.Fatal("Final.Battle = nil, want the battle that took the screen")
		}
	})
	t.Run("on entry", func(t *testing.T) {
		mem := newFakeRAM()
		mem[sym.IsInBattle] = 1
		f := &fakeClock{mem: mem}

		res := recoverDialogue(f, 100)

		if res.Stop != DialogueUnexpectedMode {
			t.Fatalf("Stop = %d, want DialogueUnexpectedMode", res.Stop)
		}
		if res.Presses != 0 || f.taps != 0 {
			t.Fatalf("Presses = %d taps = %d, want zero input on a battle", res.Presses, f.taps)
		}
	})
}

// TestAdvanceUntilKeepsPressingA: the wrapper the story, heal and gym code
// calls is the core with no stopBeforeA — it keeps tapping A on an open box
// exactly as before the extraction.
func TestAdvanceUntilKeepsPressingA(t *testing.T) {
	mem := newFakeRAM()
	openTextBox(mem, "PAGE")
	f := &fakeClock{mem: mem, closesOnTap: false}

	final := advanceUntil(f, 3, func(*state.Mem) bool { return false })

	if f.taps != 3 {
		t.Fatalf("taps = %d, want 3 (one per iteration)", f.taps)
	}
	if final.U8(sym.FontLoaded) == 0 {
		t.Fatal("the box closed, want it still up (the fake ignores A)")
	}
}

// TestTravelRetriesGoToAfterRecoveredBox: a recovered box is an
// interruption like a won battle — the loop re-plans and walks again.
func TestTravelRetriesGoToAfterRecoveredBox(t *testing.T) {
	var gotos, recoveries int
	goTo := func() error {
		gotos++
		if gotos == 1 {
			return ErrDialogueInterrupted
		}
		return nil
	}
	recoverBox := func() DialogueRecoveryResult {
		recoveries++
		return DialogueRecoveryResult{Stop: DialogueRecovered}
	}

	res, err := travel(nil, nil, 5, goTo, recoverBox, func() bool { return false }, noBattles)

	if err != nil {
		t.Fatalf("travel: %v", err)
	}
	if gotos != 2 {
		t.Fatalf("GoTo called %d times, want 2 (retry after the recovery)", gotos)
	}
	if recoveries != 1 {
		t.Fatalf("recovered %d boxes, want 1", recoveries)
	}
	if res.Dialogues != 1 {
		t.Fatalf("Dialogues = %d, want 1", res.Dialogues)
	}
}

// TestTravelDoesNotRetryAfterChoiceRequired: the choice is unanswered and
// the box is still up, so the next walk would meet it again forever. Travel
// stops after one GoTo and hands back the typed outcome.
func TestTravelDoesNotRetryAfterChoiceRequired(t *testing.T) {
	var gotos int
	goTo := func() error {
		gotos++
		return ErrDialogueInterrupted
	}
	recoverBox := func() DialogueRecoveryResult {
		return DialogueRecoveryResult{Stop: DialogueChoiceRequired, Text: "HEAL"}
	}

	res, err := travel(nil, nil, 5, goTo, recoverBox, func() bool { return false }, noBattles)

	var choice *ErrDialogueChoice
	if !errors.As(err, &choice) {
		t.Fatalf("err = %v, want *ErrDialogueChoice", err)
	}
	if gotos != 1 {
		t.Fatalf("GoTo called %d times, want 1 (no retry after a choice)", gotos)
	}
	if res.Dialogues != 1 {
		t.Fatalf("Dialogues = %d, want 1", res.Dialogues)
	}
	if choice.Result.Stop != DialogueChoiceRequired || choice.Result.Text != "HEAL" {
		t.Fatalf("Result = %+v, want the unanswered choice", choice.Result)
	}
}

// TestTravelBoundsDialogueRecoveries: the recovery counter bounds the loop
// the way maxBattles bounds the fight loop.
func TestTravelBoundsDialogueRecoveries(t *testing.T) {
	goTo := func() error { return ErrDialogueInterrupted }
	recoverBox := func() DialogueRecoveryResult {
		return DialogueRecoveryResult{Stop: DialogueRecovered}
	}

	res, err := travel(nil, nil, 5, goTo, recoverBox, func() bool { return false }, noBattles)

	if err == nil {
		t.Fatal("travel = nil error, want the bound to trip")
	}
	if res.Dialogues != maxDialogueRecoveries {
		t.Fatalf("Dialogues = %d, want %d", res.Dialogues, maxDialogueRecoveries)
	}
	if !strings.Contains(err.Error(), "still interrupted by a text box") {
		t.Fatalf("err = %q, want the recovery bound named", err)
	}
}

// TestTravelStopsOnBlackoutAfterRecoveredBox: the non-battle blackout. Poison
// fainted the last mon out of it while walking: no battle ever fired, the
// death surfaced as an ordinary text box, and the game set the blackout bit
// the frame the box closed. Travel must return the typed outcome instead of
// walking on — the assertion is on the error, not on a BlackedOut flag in a
// result nobody checks (that flag with a nil error is exactly today's bug),
// and on the fact that the walk is not retried: re-planning from the
// respawn spot is the caller's decision, made knowing the party just lost
// (the game heals it there, but the loss is the fact that matters).
func TestTravelStopsOnBlackoutAfterRecoveredBox(t *testing.T) {
	var gotos, blackouts int
	goTo := func() error {
		gotos++
		return ErrDialogueInterrupted
	}
	recoverBox := func() DialogueRecoveryResult {
		return DialogueRecoveryResult{Stop: DialogueRecovered}
	}
	blackout := func() bool {
		blackouts++
		return true
	}

	res, err := travel(nil, nil, 5, goTo, recoverBox, blackout, noBattles)

	if !errors.Is(err, ErrBlackedOut) {
		t.Fatalf("err = %v, want ErrBlackedOut", err)
	}
	if gotos != 1 {
		t.Fatalf("GoTo called %d times, want 1 (no walk after a blackout)", gotos)
	}
	if blackouts != 1 {
		t.Fatalf("blackout checked %d times, want 1 (only after the recovered box)", blackouts)
	}
	if !res.BlackedOut {
		t.Error("BlackedOut = false, want true in the returned result")
	}
	if res.Battles != 0 {
		t.Errorf("Battles = %d, want 0 (a poison death is not a battle)", res.Battles)
	}
	if res.Dialogues != 1 {
		t.Errorf("Dialogues = %d, want 1 (the box the blackout's text came in)", res.Dialogues)
	}
}

// TestTravelIgnoresClearedBlackoutBit: the bit is a fact about the world at
// the stop, not a standing condition — a recovered box with the bit clear
// (the ordinary sign or NPC line) walks on exactly as before.
func TestTravelIgnoresClearedBlackoutBit(t *testing.T) {
	var gotos, blackouts int
	goTo := func() error {
		gotos++
		if gotos == 1 {
			return ErrDialogueInterrupted
		}
		return nil
	}
	recoverBox := func() DialogueRecoveryResult {
		return DialogueRecoveryResult{Stop: DialogueRecovered}
	}
	blackout := func() bool {
		blackouts++
		return false
	}

	res, err := travel(nil, nil, 5, goTo, recoverBox, blackout, noBattles)

	if err != nil {
		t.Fatalf("travel: %v", err)
	}
	if gotos != 2 {
		t.Fatalf("GoTo called %d times, want 2 (the walk resumes after a recovered box)", gotos)
	}
	if blackouts != 1 {
		t.Fatalf("blackout checked %d times, want 1", blackouts)
	}
	if res.BlackedOut {
		t.Error("BlackedOut = true, want false")
	}
}
