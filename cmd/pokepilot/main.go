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
	"math/rand/v2"
	"os"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
)

const version = "pokepilot v0.1.0-dev"

// llmMaxRounds and llmMaxFrames are guardrails for an unattended run, not
// goals: a healthy run stops well before either, on stuck or error.
const (
	llmMaxRounds = 32
	llmMaxFrames = 8 * 60 * 60 * 60 // eight hours of emulated frames at 60 fps
)

func main() {
	addr := flag.String("http", "localhost:8099", "address to serve the screen on")
	every := flag.Int("capture-every", 4, "capture a frame for the browser every N frames")
	dest := flag.String("goto", "viridian pokemon center", "named destination to walk to")
	fps := flag.Int("fps", 60, "pace the walk to this many frames per second so it is watchable; 0 runs flat out")
	hold := flag.Duration("hold", 30*time.Second, "how long to keep serving after the run finishes")
	starter := flag.String("starter", "squirtle", "starter to take: charmander, squirtle or bulbasaur (bulbasaur loses the rival battle)")
	planner := flag.String("planner", "scripted", "how to choose objectives: scripted or llm")
	seed := flag.Int64("seed", 0, "diverge this run's luck by burning seed-derived idle frames after boot; 0 replays bit-identically")
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

	// Gen 1 has no seed to set. Its RNG is hRandomAdd/hRandomSub ($FFD3,
	// $FFD4), reseeded from DIV, which counts CPU cycles — so the run is
	// bit-identical every time unless the cycle count before the first
	// decision differs. Idle frames in the overworld are the cheapest way
	// to shift it: they do nothing to the game state and reroute every
	// encounter that follows.
	if *seed != 0 {
		n := rand.New(rand.NewPCG(uint64(*seed), 0)).IntN(600) // up to ten seconds of game time
		m.StepFrames(n)
		fmt.Printf("seed %d: burned %d idle frames, so this run's luck differs\n", *seed, n)
	}

	m.Pace(*fps)
	if *fps > 0 {
		fmt.Printf("paced to %d fps — open the page now\n", *fps)
	}

	switch *planner {
	case "scripted":
		runScripted(m, *starter, *dest, *hold, served)
	case "llm":
		runLLM(m)
	default:
		log.Fatalf("unknown planner %q: want scripted or llm", *planner)
	}
}

// runScripted is the original flow, unchanged: take the starter, walk to the
// destination, then keep serving so the screen stays live.
func runScripted(m *emu.Emu, starter, dest string, hold time.Duration, served string) {
	// The north exit of Pallet Town is gated on the opening story: walking to
	// y==1 without a Pokemon triggers Oak's "Don't go out!" cutscene and sets
	// wJoyIgnore, which no amount of walking gets past.
	var which skill.Starter
	switch starter {
	case "charmander":
		which = skill.StarterCharmander
	case "squirtle":
		which = skill.StarterSquirtle
	case "bulbasaur":
		which = skill.StarterBulbasaur
	default:
		log.Fatalf("unknown starter %q: want charmander, squirtle or bulbasaur", starter)
	}

	fmt.Printf("getting the %s starter (this includes the rival battle)...\n", starter)
	if err := skill.GetStarter(m, m.ROM(), which, skill.StatAwareMove(m.ROM())); err != nil {
		log.Fatalf("get starter: %v", err)
	}
	report(m, "got starter")

	target, ok := skill.Place(dest)
	if !ok {
		log.Fatalf("unknown destination %q", dest)
	}

	fmt.Printf("walking to %q (map %02x, %d,%d)...\n", dest, target.Map, target.X, target.Y)
	start := time.Now()
	err := skill.GoTo(m, m.ROM(), target)
	report(m, fmt.Sprintf("after GoTo (%s)", time.Since(start).Round(time.Millisecond)))
	if err != nil {
		fmt.Printf("\nGoTo failed: %v\n", err)
	} else {
		fmt.Println("\narrived.")
	}

	// Keep stepping while serving. Sleeping here instead would freeze the
	// emulator, and the browser would show a still frame that looks like a
	// hang rather than a game waiting for input nobody is pressing.
	fmt.Printf("\nstill serving http://%s for %s, ctrl-c to quit\n", served, hold)
	m.Pace(60)
	for deadline := time.Now().Add(hold); time.Now().Before(deadline); {
		m.StepFrames(4)
	}
}

// runLLM drives the objective loop: the model picks the next objective from
// the offered list each round, and every round is printed to stdout so an
// unattended run leaves something a human can read in the morning.
func runLLM(m *emu.Emu) {
	offered := []agent.Objective{{Kind: agent.KindStarter}}
	for _, name := range skill.PlaceNames() {
		offered = append(offered, agent.Objective{Kind: agent.KindGoTo, Place: name})
	}
	fmt.Printf("planner: llm — the model picks from %d offered objectives\n", len(offered))

	planner := agent.NewLLMPlanner()
	planner.Log = os.Stdout // one line per model call, above its round line
	res := agent.Run(m, m.ROM(), planner, offered, agent.Budget{
		MaxRounds: llmMaxRounds,
		MaxFrames: llmMaxFrames,
		Log:       os.Stdout,
	})

	fmt.Printf("\nrun stopped: %s after %d round(s)\n", stopName(res.Stop), res.Rounds)
	for i, o := range res.Completed {
		fmt.Printf("  completed %d: %s\n", i+1, o)
	}
	if res.Err != nil {
		fmt.Printf("  error: %v\n", res.Err)
	}

	// A run that stops on error or stuck has not done what it was told to
	// do; exiting zero would make an overnight failure invisible.
	if res.Stop == agent.StopError || res.Stop == agent.StopStuck {
		os.Exit(1)
	}
}

// stopName renders an agent.Stop for the final line. The type has no
// String() of its own, so the mapping lives next to where it is shown.
func stopName(s agent.Stop) string {
	switch s {
	case agent.StopDone:
		return "done"
	case agent.StopStuck:
		return "stuck"
	case agent.StopBudget:
		return "budget"
	case agent.StopError:
		return "error"
	}
	return fmt.Sprintf("unknown stop %d", int(s))
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
