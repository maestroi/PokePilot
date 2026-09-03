// Command pokepilot boots Pokemon Red, serves the screen over HTTP so a
// human can watch, and drives the deterministic skills built so far.
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

var version = "dev"

const (
	llmMaxRounds = 32
	llmMaxFrames = 8 * 60 * 60 * 60
)

const defaultGoal = "elite-four"

func main() {
	addr := flag.String("http", "localhost:8099", "address to serve the screen on")
	every := flag.Int("capture-every", 4, "capture a frame for the browser every N frames")
	dest := flag.String("goto", "viridian pokemon center", "named destination to walk to")
	fps := flag.Int("fps", 60, "pace the walk to this many frames per second so it is watchable; 0 runs flat out")
	hold := flag.Duration("hold", 30*time.Second, "how long to keep serving after the run finishes")
	starter := flag.String("starter", "squirtle", "starter to take: charmander, squirtle or bulbasaur (bulbasaur loses the rival battle)")
	planner := flag.String("planner", "scripted", "how to choose objectives: scripted or llm")
	seed := flag.Int64("seed", 0, "diverge this run's luck by burning seed-derived idle frames after boot; 0 replays bit-identically")
	maxRounds := flag.Int("max-rounds", llmMaxRounds, "objectives one llm run may spend; each costs a model call")
	goal := flag.String("goal", defaultGoal, "structured goal: badges:N | reach:<place> | level:N | item:<name> | elite-four")
	checkpointDir := flag.String("checkpoint-dir", "", "directory for the per-objective save-state ring")
	resume := flag.String("resume", "", "resume an llm run from a round checkpoint, checkpoint directory, or run directory")
	flag.Parse()

	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		log.Fatal("POKEMON_RED_ROM is not set; point it at a Pokemon Red ROM")
	}

	resumeFrom := ""
	if *resume != "" {
		if *planner != "llm" {
			log.Fatal("-resume requires -planner llm")
		}
		if os.Getenv("POKEPILOT_ORCH_URL") != "" {
			log.Fatal("-resume cannot be combined with farm mode; farm leases define their own run state")
		}
		var resumeDir string
		var err error
		resumeFrom, resumeDir, err = resolveResume(*resume)
		if err != nil {
			log.Fatalf("resume: %v", err)
		}
		if *checkpointDir == "" {
			*checkpointDir = resumeDir
		}
	}

	if *checkpointDir != "" {
		if err := os.MkdirAll(*checkpointDir, 0o755); err != nil {
			log.Fatalf("checkpoint-dir: %v", err)
		}
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
	var watchMem state.Mem
	tracer := newDialogueTracer()
	m.OnSample(func(m *emu.Emu) {
		tracer.sample(m)
		m.TracePlayer(playerSnapshot(state.Read(m, &watchMem)))
	})
	fmt.Printf("%s\nwatch: http://%s\n\n", version, served)

	burn := 0
	if resumeFrom == "" && *seed != 0 {
		burn = rand.New(rand.NewPCG(uint64(*seed), 0)).IntN(600)
	}
	m.TraceHeader(runHeader(*planner, *starter, *dest, *seed, burn))

	fmt.Println("booting to the overworld (unthrottled)...")
	if _, err := skill.BootToOverworld(m); err != nil {
		log.Fatalf("boot: %v", err)
	}
	report(m, "booted")

	if orchURL := os.Getenv("POKEPILOT_ORCH_URL"); orchURL != "" {
		bootState, err := m.SaveState()
		if err != nil {
			log.Fatalf("save boot state: %v", err)
		}
		fmt.Printf("farm mode: leasing runs from %s\n", orchURL)
		client := farm.NewClient(orchURL)
		client.Version = version
		runFarm(m, client, bootState, watchPort(served), *checkpointDir)
		return
	}

	if burn > 0 {
		m.StepFrames(burn)
		fmt.Printf("seed %d: burned %d idle frames, so this run's luck differs\n", *seed, burn)
	}
	if resumeFrom != "" {
		fmt.Printf("resuming run from %s; checkpoint ring: %s\n", resumeFrom, *checkpointDir)
		m.TraceNote("resume", resumeFrom)
	}

	m.Pace(*fps)
	if *fps > 0 {
		fmt.Printf("paced to %d fps — open the page now\n", *fps)
	}

	switch *planner {
	case "scripted":
		runScripted(m, *starter, *dest, *hold, served)
	case "llm":
		runLLM(m, *goal, *maxRounds, *checkpointDir, resumeFrom)
	default:
		log.Fatalf("unknown planner %q: want scripted or llm", *planner)
	}
}

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

func runScripted(m *emu.Emu, starter, dest string, hold time.Duration, served string) {
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

	fmt.Printf("\nstill serving http://%s for %s, ctrl-c to quit\n", served, hold)
	m.Pace(60)
	for deadline := time.Now().Add(hold); time.Now().Before(deadline); {
		m.StepFrames(4)
	}
}

func runLLM(m *emu.Emu, goal string, maxRounds int, checkpointDir, resumeFrom string) {
	fmt.Println("planner: llm — the model picks from a menu rebuilt every round")
	log := &agentTraceLog{w: os.Stdout, note: m.TraceNote}
	stats := newStatsPlanner("", goal, m, m.TraceStats, nil)
	stats.wirePlannerLogs(log, nil)
	res := agent.Run(m, m.ROM(), stats, agent.Budget{
		MaxRounds:     maxRounds,
		MaxFrames:     llmMaxFrames,
		Log:           log,
		CheckpointDir: checkpointDir,
		ResumeFrom:    resumeFrom,
	})

	fmt.Printf("\nrun stopped: %s after %d round(s)\n", stopName(res.Stop), res.Rounds)
	for i, o := range res.Completed {
		fmt.Printf("  completed %d: %s\n", i+1, o)
	}
	printProgress(res.ProgressEarly, res.ProgressFinal)
	if res.Err != nil {
		fmt.Printf("  error: %v\n", res.Err)
	}
	if res.Stop == agent.StopError || res.Stop == agent.StopStuck || res.Stop == agent.StopFailed {
		os.Exit(1)
	}
}

func printProgress(early, final *agent.Progress) {
	if early == nil || final == nil {
		return
	}
	fmt.Printf("  progress: %s -> %s\n", describeProgress(early), describeProgress(final))
}

func describeProgress(p *agent.Progress) string {
	place := p.MapName
	if place == "" {
		place = fmt.Sprintf("map %02x", p.Map)
	}
	return fmt.Sprintf("round %d: %d badge(s), %d event(s), %d map(s), %s", p.Round, p.Badges, p.Events, p.Maps, place)
}

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

func report(m *emu.Emu, label string) {
	var mem state.Mem
	g := state.Read(m, &mem)
	fmt.Printf("  %-22s map=%02x pos=(%d,%d) facing=%v controllable=%t frame=%d\n",
		label, mem.U8(sym.CurMap), g.Player.X, g.Player.Y, g.Player.Facing,
		state.Controllable(&mem), m.FrameCount())
}
