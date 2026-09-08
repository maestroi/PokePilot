// Command pokequal runs the private ROM-backed qualification pyramid.
//
// The commercial ROM stays on the local/self-hosted runner, is verified before
// any scenario runs, and is never copied into qualification output. Evidence is
// limited to logs, save states/checkpoints, semantic observations, and runner
// metadata needed to replay a failing leg.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/qualification"
	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/skill"
)

const (
	manifestVersion = 1
	fullMaxFrames   = 8 * 60 * 60 * 60
)

type config struct {
	romPath string
	profile string
	only    string
	corpus  string
	out     string
	list    bool
}

type manifest struct {
	SchemaVersion int          `json:"schema_version"`
	RunID         string       `json:"run_id"`
	Profile       string       `json:"profile"`
	Revision      string       `json:"revision,omitempty"`
	Ref           string       `json:"ref,omitempty"`
	StartedAt     time.Time    `json:"started_at"`
	FinishedAt    time.Time    `json:"finished_at"`
	ROM           romIdentity  `json:"rom"`
	Runner        runnerInfo   `json:"runner"`
	Model         modelInfo    `json:"model"`
	Cases         []caseResult `json:"cases"`
}

type romIdentity struct {
	SHA1     string `json:"sha1"`
	Verified bool   `json:"verified"`
}

type runnerInfo struct {
	Name      string `json:"name,omitempty"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version"`
}

// modelInfo intentionally has no credential/token field.
type modelInfo struct {
	Profile       string `json:"profile,omitempty"`
	URL           string `json:"url,omitempty"`
	Model         string `json:"model,omitempty"`
	NoThink       bool   `json:"no_think,omitempty"`
	MaxTokens     int    `json:"max_tokens,omitempty"`
	Timeout       string `json:"timeout,omitempty"`
	FallbackURL   string `json:"fallback_url,omitempty"`
	FallbackModel string `json:"fallback_model,omitempty"`
}

type caseResult struct {
	ID             string                    `json:"id"`
	Layer          qualification.Layer       `json:"layer"`
	Description    string                    `json:"description"`
	Status         string                    `json:"status"`
	StartedAt      time.Time                 `json:"started_at"`
	FinishedAt     time.Time                 `json:"finished_at"`
	Duration       string                    `json:"duration"`
	Command        []string                  `json:"command,omitempty"`
	Error          string                    `json:"error,omitempty"`
	Checkpoint     string                    `json:"checkpoint,omitempty"`
	CheckpointHash string                    `json:"checkpoint_sha256,omitempty"`
	Expectation    qualification.Expectation `json:"expect,omitempty"`
	Evidence       []string                  `json:"evidence,omitempty"`
}

type fullRunEvidence struct {
	Stop          string            `json:"stop"`
	Rounds        int               `json:"rounds"`
	Completed     int               `json:"completed"`
	Goal          *agent.GoalStatus `json:"goal_status,omitempty"`
	Route         agent.LLMRoute    `json:"llm_route"`
	Health        agent.LLMHealth   `json:"llm_health"`
	PromptTokens  int               `json:"prompt_tokens"`
	OutputTokens  int               `json:"completion_tokens"`
	Final         agent.Observation `json:"final"`
	TerminalError string            `json:"terminal_error,omitempty"`
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("pokequal", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	romPath := fs.String("rom", os.Getenv("POKEMON_RED_ROM"), "path to supported Pokemon Red ROM")
	profile := fs.String("profile", "milestones", "qualification profile: skills, milestones, full, all")
	only := fs.String("case", "", "run one exact qualification case")
	corpus := fs.String("corpus", os.Getenv("POKEPILOT_QUALIFICATION_CORPUS"), "private checkpoint corpus root")
	out := fs.String("out", "qualification-out", "qualification evidence directory")
	list := fs.Bool("list", false, "list qualification cases without requiring a ROM")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cfg := config{
		romPath: strings.TrimSpace(*romPath),
		profile: strings.TrimSpace(*profile),
		only:    strings.TrimSpace(*only),
		corpus:  strings.TrimSpace(*corpus),
		out:     strings.TrimSpace(*out),
		list:    *list,
	}
	if cfg.out == "" {
		return config{}, fmt.Errorf("pokequal: -out must not be empty")
	}
	return cfg, nil
}

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	if cfg.list {
		return printCatalog(stdout)
	}
	cases, err := qualification.Select(cfg.profile, cfg.only)
	if err != nil {
		return err
	}
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
		return fmt.Errorf("pokequal: read ROM: %w", err)
	}
	if err := redrom.Verify(romBytes); err != nil {
		return fmt.Errorf("pokequal: unsupported ROM: %w", err)
	}
	romSHA1 := redrom.SHA1Hex(romBytes)

	if err := os.MkdirAll(cfg.out, 0o755); err != nil {
		return fmt.Errorf("pokequal: create output: %w", err)
	}
	report := manifest{
		SchemaVersion: manifestVersion,
		RunID:         qualificationRunID(),
		Profile:       selectedProfile(cfg),
		Revision:      revision(),
		Ref:           strings.TrimSpace(os.Getenv("GITHUB_REF_NAME")),
		StartedAt:     time.Now().UTC(),
		ROM:           romIdentity{SHA1: romSHA1, Verified: true},
		Runner: runnerInfo{
			Name:      firstNonEmpty(os.Getenv("RUNNER_NAME"), os.Getenv("HOSTNAME")),
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			GoVersion: runtime.Version(),
		},
		Model: resolvedModelInfo(),
	}

	fmt.Fprintf(stdout, "pokequal: run %s, profile %s, revision %s, ROM %s verified\n",
		report.RunID, report.Profile, shortRevision(report.Revision), romSHA1)
	failed := false
	for _, c := range cases {
		result := executeCase(cfg, c, stdout)
		report.Cases = append(report.Cases, result)
		failed = failed || result.Status != "passed"
		report.FinishedAt = time.Now().UTC()
		if err := writeJSON(filepath.Join(cfg.out, "qualification.json"), report); err != nil {
			return err
		}
	}
	if failed {
		return fmt.Errorf("pokequal: one or more qualification cases failed; evidence: %s", filepath.Join(cfg.out, "qualification.json"))
	}
	return nil
}

func printCatalog(w io.Writer) error {
	for _, c := range qualification.Catalog() {
		state := "ready"
		if !c.Available {
			state = fmt.Sprintf("blocked #%d", c.BlockedBy)
		} else if c.BlockedBy != 0 {
			state = fmt.Sprintf("runner ready; product blocked #%d", c.BlockedBy)
		}
		fmt.Fprintf(w, "%-30s %-10s %-28s %s\n", c.ID, c.Layer, state, c.Description)
	}
	return nil
}

func executeCase(cfg config, c qualification.Case, stdout io.Writer) caseResult {
	result := caseResult{
		ID:          c.ID,
		Layer:       c.Layer,
		Description: c.Description,
		Status:      "failed",
		StartedAt:   time.Now().UTC(),
		Checkpoint:  c.Checkpoint,
		Expectation: c.Expect,
	}
	caseDir := filepath.Join(cfg.out, "cases", c.ID)
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		return finishCase(result, err)
	}
	fmt.Fprintf(stdout, "\n=== %s [%s] ===\n", c.ID, c.Layer)

	var err error
	switch c.Runner {
	case qualification.RunnerGoTest:
		result.Command, result.Evidence, err = runGoTestCase(cfg, c, caseDir, stdout)
	case qualification.RunnerRedSkill:
		result.Evidence, result.CheckpointHash, err = runRedSkillCase(cfg, c, caseDir)
	case qualification.RunnerFullRun:
		result.Evidence, err = runFullCase(cfg, caseDir, stdout)
	default:
		err = fmt.Errorf("pokequal: case %s has unsupported runner %q", c.ID, c.Runner)
	}
	if err == nil {
		result.Status = "passed"
	}
	result = finishCase(result, err)
	if err != nil {
		fmt.Fprintf(stdout, "FAIL %s: %v\n", c.ID, err)
	} else {
		fmt.Fprintf(stdout, "PASS %s\n", c.ID)
	}
	_ = writeJSON(filepath.Join(caseDir, "result.json"), result)
	return result
}

func runGoTestCase(cfg config, c qualification.Case, caseDir string, stdout io.Writer) ([]string, []string, error) {
	args := []string{"test"}
	if c.Short {
		args = append(args, "-short")
	}
	args = append(args, "-count=1", c.Package)
	if c.Test != "" {
		args = append(args, "-run", c.Test, "-v")
	}
	command := append([]string{"go"}, args...)
	logPath := filepath.Join(caseDir, "run.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return command, nil, err
	}
	defer logFile.Close()
	fixtureDir := filepath.Join(cfg.out, "fixtures")
	cmd := exec.Command("go", args...)
	cmd.Env = qualificationEnv(cfg, fixtureDir)
	cmd.Stdout = io.MultiWriter(stdout, logFile)
	cmd.Stderr = io.MultiWriter(stdout, logFile)
	err = cmd.Run()
	evidence := []string{relativeEvidence(cfg.out, logPath)}
	if strings.HasPrefix(c.Checkpoint, "fixture:") {
		// The named fixture cache is inside the uploaded output. It is the exact
		// replayable input used by this run, whether the test passed or failed.
		evidence = append(evidence, relativeEvidence(cfg.out, fixtureDir))
	}
	if err != nil && c.Test != "" {
		if copied, copyErr := copyJourneyFailureState(c.Test, caseDir); copyErr == nil && copied != "" {
			evidence = append(evidence, relativeEvidence(cfg.out, copied))
		}
	}
	return command, evidence, err
}

func runRedSkillCase(cfg config, c qualification.Case, caseDir string) ([]string, string, error) {
	checkpoint, err := corpusCheckpoint(cfg.corpus, c.Checkpoint)
	if err != nil {
		return nil, "", err
	}
	stateBytes, err := os.ReadFile(checkpoint)
	if err != nil {
		return nil, "", fmt.Errorf("pokequal: %s checkpoint %s: %w", c.ID, checkpoint, err)
	}
	checkpointHash := sha256Hex(stateBytes)
	startCopy := filepath.Join(caseDir, "start.state")
	if err := os.WriteFile(startCopy, stateBytes, 0o600); err != nil {
		return nil, checkpointHash, err
	}
	evidence := []string{relativeEvidence(cfg.out, startCopy)}

	m, err := emu.Open(cfg.romPath)
	if err != nil {
		return evidence, checkpointHash, fmt.Errorf("pokequal: %s open ROM: %w", c.ID, err)
	}
	defer m.Close()
	if err := m.LoadState(stateBytes); err != nil {
		return evidence, checkpointHash, fmt.Errorf("pokequal: %s load checkpoint: %w", c.ID, err)
	}
	romBytes := m.ROM()
	initial := agent.Observe(m, romBytes)
	initialPath := filepath.Join(caseDir, "initial-observation.json")
	if err := writeJSON(initialPath, initial); err == nil {
		evidence = append(evidence, relativeEvidence(cfg.out, initialPath))
	}
	if !initial.Controllable {
		return evidence, checkpointHash, fmt.Errorf("pokequal: %s checkpoint is not a settled controllable state", c.ID)
	}

	policy := skill.StatAwareMove(romBytes)
	var actionErr error
	switch c.Action {
	case "rocket-hideout":
		actionErr = skill.RocketHideout(m, romBytes, policy)
	case "pokemon-tower":
		actionErr = skill.PokemonTower(m, romBytes, policy)
	default:
		actionErr = fmt.Errorf("unknown Red qualification action %q", c.Action)
	}

	final := agent.Observe(m, romBytes)
	finalPath := filepath.Join(caseDir, "final-observation.json")
	if err := writeJSON(finalPath, final); err == nil {
		evidence = append(evidence, relativeEvidence(cfg.out, finalPath))
	}
	if checked, saveErr := m.SaveStateChecked(); saveErr == nil {
		finalState := filepath.Join(caseDir, "final.state")
		if writeErr := os.WriteFile(finalState, checked, 0o600); writeErr == nil {
			evidence = append(evidence, relativeEvidence(cfg.out, finalState))
		}
	}
	logPath := filepath.Join(caseDir, "run.log")
	logText := fmt.Sprintf("case=%s\naction=%s\ncheckpoint_sha256=%s\naction_error=%v\nfinal_location=%s\nfinal_xy=%d,%d\n",
		c.ID, c.Action, checkpointHash, actionErr, final.Location, final.X, final.Y)
	if err := os.WriteFile(logPath, []byte(logText), 0o600); err == nil {
		evidence = append(evidence, relativeEvidence(cfg.out, logPath))
	}
	if actionErr != nil {
		return evidence, checkpointHash, actionErr
	}
	if err := verifyExpectation(c.Expect, final); err != nil {
		return evidence, checkpointHash, fmt.Errorf("pokequal: %s postcondition: %w", c.ID, err)
	}
	return evidence, checkpointHash, nil
}

// runFullCase owns a fresh emulator directly so success comes from typed
// GoalStatus/semantic observation, not from parsing pokepilot console prose.
func runFullCase(cfg config, caseDir string, stdout io.Writer) ([]string, error) {
	checkpointDir := filepath.Join(caseDir, "checkpoints")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(caseDir, "run.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()
	promptPath := filepath.Join(caseDir, "prompts.txt")
	promptFile, err := os.Create(promptPath)
	if err != nil {
		return nil, err
	}
	defer promptFile.Close()
	replyPath := filepath.Join(caseDir, "replies.txt")
	replyFile, err := os.Create(replyPath)
	if err != nil {
		return nil, err
	}
	defer replyFile.Close()
	evidence := []string{
		relativeEvidence(cfg.out, logPath),
		relativeEvidence(cfg.out, promptPath),
		relativeEvidence(cfg.out, replyPath),
		relativeEvidence(cfg.out, checkpointDir),
	}
	logWriter := io.MultiWriter(stdout, logFile)

	m, err := emu.Open(cfg.romPath)
	if err != nil {
		return evidence, fmt.Errorf("pokequal: full run open ROM: %w", err)
	}
	defer m.Close()
	if _, err := skill.BootToOverworld(m); err != nil {
		return evidence, fmt.Errorf("pokequal: full run boot: %w", err)
	}

	profile := agent.NormalizeLLMProfile(os.Getenv("POKEPILOT_LLM_PROFILE"))
	primaryCfg, fallbackCfg := agent.ResolveLLMEndpoints(profile)
	primary := agent.NewLLMPlannerFromConfig(primaryCfg)
	primary.Goal = "elite-four"
	primary.Log = logWriter
	primary.PromptLog = promptFile
	primary.ReplyLog = replyFile
	var fallback *agent.LLMPlanner
	if fallbackCfg != nil {
		fallback = agent.NewLLMPlannerFromConfig(*fallbackCfg)
	}
	planner := agent.NewFailoverPlanner(primary, fallback)

	res := agent.Run(m, m.ROM(), planner, agent.Budget{
		MaxRounds:     0,
		MaxFrames:     fullMaxFrames,
		Log:           logWriter,
		CheckpointDir: checkpointDir,
	})
	final := agent.Observe(m, m.ROM())
	goal, structured, goalErr := agent.PlannerGoalStatus("elite-four", final)
	if !structured && goalErr == nil {
		goalErr = fmt.Errorf("elite-four goal was not recognized as structured")
	}
	if res.GoalStatus == nil {
		res.GoalStatus = &goal
	}

	if checked, saveErr := m.SaveStateChecked(); saveErr == nil {
		finalState := filepath.Join(caseDir, "final.state")
		if writeErr := os.WriteFile(finalState, checked, 0o600); writeErr == nil {
			evidence = append(evidence, relativeEvidence(cfg.out, finalState))
		}
	}
	obsPath := filepath.Join(caseDir, "final-observation.json")
	if err := writeJSON(obsPath, final); err == nil {
		evidence = append(evidence, relativeEvidence(cfg.out, obsPath))
	}

	promptTokens, completionTokens := planner.Usage()
	runEvidence := fullRunEvidence{
		Stop:         stopName(res.Stop),
		Rounds:       res.Rounds,
		Completed:    len(res.Completed),
		Goal:         res.GoalStatus,
		Route:        planner.Route(),
		Health:       planner.Health(),
		PromptTokens: promptTokens,
		OutputTokens: completionTokens,
		Final:        final,
	}
	if res.Err != nil {
		runEvidence.TerminalError = res.Err.Error()
	}
	runResultPath := filepath.Join(caseDir, "run-result.json")
	if err := writeJSON(runResultPath, runEvidence); err == nil {
		evidence = append(evidence, relativeEvidence(cfg.out, runResultPath))
	}

	if goalErr != nil {
		return evidence, fmt.Errorf("pokequal: full run goal evaluation: %w", goalErr)
	}
	if res.Stop != agent.StopDone || !goal.Complete {
		return evidence, fmt.Errorf("pokequal: fresh campaign did not reach Hall of Fame: stop=%s goal=%s", stopName(res.Stop), goal.Summary)
	}
	return evidence, nil
}

func verifyExpectation(expect qualification.Expectation, obs agent.Observation) error {
	switch expect.Kind {
	case "":
		return nil
	case "item":
		for _, item := range obs.Bag {
			if strings.EqualFold(item.Name, expect.Value) && item.Quantity > 0 {
				return nil
			}
		}
		return fmt.Errorf("item %q is not owned", expect.Value)
	default:
		return fmt.Errorf("unsupported expectation kind %q", expect.Kind)
	}
}

func corpusCheckpoint(root, rel string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("pokequal: qualification corpus is required")
	}
	if rel == "" || strings.HasPrefix(rel, "fixture:") {
		return "", fmt.Errorf("pokequal: %q is not a private corpus checkpoint", rel)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(rootAbs, filepath.Clean(rel)))
	if err != nil {
		return "", err
	}
	within, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return "", err
	}
	if within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("pokequal: checkpoint %q escapes corpus root", rel)
	}
	return pathAbs, nil
}

func qualificationEnv(cfg config, fixtureDir string) []string {
	env := os.Environ()
	env = setEnv(env, "POKEMON_RED_ROM", cfg.romPath)
	env = setEnv(env, "POKEPILOT_FIXTURE_DIR", fixtureDir)
	// A self-hosted runner may also be a farm worker. Qualification must not
	// accidentally lease unrelated work while running child tests.
	env = setEnv(env, "POKEPILOT_ORCH_URL", "")
	return env
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := env[:0]
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		out = append(out, entry)
	}
	return append(out, prefix+value)
}

func copyJourneyFailureState(testPattern, caseDir string) (string, error) {
	name := strings.TrimSuffix(strings.TrimPrefix(testPattern, "^"), "$")
	if name == "" || strings.ContainsAny(name, "/\\|()[]*+?") {
		return "", nil
	}
	src := filepath.Join("skill", "failure", name+".state")
	data, err := os.ReadFile(src)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	dst := filepath.Join(caseDir, "failure.state")
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return "", err
	}
	return dst, nil
}

func defaultPrivatePath(current, leaf string) (string, error) {
	if current != "" {
		return current, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("pokequal: resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "pokepilot", leaf), nil
}

func qualificationRunID() string {
	if id := strings.TrimSpace(os.Getenv("GITHUB_RUN_ID")); id != "" {
		attempt := firstNonEmpty(os.Getenv("GITHUB_RUN_ATTEMPT"), "1")
		return "github-" + id + "-attempt-" + attempt
	}
	return "local-" + time.Now().UTC().Format("20060102T150405.000000000Z")
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

func resolvedModelInfo() modelInfo {
	profile := agent.NormalizeLLMProfile(os.Getenv("POKEPILOT_LLM_PROFILE"))
	primary, fallback := agent.ResolveLLMEndpoints(profile)
	info := modelInfo{
		Profile:   string(profile),
		URL:       primary.BaseURL,
		Model:     primary.Model,
		NoThink:   primary.NoThink,
		MaxTokens: primary.MaxTokens,
		Timeout:   primary.Timeout.String(),
	}
	if fallback != nil {
		info.FallbackURL = fallback.BaseURL
		info.FallbackModel = fallback.Model
	}
	return info
}

func selectedProfile(cfg config) string {
	if cfg.only != "" {
		return "case:" + cfg.only
	}
	if cfg.profile == "" {
		return "milestones"
	}
	return cfg.profile
}

func finishCase(r caseResult, err error) caseResult {
	r.FinishedAt = time.Now().UTC()
	r.Duration = r.FinishedAt.Sub(r.StartedAt).Round(time.Millisecond).String()
	if err != nil {
		r.Error = err.Error()
	}
	return r
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("pokequal: write %s: %w", path, err)
	}
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func relativeEvidence(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	default:
		return "unset"
	}
}
