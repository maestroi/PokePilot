// Command pokebench runs reproducible Pokémon qualification benchmarks and
// compares their versioned results. It deliberately reuses agent.Run,
// qualification checkpoints, semantic observations, and farm failure identity.
package main

import (
	"context"
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

type policyPlanner struct {
	inner         *agent.FailoverPlanner
	style         agent.PlayStyleProfile
	risk          string
	wild          string
	decision      agent.DecisionSettings
	decisionCalls []benchmark.DecisionCall
}

func (p *policyPlanner) RoutePriority() agent.RoutePriority {
	return agent.RoutePriorityForPlayStyle(p.style)
}

func (p *policyPlanner) offered(obs agent.Observation, offered []agent.Objective) []agent.Objective {
	offered = agent.ApplyRunPolicy(obs, offered, p.risk, p.wild)
	return agent.AnnotatePlayStyle(obs, offered, p.style)
}

func (p *policyPlanner) Next(obs agent.Observation, offered []agent.Objective) (agent.Objective, error) {
	offered = p.offered(obs, offered)
	if p.decision.Engine != nil && p.decision.ObjectiveSelection {
		if objective, ok := p.typedObjective(obs, offered); ok {
			return objective, nil
		}
	}
	return p.inner.Next(obs, offered)
}

func (p *policyPlanner) NextRetry(obs agent.Observation, offered []agent.Objective, retry agent.Retry) (agent.Objective, error) {
	return p.inner.NextRetry(obs, p.offered(obs, offered), retry)
}

func (p *policyPlanner) Strategize(obs agent.Observation, offered []agent.Objective, reason string) (agent.Plan, error) {
	offered = p.offered(obs, offered)
	plan, err := p.inner.Strategize(obs, offered, reason)
	if err != nil {
		return plan, err
	}
	return p.boundRiskPlan(plan, offered), nil
}

func (p *policyPlanner) StrategizeRetry(obs agent.Observation, offered []agent.Objective, reason string, retry agent.Retry) (agent.Plan, error) {
	offered = p.offered(obs, offered)
	plan, err := p.inner.StrategizeRetry(obs, offered, reason, retry)
	if err != nil {
		return plan, err
	}
	return p.boundRiskPlan(plan, offered), nil
}

func (p *policyPlanner) typedObjective(obs agent.Observation, offered []agent.Objective) (agent.Objective, bool) {
	req, err := agent.ObjectiveDecisionRequest(obs, offered, p.inner.RunGoal())
	if err != nil {
		p.recordDecision(req, agent.DecisionResponse{}, err)
		return agent.Objective{}, false
	}
	resp, err := agent.DecideChecked(context.Background(), p.decision.Engine, req)
	if err == nil && p.decision.MinConfidence > 0 && resp.Confidence < p.decision.MinConfidence {
		err = fmt.Errorf("%w: %.3f < %.3f", agent.ErrDecisionLowConfidence, resp.Confidence, p.decision.MinConfidence)
	}
	var objective agent.Objective
	if err == nil {
		objective, err = agent.Chosen(offered, resp.Choice)
	}
	p.recordDecision(req, resp, err)
	return objective, err == nil
}

func (p *policyPlanner) DecideFailure(result agent.ObjectiveResult) (agent.DecisionResponse, error) {
	if p.decision.Engine == nil || !p.decision.FailureRecovery {
		return agent.DecisionResponse{}, agent.ErrDecisionDisabled
	}
	req, err := agent.FailureDecisionRequest(result)
	if err != nil {
		p.recordDecision(req, agent.DecisionResponse{}, err)
		return agent.DecisionResponse{}, err
	}
	resp, err := agent.DecideChecked(context.Background(), p.decision.Engine, req)
	if err == nil && p.decision.MinConfidence > 0 && resp.Confidence < p.decision.MinConfidence {
		err = fmt.Errorf("%w: %.3f < %.3f", agent.ErrDecisionLowConfidence, resp.Confidence, p.decision.MinConfidence)
	}
	p.recordDecision(req, resp, err)
	return resp, err
}

func (p *policyPlanner) recordDecision(req agent.DecisionRequest, resp agent.DecisionResponse, err error) {
	backend := resp.Backend
	if backend == "" {
		backend = p.decision.Backend
	}
	p.decisionCalls = append(p.decisionCalls, benchmark.DecisionCall{
		Kind: req.Kind, Duration: resp.Duration, PromptTokens: resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens, Backend: backend, Model: resp.Model, Err: err,
	})
}

func (p *policyPlanner) boundRiskPlan(plan agent.Plan, offered []agent.Objective) agent.Plan {
	if p.risk == agent.RiskToleranceAggressive || len(plan.Steps) <= 1 {
		return plan
	}
	for i, step := range plan.Steps {
		objective, err := agent.Chosen(offered, step)
		if err != nil {
			continue
		}
		risky := false
		switch objective.Kind {
		case agent.KindTrainer, agent.KindGym, agent.KindTrain, agent.KindCatch:
			risky = true
		case agent.KindGoTo:
			risky = !objective.Flee || p.wild == agent.WildEncountersFight
		}
		if risky && i+1 < len(plan.Steps) {
			plan.Steps = append([]string(nil), plan.Steps[:i+1]...)
			break
		}
	}
	return plan
}

func decisionIdentity(settings agent.DecisionSettings) map[string]string {
	out := map[string]string{
		"POKEPILOT_DECISION_BACKEND":        settings.Backend,
		"POKEPILOT_DECISION_MIN_CONFIDENCE": fmt.Sprintf("%.3f", settings.MinConfidence),
		"POKEPILOT_DECISION_OBJECTIVES":     strconv.FormatBool(settings.ObjectiveSelection),
		"POKEPILOT_DECISION_FAILURES":       strconv.FormatBool(settings.FailureRecovery),
	}
	if engine, ok := settings.Engine.(*agent.OpenAIDecisionEngine); ok {
		out["POKEPILOT_DECISION_URL"] = engine.BaseURL
		out["POKEPILOT_DECISION_MODEL"] = engine.Model
		out["POKEPILOT_DECISION_TIMEOUT"] = engine.Timeout.String()
		out["POKEPILOT_DECISION_MAX_TOKENS"] = strconv.Itoa(engine.MaxTokens)
	}
	return benchmark.SanitizeSettings(out)
}

func (p *policyPlanner) Usage() (int, int) { return p.inner.Usage() }
func (p *policyPlanner) RunGoal() string   { return p.inner.RunGoal() }

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
		seeds, err := parseSeeds(*seedsRaw)
		if err != nil {
			return redConfig{}, err
		}
		cfg.seeds = seeds
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
		return fmt.Errorf("pokebench: qualification failed; see %s", cfg.output)
	}
	return nil
}

func runRedOnce(cfg redConfig, source benchmark.Source, resumeFrom, goal, romSHA256, commit string, index int, seed int64, stdout io.Writer) (benchmark.Result, error) {
	runID := fmt.Sprintf("%s-%02d-seed%d", time.Now().UTC().Format("20060102T150405.000000000Z"), index, seed)
	sourceName := source.Kind
	runDir := filepath.Join(cfg.output, fmt.Sprintf("pokemon-red-%s-to-%s-%s-%s", sourceName, strings.ReplaceAll(cfg.until, "_", "-"), shortRevision(commit), runID))
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
	primary.ExtraSystem = agent.PlayStyleSystemNote(cfg.mode)
	primary.Log = logWriter
	primary.PromptLog = promptFile
	primary.ReplyLog = replyFile
	var fallback *agent.LLMPlanner
	if fallbackCfg != nil {
		fallback = agent.NewLLMPlannerFromConfig(*fallbackCfg)
	}
	router := agent.NewFailoverPlanner(primary, fallback)
	risk := agent.NormalizeRiskTolerance(os.Getenv("POKEPILOT_RISK_TOLERANCE"))
	wild := agent.NormalizeWildEncounters(os.Getenv("POKEPILOT_WILD_ENCOUNTERS"))
	decision := agent.DecisionSettingsFromEnv()
	planner := &policyPlanner{inner: router, style: agent.PlayStyle(cfg.mode), risk: risk, wild: wild, decision: decision}
	var calls []agent.LLMCall
	router.OnCall = func(call agent.LLMCall) { calls = append(calls, call) }

	config := benchmark.Configuration{
		Planner: "agent.FailoverPlanner+run-policy", Goal: goal, LLMProfile: string(profile),
		ReasoningEffort: primaryCfg.ReasoningEffort, PlayStyle: cfg.mode, RiskTolerance: risk, WildEncounters: wild,
		DecisionBackend: decision.Backend,
		EmulatorSpeed:   "unthrottled; canonical score=emulator frames",
		MaxFrames:       cfg.maxFrames,
		Model: benchmark.ModelIdentity{
			Profile: string(profile), PrimaryModel: primaryCfg.Model, PrimaryURL: benchmark.SafeEndpoint(primaryCfg.BaseURL),
			NoThink: primaryCfg.NoThink, MaxTokens: primaryCfg.MaxTokens, Timeout: primaryCfg.Timeout.String(),
			ReasoningEffort: primaryCfg.ReasoningEffort, RecoveryReasoningEffort: primaryCfg.RecoveryReasoningEffort,
			PromptHash: primary.PromptHash(),
		},
		FeatureFlags: decisionIdentity(decision),
	}
	if fallbackCfg != nil {
		config.Model.FallbackModel = fallbackCfg.Model
		config.Model.FallbackURL = benchmark.SafeEndpoint(fallbackCfg.BaseURL)
		config.Model.FallbackNoThink = fallbackCfg.NoThink
		config.Model.FallbackMaxTokens = fallbackCfg.MaxTokens
		config.Model.FallbackTimeout = fallbackCfg.Timeout.String()
		config.Model.FallbackReasoningEffort = fallbackCfg.ReasoningEffort
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
		AgentResult: res, Calls: calls, DecisionCalls: planner.decisionCalls, Route: router.Route(), Health: router.Health(),
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
	source := benchmark.Source{Kind: "checkpoint", Checkpoint: strings.TrimSpace(name), CheckpointSHA256: fmt.Sprintf("%x", sum[:])}
	if meta, ok, metaErr := benchmark.ReadCheckpointMetadata(path); metaErr != nil {
		return benchmark.Source{}, "", fmt.Errorf("pokebench: checkpoint metadata: %w", metaErr)
	} else if ok {
		source.OriginRunID = meta.RunID
		source.OriginCommit = meta.Commit
		source.OriginMilestone = meta.Milestone
		source.OriginSeed = meta.Seed
	}
	return source, path, nil
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
