package main

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
)

// runFarmBoxxle is the farm run path for a Boxxle cartridge. The cartridge is
// already booted by prepareFarmAttempt (bootStateFor -> boxxlesession.Boot);
// this slice registers and launches Boxxle but does not play it. Autonomous
// puzzle play is a later slice, so the run finishes as registered.
func runFarmBoxxle(m *emu.Emu, spec farm.Spec) (reason, detail string) {
	m.TraceNote("boxxle", "launched; autonomous puzzle play is a later slice")
	return "registered", "Boxxle launched; autonomous puzzle play is a later slice"
}
