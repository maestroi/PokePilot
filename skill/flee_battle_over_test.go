package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// fakeFleeMachine models only the facts a RUN-menu wait reads: whether the
// battle is still live, when (if ever) a RUN-capable menu is rendered, and how
// many frames the wait consumed. Keeping it a plain value lets a wait be
// driven to its exit condition deterministically, without a cartridge.
type fakeFleeMachine struct {
	liveUntil int // the battle is live while frames < liveUntil
	menuAt    int // the menu is rendered from this frame on; 0 = never
	frames    int
	taps      []emu.Button
}

func (m *fakeFleeMachine) FrameCount() uint64          { return uint64(m.frames) }
func (*fakeFleeMachine) Peek8(uint16) byte             { return 0 }
func (*fakeFleeMachine) PeekInto(_ uint16, dst []byte) { clear(dst) }
func (m *fakeFleeMachine) StepFrame()                  { m.frames++ }
func (m *fakeFleeMachine) StepFrames(n int)            { m.frames += n }
func (m *fakeFleeMachine) Tap(b emu.Button, hold, gap int) {
	m.taps = append(m.taps, b)
	m.frames += hold + gap
}

type fakeFleeDecoders struct{ m *fakeFleeMachine }

func (d fakeFleeDecoders) DecodeBattleRuntime(game.MemoryReader) game.BattleRuntimeState {
	return game.BattleRuntimeState{InBattle: d.m.frames < d.m.liveUntil}
}

func (d fakeFleeDecoders) DecodeBattleEscapeMenu(game.MemoryReader) game.BattleEscapeMenuState {
	if d.m.menuAt > 0 && d.m.frames >= d.m.menuAt {
		return game.BattleEscapeMenuState{Visible: true, Kind: game.BattleEscapeMenuOrdinary}
	}
	return game.BattleEscapeMenuState{}
}

func (fakeFleeDecoders) BattleEscapeRunPosition(kind game.BattleEscapeMenuKind) (game.BattleMenuPosition, bool) {
	if kind != game.BattleEscapeMenuOrdinary {
		return game.BattleMenuPosition{}, false
	}
	return game.BattleMenuPosition{Column: 1, Row: 1}, true
}

func (d fakeFleeDecoders) DecodeBattleExecution(game.MemoryReader) game.BattleExecutionState {
	return game.BattleExecutionState{InBattle: d.m.frames < d.m.liveUntil}
}

func fakeFleeControllers(m *fakeFleeMachine) fleeControllers {
	decoders := fakeFleeDecoders{m: m}
	return fleeControllers{runtime: decoders, escape: decoders, execution: decoders}
}

// TestFleeMenuWaitEndsWhenTheBattleEnds is the invariant that
// run-1wsyy1f75ssxsheu3o4xpui4 died against: the Old Man's scripted catch demo
// (Gen I wBattleType = BATTLE_TYPE_OLD_MAN) resolves itself about 950 frames
// after it starts, without ever offering the player a RUN menu, and its
// command menu is drawn with a stale wMaxMenuItem so the escape decoder
// deliberately rejects it. The wait for that menu must not outlive the battle:
// otherwise Flee burns its whole budget and reports a stuck menu for a battle
// that ended long ago.
func TestFleeMenuWaitEndsWhenTheBattleEnds(t *testing.T) {
	const battleFrames = 900
	m := &fakeFleeMachine{liveUntil: battleFrames}

	wait, err := waitFleeMenuWithControllers(m, fakeFleeControllers(m))
	if err != nil {
		t.Fatalf("waitFleeMenuWithControllers: %v", err)
	}
	if !wait.BattleOver {
		t.Fatalf("wait = %+v, want BattleOver for a battle that ended without a RUN menu", wait)
	}
	if wait.Kind != "" {
		t.Fatalf("wait kind = %q, want no menu kind for a battle that never offered RUN", wait.Kind)
	}
	if m.frames != battleFrames {
		t.Fatalf("wait consumed %d frames, want %d (the battle's length, not the %d-frame budget)",
			m.frames, battleFrames, bagMainMenuBudget)
	}
	if len(m.taps) == 0 {
		t.Fatal("wait never advanced the ROM while the battle was live")
	}
}

// TestFleeMenuWaitTimesOutWhileTheBattleIsLive is the other direction: a battle
// that stays live and never renders a RUN menu is still a structured failure,
// not a silent success.
func TestFleeMenuWaitTimesOutWhileTheBattleIsLive(t *testing.T) {
	m := &fakeFleeMachine{liveUntil: 1 << 30}

	_, err := waitFleeMenuWithControllers(m, fakeFleeControllers(m))
	if err == nil {
		t.Fatal("wait of a live battle with no RUN menu returned success")
	}
	if !errors.Is(err, ErrMenuStuck) {
		t.Fatalf("error %v does not wrap ErrMenuStuck", err)
	}
	if m.frames <= bagMainMenuBudget {
		t.Fatalf("wait gave up after %d frames, want more than the %d-frame budget", m.frames, bagMainMenuBudget)
	}
}

// TestFleeMenuWaitReturnsTheMenu pins that the ordinary case is untouched: the
// rendered RUN-capable menu is what Flee drives.
func TestFleeMenuWaitReturnsTheMenu(t *testing.T) {
	m := &fakeFleeMachine{liveUntil: 1 << 30, menuAt: 40}

	wait, err := waitFleeMenuWithControllers(m, fakeFleeControllers(m))
	if err != nil {
		t.Fatalf("waitFleeMenuWithControllers: %v", err)
	}
	if wait.BattleOver {
		t.Fatalf("wait = %+v, want the rendered menu", wait)
	}
	if wait.Kind != game.BattleEscapeMenuOrdinary {
		t.Fatalf("wait kind = %q, want %q", wait.Kind, game.BattleEscapeMenuOrdinary)
	}
}
