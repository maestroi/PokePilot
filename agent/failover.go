package agent

import (
	"fmt"
	"time"
)

// LLMCall describes one actual endpoint ask made by FailoverPlanner. A
// primary transport failure followed by a fallback therefore produces two
// calls; a model-reply retry is likewise one call each time Run re-asks.
type LLMCall struct {
	Observation Observation
	Offered     int
	Objective   Objective
	Err         error
	Duration    time.Duration
}

// LLMRoute is the currently pinned endpoint and the number of primary-to-
// fallback transitions made this run.
type LLMRoute struct {
	Backend   string
	Model     string
	Failovers int
}

// FailoverPlanner routes asks between a primary LLM endpoint and an optional
// fallback. It switches only when the active primary planner's Transport
// counter rises during an ask; content/model/schema rejections stay on the
// same backend. Once switched, fallback remains pinned for the run.
type FailoverPlanner struct {
	Primary  *LLMPlanner
	Fallback *LLMPlanner
	OnCall   func(LLMCall)

	active    *LLMPlanner
	backend   string
	failovers int
}

func NewFailoverPlanner(primary, fallback *LLMPlanner) *FailoverPlanner {
	return &FailoverPlanner{
		Primary:  primary,
		Fallback: fallback,
		active:   primary,
		backend:  "primary",
	}
}

func (p *FailoverPlanner) Next(obs Observation, offered []Objective) (Objective, error) {
	return p.ask(obs, offered, nil)
}

func (p *FailoverPlanner) NextRetry(obs Observation, offered []Objective, r Retry) (Objective, error) {
	return p.ask(obs, offered, &r)
}

func (p *FailoverPlanner) ask(obs Observation, offered []Objective, retry *Retry) (Objective, error) {
	active := p.active
	p.syncContext(active)
	o, err, transport := p.call(active, obs, offered, retry)
	if transport && active == p.Primary && p.Fallback != nil {
		p.failovers++
		p.active = p.Fallback
		p.backend = "fallback"
		if p.Primary.Log != nil {
			fmt.Fprintf(p.Primary.Log,
				"  llm route: primary %s at %s had a transport failure; pinning fallback %s at %s for the rest of the run\n",
				p.Primary.Model, p.Primary.BaseURL, p.Fallback.Model, p.Fallback.BaseURL)
		}
		active = p.Fallback
		p.syncContext(active)
		o, err, transport = p.call(active, obs, offered, retry)
	}
	if err != nil && transport {
		return Objective{}, fmt.Errorf("%w: %v", ErrTransport, err)
	}
	return o, err
}

func (p *FailoverPlanner) call(active *LLMPlanner, obs Observation, offered []Objective, retry *Retry) (Objective, error, bool) {
	beforeTransport := active.Health.Transport
	start := time.Now()
	var (
		o   Objective
		err error
	)
	if retry == nil {
		o, err = active.Next(obs, offered)
	} else {
		o, err = active.NextRetry(obs, offered, *retry)
	}
	if p.OnCall != nil {
		p.OnCall(LLMCall{
			Observation: obs,
			Offered:     len(offered),
			Objective:   o,
			Err:         err,
			Duration:    time.Since(start),
		})
	}
	return o, err, active.Health.Transport > beforeTransport
}

// syncContext makes the fallback answer the same run-level question as the
// primary without overwriting endpoint-specific configuration.
func (p *FailoverPlanner) syncContext(active *LLMPlanner) {
	if active == nil || active == p.Primary {
		return
	}
	active.Goal = p.Primary.Goal
	active.ExtraSystem = p.Primary.ExtraSystem
	active.Log = p.Primary.Log
	active.PromptLog = p.Primary.PromptLog
	active.ReplyLog = p.Primary.ReplyLog
}

func (p *FailoverPlanner) Route() LLMRoute {
	active := p.active
	if active == nil {
		active = p.Primary
	}
	model := ""
	if active != nil {
		model = active.Model
	}
	return LLMRoute{Backend: p.backend, Model: model, Failovers: p.failovers}
}

func (p *FailoverPlanner) Health() LLMHealth {
	var h LLMHealth
	if p.Primary != nil {
		h = p.Primary.Health
	}
	if p.Fallback != nil {
		f := p.Fallback.Health
		h.Transport += f.Transport
		h.Rejected += f.Rejected
		h.Fallbacks += f.Fallbacks
		h.PromptTokens += f.PromptTokens
		h.CompletionTokens += f.CompletionTokens
	}
	return h
}

func (p *FailoverPlanner) Usage() (prompt, completion int) {
	h := p.Health()
	return h.PromptTokens, h.CompletionTokens
}
