package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/maestroi/pokepilot/agent"
)

type output struct {
	Model      string           `json:"model"`
	PromptHash string           `json:"prompt_hash"`
	Goal       string           `json:"goal"`
	Score      float64          `json:"score"`
	Report     agent.EvalReport `json:"report"`
	Transport  int              `json:"transport_errors"`
	Rejected   int              `json:"rejected_replies"`
	Fallbacks  int              `json:"fallback_replies"`
}

func main() {
	os.Exit(run())
}

func run() int {
	list := flag.Bool("list", false, "list the built-in ROM-free fixtures without calling a model")
	jsonOut := flag.Bool("json", false, "write the report as JSON")
	minScore := flag.Float64("min-score", 0, "exit 1 when score is below this 0..1 threshold; 0 only measures")
	goal := flag.String("goal", "Make safe, efficient progress toward completing Pokemon Red.", "task statement supplied to the planner for every fixture")
	model := flag.String("model", "", "override POKEPILOT_LLM_MODEL for this run")
	baseURL := flag.String("url", "", "override POKEPILOT_LLM_URL for this run")
	flag.Parse()

	if *minScore < 0 || *minScore > 1 {
		fmt.Fprintln(os.Stderr, "agent-eval: -min-score must be between 0 and 1")
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

	p := agent.NewLLMPlanner()
	if *model != "" {
		p.Model = *model
	}
	if *baseURL != "" {
		p.BaseURL = *baseURL
	}
	p.Goal = *goal
	p.Log = os.Stderr

	report := agent.EvaluatePlanner(p, cases)
	out := output{
		Model:      p.Model,
		PromptHash: p.PromptHash(),
		Goal:       p.Goal,
		Score:      report.Score(),
		Report:     report,
		Transport:  p.Health.Transport,
		Rejected:   p.Health.Rejected,
		Fallbacks:  p.Health.Fallbacks,
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "agent-eval: encode report: %v\n", err)
			return 2
		}
	} else {
		fmt.Printf("agent-eval: %d/%d passed (score %.3f) model=%s prompt=%s\n",
			report.Passed, report.Cases, report.Score(), p.Model, p.PromptHash())
		for _, failure := range report.FailureSummary() {
			fmt.Printf("FAIL %s\n", failure)
		}
		fmt.Printf("health: transport=%d rejected=%d fallbacks=%d\n",
			p.Health.Transport, p.Health.Rejected, p.Health.Fallbacks)
	}

	// Transport failures are infrastructure failures, not planner quality.
	// Keep them distinct from a low strategic score for scripts and sweeps.
	if p.Health.Transport > 0 {
		return 2
	}
	if report.Score() < *minScore {
		return 1
	}
	return 0
}
