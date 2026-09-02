package agent

import (
	"errors"
	"testing"
)

type evalPlanner struct {
	choices []int
	doneAt  map[int]bool
	i       int
}

func (p *evalPlanner) Next(_ Observation, offered []Objective) (Objective, error) {
	idx := p.i
	p.i++
	if p.doneAt[idx] {
		return Objective{}, ErrDone
	}
	if idx >= len(p.choices) || p.choices[idx] < 0 || p.choices[idx] >= len(offered) {
		return Objective{}, errors.New("no scripted choice")
	}
	return offered[p.choices[idx]], nil
}

func TestEvaluatePlannerScoresAcceptedObjectives(t *testing.T) {
	goPallet := Objective{Kind: KindGoTo, Place: "pallet town"}
	heal := Objective{Kind: KindHeal, Place: "viridian pokemon center"}
	p := &evalPlanner{choices: []int{1, 0}}
	report := EvaluatePlanner(p, []EvalCase{
		{Name: "hurt party", Offered: []Objective{goPallet, heal}, Accept: []string{heal.String()}},
		{Name: "travel", Offered: []Objective{goPallet, heal}, Accept: []string{goPallet.String()}},
	})
	if report.Cases != 2 || report.Passed != 2 || report.Failed != 0 || report.Score() != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestEvaluatePlannerReportsBadChoice(t *testing.T) {
	goPallet := Objective{Kind: KindGoTo, Place: "pallet town"}
	heal := Objective{Kind: KindHeal, Place: "viridian pokemon center"}
	p := &evalPlanner{choices: []int{0}}
	report := EvaluatePlanner(p, []EvalCase{{Name: "recover", Offered: []Objective{goPallet, heal}, Accept: []string{heal.String()}}})
	if report.Failed != 1 || len(report.FailureSummary()) != 1 {
		t.Fatalf("report = %+v failures=%v", report, report.FailureSummary())
	}
}

func TestEvaluatePlannerCanAcceptDone(t *testing.T) {
	p := &evalPlanner{doneAt: map[int]bool{0: true}}
	report := EvaluatePlanner(p, []EvalCase{{Name: "goal complete", AllowDone: true}})
	if report.Passed != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestMeasureProgress(t *testing.T) {
	r := Result{
		Rounds:        9,
		ProgressEarly: &Progress{Badges: 1, Events: 10, Maps: 5},
		ProgressFinal: &Progress{Badges: 3, Events: 17, Maps: 11},
	}
	got := MeasureProgress(r)
	if got.Badges != 2 || got.Events != 7 || got.Maps != 6 || got.Rounds != 9 {
		t.Fatalf("delta = %+v", got)
	}
}
