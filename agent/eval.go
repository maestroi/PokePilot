package agent

import (
	"errors"
	"fmt"
)

// EvalCase is one ROM-free planner decision fixture. Offered contains the
// exact objective menu the planner sees; Accept contains the String() values
// considered strategically acceptable for this state.
type EvalCase struct {
	Name      string
	Obs       Observation
	Offered   []Objective
	Accept    []string
	AllowDone bool
}

// EvalCaseResult is one fixture outcome.
type EvalCaseResult struct {
	Name     string
	Passed   bool
	Choice   string
	Error    string
	Accepted []string
}

// EvalReport aggregates a deterministic replay of planner decisions.
type EvalReport struct {
	Cases   int
	Passed  int
	Failed  int
	Results []EvalCaseResult
}

// EvaluatePlanner runs a planner against observation/menu fixtures without
// an emulator or ROM. It deliberately scores only the chosen typed objective
// (or ErrDone), never free-form reasoning text.
func EvaluatePlanner(p Planner, cases []EvalCase) EvalReport {
	report := EvalReport{Cases: len(cases), Results: make([]EvalCaseResult, 0, len(cases))}
	for _, tc := range cases {
		row := EvalCaseResult{Name: tc.Name, Accepted: append([]string(nil), tc.Accept...)}
		choice, err := p.Next(tc.Obs, append([]Objective(nil), tc.Offered...))
		switch {
		case errors.Is(err, ErrDone):
			row.Choice = "done"
			row.Passed = tc.AllowDone
		case err != nil:
			row.Error = err.Error()
		case len(tc.Accept) == 0:
			row.Choice = choice.String()
			row.Passed = true
		default:
			row.Choice = choice.String()
			for _, want := range tc.Accept {
				if row.Choice == want {
					row.Passed = true
					break
				}
			}
		}
		if row.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, row)
	}
	return report
}

// Score returns a 0..1 pass ratio. Empty suites score 0 rather than NaN.
func (r EvalReport) Score() float64 {
	if r.Cases == 0 {
		return 0
	}
	return float64(r.Passed) / float64(r.Cases)
}

// FailureSummary renders failed cases compactly for CI output.
func (r EvalReport) FailureSummary() []string {
	out := make([]string, 0, r.Failed)
	for _, row := range r.Results {
		if row.Passed {
			continue
		}
		if row.Error != "" {
			out = append(out, fmt.Sprintf("%s: planner error: %s", row.Name, row.Error))
			continue
		}
		out = append(out, fmt.Sprintf("%s: chose %q; accepted %v", row.Name, row.Choice, row.Accepted))
	}
	return out
}

// ProgressDelta turns a completed run's existing early/final progress samples
// into a dense ROM-free benchmark signal. It does not claim these units are
// equivalent; callers can inspect each dimension independently.
type ProgressDelta struct {
	Badges int
	Events int
	Maps   int
	Rounds int
}

func MeasureProgress(r Result) ProgressDelta {
	if r.ProgressEarly == nil || r.ProgressFinal == nil {
		return ProgressDelta{Rounds: r.Rounds}
	}
	return ProgressDelta{
		Badges: r.ProgressFinal.Badges - r.ProgressEarly.Badges,
		Events: r.ProgressFinal.Events - r.ProgressEarly.Events,
		Maps:   r.ProgressFinal.Maps - r.ProgressEarly.Maps,
		Rounds: r.Rounds,
	}
}
