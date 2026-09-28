// Package session owns the Boxxle launch boundary: the minimal runtime the
// standard run path needs to select, boot, and identify a Boxxle cartridge.
//
// Board decoding, autonomous puzzle execution, and state observation are later
// slices of the Boxxle epic. This slice deliberately sends no input and reads
// no board state, so the launch stays a pure power-on settle that any later
// play layer can build on.
package session

import "github.com/maestroi/pokepilot/game"

// bootFrameBudget is the power-on settle budget. Boxxle reaches its title
// screen well within this; the budget only bounds how long a boot may take
// before the run path treats it as a failure.
const bootFrameBudget = 3600

// Machine is the emulator surface the launch boundary needs. emu.Emu satisfies
// it; the surface is intentionally narrow because this slice neither sends
// input nor observes RAM.
type Machine interface {
	FrameCount() uint64
	StepFrame()
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
