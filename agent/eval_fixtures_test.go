package agent

import (
	"errors"
	"testing"
)

type coreEvalOracle struct {
	cases []EvalCase
	i     int
}

func (p *coreEvalOracle) Next(_ Observation, offered []Objective) (Objective, error) {
	if p.i >= len(p.cases) {
		return Objective{}, errors.New("oracle exhausted")
	}
	tc := p.cases[p.i]
	p.i++
	if tc.AllowDone && len(tc.Accept) == 0 {
		return Objective{}, ErrDone
	}
	for _, want := range tc.Accept {
		for _, got := range offered {
			if got.String() == want {
				return got, nil
			}
		}
	}
	return Objective{}, errors.New("accepted objective was not offered")
}

func TestCoreEvalCasesAreValidAndScorable(t *testing.T) {
	cases := CoreEvalCases()
	if len(cases) < 10 {
		t.Fatalf("core eval corpus has %d cases, want at least 10", len(cases))
	}
	if err := ValidateEvalCases(cases); err != nil {
		t.Fatalf("ValidateEvalCases: %v", err)
	}

	report := EvaluatePlanner(&coreEvalOracle{cases: cases}, cases)
	if report.Failed != 0 || report.Passed != len(cases) || report.Score() != 1 {
		t.Fatalf("oracle report = %+v; failures=%v", report, report.FailureSummary())
	}
}

func TestValidateEvalCasesRejectsBrokenCorpus(t *testing.T) {
	good := Objective{Kind: KindHeal}
	bad := Objective{Kind: KindGym}

	for _, tc := range []struct {
		name  string
		cases []EvalCase
	}{
		{name: "empty name", cases: []EvalCase{{Offered: []Objective{good}, Accept: []string{good.String()}}}},
		{name: "no accepted outcome", cases: []EvalCase{{Name: "x", Offered: []Objective{good}}}},
		{name: "accept not offered", cases: []EvalCase{{Name: "x", Offered: []Objective{good}, Accept: []string{bad.String()}}}},
		{name: "duplicate name", cases: []EvalCase{
			{Name: "x", Offered: []Objective{good}, Accept: []string{good.String()}},
			{Name: "x", Offered: []Objective{good}, Accept: []string{good.String()}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateEvalCases(tc.cases); err == nil {
				t.Fatal("ValidateEvalCases unexpectedly succeeded")
			}
		})
	}
}
