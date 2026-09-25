package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/agent"
)

type battleOutput struct {
	Suite            string           `json:"suite"`
	Backend          string           `json:"backend"`
	Model            string           `json:"model"`
	PromptHash       string           `json:"prompt_hash"`
	Score            float64          `json:"score"`
	Seconds          float64          `json:"seconds"`
	Calls            int              `json:"calls"`
	Rejected         int              `json:"rejected_replies"`
	PromptTokens     int              `json:"prompt_tokens,omitempty"`
	CompletionTokens int              `json:"completion_tokens,omitempty"`
	Report           agent.EvalReport `json:"report"`
}

// runBattleSuite is the battle-turn evaluation mode. It is separate from the
// planner suite and from live runs: it only scores typed backends against
// ROM-free battle states, and nothing here drives the emulator.
func runBattleSuite(backend, model, baseURL string, minConfidence, minScore float64, list, jsonOut bool) int {
	cases := agent.CoreBattleEvalCases()
	if err := agent.ValidateBattleEvalCases(cases); err != nil {
		fmt.Fprintf(os.Stderr, "agent-eval: invalid built-in battle corpus: %v\n", err)
		return 2
	}
	if list {
		for _, tc := range cases {
			fmt.Println(tc.Name)
			for _, want := range tc.Accept {
				fmt.Printf("  accept: %s\n", want)
			}
		}
		return 0
	}

	var (
		engine agent.DecisionEngine
		out    = battleOutput{Suite: "battle"}
	)
	switch strings.ToLower(strings.TrimSpace(backend)) {
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
		fmt.Fprintf(os.Stderr, "agent-eval: -suite battle needs a typed -backend (decision or jev), got %q\n", backend)
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
		fmt.Printf("agent-eval battle: %d/%d passed (score %.3f) backend=%s model=%s prompt=%s elapsed=%s\n",
			report.Passed, report.Cases, out.Score, out.Backend, out.Model, out.PromptHash, elapsed.Round(time.Millisecond))
		for _, failure := range report.FailureSummary() {
			fmt.Printf("FAIL %s\n", failure)
		}
		fmt.Printf("health: calls=%d rejected=%d tokens=%d/%d\n", out.Calls, out.Rejected, out.PromptTokens, out.CompletionTokens)
	}
	if report.Score() < minScore {
		return 1
	}
	return 0
}
