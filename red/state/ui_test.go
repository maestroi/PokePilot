package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeMenuClosed(t *testing.T) {
	var m Mem
	m[sym.FontLoaded] = 0
	// Stale RAM from a previous menu/text box must not be read as open.
	m[sym.CurrentMenuItem] = 3
	m[sym.MaxMenuItem] = 4
	m[sym.TextBoxID] = 0x2A

	if s := DecodeMenu(&m); s != nil {
		t.Errorf("DecodeMenu = %+v, want nil when FontLoaded is 0", s)
	}
	if d := DecodeDialogue(&m); d != nil {
		t.Errorf("DecodeDialogue = %+v, want nil when FontLoaded is 0", d)
	}
}

func TestDecodeMenuOpen(t *testing.T) {
	var m Mem
	m[sym.FontLoaded] = 1
	m[sym.CurrentMenuItem] = 5
	m[sym.MaxMenuItem] = 7

	s := DecodeMenu(&m)
	if s == nil {
		t.Fatal("DecodeMenu = nil, want open menu")
	}
	if s.Current != 5 || s.Max != 7 {
		t.Errorf("MenuState = %+v, want {5 7}", s)
	}
}

func TestDecodeDialogueOpen(t *testing.T) {
	var m Mem
	m[sym.FontLoaded] = 1
	m[sym.TextBoxID] = 0x2A

	d := DecodeDialogue(&m)
	if d == nil {
		t.Fatal("DecodeDialogue = nil, want open text box")
	}
	if d.TextBoxID != 0x2A {
		t.Errorf("TextBoxID = 0x%02X, want 0x2A", d.TextBoxID)
	}
}

func TestControllable(t *testing.T) {
	var m Mem
	m[sym.FontLoaded] = 0
	m[sym.JoyIgnore] = 0
	m[sym.WalkCounter] = 0
	// Map dimensions are 0: the intro is still running (wCurMap and the
	// coordinates are written during new-game init, long before the map loads).
	if Controllable(&m) {
		t.Fatal("Controllable = true, want false when map dimensions are 0")
	}

	m[sym.CurMapWidth] = 4
	if Controllable(&m) {
		t.Fatal("Controllable = true, want false when only one dimension is non-zero")
	}
	m[sym.CurMapHeight] = 4
	if !Controllable(&m) {
		t.Fatal("Controllable = false, want true when all conditions hold")
	}

	m[sym.FontLoaded] = 1
	if Controllable(&m) {
		t.Errorf("Controllable = true, want false when FontLoaded is non-zero")
	}
	m[sym.FontLoaded] = 0

	m[sym.JoyIgnore] = 1
	if Controllable(&m) {
		t.Errorf("Controllable = true, want false when JoyIgnore is non-zero")
	}
	m[sym.JoyIgnore] = 0

	m[sym.WalkCounter] = 1
	if Controllable(&m) {
		t.Errorf("Controllable = true, want false when WalkCounter is non-zero")
	}
}
