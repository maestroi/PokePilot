package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
)

// slowMenuPressFrames stands in for the measured Gold/Silver cadence: a menu
// press shorter than this is swallowed by the input pipeline and leaves no
// trace, so the driver cannot tell it apart from a menu that ignored the key.
const slowMenuPressFrames = 8

// cadenceMenuDecoder declares the press hold its menus need. A decoder that does
// not implement game.MenuPressTiming keeps the generic default.
type cadenceMenuDecoder struct {
	fakeGen2MenuDecoder
	hold int
}

func (d cadenceMenuDecoder) MenuPressHoldFrames() int { return d.hold }

// cadenceMachine drops any press shorter than requiredHold, exactly as a slow
// game menu does.
type cadenceMachine struct {
	fakeMenuMachine
	requiredHold int
	dropped      int
}

func (m *cadenceMachine) Tap(btn emu.Button, hold, gap int) {
	if (btn == emu.Down || btn == emu.Up) && hold < m.requiredHold {
		m.dropped++
		return
	}
	m.fakeMenuMachine.Tap(btn, hold, gap)
}

func TestMenuSelectionHoldsPressForProfileInputCadence(t *testing.T) {
	m := &cadenceMachine{requiredHold: slowMenuPressFrames}
	m.mem[fakeMenuCurrent] = 0
	m.mem[fakeMenuMax] = 5

	err := selectMenuItemWithDecoder(m, cadenceMenuDecoder{hold: slowMenuPressFrames}, 2)
	if err != nil {
		t.Fatalf("select item on a menu that needs a %d-frame press: %v", slowMenuPressFrames, err)
	}
	if m.dropped != 0 {
		t.Fatalf("driver sent %d press(es) shorter than the declared hold", m.dropped)
	}
	if got := m.mem[fakeMenuCurrent]; got != 2 {
		t.Fatalf("cursor=%d, want 2", got)
	}
}

// TestMenuSelectionWithoutDeclaredHoldRepeatsTheSameMiss is the control: with the
// generic 3-frame tap a slow menu swallows every retry, so the test above is
// only meaningful because this one fails.
func TestMenuSelectionWithoutDeclaredHoldRepeatsTheSameMiss(t *testing.T) {
	m := &cadenceMachine{requiredHold: slowMenuPressFrames}
	m.mem[fakeMenuCurrent] = 0
	m.mem[fakeMenuMax] = 5

	err := selectMenuItemWithDecoder(m, fakeGen2MenuDecoder{}, 2)
	if err == nil {
		t.Fatal("a press the menu never sampled was reported as a successful selection")
	}
	if m.dropped == 0 {
		t.Fatal("control machine never dropped a short press")
	}
}

// droppedBPressMachine swallows the first B press, as Gold/Silver do when the
// press lands inside the previous press's input blackout.
type droppedBPressMachine struct {
	fakeMenuMachine
	swallowB int
	bPresses int
}

func (m *droppedBPressMachine) Tap(btn emu.Button, hold, gap int) {
	if btn != emu.B {
		m.fakeMenuMachine.Tap(btn, hold, gap)
		return
	}
	m.bPresses++
	if m.swallowB > 0 {
		m.swallowB--
		return
	}
	m.mem[fakeStartVisible] = 0
	m.mem[fakeStartReady] = 0
}

func TestCloseStartMenuRetriesAPressTheGameNeverSampled(t *testing.T) {
	m := &droppedBPressMachine{swallowB: 1}
	m.mem[fakeStartVisible] = 1
	m.mem[fakeStartReady] = 1

	if err := closeStartMenuWithDecoder(m, fakeGen2MenuDecoder{}); err != nil {
		t.Fatalf("close START menu after one swallowed press: %v", err)
	}
	if m.bPresses != 2 {
		t.Fatalf("B presses=%d, want 2: one swallowed, one accepted", m.bPresses)
	}
	if m.mem[fakeStartVisible] != 0 {
		t.Fatal("START menu still visible after a successful close")
	}
}

func TestCloseStartMenuReportsAStartMenuThatNeverAnswers(t *testing.T) {
	m := &droppedBPressMachine{swallowB: 1000}
	m.mem[fakeStartVisible] = 1
	m.mem[fakeStartReady] = 1

	err := closeStartMenuWithDecoder(m, fakeGen2MenuDecoder{})
	if err == nil {
		t.Fatal("an unclosable START menu was reported as closed")
	}
	if m.bPresses != startMenuCloseBudget/startMenuRetryWindow {
		t.Fatalf("B presses=%d, want %d bounded attempts", m.bPresses, startMenuCloseBudget/startMenuRetryWindow)
	}
}

// returnToStartMenuMachine reports the START menu live again only after the
// pack has answered a B press, and swallows the first one.
type returnToStartMenuMachine struct {
	fakeMenuMachine
	swallowB int
	bPresses int
}

func (m *returnToStartMenuMachine) Tap(btn emu.Button, hold, gap int) {
	if btn != emu.B {
		m.fakeMenuMachine.Tap(btn, hold, gap)
		return
	}
	m.bPresses++
	if m.swallowB > 0 {
		m.swallowB--
		return
	}
	m.mem[fakeStartReady] = 1
}

func TestReturnToStartMenuRetriesASwallowedPackExit(t *testing.T) {
	m := &returnToStartMenuMachine{swallowB: 1}
	if !returnToStartMenu(m, fakeGen2MenuDecoder{}, 3) {
		t.Fatal("a swallowed PACK exit was never retried")
	}
	if m.bPresses != 2 {
		t.Fatalf("B presses=%d, want 2: one swallowed, one accepted", m.bPresses)
	}
}
