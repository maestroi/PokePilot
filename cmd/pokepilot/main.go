// Command pokepilot boots Pokemon Red, serves the screen over HTTP so a
// human can watch, and drives the deterministic skills built so far.
//
// The browser view is for humans only. PokePilot itself never reads
// gameplay truth from pixels — see docs/DESIGN.md.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
)

const version = "pokepilot v0.1.0-dev"

func main() {
	addr := flag.String("http", "localhost:8099", "address to serve the screen on")
	every := flag.Int("capture-every", 4, "capture a frame for the browser every N frames")
	dest := flag.String("goto", "viridian pokemon center", "named destination to walk to")
	fps := flag.Int("fps", 60, "pace the walk to this many frames per second so it is watchable; 0 runs flat out")
	hold := flag.Duration("hold", 30*time.Second, "how long to keep serving after the run finishes")
	flag.Parse()

	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		log.Fatal("POKEMON_RED_ROM is not set; point it at a Pokemon Red ROM")
	}

	m, err := emu.Open(romPath)
	if err != nil {
		log.Fatalf("open ROM: %v", err)
	}
	defer m.Close()

	served, err := m.Watch(*addr, *every)
	if err != nil {
		log.Fatalf("serve screen: %v", err)
	}
	m.OnSample(newDialogueTracer().sample)
	fmt.Printf("%s\nwatch: http://%s\n\n", version, served)

	// Boot runs unthrottled: it is three thousand frames of Oak's intro
	// speech and nobody wants to watch that in real time. Pacing starts
	// once there is something to see.
	fmt.Println("booting to the overworld (unthrottled)...")
	if _, err := skill.BootToOverworld(m); err != nil {
		log.Fatalf("boot: %v", err)
	}
	report(m, "booted")

	m.Pace(*fps)
	if *fps > 0 {
		fmt.Printf("paced to %d fps — open the page now\n", *fps)
	}

	target, ok := skill.Place(*dest)
	if !ok {
		log.Fatalf("unknown destination %q", *dest)
	}

	fmt.Printf("walking to %q (map %02x, %d,%d)...\n", *dest, target.Map, target.X, target.Y)
	start := time.Now()
	err = skill.GoTo(m, m.ROM(), target)
	report(m, fmt.Sprintf("after GoTo (%s)", time.Since(start).Round(time.Millisecond)))
	if err != nil {
		fmt.Printf("\nGoTo failed: %v\n", err)
		fmt.Println("(this is expected until slice 3 opens the Pallet Town story gate)")
	} else {
		fmt.Println("\narrived.")
	}

	fmt.Printf("\nstill serving http://%s for %s, ctrl-c to quit\n", served, *hold)
	time.Sleep(*hold)
}

// report prints the state PokePilot actually reasons about: decoded RAM,
// never pixels.
func report(m *emu.Emu, label string) {
	var mem state.Mem
	g := state.Read(m, &mem)
	fmt.Printf("  %-22s map=%02x pos=(%d,%d) facing=%v controllable=%t frame=%d\n",
		label, mem.U8(sym.CurMap), g.Player.X, g.Player.Y, g.Player.Facing,
		state.Controllable(&mem), m.FrameCount())
}
