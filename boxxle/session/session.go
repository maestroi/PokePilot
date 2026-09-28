// Package session owns the Boxxle launch boundary and the autonomous puzzle
// run loop that sits above the semantic policy/controller layers and below
// farm/operator integration.
//
// Boot is the pure power-on settle the run path needs to identify and start a
// Boxxle cartridge. Run (see run.go) builds the observe/choose/execute loop on
// top of it: a bounded selector (deterministic policy by default, or a model
// planner) picks a legal semantic push and the deterministic controller from
// the control package executes it, so the model never emits raw D-pad input.
package session

import (
	"github.com/maestroi/pokepilot/boxxle/control"
	"github.com/maestroi/pokepilot/game"
)

// bootFrameBudget is the power-on settle budget. Boxxle reaches its puzzle
// screen well within this; the budget only bounds how long a boot may take
// before the run path treats it as a failure.
const bootFrameBudget = 3600

// Machine is the emulator surface the session needs: the controller's input
// and memory surface plus a frame counter. emu.Emu satisfies it.
type Machine interface {
	control.Machine
	FrameCount() uint64
}

// Boot powers on the Boxxle cartridge and lets it settle. It returns the
// number of frames stepped. It sends no input and decodes no board state: it
// only establishes the launch boundary the run path needs to identify and
// boot the cartridge, after which a Boxxle run is complete for this slice.
func Boot(profile game.CartridgeProfile, m Machine) (uint64, error) {
	if profile == nil || profile.ID() != "boxxle" {
		return 0, &LaunchError{Game: "boxxle", Detail: "not a Boxxle cartridge"}
	}
	start := m.FrameCount()
	for m.FrameCount()-start < bootFrameBudget {
		m.StepFrame()
	}
	return m.FrameCount() - start, nil
}

// LaunchError reports that a cartridge could not be launched as Boxxle.
type LaunchError struct {
	Game   string
	Detail string
}

func (e *LaunchError) Error() string {
	return "boxxle session: " + e.Detail
}
