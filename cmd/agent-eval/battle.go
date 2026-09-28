package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/agent"
)

type battleOutput struct {
	Suite             string                 `json:"suite"`
	Corpus            string                 `json:"corpus"`
	Metric            string                 `json:"metric"`
	Reference         string                 `json:"reference"`
	Backend           string                 `json:"backend"`
	Model             string                 `json:"model"`
	PromptHash        string                 `json:"prompt_hash,omitempty"`
	Score             float64                `json:"score"`
	Seconds           float64                `json:"seconds"`
	Calls             int                    `json:"calls"`
	Rejected          int                    `json:"rejected_replies"`
	Errors            int                    `json:"errors"`
	LowConfidence     int                    `json:"low_confidence"`
	ErrorRate         float64                `json:"error_rate"`
	RejectionRate     float64                `json:"rejection_rate"`
	AverageConfidence float64                `json:"average_confidence"`
	LatencyP50Seconds float64                `json:"latency_p50_seconds"`
	LatencyP95Seconds float64                `json:"latency_p95_seconds"`
	PromptTokens      int                    `json:"prompt_tokens,omitempty"`
	CompletionTokens  int                    `json:"completion_tokens,omitempty"`
	Report            agent.BattleEvalReport `json:"report"`
}

// firstUsableMoveDecisionEngine is the portable benchmark representation of
// the current deterministic battle move policy: choose the first legal move.
// It is a reference baseline, not an oracle for switch/item/run fixtures.
type firstUsableMoveDecisionEngine struct{}

func (firstUsableMoveDecisionEngine) Decide(_ context.Context, req agent.DecisionRequest) (agent.DecisionResponse, error) {
	if len(req.Choices) == 0 {
		return agent.DecisionResponse{}, fmt.Errorf("deterministic battle baseline: no legal choices")
	}
	choice := req.Choices[0].ID
	for _, candidate := range req.Choices {
		if strings.HasPrefix(candidate.ID, "move:") {
			choice = candidate.ID
			break
		}
	}
	return agent.DecisionResponse{
		Choice:        choice,
		Probabilities: map[string]float64{choice: 1},
		Confidence:    1,
		Backend:       "deterministic",
		Model:         "first-usable-move",
	}, nil
}

// runBattleSuite is the battle-turn evaluation mode. It is separate from the
// planner suite and from live runs: it only scores typed backends against
// portable battle states, and nothing here drives the emulator.
func runBattleSuite(backend, model, baseURL, corpusPath string, minConfidence, minScore float64, list, jsonOut bool) int {
	cases := agent.CoreBattleEvalCases()
	corpus := "built-in"
	metric := "fixture_accuracy"
	reference := "checked-in accepted actions"
	if strings.TrimSpace(corpusPath) != "" {
		file, err := os.Open(corpusPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent-eval: open battle corpus: %v\n", err)
			return 2
		}
		samples, err := agent.ReadBattleShadowCorpus(file)
		_ = file.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent-eval: read battle corpus: %v\n", err)
			return 2
		}
		cases, err = agent.BattleEvalCasesFromShadow(samples)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent-eval: invalid battle corpus: %v\n", err)
			return 2
		}
		corpus = corpusPath
		metric = "executed_policy_agreement"
		reference = "action executed by the deterministic policy in the captured run"
	}
	if err := agent.ValidateBattleEvalCases(cases); err != nil {
		fmt.Fprintf(os.Stderr, "agent-eval: invalid battle corpus: %v\n", err)
		return 2
	}
	if list {
		for _, tc := range cases {
			fmt.Println(tc.Name)
			if len(tc.Tags) > 0 {
				fmt.Printf("  tags: %s\n", strings.Join(tc.Tags, ","))
			}
			for _, want := range tc.Accept {
				fmt.Printf("  accept: %s\n", want)
			}
		}
		return 0
	}

	var (
		engine agent.DecisionEngine
		out    = battleOutput{Suite: "battle", Corpus: corpus, Metric: metric, Reference: reference}
	)
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "deterministic", "baseline", "first-usable":
		engine, out.Backend, out.Model = firstUsableMoveDecisionEngine{}, "deterministic", "first-usable-move"
	case "decision", "typed", "system-one", "system_one":
		e := agent.NewOpenAIDecisionEngineFromEnv()
		if model != "" {
			e.Model = model
		}
		if baseURL != "" {
			e.BaseURL = baseURL
		}
		engine, out.Backend, out.Model, out.PromptHash = e, "decision", e.Model, agent.DecisionPromptHash()
	case "jev", "typesafe", "typesafe-jev":
		e := agent.NewJevDecisionEngineFromEnv()
		if model != "" {
			e.Model = model
		}
		if baseURL != "" {
			e.BaseURL = baseURL
		}
		engine, out.Backend, out.Model, out.PromptHash = e, "jev", e.Model, agent.JevDecisionPromptHash()
	default:
		fmt.Fprintf(os.Stderr, "agent-eval: -suite battle needs -backend deterministic, decision, or jev; got %q\n", backend)
		return 2
	}

	started := time.Now()
	report, stats := agent.EvaluateBattleDecisions(engine, cases, minConfidence)
	elapsed := time.Since(started)
	if stats.Model != "" {
		out.Model = stats.Model
	}
	out.Score = report.Score()
	out.Seconds = elapsed.Seconds()
	out.Calls = stats.Calls
	out.Rejected = stats.Rejected
	out.Errors = stats.Errors
	out.LowConfidence = stats.LowConfidence
	out.ErrorRate = stats.ErrorRate()
	out.RejectionRate = stats.RejectionRate()
	out.AverageConfidence = stats.AverageConfidence()
	out.LatencyP50Seconds = stats.LatencyPercentile(0.50)
	out.LatencyP95Seconds = stats.LatencyPercentile(0.95)
	out.PromptTokens = stats.PromptTokens
	out.CompletionTokens = stats.CompletionTokens
	out.Report = report

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "agent-eval: encode report: %v\n", err)
			return 2
		}
	} else {
		fmt.Printf("agent-eval battle: %d/%d matched (%.3f %s) backend=%s model=%s corpus=%s elapsed=%s\n",
			report.Passed, report.Cases, out.Score, out.Metric, out.Backend, out.Model, out.Corpus, elapsed.Round(time.Millisecond))
		fmt.Printf("reference: %s\n", out.Reference)
		for _, failure := range report.FailureSummary() {
			fmt.Printf("DIFF %s\n", failure)
		}
		fmt.Printf("health: calls=%d rejected=%d errors=%d low_confidence=%d avg_confidence=%.3f p50=%s p95=%s tokens=%d/%d\n",
			out.Calls, out.Rejected, out.Errors, out.LowConfidence, out.AverageConfidence,
			time.Duration(out.LatencyP50Seconds*float64(time.Second)).Round(time.Millisecond),
			time.Duration(out.LatencyP95Seconds*float64(time.Second)).Round(time.Millisecond),
			out.PromptTokens, out.CompletionTokens)
	}
	if report.Score() < minScore {
		return 1
	}
	return 0
}
