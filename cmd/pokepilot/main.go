// Command pokepilot boots a supported Game Boy ROM, serves
// the screen over HTTP so a human can watch, and drives the built skills.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
	redrenderstate "github.com/maestroi/pokepilot/red/renderstate"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	tetrissession "github.com/maestroi/pokepilot/tetris/session"
)

var version = "dev"

const (
	// Zero means goal-driven: no hard decision count. agent.Run still has
	// its short loop detector, long stagnation watchdog and frame watchdog.
	llmMaxRounds = 0
	llmMaxFrames = 8 * 60 * 60 * 60
)

const defaultGoal = "elite-four"

func main() {
	addr := flag.String("http", "localhost:8099", "address to serve the screen on")
	every := flag.Int("capture-every", 3, "capture a frame for the browser every N frames")
	dest := flag.String("goto", "viridian pokemon center", "named destination to walk to")
	fps := flag.Int("fps", 60, "pace the walk to this many frames per second so it is watchable; 0 runs flat out")
	hold := flag.Duration("hold", 30*time.Second, "how long to keep serving after the run finishes")
	starter := flag.String("starter", "", "starter to take; empty uses the game's scripted default when it has one, otherwise Squirtle for the legacy Red/Blue CLI")
	planner := flag.String("planner", "scripted", "how to choose objectives: scripted, llm, or policy (Tetris)")
	seed := flag.Int64("seed", 0, "diverge this run's luck by burning seed-derived idle frames after boot; 0 replays bit-identically")
	maxRounds := flag.Int("max-rounds", llmMaxRounds, "optional emergency objective cap for one llm run; 0 means no round cap")
	maxChoices := flag.Int("max-choices", 0, "Tetris: offer a typed decision backend only the policy's best N placements; 0 offers all")
	goal := flag.String("goal", defaultGoal, "structured goal: Pokemon goals or Tetris auto | endless | survival | complete | lines:N | score:N")
	checkpointDir := flag.String("checkpoint-dir", "", "directory for the per-objective save-state ring")
	llmProfile := flag.String("llm-profile", "", "llm endpoint routing: default, gpu, or auto (GPU primary with LAN fallback)")
	resume := flag.String("resume", "", "resume an llm run from a round checkpoint, checkpoint directory, or run directory")
	flag.Parse()
	*goal = resolveLocalGoal(*goal)

	// POKEPILOT_ROM is the generic path; POKEMON_RED_ROM stays supported
	// because deploy/ and the docs set it. Which game the ROM is comes from
	// the bytes via profiles.Detect, not from this variable's name.
	romPath := os.Getenv("POKEPILOT_ROM")
	if romPath == "" {
		romPath = os.Getenv("POKEMON_RED_ROM")
	}
	if romPath == "" {
		log.Fatal("POKEPILOT_ROM is not set; point it at a supported Game Boy ROM (POKEMON_RED_ROM still works)")
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

	m, err := emu.OpenCGB(romPath)
	if err != nil {
		log.Fatalf("open ROM: %v", err)
	}
	defer m.Close()

	renderFeed := newRenderStateFeed()
	if err := m.HandleWatch("/render-state.json", renderFeed); err != nil {
		log.Fatalf("serve semantic state: %v", err)
	}
	cartridgeProfile, _, err := profiles.DetectCartridge(m.ROM())
	if err != nil {
		log.Fatalf("detect cartridge profile: %v", err)
	}
	watchProfile, isPokemon := cartridgeProfile.(game.GameProfile)
	captureRender := func(*emu.Emu) {}
	if isPokemon {
		redRenderer, renderErr := redrenderstate.New(m.ROM())
		if renderErr != nil {
			log.Printf("semantic renderer unavailable for loaded ROM: %v", renderErr)
		} else {
			captureRender = func(em *emu.Emu) { renderFeed.capture(em, redRenderer) }
		}
	}

	served, err := m.Watch(*addr, *every)
	if err != nil {
		log.Fatalf("serve screen: %v", err)
	}
	var watchMem state.Mem
	tracer := newDialogueTracer()
	m.OnSample(func(em *emu.Emu) {
		if !isPokemon {
			return
		}
		if watchProfile.Features().Has(game.FeatureBattles) {
			tracer.sample(em)
		}
		captureRender(em)
		em.TracePlayer(livePlayerForProfile(em, watchProfile, &watchMem))
	})
	fmt.Printf("%s\nwatch: http://%s\n\n", version, served)

	burn := 0
	if resumeFrom == "" && *seed != 0 {
		burn = rand.New(rand.NewPCG(uint64(*seed), 0)).IntN(600)
	}
	m.TraceHeader(runHeader(*planner, *starter, *dest, *seed, burn))

	if isPokemon {
		fmt.Println("booting to the overworld (unthrottled)...")
		if _, err := skill.BootToOverworld(m); err != nil {
			log.Fatalf("boot: %v", err)
		}
		report(m, "booted")
	} else if string(cartridgeProfile.ID()) == "tetris" {
		fmt.Println("booting to the Tetris title screen (unthrottled)...")
		if _, err := tetrissession.BootToTitle(cartridgeProfile, m); err != nil {
			log.Fatalf("boot: %v", err)
		}
		fmt.Printf("  booted Tetris at frame %d\n", m.FrameCount())
	} else {
		log.Fatalf("game %q has no runtime", cartridgeProfile.ID())
	}

	if orchURL := os.Getenv("POKEPILOT_ORCH_URL"); orchURL != "" {
		if err := startWorkerControlServer(); err != nil {
			log.Fatalf("farm: worker control: %v", err)
		}
		bootState, err := m.SaveState()
		if err != nil {
			log.Fatalf("save boot state: %v", err)
		}
		library := buildROMLibrary(romPath, bootState)
		fmt.Printf("farm mode: leasing runs from %s; games mounted: %s\n", orchURL, library.games())
		client := farm.NewClient(orchURL)
		client.Version = version
		// Swarm sends SIGTERM before replacing this task. Convert it into the
		// farm's cooperative safe-boundary cancellation instead of letting the
		// wall discover a dead heartbeat thirty seconds later (#1933).
		drainCtx, stopDrain := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stopDrain()
		if runFarm(m, client, library, watchPort(served), *checkpointDir, renderFeed, drainCtx.Done()) {
			// ErrLinkStalled can leave a goroutine inside the emulator. os.Exit
			// intentionally skips the deferred m.Close so this poisoned instance
			// is never touched again; Swarm restarts the failed worker task.
			os.Exit(1)
		}
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
		if !isPokemon {
			log.Fatalf("planner scripted requires a Pokemon gameplay profile")
		}
		runScripted(m, *starter, *dest, *hold, served)
	case "llm":
		if !isPokemon {
			log.Fatalf("planner llm requires a Pokemon gameplay profile")
		}
		runLLM(m, *goal, *llmProfile, *maxRounds, *checkpointDir, resumeFrom)
	case "policy":
		if string(cartridgeProfile.ID()) != "tetris" {
			log.Fatalf("planner policy currently supports Tetris only")
		}
		if *maxChoices < 0 || *maxChoices == 1 {
			log.Fatalf("-max-choices must be 0 or at least 2")
		}
		tetrisGoal := *goal
		if tetrisGoal == defaultGoal {
			tetrisGoal = "auto"
		}
		runLocalTetris(m, cartridgeProfile, tetrisGoal, *maxRounds, *maxChoices)
	default:
		log.Fatalf("unknown planner %q: want scripted, llm, or policy", *planner)
	}
}

func runHeader(planner, starter, dest string, seed int64, burn int) string {
	what := "planner " + planner
	if planner == "scripted" {
		if starter == "" {
			starter = "profile/default starter"
		}
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
	request := starter
	if request == "" {
		// Preserve the historical Red/Blue CLI default while allowing games
		// with exactly one profile-owned opening starter (Yellow) to select it
		// semantically instead of pretending they use Oak's three-ball menu.
		obs, err := agent.ObserveChecked(m, m.ROM())
		if err != nil {
			log.Fatalf("observe starter choice: %v", err)
		}
		if _, ok := agent.DefaultStarterObjective(obs); !ok {
			request = "squirtle"
		}
	}

	starterObj, err := scriptedStarterObjective(m, request, 0)
	if err != nil {
		log.Fatalf("starter objective: %v", err)
	}
	starterName := string(starterObj.Species)
	if starterName == "" {
		starterName = request
	}
	fmt.Printf("getting the %s starter (this includes the rival battle)...\n", starterName)
	starterResult, err := executeScriptedObjective(m, starterObj)
	if err != nil {
		log.Fatalf("get starter: %s", scriptedObjectiveDetail(starterResult, err))
	}
	report(m, "got starter")

	target, ok := skill.Place(dest)
	if !ok {
		log.Fatalf("unknown destination %q", dest)
	}

	fmt.Printf("walking to %q (map %02x, %d,%d)...\n", dest, target.Map, target.X, target.Y)
	start := time.Now()
	travelResult, err := executeScriptedObjective(m, agent.Objective{Kind: agent.KindGoTo, Place: agent.PlaceID(dest)})
	report(m, fmt.Sprintf("after GoTo (%s)", time.Since(start).Round(time.Millisecond)))
	if err != nil {
		fmt.Printf("\nGoTo failed: %s\n", scriptedObjectiveDetail(travelResult, err))
	} else {
		fmt.Println("\narrived.")
	}

	fmt.Printf("\nstill serving http://%s for %s, ctrl-c to quit\n", served, hold)
	m.Pace(60)
	for deadline := time.Now().Add(hold); time.Now().Before(deadline); {
		m.StepFrames(4)
	}
}

func runLLM(m *emu.Emu, goal, llmProfile string, maxRounds int, checkpointDir, resumeFrom string) {
	fmt.Println("planner: llm — the model picks from a menu rebuilt every round")
	log := &agentTraceLog{w: os.Stdout, note: m.TraceNote}
	stats := newStatsPlannerWithRunPolicy(localRunPolicy(goal), llmProfile, "", nil, m, m.TraceStats, nil)
	stats.wirePlannerLogs(log, nil)
	res := agent.Run(m, m.ROM(), stats, agent.Budget{
		MaxRounds:     maxRounds,
		MaxFrames:     llmMaxFrames,
		Build:         version,
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
