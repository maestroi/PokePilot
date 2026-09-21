// Command pokebench runs reproducible Pokémon qualification benchmarks and
// compares their versioned results. It deliberately reuses agent.Run,
// qualification checkpoints, semantic observations, and farm failure identity.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/benchmark"
	"github.com/maestroi/pokepilot/emu"
	redbench "github.com/maestroi/pokepilot/red/benchmark"
	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/skill"
)

const defaultMaxFrames = 8 * 60 * 60 * 60

type redConfig struct {
	romPath         string
	corpus          string
	mode            string
	from            string
	until           string
	runs            int
	seed            int64
	seeds           []int64
	output          string
	maxFrames       int
	llmProfile      string
	reasoningEffort string
}

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	var err error
	switch strings.ToLower(os.Args[1]) {
	case "red", "pokemon-red":
		err = runRedCommand(os.Args[2:], os.Stdout)
	case "compare":
		err = runCompare(os.Args[2:], os.Stdout)
	default:
		usage(os.Stderr)
		err = fmt.Errorf("pokebench: unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  pokebench red --mode speedrun --from fresh --until hall-of-fame --runs 3 --output DIR")
	fmt.Fprintln(w, "  pokebench red --from checkpoint:sabrina --until blaine --runs 3 --output DIR")
	fmt.Fprintln(w, "  pokebench compare BASELINE_DIR CANDIDATE_DIR")
}

func runRedCommand(args []string, stdout io.Writer) error {
	cfg, err := parseRedConfig(args)
	if err != nil {
		return err
	}
	return runRed(cfg, stdout)
}

func parseRedConfig(args []string) (redConfig, error) {
	fs := flag.NewFlagSet("pokebench red", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	romPath := fs.String("rom", os.Getenv("POKEMON_RED_ROM"), "path to supported Pokemon Red ROM")
	corpus := fs.String("corpus", os.Getenv("POKEPILOT_QUALIFICATION_CORPUS"), "private checkpoint corpus/root")
	mode := fs.String("mode", "speedrun", "run mode recorded in benchmark identity")
	from := fs.String("from", "fresh", "fresh or checkpoint:<name-or-path>")
	until := fs.String("until", "hall-of-fame", "semantic milestone to stop at")
	runs := fs.Int("runs", 1, "number of sequential benchmark runs")
	seed := fs.Int64("seed", 1, "first deterministic fresh-run seed")
	seedsRaw := fs.String("seeds", "", "comma-separated exact seed sequence; overrides --seed")
	output := fs.String("output", "benchmarks/current", "benchmark result root")
	maxFrames := fs.Int("max-frames", defaultMaxFrames, "per-run emulator frame budget")
	llmProfile := fs.String("llm-profile", os.Getenv("POKEPILOT_LLM_PROFILE"), "LLM deployment profile")
	reasoning := fs.String("reasoning-effort", os.Getenv("POKEPILOT_REASONING_EFFORT"), "LLM reasoning effort")
	if err := fs.Parse(args); err != nil {
		return redConfig{}, err
	}
	if fs.NArg() != 0 {
		return redConfig{}, fmt.Errorf("pokebench red: unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	cfg := redConfig{
		romPath: strings.TrimSpace(*romPath), corpus: strings.TrimSpace(*corpus),
		mode: strings.TrimSpace(*mode), from: strings.TrimSpace(*from), until: strings.TrimSpace(*until),
		runs: *runs, seed: *seed, output: strings.TrimSpace(*output), maxFrames: *maxFrames,
		llmProfile: strings.TrimSpace(*llmProfile), reasoningEffort: strings.TrimSpace(*reasoning),
	}
	if cfg.mode == "" {
		cfg.mode = "speedrun"
	}
	if cfg.mode != "speedrun" {
		return redConfig{}, fmt.Errorf("pokebench red: unsupported --mode %q; current qualification mode is speedrun", cfg.mode)
	}
	if cfg.runs < 1 {
		return redConfig{}, fmt.Errorf("pokebench red: --runs must be >= 1")
	}
	if cfg.maxFrames <= 0 {
		return redConfig{}, fmt.Errorf("pokebench red: --max-frames must be > 0")
	}
	if cfg.output == "" {
		return redConfig{}, fmt.Errorf("pokebench red: --output must not be empty")
	}
	if cfg.until == "" {
		return redConfig{}, fmt.Errorf("pokebench red: --until must not be empty")
	}
	if strings.TrimSpace(*seedsRaw) != "" {
		cfg.seeds, err = parseSeeds(*seedsRaw)
		if err != nil {
			return redConfig{}, err
		}
	}
	return cfg, nil
}

func runRed(cfg redConfig, stdout io.Writer) error {
	var err error
	cfg.romPath, err = defaultPrivatePath(cfg.romPath, "pokemon_red.gb")
	if err != nil {
		return err
	}
	cfg.corpus, err = defaultPrivatePath(cfg.corpus, "qualification")
	if err != nil {
		return err
	}
	romBytes, err := os.ReadFile(cfg.romPath)
	if err != nil {
		return fmt.Errorf("pokebench: read ROM: %w", err)
	}
	if err := redrom.Verify(romBytes); err != nil {
		return fmt.Errorf("pokebench: unsupported ROM: %w", err)
	}
	sum := sha256.Sum256(romBytes)
	romSHA256 := fmt.Sprintf("%x", sum[:])
	goal, ok := redbench.GoalFor(cfg.until)
	if !ok {
		return fmt.Errorf("pokebench: Red milestone %q does not have a deterministic stop condition", cfg.until)
	}
	source, resumeFrom, err := resolveSource(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.output, 0o755); err != nil {
		return err
	}
	commit := revision()
	fmt.Fprintf(stdout, "pokebench: Red %s -> %s, %d run(s), revision %s, ROM sha256 %s verified\n",
		cfg.from, cfg.until, cfg.runs, shortRevision(commit), romSHA256[:12])

	results := make([]benchmark.Result, 0, cfg.runs)
	failed := false
	for i := 0; i < cfg.runs; i++ {
		seed := cfg.seed + int64(i)
		if len(cfg.seeds) > 0 {
			seed = cfg.seeds[i%len(cfg.seeds)]
		}
		result, runErr := runRedOnce(cfg, source, resumeFrom, goal, romSHA256, commit, i+1, seed, stdout)
		if runErr != nil {
			return runErr
		}
		results = append(results, result)
		failed = failed || result.Outcome != "completed"
	}
	aggregate := benchmark.Summarize(results)
	if err := benchmark.WriteJSON(filepath.Join(cfg.output, "benchmark-summary.json"), aggregate); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "\ncompletion: %d/%d", aggregate.Completed, aggregate.Runs)
	if aggregate.Completed > 0 {
		fmt.Fprintf(stdout, ", median frames: %d, median wall: %.1fs", aggregate.MedianFrames, aggregate.MedianWallSeconds)
	}
	fmt.Fprintln(stdout)
	if failed {
		fmt.Fprintf(stdout, "pokebench: one or more runs failed; structured failure results and replay checkpoints were preserved under %s\n", cfg.output)
	}
	return nil
}

func runRedOnce(cfg redConfig, source benchmark.Source, resumeFrom, goal, romSHA256, commit string, index int, seed int64, stdout io.Writer) (benchmark.Result, error) {
	runID := fmt.Sprintf("%s-%02d-seed%d", time.Now().UTC().Format("20060102T150405.000000000Z"), index, seed)
	sourceName := source.Kind
	runDir := filepath.Join(cfg.output, fmt.Sprintf("pokemon-red-%s-%s-%s", sourceName, shortRevision(commit), runID))
	checkpointDir := filepath.Join(runDir, "objective-checkpoints")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		return benchmark.Result{}, err
	}
	logFile, err := os.Create(filepath.Join(runDir, "run.log"))
	if err != nil {
		return benchmark.Result{}, err
	}
	defer logFile.Close()
	promptFile, err := os.Create(filepath.Join(runDir, "prompts.txt"))
	if err != nil {
		return benchmark.Result{}, err
	}
	defer promptFile.Close()
	replyFile, err := os.Create(filepath.Join(runDir, "replies.txt"))
	if err != nil {
		return benchmark.Result{}, err
	}
	defer replyFile.Close()
	logWriter := io.MultiWriter(stdout, logFile)

	m, err := emu.Open(cfg.romPath)
	if err != nil {
		return benchmark.Result{}, fmt.Errorf("pokebench: open ROM: %w", err)
	}
	defer m.Close()
	if source.Kind == "fresh" {
		if _, err := skill.BootToOverworld(m); err != nil {
			return benchmark.Result{}, fmt.Errorf("pokebench: boot: %w", err)
		}
		if seed != 0 {
			burn := rand.New(rand.NewPCG(uint64(seed), 0)).IntN(600)
			m.StepFrames(burn)
		}
	}

	profile := agent.NormalizeLLMProfile(cfg.llmProfile)
	primaryCfg, fallbackCfg := agent.ResolveLLMEndpointsWithEffort(profile, agent.NormalizeReasoningEffort(cfg.reasoningEffort))
	primary := agent.NewLLMPlannerFromConfig(primaryCfg)
	primary.Goal = goal
	primary.Log = logWriter
	primary.PromptLog = promptFile
	primary.ReplyLog = replyFile
	var fallback *agent.LLMPlanner
	if fallbackCfg != nil {
		fallback = agent.NewLLMPlannerFromConfig(*fallbackCfg)
	}
	planner := agent.NewFailoverPlanner(primary, fallback)
	var calls []agent.LLMCall
	planner.OnCall = func(call agent.LLMCall) { calls = append(calls, call) }

	config := benchmark.Configuration{
		Planner: "agent.FailoverPlanner/qualification", Goal: goal, LLMProfile: string(profile),
		ReasoningEffort: cfg.reasoningEffort, PlayStyle: cfg.mode, EmulatorSpeed: "unthrottled; canonical score=emulator frames",
		Model: benchmark.ModelIdentity{
			Profile: string(profile), PrimaryModel: primaryCfg.Model, PrimaryURL: benchmark.SafeEndpoint(primaryCfg.BaseURL),
			NoThink: primaryCfg.NoThink, MaxTokens: primaryCfg.MaxTokens, Timeout: primaryCfg.Timeout.String(), PromptHash: primary.PromptHash(),
		},
		FeatureFlags: benchmark.SanitizeSettings(map[string]string{
			"POKEPILOT_RISK_TOLERANCE": os.Getenv("POKEPILOT_RISK_TOLERANCE"),
			"POKEPILOT_WILD_ENCOUNTERS": os.Getenv("POKEPILOT_WILD_ENCOUNTERS"),
			"POKEPILOT_DECISION_BACKEND": os.Getenv("POKEPILOT_DECISION_BACKEND"),
		}),
	}
	if fallbackCfg != nil {
		config.Model.FallbackModel = fallbackCfg.Model
		config.Model.FallbackURL = benchmark.SafeEndpoint(fallbackCfg.BaseURL)
	}

	started := time.Now()
	res := agent.Run(m, m.ROM(), planner, agent.Budget{
		MaxRounds: 0, MaxFrames: cfg.maxFrames, Goal: goal, Build: commit,
		Log: logWriter, CheckpointDir: checkpointDir, CheckpointKeep: 8192, ResumeFrom: resumeFrom,
	})
	finished := time.Now()
	finalState := filepath.Join(runDir, "final.state")
	if state, saveErr := m.SaveStateChecked(); saveErr == nil {
		if writeErr := os.WriteFile(finalState, state, 0o600); writeErr != nil {
			return benchmark.Result{}, writeErr
		}
	} else {
		finalState = ""
	}
	result := benchmark.Build(benchmark.BuildInput{
		RunID: runID, Commit: commit, Game: "pokemon-red", ROMSHA256: romSHA256, Mode: cfg.mode,
		Seed: seed, Source: source, EndCondition: cfg.until, Configuration: config, Profile: redbench.Profile(),
		AgentResult: res, Calls: calls, Route: planner.Route(), Health: planner.Health(),
		StartedAt: started, FinishedAt: finished, TerminalError: res.Err,
	})
	if err := benchmark.MaterializeCheckpoints(&result, checkpointDir, runDir, finalState); err != nil {
		return benchmark.Result{}, fmt.Errorf("pokebench: materialize milestone checkpoints: %w", err)
	}
	resultPath := filepath.Join(runDir, "benchmark-result.json")
	if err := benchmark.WriteJSON(resultPath, result); err != nil {
		return benchmark.Result{}, err
	}
	fmt.Fprintf(stdout, "\nRESULT: %s\n", strings.ToUpper(result.Outcome))
	fmt.Fprintf(stdout, "Run: %s\nFrames: %d\nWall: %.1fs\nLast completed split: %s\n",
		result.RunID, result.Frames, result.WallSeconds, result.LastMilestone)
	if len(result.Failures) > 0 {
		failure := result.Failures[0]
		fmt.Fprintf(stdout, "Failed objective: %s\nFailure fingerprint: %s\nCheckpoint: %s\n",
			failure.Objective, failure.Fingerprint, failure.Checkpoint)
	}
	fmt.Fprintf(stdout, "Result: %s\n", resultPath)
	return result, nil
}

func resolveSource(cfg redConfig) (benchmark.Source, string, error) {
	raw := strings.TrimSpace(cfg.from)
	if strings.EqualFold(raw, "fresh") {
		return benchmark.Source{Kind: "fresh"}, "", nil
	}
	name, ok := strings.CutPrefix(raw, "checkpoint:")
	if !ok || strings.TrimSpace(name) == "" {
		return benchmark.Source{}, "", fmt.Errorf("pokebench: --from must be fresh or checkpoint:<name-or-path>")
	}
	path, err := resolveCheckpoint(cfg.corpus, strings.TrimSpace(name))
	if err != nil {
		return benchmark.Source{}, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return benchmark.Source{}, "", err
	}
	sum := sha256.Sum256(data)
	return benchmark.Source{Kind: "checkpoint", Checkpoint: path, CheckpointSHA256: fmt.Sprintf("%x", sum[:])}, path, nil
}

func resolveCheckpoint(corpus, name string) (string, error) {
	candidates := []string{name}
	if corpus != "" {
		candidates = append(candidates,
			filepath.Join(corpus, name+".state"),
			filepath.Join(corpus, "checkpoints", name+".state"),
			filepath.Join(corpus, name, "start.state"),
		)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			absolute, absErr := filepath.Abs(candidate)
			if absErr != nil {
				return "", absErr
			}
			return absolute, nil
		}
	}
	return "", fmt.Errorf("pokebench: checkpoint %q not found (checked direct path and corpus %s)", name, corpus)
}

func runCompare(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("pokebench compare", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("pokebench compare: want BASELINE_DIR CANDIDATE_DIR")
	}
	baselineResults, err := benchmark.LoadResults(fs.Arg(0))
	if err != nil {
		return err
	}
	candidateResults, err := benchmark.LoadResults(fs.Arg(1))
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(stdout, benchmark.CompareText(benchmark.Summarize(baselineResults), benchmark.Summarize(candidateResults)))
	return err
}

func parseSeeds(raw string) ([]int64, error) {
	fields := strings.Split(raw, ",")
	out := make([]int64, 0, len(fields))
	for _, field := range fields {
		value, err := strconv.ParseInt(strings.TrimSpace(field), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("pokebench: invalid seed %q", field)
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("pokebench: --seeds must not be empty")
	}
	return out, nil
}

func defaultPrivatePath(current, leaf string) (string, error) {
	if current != "" {
		return current, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "pokepilot", leaf), nil
}

func revision() string {
	if sha := strings.TrimSpace(os.Getenv("GITHUB_SHA")); sha != "" {
		return sha
	}
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func shortRevision(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	if rev == "" {
		return "unknown"
	}
	return rev
}
