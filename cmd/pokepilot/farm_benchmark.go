package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/benchmark"
	"github.com/maestroi/pokepilot/farm"
	redbench "github.com/maestroi/pokepilot/red/benchmark"
)

const farmBenchmarkResultName = "benchmark-result.json"

func writeFarmBenchmarkResult(spec farm.Spec, res agent.Result, stats *statsPlanner, startedAt, finishedAt time.Time, resumeFrom, checkpointDir, romSHA256 string, effectiveMaxFrames int) error {
	if checkpointDir == "" || stats == nil || spec.Planner != "llm" {
		return nil
	}
	game := strings.TrimSpace(spec.Game)
	if game == "" {
		game = "pokemon-red"
	}
	if game != "pokemon-red" {
		// The schema is game-agnostic, but Red is the only game with a
		// milestone profile today. Future profiles can join here without
		// changing the farm/run contract.
		return nil
	}

	source := benchmark.Source{Kind: "fresh"}
	if resumeFrom != "" {
		source.Kind = "checkpoint"
		source.Checkpoint = filepath.Base(resumeFrom)
		if data, err := os.ReadFile(resumeFrom); err == nil {
			sum := sha256.Sum256(data)
			source.CheckpointSHA256 = fmt.Sprintf("%x", sum[:])
		}
	}

	mode := farm.PlayStyleForSpec(spec)
	if mode == "" {
		mode = agent.PlayStyleSpeedrun
	}
	endCondition := strings.TrimSpace(spec.ExperimentCase)
	if endCondition == "" {
		endCondition = strings.TrimSpace(spec.Goal)
	}

	primary := stats.inner
	model := benchmark.ModelIdentity{}
	if primary != nil {
		model = benchmark.ModelIdentity{
			Profile:                 spec.LLMProfile,
			PrimaryModel:            primary.Model,
			PrimaryURL:              benchmark.SafeEndpoint(primary.BaseURL),
			NoThink:                 primary.NoThink,
			MaxTokens:               primary.MaxTokens,
			Timeout:                 primary.Timeout.String(),
			ReasoningEffort:         primary.ReasoningEffort,
			RecoveryReasoningEffort: primary.RecoveryReasoningEffort,
			PromptHash:              primary.PromptHash(),
		}
	}
	if stats.router != nil && stats.router.Fallback != nil {
		fallback := stats.router.Fallback
		model.FallbackModel = fallback.Model
		model.FallbackURL = benchmark.SafeEndpoint(fallback.BaseURL)
		model.FallbackNoThink = fallback.NoThink
		model.FallbackMaxTokens = fallback.MaxTokens
		model.FallbackTimeout = fallback.Timeout.String()
		model.FallbackReasoningEffort = fallback.ReasoningEffort
	}

	featureFlags := map[string]string{
		"typed_decision_backend":        stats.decision.Backend,
		"typed_decision_objectives":     strconv.FormatBool(stats.decision.ObjectiveSelection),
		"typed_decision_failures":       strconv.FormatBool(stats.decision.FailureRecovery),
		"typed_decision_min_confidence": fmt.Sprintf("%.3f", stats.decision.MinConfidence),
	}
	if engine, ok := stats.decision.Engine.(*agent.OpenAIDecisionEngine); ok {
		featureFlags["typed_decision_url"] = engine.BaseURL
		featureFlags["typed_decision_model"] = engine.Model
		featureFlags["typed_decision_timeout"] = engine.Timeout.String()
		featureFlags["typed_decision_max_tokens"] = strconv.Itoa(engine.MaxTokens)
	}

	result := benchmark.Build(benchmark.BuildInput{
		RunID:        spec.RunID,
		Commit:       version,
		Game:         game,
		ROMSHA256:    romSHA256,
		Mode:         mode,
		Seed:         spec.Seed,
		Source:       source,
		EndCondition: endCondition,
		Configuration: benchmark.Configuration{
			Planner:         spec.Planner,
			Goal:            spec.Goal,
			Starter:         spec.Starter,
			LLMProfile:      spec.LLMProfile,
			ReasoningEffort: spec.ReasoningEffort,
			PlayStyle:       mode,
			RiskTolerance:   farm.RiskToleranceForSpec(spec),
			WildEncounters:  farm.WildEncountersForSpec(spec),
			DecisionBackend: stats.decision.Backend,
			EmulatorSpeed:   fmt.Sprintf("farm fps=%d; canonical score=emulator frames", spec.FPS),
			MaxFrames:       effectiveMaxFrames,
			Model:           model,
			FeatureFlags:    benchmark.SanitizeSettings(featureFlags),
		},
		Profile:       redbench.Profile(),
		AgentResult:   res,
		Calls:         append([]agent.LLMCall(nil), stats.benchmarkCalls...),
		DecisionCalls: append([]benchmark.DecisionCall(nil), stats.benchmarkDecisionCalls...),
		Route:         stats.router.Route(),
		Health:        stats.router.Health(),
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		TerminalError: res.Err,
	})
	result.ExperimentID = spec.ExperimentID
	result.ExperimentArm = spec.ExperimentArm
	result.ExperimentCase = spec.ExperimentCase

	attachFarmBenchmarkCheckpoints(&result, checkpointDir)
	return benchmark.WriteJSON(filepath.Join(checkpointDir, farmBenchmarkResultName), result)
}

func attachFarmBenchmarkCheckpoints(result *benchmark.Result, checkpointDir string) {
	if result == nil || checkpointDir == "" {
		return
	}
	major := map[string]int{
		"brock": 1, "misty": 2, "fuchsia-koga": 5, "sabrina": 6, "blaine": 7, "giovanni": 8,
	}
	entries, _ := os.ReadDir(checkpointDir)
	for i := range result.Milestones {
		badge, ok := major[result.Milestones[i].ID]
		if !ok {
			continue
		}
		prefix := fmt.Sprintf("%s%02d-", majorCheckpointStatePrefix, badge)
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ".state") {
				result.Milestones[i].Checkpoint = entry.Name()
				break
			}
		}
	}
	if len(result.Failures) == 0 {
		return
	}
	if latest := latestObjectiveState(checkpointDir); latest != "" {
		result.Failures[0].Checkpoint = latest
		result.Failures[0].Reproduce = fmt.Sprintf("Open run %s in RomPilot and replay from checkpoint %s", result.RunID, latest)
	}
}
