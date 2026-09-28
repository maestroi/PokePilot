package main

import (
	"fmt"
	"time"

	"github.com/maestroi/pokepilot/emu"
)

// runLocalBoxxle is the local run path for a Boxxle cartridge. This slice
// registers and launches Boxxle through the standard run path but does not
// play it: autonomous puzzle play is a later slice of the Boxxle epic. It keeps
// the screen server up so a human can watch the title screen, then stops.
func runLocalBoxxle(m *emu.Emu, hold time.Duration, served string) {
	fmt.Println("Boxxle is registered and launched; autonomous puzzle play is a later slice.")
	fmt.Printf("still serving http://%s for %s, ctrl-c to quit\n", served, hold)
	m.Pace(60)
	for deadline := time.Now().Add(hold); time.Now().Before(deadline); {
		m.StepFrames(4)
	}
}
