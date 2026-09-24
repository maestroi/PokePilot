package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/agent"
)

type output struct {
	Backend          string           `json:"backend"`
	Model            string           `json:"model"`
	PromptHash       string           `json:"prompt_hash"`
	Goal             string           `json:"goal"`
	Score            float64          `json:"score"`
	Seconds          float64          `json:"seconds"`
	PromptTokens     int              `json:"prompt_tokens,omitempty"`
	CompletionTokens int              `json:"completion_tokens,omitempty"`
	Report           agent.EvalReport `json:"report"`
	Transport        int              `json:"transport_errors"`
	Rejected         int              `json:"rejected_replies"`
	Fallbacks        int              `json:"fallback_replies"`
}

func main() {
	os.Exit(run())
}

func run() int {
	list := flag.Bool("list", false, "list the built-in ROM-free fixtures without calling a model")
	jsonOut := flag.Bool("json", false, "write the report as JSON")
	minScore := flag.Float64("min-score", 0, "exit 1 when score is below this 0..1 threshold; 0 only measures")
	goal := flag.String("goal", "Make safe, efficient progress toward completing Pokemon Red.", "task statement supplied to the planner for every fixture")
	backend := flag.String("backend", "llm", "planner backend: llm, decision, or jev")
	model := flag.String("model", "", "override the selected backend model for this run")
	baseURL := flag.String("url", "", "override the selected backend URL for this run")
	decisionMinConfidence := flag.Float64("decision-min-confidence", 0, "for typed backends, reject choices below this 0..1 confidence; 0 scores every valid decision")
	flag.Parse()

	if *minScore < 0 || *minScore > 1 {
		fmt.Fprintln(os.Stderr, "agent-eval: -min-score must be between 0 and 1")
		return 2
	}
	if *decisionMinConfidence < 0 || *decisionMinConfidence > 1 {
		fmt.Fprintln(os.Stderr, "agent-eval: -decision-min-confidence must be between 0 and 1")
		return 2
	}

	cases := agent.CoreEvalCases()
	if err := agent.ValidateEvalCases(cases); err != nil {
		fmt.Fprintf(os.Stderr, "agent-eval: invalid built-in corpus: %v\n", err)
		return 2
	}

	if *list {
		for _, tc := range cases {
			fmt.Printf("%s\n", tc.Name)
			for _, want := range tc.Accept {
				fmt.Printf("  accept: %s\n", want)
			}
			if tc.AllowDone {
				fmt.Println("  accept: done")
			}
		}
		return 0
	}

	var (
		planner         agent.Planner
		llmPlanner      *agent.LLMPlanner
		decisionPlanner *agent.DecisionObjectivePlanner
		backendName     string
		modelName       string
		promptHash      string
	)
	switch strings.ToLower(strings.TrimSpace(*backend)) {
	case "", "llm", "generative":
		llmPlanner = agent.NewLLMPlanner()
		if *model != "" {
			llmPlanner.Model = *model
		}
		if *baseURL != "" {
			llmPlanner.BaseURL = *baseURL
		}
		llmPlanner.Goal = *goal
		llmPlanner.Log = os.Stderr
		planner = llmPlanner
		backendName = "llm"
		modelName = llmPlanner.Model
		promptHash = llmPlanner.PromptHash()
	case "decision", "typed", "system-one", "system_one":
		engine := agent.NewOpenAIDecisionEngineFromEnv()
		if *model != "" {
			engine.Model = *model
		}
		if *baseURL != "" {
			engine.BaseURL = *baseURL
		}
		decisionPlanner = &agent.DecisionObjectivePlanner{
			Engine:        engine,
			Goal:          *goal,
			MinConfidence: *decisionMinConfidence,
		}
		planner = decisionPlanner
		backendName = "decision"
		modelName = engine.Model
		promptHash = agent.DecisionPromptHash()
	case "jev", "typesafe", "typesafe-jev":
		engine := agent.NewJevDecisionEngineFromEnv()
		if *model != "" {
			engine.Model = *model
		}
		if *baseURL != "" {
			engine.BaseURL = *baseURL
		}
		decisionPlanner = &agent.DecisionObjectivePlanner{
			Engine:        engine,
			Goal:          *goal,
			MinConfidence: *decisionMinConfidence,
		}
		planner = decisionPlanner
		backendName = "jev"
		modelName = engine.Model
		promptHash = agent.JevDecisionPromptHash()
	default:
		fmt.Fprintf(os.Stderr, "agent-eval: unknown -backend %q; want llm, decision, or jev\n", *backend)
		return 2
	}

	started := time.Now()
	report := agent.EvaluatePlanner(planner, cases)
	elapsed := time.Since(started)
	if decisionPlanner != nil && decisionPlanner.Last.Model != "" {
		modelName = decisionPlanner.Last.Model
	}
	promptTokens, completionTokens := 0, 0
	if usage, ok := planner.(agent.UsagePlanner); ok {
		promptTokens, completionTokens = usage.Usage()
	}
	out := output{
		Backend:          backendName,
		Model:            modelName,
		PromptHash:       promptHash,
		Goal:             *goal,
		Score:            report.Score(),
		Seconds:          elapsed.Seconds(),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		Report:           report,
	}
	if llmPlanner != nil {
		out.Transport = llmPlanner.Health.Transport
		out.Rejected = llmPlanner.Health.Rejected
		out.Fallbacks = llmPlanner.Health.Fallbacks
	}
	if decisionPlanner != nil {
		out.Rejected = decisionPlanner.Rejected
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "agent-eval: encode report: %v\n", err)
			return 2
		}
	} else {
		fmt.Printf("agent-eval: %d/%d passed (score %.3f) backend=%s model=%s prompt=%s elapsed=%s\n",
			report.Passed, report.Cases, report.Score(), out.Backend, out.Model, out.PromptHash, elapsed.Round(time.Millisecond))
		for _, failure := range report.FailureSummary() {
			fmt.Printf("FAIL %s\n", failure)
		}
		fmt.Printf("health: transport=%d rejected=%d fallbacks=%d tokens=%d/%d\n",
			out.Transport, out.Rejected, out.Fallbacks, out.PromptTokens, out.CompletionTokens)
	}

	// Transport failures are infrastructure failures, not planner quality.
	// Keep them distinct from a low strategic score for scripts and sweeps.
	if out.Transport > 0 {
		return 2
	}
	if report.Score() < *minScore {
		return 1
	}
	return 0
}
