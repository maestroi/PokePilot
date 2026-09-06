package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestFleeMenuFromMemRecognizesSafariBattleMenu(t *testing.T) {
	normal := newFakeRAM()
	openTextBox(normal, "FIGHT ITEM PKMN RUN")
	if got := fleeMenuFromMem(normal); got != fleeMenuNormal {
		t.Fatalf("normal menu classified as %d, want %d", got, fleeMenuNormal)
	}

	safari := newFakeRAM()
	openTextBox(safari, "BALL BAIT THROW ROCK RUN")
	if got := fleeMenuFromMem(safari); got != fleeMenuSafari {
		t.Fatalf("Safari menu classified as %d, want %d", got, fleeMenuSafari)
	}

	text := newFakeRAM()
	openTextBox(text, "A wild NIDORAN appeared")
	if got := fleeMenuFromMem(text); got != fleeMenuNone {
		t.Fatalf("ordinary battle text classified as %d, want no menu", got)
	}
}

func TestSafariRunNextInputUsesLiveMenuState(t *testing.T) {
	mem := newFakeRAM()
	openTextBox(mem, "BALL BAIT THROW ROCK RUN")

	mem[sym.TopMenuItemX] = safariBattleMenuLeftX
	mem[sym.CurrentMenuItem] = 0
	if btn, done := safariRunNextInput(mem); done || btn != emu.Down {
		t.Fatalf("top-left -> (%v,%v), want Down,false", btn, done)
	}

	mem[sym.CurrentMenuItem] = mainMenuMax
	if btn, done := safariRunNextInput(mem); done || btn != emu.Right {
		t.Fatalf("bottom-left -> (%v,%v), want Right,false", btn, done)
	}

	mem[sym.TopMenuItemX] = safariBattleMenuRightX
	if btn, done := safariRunNextInput(mem); !done || btn != 0 {
		t.Fatalf("RUN -> (%v,%v), want zero,true", btn, done)
	}
	if !safariRunCursor(mem) {
		t.Fatal("Safari RUN cursor should be positively recognized")
	}
}

func TestSafariRunCursorRequiresSafariMenu(t *testing.T) {
	mem := newFakeRAM()
	openTextBox(mem, "FIGHT ITEM PKMN RUN")
	mem[sym.TopMenuItemX] = safariBattleMenuRightX
	mem[sym.CurrentMenuItem] = mainMenuMax
	if safariRunCursor(mem) {
		t.Fatal("cursor coordinates alone must not classify a normal battle menu as Safari RUN")
	}
}
