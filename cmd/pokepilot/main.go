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
	"net"
	"os"
	"strconv"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
)

// version is this build's identity (git SHA), stamped by the Dockerfile via
// -ldflags "-X main.version=..."; "dev" for local builds.
var version = "dev"

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
	maxRounds := flag.Int("max-rounds", llmMaxRounds, "objectives one llm run may spend; each costs a model call. The default is a guardrail for an unattended run, not a target — raise it to ask how far the greedy loop actually gets")
	goal := flag.String("goal", "Earn the Boulder Badge.", "the task statement given to the llm planner: what it is trying to do, never how. Empty means no goal, which is what made slice 6's scoreboard void")
	checkpointDir := flag.String("checkpoint-dir", "", "write a save-state ring here before every objective, so a run that dies (a wedged battle, a stuck loop) leaves a state to replay instead of a stack trace. Empty means off, which is why SLICE10-CANDIDATES.md item 19's wedged battle has never been reproduced")
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

	// Gen 1 has no seed to set. Its RNG is hRandomAdd/hRandomSub ($FFD3,
	// $FFD4), reseeded from DIV, which counts CPU cycles — so the run is
	// bit-identical every time unless the cycle count before the first
	// decision differs. Idle frames in the overworld are the cheapest way
	// to shift it: they do nothing to game state and reroute every
	// encounter that follows. Decided here, burned after boot.
	burn := 0
	if *seed != 0 {
		burn = rand.New(rand.NewPCG(uint64(*seed), 0)).IntN(600) // up to ten seconds of game time
	}
	m.TraceHeader(runHeader(*planner, *starter, *dest, *seed, burn))

	// Boot runs unthrottled: it is three thousand frames of Oak's intro
	// speech and nobody wants to watch that in real time. Pacing starts
	// once there is something to see.
	fmt.Println("booting to the overworld (unthrottled)...")
	if _, err := skill.BootToOverworld(m); err != nil {
		log.Fatalf("boot: %v", err)
	}
	report(m, "booted")

	// Farm mode: the wall, not the flags, decides what runs. The boot state
	// is saved at this clean post-boot point and restored per lease, so a
	// CLI -seed never burns frames here; each leased spec's seed is applied
	// exactly once by runOne.
	if orchURL := os.Getenv("POKEPILOT_ORCH_URL"); orchURL != "" {
		bootState, err := m.SaveState()
		if err != nil {
			log.Fatalf("save boot state: %v", err)
		}
		fmt.Printf("farm mode: leasing runs from %s\n", orchURL)
		client := farm.NewClient(orchURL)
		client.Version = version
		runFarm(m, client, bootState, watchPort(served))
		return
	}

	if burn > 0 {
		m.StepFrames(burn)
		fmt.Printf("seed %d: burned %d idle frames, so this run's luck differs\n", *seed, burn)
	}

	m.Pace(*fps)
	if *fps > 0 {
		fmt.Printf("paced to %d fps — open the page now\n", *fps)
	}

	switch *planner {
	case "scripted":
		runScripted(m, *starter, *dest, *hold, served)
	case "llm":
		runLLM(m, *goal, *maxRounds, *checkpointDir)
	default:
		log.Fatalf("unknown planner %q: want scripted or llm", *planner)
	}
}

// runHeader is the one line pinned above the watch page's trace: what is
// driving this run, and the seed, which is the only thing that makes one
// run differ from another.
func runHeader(planner, starter, dest string, seed int64, burn int) string {
	what := "planner " + planner
	if planner == "scripted" {
		what += " · " + starter + " → " + dest
	}
	if seed == 0 {
		return what + " · seed 0 (replays identically)"
	}
	return fmt.Sprintf("%s · seed %d (+%d idle frames)", what, seed, burn)
}

// watchPort extracts the port actually listened on from the address
// Watch reported, so farm mode can tell the wall where to fetch this
// runner's live screen. Zero means "not known", which the wall treats as
// "no frames".
func watchPort(served string) int {
	_, portStr, err := net.SplitHostPort(served)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return port
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
//
// The list is the whole safety argument: the model picks a number, never
// invents an action. The list is not built here at all — Run rebuilds it
// every round from the current observation and what the run has actually
// seen (agent.Offer): places already visited or one step out of where the
// player stands, verbs whose preconditions currently hold, the starter only
// while the party is empty. A menu of every place in the ROM would be both
// a worse prompt and a worse question.
func runLLM(m *emu.Emu, goal string, maxRounds int, checkpointDir string) {
	fmt.Println("planner: llm — the model picks from a menu rebuilt every round")

	// Tee llm/round lines to the watch panel as well as stdout.
	log := &agentTraceLog{w: os.Stdout, note: m.TraceNote}
	planner := agent.NewLLMPlanner()
	// Without this the planner is told only "prefer something new", which is
	// how a run wanders: lab -> pallet -> route 1 -> lab. badgerun has had a
	// goal since S7-2; this binary did not, so `make run-llm` was still
	// measuring a goal-less planner.
	planner.Goal = goal
	planner.Log = log // one line per model call, above its round line
	// The watch page's statistics panel: the same asks, tallied. See
	// runStats — the trace shows one round at a time, the tally is what
	// makes a wandering run visible while it is still wandering.
	stats := newStatsPlanner(planner, m.TraceStats, nil)
	res := agent.Run(m, m.ROM(), stats, agent.Budget{
		MaxRounds:     maxRounds,
		MaxFrames:     llmMaxFrames,
		Log:           log,
		CheckpointDir: checkpointDir,
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
	if res.Stop == agent.StopError || res.Stop == agent.StopStuck || res.Stop == agent.StopFailed {
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
	case agent.StopFailed:
		return "failed"
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
