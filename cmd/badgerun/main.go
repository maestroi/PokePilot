// Command badgerun measures whether the LLM planner can earn badge 1 on its
// own, and diagnoses why when it cannot.
//
// It runs agent.Run with the real LLM planner to the Boulder Badge, N times
// per starter, across several seeds, and prints a table. It is a harness,
// not a service: no session registry, no pool abstraction, no UI (those are
// slice 4). A loop and a table is the whole task.
//
// Nobody tells the model the answer. The prompt offers verbs and the
// observation; it does not say "take Squirtle", does not mention Caterpie,
// and does not hint that Butterfree beats Onix. If the model must learn that
// Fire loses to Rock, it learns it by losing or from an NPC who says so.
// The one controlled variable is the starter itself: the harness takes it
// (skill.GetStarter) before handing control to the model, because "across
// all three starters" requires fixing the experimental variable rather than
// letting a model that knows Pokemon always pick Squirtle.
//
// -seed varies LUCK, not skill: Gen 1 has no seed of its own, so seed N
// burns N-derived idle frames after boot, shifting DIV and rerouting every
// encounter. Different seeds are different luck, not different models.
//
// All durations are EMULATED FRAMES (emu.FrameCount), never wall clock.
//
// Concurrency is deliberately absent: the emulator is not thread-safe, so
// concurrent runs would need one *emu.Emu per goroutine and a pool sized
// from runtime.NumCPU(). Sequential is the correct default for a harness
// whose output is a table; add it when the table is not the bottleneck.
//
// This command is NOT part of `go test ./...`: its tests cover argument
// parsing and table formatting only, and a real scoreboard needs a ROM and
// a live model server.
package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
)

// defaultFact is the one sentence -inject-fact adds to the system prompt.
// It is the minimal true fact this project believes a failing run is
// missing: Rock resists Fire, so Charmander cannot beat Brock the obvious
// way and needs a Bug (or Fighting) answer. Anything more specific ("catch
// a Caterpie") would be an instruction, not a fact, and would turn the
// ablation into a walkthrough.
const defaultFact = "Rock-type Pokemon resist Fire."

type config struct {
	romPath    string
	starters   []string // canonical names: charmander, squirtle, bulbasaur
	n          int      // runs per starter
	seeds      []int64
	maxRounds  int
	maxFrames  int
	outDir     string
	injectFact bool
	fact       string
	goal       string // task statement rendered into the planner's system prompt
	badge      string // badge name that ends the run (default "Boulder")
}

// parseConfig parses and validates the harness arguments. It is a pure
// function of args (plus POKEMON_RED_ROM for the -rom default) so the tests
// can cover it without an emulator or a model.
func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("badgerun", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		rom       = fs.String("rom", os.Getenv("POKEMON_RED_ROM"), "path to the Pokemon Red ROM (default $POKEMON_RED_ROM)")
		starter   = fs.String("starter", "all", "charmander, squirtle, bulbasaur or all")
		n         = fs.Int("n", 3, "runs per starter")
		seedsFlag = fs.String("seeds", "1,2,3", "comma-separated seeds; run i of a starter uses seed i mod len(seeds)")
		maxRounds = fs.Int("max-rounds", 64, "agent.Budget.MaxRounds per run")
		maxFrames = fs.Int("max-frames", 3*60*60*60, "agent.Budget.MaxFrames per run (emulated frames)")
		outDir    = fs.String("out", "badgerun-out", "directory for per-run logs, prompts and checkpoints")
		inject    = fs.Bool("inject-fact", false,
			"DIAGNOSTIC ONLY, default off: append -fact to the system prompt. "+
				"The injected fact is the thing being measured; leaving it on turns the benchmark into a walkthrough.")
		fact  = fs.String("fact", defaultFact, "the one sentence -inject-fact injects")
		goal  = fs.String("goal", "Earn the Boulder Badge.",
			"the task statement rendered into the planner's system prompt above everything else. "+
				"A run parameter, not a constant: later slices need a different goal with a checkpoint. "+
				"It must name the task and nothing else — no strategy, which is what -inject-fact is for.")
		badge = fs.String("badge", "Boulder",
			"the badge name in the observation that ends the run (default Boulder; "+
				"set Cascade for the S9-12 milestone so a run that earns Pewter keeps going)")
	)
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	cfg := config{romPath: *rom, n: *n, maxRounds: *maxRounds, maxFrames: *maxFrames, outDir: *outDir, injectFact: *inject, fact: *fact, goal: *goal, badge: *badge}
	if cfg.romPath == "" {
		return config{}, fmt.Errorf("badgerun: no ROM (-rom or POKEMON_RED_ROM)")
	}
	if cfg.n < 1 {
		return config{}, fmt.Errorf("badgerun: -n must be >= 1, got %d", cfg.n)
	}
	if cfg.maxRounds < 1 || cfg.maxFrames < 1 {
		return config{}, fmt.Errorf("badgerun: -max-rounds and -max-frames must be >= 1")
	}
	seeds, err := parseSeeds(*seedsFlag)
	if err != nil {
		return config{}, err
	}
	cfg.seeds = seeds
	starters, err := parseStarters(*starter)
	if err != nil {
		return config{}, err
	}
	cfg.starters = starters
	return cfg, nil
}

// parseSeeds splits a comma-separated seed list. Empty or malformed fields
// are errors: a seed of 0 is legal (replays identically), so "1,,2" must not
// silently become [1 2].
func parseSeeds(s string) ([]int64, error) {
	var out []int64
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		v, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("badgerun: -seeds: %q is not an integer", f)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("badgerun: -seeds must name at least one seed")
	}
	return out, nil
}

var starterNames = []string{"charmander", "squirtle", "bulbasaur"}

// parseStarters resolves -starter. "all" expands to the three starters in
// canonical order; anything else must be exactly one of them.
func parseStarters(s string) ([]string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "all":
		return starterNames, nil
	case "charmander", "squirtle", "bulbasaur":
		return []string{s}, nil
	default:
		return nil, fmt.Errorf("badgerun: unknown starter %q (want charmander, squirtle, bulbasaur or all)", s)
	}
}

func starterFor(name string) skill.Starter {
	switch name {
	case "charmander":
		return skill.StarterCharmander
	case "squirtle":
		return skill.StarterSquirtle
	default:
		return skill.StarterBulbasaur
	}
}

// runResult is one row of the scoreboard. Every duration is emulated frames.
type runResult struct {
	starter   string
	seed      int64
	badge     bool
	frames    uint64 // from starter-in-hand to stop
	toBadge   uint64 // frames until the badge was observed; 0 when no badge
	calls     int    // planner calls
	ok        int    // objectives completed
	failed    int    // objectives attempted that failed
	battles   int
	blackouts int
	stop      string
	where     string // final map and position
}

// formatTable renders the scoreboard. It is a pure function so the tests can
// pin its shape: one row per run, columns are the per-run facts the task
// asks for. In the frames column, * marks frames-to-badge (the run kept no
// counting after that point by design).
func formatTable(rs []runResult) string {
	var b strings.Builder
	// Header uses the same field widths as the rows, so columns line up.
	fmt.Fprintf(&b, "%-10s %-4s %-5s %10s %6s %-6s %7s %9s %-8s %s\n",
		"starter", "seed", "badge", "frames", "calls", "ok/fail", "battles", "blackouts", "stop", "where")
	for _, r := range rs {
		badge := "no"
		if r.badge {
			badge = "yes"
		}
		frames := fmt.Sprintf("%d", r.frames)
		if r.badge {
			frames = fmt.Sprintf("%d*", r.toBadge)
		}
		fmt.Fprintf(&b, "%-10s %-4d %-5s %10s %6d %3d/%-3d %7d %9d %-8s %s\n",
			r.starter, r.seed, badge, frames, r.calls, r.ok, r.failed, r.battles, r.blackouts, r.stop, r.where)
	}
	return b.String()
}

// badgePlanner wraps the real planner and does two things the run loop does
// not: it counts planner calls and blackouts (Run carries the blackout fact
// across exactly the round that follows a blackout failure), and it ends the
// run the moment the Boulder Badge is visible in the observation. Ending on
// ErrDone rather than letting the model keep playing makes "frames to badge"
// a well-defined number instead of "frames until the model stopped".
type badgePlanner struct {
	inner         *agent.LLMPlanner // the real planner; typed so NextFeedback can be forwarded
	m             *emu.Emu
	badge         string // -badge: the badge name that ends the run
	calls         int
	blackouts     int
	sawBlackout   bool // the carried flag is live for one round; count it once
	battles       int
	framesToBadge uint64
}

// NextFeedback forwards the rejection feedback, so planWithRetries can re-ask
// a rejected reply up to MaxReplyRetries times. Without this method the
// wrapper fails the FeedbackPlanner type assertion and ONE superfluous-
// argument reply (the S9-11 rejection storm) stops the whole run after a
// single call — which is exactly how the first S9-12 attempt died on round 1.
func (b *badgePlanner) NextFeedback(obs agent.Observation, offered []agent.Objective, feedback string) (agent.Objective, error) {
	b.calls++
	if obs.BlackedOut && !b.sawBlackout {
		b.blackouts++
	}
	b.sawBlackout = obs.BlackedOut
	if b.framesToBadge == 0 && hasBadge(obs, b.badge) {
		b.framesToBadge = b.m.FrameCount()
		return agent.Objective{}, agent.ErrDone
	}
	return b.inner.NextFeedback(obs, offered, feedback)
}

func (b *badgePlanner) Next(obs agent.Observation, offered []agent.Objective) (agent.Objective, error) {
	return b.NextFeedback(obs, offered, "")
}

// hasBadge reports whether the named badge is visible in the observation.
// The name comes from -badge, not a constant: the S9-12 milestone chases
// Cascade through Pewter, and a run that stops at Boulder would answer the
// wrong question.
func hasBadge(obs agent.Observation, name string) bool {
	for _, n := range obs.Badges {
		if n == name {
			return true
		}
	}
	return false
}

// runOne boots the game, takes the controlled starter, hands control to the
// model, and returns the row. Everything the run said (round lines, llm
// lines, skill outcome lines) is captured into <dir>/run.log; every prompt,
// verbatim, into <dir>/prompts.txt; per-objective checkpoints land in
// <dir>/checkpoints (S6-11's ring), so a failed run is resumable and
// inspectable rather than replayed from boot.
func runOne(cfg config, starter string, seed int64) (runResult, error) {
	m, err := emu.Open(cfg.romPath)
	if err != nil {
		return runResult{}, fmt.Errorf("badgerun: open ROM: %w", err)
	}
	defer m.Close()
	rom := m.ROM()

	if _, err := skill.BootToOverworld(m); err != nil {
		return runResult{}, fmt.Errorf("badgerun: boot: %w", err)
	}

	// Seed = luck. Same mechanism as pokepilot -seed: burn idle frames so
	// DIV differs and every encounter reroutes.
	burn := 0
	if seed != 0 {
		burn = rand.New(rand.NewPCG(uint64(seed), 0)).IntN(600)
	}
	m.StepFrames(burn)

	dir := fmt.Sprintf("%s/%s-seed%d", cfg.outDir, starter, seed)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return runResult{}, fmt.Errorf("badgerun: mkdir %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir+"/checkpoints", 0o755); err != nil {
		return runResult{}, fmt.Errorf("badgerun: mkdir checkpoints: %w", err)
	}
	promptFile, err := os.Create(fmt.Sprintf("%s/prompts.txt", dir))
	if err != nil {
		return runResult{}, fmt.Errorf("badgerun: create prompts.txt: %w", err)
	}

	logBuf := &strings.Builder{}
	// captureStdout tees to the original stdout itself; passing a writer
	// that also contains it would print every line twice.
	capture := captureStdout(logBuf)
	var (
		res        agent.Result
		wrapped    *badgePlanner
		startFrame uint64
		starterErr error
	)
	capture(func() {
		// The harness takes the starter: the controlled variable of the
		// experiment. From here on the model decides everything.
		if err := skill.GetStarter(m, rom, starterFor(starter), skill.StatAwareMove(rom)); err != nil {
			starterErr = fmt.Errorf("get starter %s: %w", starter, err)
			fmt.Fprintf(os.Stdout, "get starter %s: %v\n", starter, err)
			return
		}
		startFrame = m.FrameCount()

		planner := plannerFor(cfg)
		planner.Log = os.Stdout // one line per model call, captured into logBuf
		planner.PromptLog = promptFile // every prompt, verbatim, for the record
		wrapped = &badgePlanner{inner: planner, m: m, badge: cfg.badge}
		// Battles are counted per frame from the battle flag's rising
		// edges: every battle in this slice is stepped frame by frame, so
		// no transition is missed. (wStatusFlags4 bit 5 is NOT a blackout
		// signal — it is also set on ordinary battle end.)
		var prevBattle bool
		m.OnFrame(func(mm *emu.Emu) {
			in := mm.Peek8(sym.IsInBattle) != 0
			if in && !prevBattle {
				wrapped.battles++
			}
			prevBattle = in
		})

		res = agent.Run(m, rom, wrapped, agent.Budget{
			MaxRounds:     cfg.maxRounds,
			MaxFrames:     cfg.maxFrames,
			Log:           os.Stdout, // captured into logBuf above
			CheckpointDir: dir + "/checkpoints",
		})
	})
	promptFile.Close()

	if res.Err != nil {
		logBuf.WriteString("run error: " + res.Err.Error() + "\n")
	}
	if starterErr != nil {
		writeRunLog(dir, logBuf.String())
		return runResult{starter: starter, seed: seed, stop: "error", where: starterErr.Error()}, nil
	}
	writeRunLog(dir, logBuf.String())
	// The final save state is the measurement surface for "how far": map,
	// badges and party are read from RAM out of this file (PROBE_STATE),
	// not from the log's prose. The checkpoint ring is bounded and lags the
	// last objective, so it cannot stand in for where the run actually died.
	if st, err := m.SaveState(); err != nil {
		fmt.Fprintf(os.Stderr, "badgerun: save final state: %v\n", err)
	} else if err := os.WriteFile(dir+"/final.state", st, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "badgerun: write final.state: %v\n", err)
	}

	stop := stopName(res.Stop)
	where := fmt.Sprintf("%s (%d,%d)", res.Final.MapName, res.Final.X, res.Final.Y)
	if res.Final.MapName == "" {
		where = fmt.Sprintf("map %02x (%d,%d)", res.Final.Map, res.Final.X, res.Final.Y)
	}
	if res.Err != nil {
		// A non-done stop is a diagnosis input; the table row says why.
		where += " — " + res.Err.Error()
	}
	badge := wrapped.framesToBadge != 0 || hasBadge(res.Final, cfg.badge)
	r := runResult{
		starter:   starter,
		seed:      seed,
		badge:     badge,
		frames:    m.FrameCount() - startFrame,
		calls:     wrapped.calls,
		ok:        len(res.Completed),
		failed:    res.Rounds - len(res.Completed),
		battles:   wrapped.battles,
		blackouts: wrapped.blackouts,
		stop:      stop,
		where:     where,
	}
	if badge {
		r.toBadge = wrapped.framesToBadge - startFrame
	}
	return r, nil
}

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var jobs []struct {
		starter string
		seed    int64
	}
	for _, s := range cfg.starters {
		for i := 0; i < cfg.n; i++ {
			jobs = append(jobs, struct {
				starter string
				seed    int64
			}{s, cfg.seeds[i%len(cfg.seeds)]})
		}
	}

	fmt.Printf("badgerun: %d run(s), model %s @ %s, goal %q, fact-injection %s\n",
		len(jobs), plannerModel(), plannerURL(), cfg.goal, injectionState(cfg))

	var results []runResult
	for i, j := range jobs {
		fmt.Printf("\n=== run %d/%d: %s seed %d ===\n", i+1, len(jobs), j.starter, j.seed)
		r, err := runOne(cfg, j.starter, j.seed)
		if err != nil {
			r = runResult{starter: j.starter, seed: j.seed, stop: "error", where: err.Error()}
		}
		results = append(results, r)
		fmt.Printf("  -> badge=%v stop=%s %s\n", r.badge, r.stop, r.where)
	}

	fmt.Println()
	fmt.Print(formatTable(results))
	if cfg.injectFact {
		fmt.Println("\nNOTE: -inject-fact was ON for this scoreboard. It is a diagnostic;")
		fmt.Println("these rows are not comparable to a baseline scored without it.")
	}
}

// plannerFor builds the LLM planner. Model and URL come from
// POKEPILOT_LLM_MODEL / POKEPILOT_LLM_URL (agent.NewLLMPlanner); ablation A
// is "set those env vars to a larger model and re-run".
func plannerFor(cfg config) *agent.LLMPlanner {
	p := agent.NewLLMPlanner()
	// The goal is the task statement: one sentence, no strategy. It is
	// always set (the flag has a default) and stays separate from the
	// -inject-fact diagnostic below.
	p.Goal = cfg.goal
	if cfg.injectFact {
		// Ablation B, DIAGNOSTIC ONLY and default off. The fact being
		// injected is the thing being measured; if this ever ships on by
		// default the benchmark measures the sentence, not the model.
		p.ExtraSystem = "\n\nOne fact about this game: " + cfg.fact
	}
	return p
}

func plannerModel() string {
	if v := os.Getenv("POKEPILOT_LLM_MODEL"); v != "" {
		return v
	}
	return "(default)"
}

func plannerURL() string {
	if v := os.Getenv("POKEPILOT_LLM_URL"); v != "" {
		return v
	}
	return "(default)"
}

func injectionState(cfg config) string {
	if !cfg.injectFact {
		return "off (baseline)"
	}
	return "ON: " + cfg.fact
}

// writeRunLog saves the captured stdout of one run.
func writeRunLog(dir, log string) error {
	if err := os.WriteFile(dir+"/run.log", []byte(log), 0o644); err != nil {
		return fmt.Errorf("badgerun: write run.log: %w", err)
	}
	return nil
}

// captureStdout swaps os.Stdout for a pipe and tees everything printed
// during fn to both the original stdout (live progress) and dst (the run
// log). It is sequential-only on purpose: os.Stdout is global, and this
// harness runs one emu at a time.
func captureStdout(dst io.Writer) func(func()) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.MultiWriter(old, dst), r)
		close(done)
	}()
	return func(fn func()) {
		fn()
		w.Close()
		<-done
		os.Stdout = old
	}
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
	return fmt.Sprintf("unknown %d", int(s))
}
