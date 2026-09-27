// Command tetrisbench runs real-ROM autonomous Tetris qualification without a
// generative planner. A typed backend such as Jev may choose only from legal
// placements produced by the deterministic policy; backend failures fall back
// to that policy and remain visible in the benchmark report.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/tetris"
	tetrisdecision "github.com/maestroi/pokepilot/tetris/decision"
	tetrispolicy "github.com/maestroi/pokepilot/tetris/policy"
	tetrissession "github.com/maestroi/pokepilot/tetris/session"
)

const reportVersion = 1

type config struct {
	ROMPath       string
	Profile       string
	Goal          string
	Runs          int
	MaxPieces     int
	MaxFrames     int
	Decision      string
	DecisionMode  string
	MinConfidence float64
	Output        string
}

type decisionSample struct {
	Duration   time.Duration
	Fallback   bool
	Err        string
	Backend    string
	Model      string
	Confidence float64
}

type runResult struct {
	Index             int     `json:"index"`
	Seed              int64   `json:"seed"`
	SeedBurn          int     `json:"seed_burn"`
	Passed            bool    `json:"passed"`
	Reason            string  `json:"reason"`
	Pieces            int     `json:"pieces"`
	Frames            uint64  `json:"frames"`
	WallSeconds       float64 `json:"wall_seconds"`
	Score             int     `json:"score"`
	Lines             int     `json:"lines"`
	Level             int     `json:"level"`
	GameOver          bool    `json:"game_over"`
	Complete          bool    `json:"complete"`
	DecisionCalls     int     `json:"decision_calls,omitempty"`
	DecisionFallbacks int     `json:"decision_fallbacks,omitempty"`
	DecisionErrors    int     `json:"decision_errors,omitempty"`
	DecisionAvgMS     float64 `json:"decision_avg_ms,omitempty"`
	DecisionP50MS     float64 `json:"decision_p50_ms,omitempty"`
	DecisionP95MS     float64 `json:"decision_p95_ms,omitempty"`
	DecisionBackend   string  `json:"decision_backend,omitempty"`
	DecisionModel     string  `json:"decision_model,omitempty"`
}

type report struct {
	Version           int         `json:"version"`
	Profile           string      `json:"profile"`
	Game              string      `json:"game"`
	Revision          string      `json:"revision"`
	ROMSHA1           string      `json:"rom_sha1"`
	ROMSHA256         string      `json:"rom_sha256"`
	Goal              string      `json:"goal"`
	DecisionBackend   string      `json:"decision_backend"`
	DecisionMode      string      `json:"decision_mode"`
	MinConfidence     float64     `json:"min_confidence"`
	StartedAt         time.Time   `json:"started_at"`
	FinishedAt        time.Time   `json:"finished_at"`
	Passed            bool        `json:"passed"`
	Runs              []runResult `json:"runs"`
	CompletionRate    float64     `json:"completion_rate"`
	MedianScore       float64     `json:"median_score"`
	MedianLines       float64     `json:"median_lines"`
	MedianPieces      float64     `json:"median_pieces"`
	DecisionCalls     int         `json:"decision_calls,omitempty"`
	DecisionFallbacks int         `json:"decision_fallbacks,omitempty"`
	DecisionErrors    int         `json:"decision_errors,omitempty"`
	DecisionP50MS     float64     `json:"decision_p50_ms,omitempty"`
	DecisionP95MS     float64     `json:"decision_p95_ms,omitempty"`
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

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("tetrisbench", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	rom := fs.String("rom", os.Getenv("TETRIS_ROM"), "path to Tetris (World) (Rev 1)")
	profile := fs.String("profile", "fast", "qualification profile: fast or full")
	goal := fs.String("goal", "", "override profile goal: auto, survival, complete, lines:N, score:N")
	runs := fs.Int("runs", 0, "override profile run count")
	maxPieces := fs.Int("max-pieces", 0, "override placed-piece budget")
	maxFrames := fs.Int("max-frames", 0, "override emulated frame budget")
	decision := fs.String("decision-backend", "jev", "typed placement backend: jev, system-one, or off")
	mode := fs.String("decision-mode", "active", "typed placement mode: active or shadow")
	minConfidence := fs.Float64("min-confidence", 0.65, "typed decision confidence floor")
	output := fs.String("output", "tetris-benchmark-out", "benchmark evidence directory")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cfg := config{
		ROMPath:       strings.TrimSpace(*rom),
		Profile:       strings.ToLower(strings.TrimSpace(*profile)),
		Goal:          strings.TrimSpace(*goal),
		Runs:          *runs,
		MaxPieces:     *maxPieces,
		MaxFrames:     *maxFrames,
		Decision:      strings.ToLower(strings.TrimSpace(*decision)),
		DecisionMode:  strings.ToLower(strings.TrimSpace(*mode)),
		MinConfidence: *minConfidence,
		Output:        strings.TrimSpace(*output),
	}
	switch cfg.Profile {
	case "fast":
		if cfg.Goal == "" {
			cfg.Goal = "score:1000"
		}
		if cfg.Runs == 0 {
			cfg.Runs = 1
		}
		if cfg.MaxPieces == 0 {
			cfg.MaxPieces = 120
		}
		if cfg.MaxFrames == 0 {
			cfg.MaxFrames = 180000
		}
	case "full":
		if cfg.Goal == "" {
			cfg.Goal = "score:10000"
		}
		if cfg.Runs == 0 {
			cfg.Runs = 3
		}
		if cfg.MaxPieces == 0 {
			cfg.MaxPieces = 600
		}
		if cfg.MaxFrames == 0 {
			cfg.MaxFrames = 900000
		}
	default:
		return config{}, fmt.Errorf("tetrisbench: unknown profile %q (want fast or full)", cfg.Profile)
	}
	if cfg.ROMPath == "" {
		return config{}, fmt.Errorf("tetrisbench: -rom or TETRIS_ROM is required")
	}
	if cfg.Runs < 1 || cfg.MaxPieces < 1 || cfg.MaxFrames < 1 {
		return config{}, fmt.Errorf("tetrisbench: runs, max-pieces and max-frames must be positive")
	}
	if cfg.MinConfidence < 0 || cfg.MinConfidence > 1 {
		return config{}, fmt.Errorf("tetrisbench: min-confidence must be between 0 and 1")
	}
	if cfg.DecisionMode != "active" && cfg.DecisionMode != "shadow" {
		return config{}, fmt.Errorf("tetrisbench: decision-mode must be active or shadow")
	}
	if cfg.Output == "" {
		return config{}, fmt.Errorf("tetrisbench: output must not be empty")
	}
	if _, err := tetrissession.ParseGoal(cfg.Goal); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func run(cfg config, stdout io.Writer) error {
	rom, err := os.ReadFile(cfg.ROMPath)
	if err != nil {
		return fmt.Errorf("tetrisbench: read ROM: %w", err)
	}
	profile, info, err := profiles.DetectCartridge(rom)
	if err != nil {
		return fmt.Errorf("tetrisbench: detect ROM: %w", err)
	}
	if profile.ID() != "tetris" {
		return fmt.Errorf("tetrisbench: ROM is %s, want tetris", profile.ID())
	}
	goal, err := tetrissession.ParseGoal(cfg.Goal)
	if err != nil {
		return err
	}
	settings, err := agent.DecisionSettingsFor(agent.DecisionSelection{
		Backend:       cfg.Decision,
		Mode:          cfg.DecisionMode,
		MinConfidence: cfg.MinConfidence,
	})
	if err != nil {
		return fmt.Errorf("tetrisbench: decision engine: %w", err)
	}

	if err := os.MkdirAll(cfg.Output, 0o755); err != nil {
		return fmt.Errorf("tetrisbench: create output: %w", err)
	}
	out := report{
		Version:         reportVersion,
		Profile:         cfg.Profile,
		Game:            string(profile.ID()),
		Revision:        string(profile.Revision()),
		ROMSHA1:         info.SHA1,
		ROMSHA256:       info.SHA256,
		Goal:            cfg.Goal,
		DecisionBackend: settings.Backend,
		DecisionMode:    settings.Mode(),
		MinConfidence:   settings.MinConfidence,
		StartedAt:       time.Now().UTC(),
	}

	fmt.Fprintf(stdout, "tetrisbench: %s %s · profile=%s goal=%s decision=%s/%s runs=%d\n",
		profile.ID(), profile.Revision(), cfg.Profile, cfg.Goal, settings.Backend, settings.Mode(), cfg.Runs)

	var allDecisionDurations []time.Duration
	for i := 0; i < cfg.Runs; i++ {
		rr, samples, err := runOne(cfg, profile, goal, settings, i)
		if err != nil {
			return err
		}
		out.Runs = append(out.Runs, rr)
		for _, sample := range samples {
			allDecisionDurations = append(allDecisionDurations, sample.Duration)
			out.DecisionCalls++
			if sample.Fallback {
				out.DecisionFallbacks++
			}
			if sample.Err != "" {
				out.DecisionErrors++
			}
		}
		fmt.Fprintf(stdout, "  run %d: passed=%v reason=%s pieces=%d score=%d lines=%d decision=%d fallback=%d p95=%.1fms\n",
			i+1, rr.Passed, rr.Reason, rr.Pieces, rr.Score, rr.Lines, rr.DecisionCalls, rr.DecisionFallbacks, rr.DecisionP95MS)
	}
	out.FinishedAt = time.Now().UTC()
	summarize(&out, allDecisionDurations)
	path := filepath.Join(cfg.Output, "tetris-benchmark.json")
	if err := writeJSON(path, out); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "tetrisbench: evidence %s · completion %.0f%% · median score %.0f · decision p95 %.1fms\n",
		path, out.CompletionRate*100, out.MedianScore, out.DecisionP95MS)
	if !out.Passed {
		return fmt.Errorf("tetrisbench: qualification failed: %d/%d runs reached %s", passedRuns(out.Runs), len(out.Runs), cfg.Goal)
	}
	return nil
}

func runOne(cfg config, profile game.CartridgeProfile, goal tetrissession.Goal, settings agent.DecisionSettings, index int) (runResult, []decisionSample, error) {
	m, err := emu.OpenCGB(cfg.ROMPath)
	if err != nil {
		return runResult{}, nil, fmt.Errorf("tetrisbench: run %d open ROM: %w", index+1, err)
	}
	defer m.Close()

	if _, err := tetrissession.BootToTitle(profile, m); err != nil {
		return runResult{}, nil, fmt.Errorf("tetrisbench: run %d boot title: %w", index+1, err)
	}
	seed := int64(index)
	burn := seedBurn(seed)
	if burn > 0 {
		m.StepFrames(burn)
	}
	if _, err := tetrissession.BootToPlaying(profile, m, goal.Mode()); err != nil {
		return runResult{}, nil, fmt.Errorf("tetrisbench: run %d boot game: %w", index+1, err)
	}

	var (
		samples []decisionSample
		choose  func(tetris.State, tetrispolicy.Objective) (tetrispolicy.Decision, error)
	)
	if settings.Engine != nil {
		selector := tetrisdecision.Selector{
			Engine:        settings.Engine,
			MinConfidence: settings.MinConfidence,
			Shadow:        settings.Shadow,
		}
		choose = func(state tetris.State, objective tetrispolicy.Objective) (tetrispolicy.Decision, error) {
			selection, err := selector.Choose(context.Background(), state, objective)
			if err != nil {
				return tetrispolicy.Decision{}, err
			}
			sample := decisionSample{
				Duration:   selection.Response.Duration,
				Fallback:   selection.Fallback,
				Backend:    selection.Response.Backend,
				Model:      selection.Response.Model,
				Confidence: selection.Response.Confidence,
			}
			if sample.Backend == "" {
				sample.Backend = settings.Backend
			}
			if selection.DecisionErr != nil {
				sample.Err = selection.DecisionErr.Error()
			}
			samples = append(samples, sample)
			return selection.Decision, nil
		}
	}

	startFrame := m.FrameCount()
	started := time.Now()
	result := tetrissession.Run(profile, m, tetrissession.RunOptions{
		Goal:      goal,
		MaxPieces: cfg.MaxPieces,
		MaxFrames: cfg.MaxFrames,
		Choose:    choose,
	})
	finished := time.Now()
	rr := runResult{
		Index:       index + 1,
		Seed:        seed,
		SeedBurn:    burn,
		Passed:      qualificationPassed(goal, result, cfg.MaxPieces),
		Reason:      result.Reason,
		Pieces:      result.Pieces,
		Frames:      m.FrameCount() - startFrame,
		WallSeconds: finished.Sub(started).Seconds(),
		Score:       result.State.Score,
		Lines:       result.State.LinesCleared,
		Level:       result.State.Level,
		GameOver:    result.State.GameOver,
		Complete:    result.State.Complete,
	}
	applyDecisionStats(&rr, samples)
	if result.Err != nil {
		return rr, samples, fmt.Errorf("tetrisbench: run %d: %w", index+1, result.Err)
	}
	return rr, samples, nil
}

func qualificationPassed(goal tetrissession.Goal, result tetrissession.Result, maxPieces int) bool {
	switch goal.Kind {
	case tetrissession.GoalAuto, tetrissession.GoalSurvival:
		return result.Reason == "budget" && result.Pieces >= maxPieces && !result.State.GameOver
	default:
		return result.Reason == "done" && goal.Satisfied(result.State)
	}
}

func applyDecisionStats(out *runResult, samples []decisionSample) {
	if out == nil || len(samples) == 0 {
		return
	}
	durations := make([]time.Duration, 0, len(samples))
	var total time.Duration
	for _, sample := range samples {
		durations = append(durations, sample.Duration)
		total += sample.Duration
		out.DecisionCalls++
		if sample.Fallback {
			out.DecisionFallbacks++
		}
		if sample.Err != "" {
			out.DecisionErrors++
		}
		if sample.Backend != "" {
			out.DecisionBackend = sample.Backend
		}
		if sample.Model != "" {
			out.DecisionModel = sample.Model
		}
	}
	out.DecisionAvgMS = float64(total.Microseconds()) / 1000 / float64(len(samples))
	out.DecisionP50MS = durationPercentileMS(durations, 0.50)
	out.DecisionP95MS = durationPercentileMS(durations, 0.95)
}

func summarize(out *report, durations []time.Duration) {
	if out == nil || len(out.Runs) == 0 {
		return
	}
	out.Passed = passedRuns(out.Runs) == len(out.Runs)
	out.CompletionRate = float64(passedRuns(out.Runs)) / float64(len(out.Runs))
	scores := make([]float64, 0, len(out.Runs))
	lines := make([]float64, 0, len(out.Runs))
	pieces := make([]float64, 0, len(out.Runs))
	for _, run := range out.Runs {
		scores = append(scores, float64(run.Score))
		lines = append(lines, float64(run.Lines))
		pieces = append(pieces, float64(run.Pieces))
	}
	out.MedianScore = median(scores)
	out.MedianLines = median(lines)
	out.MedianPieces = median(pieces)
	out.DecisionP50MS = durationPercentileMS(durations, 0.50)
	out.DecisionP95MS = durationPercentileMS(durations, 0.95)
}

func passedRuns(runs []runResult) int {
	n := 0
	for _, run := range runs {
		if run.Passed {
			n++
		}
	}
	return n
}

func seedBurn(seed int64) int {
	if seed == 0 {
		return 0
	}
	return rand.New(rand.NewPCG(uint64(seed), 0)).IntN(600)
}

func durationPercentileMS(values []time.Duration, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(float64(len(sorted)-1)*p + 0.5)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return float64(sorted[index].Microseconds()) / 1000
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("tetrisbench: write %s: %w", path, err)
	}
	return nil
}
