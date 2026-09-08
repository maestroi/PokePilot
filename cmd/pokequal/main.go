// Command pokequal runs the private ROM-backed qualification pyramid.
//
// It is intentionally separate from public CI. The commercial ROM stays on
// the local/self-hosted runner, is verified before any scenario runs, and is
// never copied into the qualification output. Outputs contain only metadata,
// logs, checkpoints/save states, and semantic observations needed to replay a
// failing leg.
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
	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/qualification"
	"github.com/maestroi/pokepilot/skill"
)

const manifestVersion = 1

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

type modelInfo struct {
	Profile   string `json:"profile,omitempty"`
	URL       string `json:"url,omitempty"`
	Model     string `json:"model,omitempty"`
	NoThink   string `json:"no_think,omitempty"`
	MaxTokens string `json:"max_tokens,omitempty"`
	Timeout   string `json:"timeout,omitempty"`
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
	m := manifest{
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
		Model: modelInfo{
			Profile:   strings.TrimSpace(os.Getenv("POKEPILOT_LLM_PROFILE")),
			URL:       strings.TrimSpace(os.Getenv("POKEPILOT_LLM_URL")),
			Model:     strings.TrimSpace(os.Getenv("POKEPILOT_LLM_MODEL")),
			NoThink:   strings.TrimSpace(os.Getenv("POKEPILOT_LLM_NO_THINK")),
			MaxTokens: strings.TrimSpace(os.Getenv("POKEPILOT_LLM_MAX_TOKENS")),
			Timeout:   strings.TrimSpace(os.Getenv("POKEPILOT_LLM_TIMEOUT")),
		},
	}

	fmt.Fprintf(stdout, "pokequal: run %s, profile %s, revision %s, ROM %s verified\n", m.RunID, m.Profile, shortRevision(m.Revision), romSHA1)
	failed := false
	for _, c := range cases {
		result := executeCase(cfg, c, stdout)
		m.Cases = append(m.Cases, result)
		if result.Status != "passed" {
			failed = true
		}
		m.FinishedAt = time.Now().UTC()
		if err := writeJSON(filepath.Join(cfg.out, "qualification.json"), m); err != nil {
			return err
		}
	}
	m.FinishedAt = time.Now().UTC()
	if err := writeJSON(filepath.Join(cfg.out, "qualification.json"), m); err != nil {
		return err
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
			state = fmt.Sprintf("blocked #%.0d", c.BlockedBy)
		} else if c.BlockedBy != 0 {
			state = fmt.Sprintf("runner ready; product blocked #%d", c.BlockedBy)
		}
		fmt.Fprintf(w, "%-30s %-10s %-28s %s\n", c.ID, c.Layer, state, c.Description)
	}
	return nil
}

func executeCase(cfg config, c qualification.Case, stdout io.Writer) caseResult {
	started := time.Now().UTC()
	result := caseResult{
		ID:          c.ID,
		Layer:       c.Layer,
		Description: c.Description,
		Status:      "failed",
		StartedAt:   started,
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
		result.Command, result.Evidence, err = runFullCase(cfg, c, caseDir, stdout)
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
	cmd := exec.Command("go", args...)
	cmd.Env = qualificationEnv(cfg, filepath.Join(cfg.out, "fixtures"))
	cmd.Stdout = io.MultiWriter(stdout, logFile)
	cmd.Stderr = io.MultiWriter(stdout, logFile)
	err = cmd.Run()
	evidence := []string{relativeEvidence(cfg.out, logPath)}
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
	logText := fmt.Sprintf("case=%s\naction=%s\ncheckpoint_sha256=%s\naction_error=%v\nfinal_location=%s\nfinal_xy=%d,%d\n", c.ID, c.Action, checkpointHash, actionErr, final.Location, final.X, final.Y)
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

func runFullCase(cfg config, c qualification.Case, caseDir string, stdout io.Writer) ([]string, []string, error) {
	checkpointDir := filepath.Join(caseDir, "checkpoints")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		return nil, nil, err
	}
	args := []string{
		"run", "./cmd/pokepilot",
		"-planner", "llm",
		"-fps", "0",
		"-http", "localhost:0",
		"-max-rounds", "0",
		"-goal", "elite-four",
		"-checkpoint-dir", checkpointDir,
		"-require-goal",
	}
	command := append([]string{"go"}, args...)
	logPath := filepath.Join(caseDir, "run.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return command, nil, err
	}
	defer logFile.Close()
	cmd := exec.Command("go", args...)
	cmd.Env = qualificationEnv(cfg, filepath.Join(cfg.out, "fixtures"))
	cmd.Stdout = io.MultiWriter(stdout, logFile)
	cmd.Stderr = io.MultiWriter(stdout, logFile)
	err = cmd.Run()
	evidence := []string{relativeEvidence(cfg.out, logPath), relativeEvidence(cfg.out, checkpointDir)}
	return command, evidence, err
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
	path := filepath.Join(rootAbs, filepath.Clean(rel))
	pathAbs, err := filepath.Abs(path)
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
	// Qualification must never accidentally become a farm worker just because
	// the self-hosted runner also hosts pokewall configuration.
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
		attempt := strings.TrimSpace(os.Getenv("GITHUB_RUN_ATTEMPT"))
		if attempt == "" {
			attempt = "1"
		}
		return "github-" + id + "-attempt-" + attempt
	}
	return "local-" + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func revision() string {
	if sha := strings.TrimSpace(os.Getenv("GITHUB_SHA")); sha != "" {
		return sha
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
