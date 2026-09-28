package session

import (
	"testing"

	"github.com/maestroi/pokepilot/boxxle/profile"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// fakeMachine is the smallest Machine: it counts frames and satisfies the
// controller's input and memory surface with no-ops, so Boot and Run can be
// exercised without a real emulator.
type fakeMachine struct{ frames uint64 }

func (f *fakeMachine) FrameCount() uint64 { return f.frames }
func (f *fakeMachine) StepFrame()         { f.frames++ }
func (f *fakeMachine) Peek8(uint16) byte  { return 0 }
func (f *fakeMachine) PeekInto(uint16, []byte) {
}
func (f *fakeMachine) Press(emu.Button)   {}
func (f *fakeMachine) Release(emu.Button) {}

func TestBootStepsTheSettleBudget(t *testing.T) {
	m := &fakeMachine{}
	stepped, err := Boot(profile.New(), m)
	if err != nil {
		t.Fatal(err)
	}
	if stepped != bootFrameBudget {
		t.Fatalf("Boot stepped %d frames, want %d", stepped, bootFrameBudget)
	}
	if m.frames != bootFrameBudget {
		t.Fatalf("machine advanced %d frames, want %d", m.frames, bootFrameBudget)
	}
}

func TestBootRejectsForeignCartridge(t *testing.T) {
	m := &fakeMachine{}
	foreign := &idProfile{id: "tetris"}
	if _, err := Boot(foreign, m); err == nil {
		t.Fatal("expected Boot to reject a non-Boxxle cartridge")
	}
	if m.frames != 0 {
		t.Fatalf("machine advanced %d frames for a rejected cartridge", m.frames)
	}
}

// idProfile is a minimal CartridgeProfile that reports a fixed id, used to
// prove Boot only launches Boxxle.
type idProfile struct{ id game.GameID }

func (p *idProfile) ID() game.GameID           { return p.id }
func (p *idProfile) Revision() game.RevisionID { return "test" }
func (p *idProfile) Detect(game.ROMInfo) bool  { return false }

var _ game.CartridgeProfile = (*idProfile)(nil)
