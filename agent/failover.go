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
	Plan        Plan
	Strategic   bool
	Err         error
	Duration    time.Duration
}

// LLMRoute is the endpoint used for the most recent ask and the number of
// primary-to-fallback transitions made this run.
type LLMRoute struct {
	Backend   string
	Model     string
	Failovers int
}

// FailoverPlanner routes asks between a primary LLM endpoint and an optional
// fallback. Every ask retries primary first; it only reaches for fallback
// when that ask's Transport counter rises (primary unreachable this call),
// and the next ask tries primary again. Content/model/schema rejections stay
// on the same backend within one ask. Primary is never pinned away: a
// restart blip or one dropped connection costs that single call, not the
// rest of the run.
type FailoverPlanner struct {
	Primary  *LLMPlanner
	Fallback *LLMPlanner
	OnCall   func(LLMCall)

	// lastBackend/lastModel describe only the most recent ask, for Route()
	// reporting; they do not steer where the next ask starts.
	lastBackend string
	lastModel   string
	failovers   int
}

func NewFailoverPlanner(primary, fallback *LLMPlanner) *FailoverPlanner {
	return &FailoverPlanner{
		Primary:     primary,
		Fallback:    fallback,
		lastBackend: "primary",
		lastModel:   primary.Model,
	}
}

func (p *FailoverPlanner) Next(obs Observation, offered []Objective) (Objective, error) {
	return p.ask(obs, offered, nil)
}

func (p *FailoverPlanner) NextRetry(obs Observation, offered []Objective, r Retry) (Objective, error) {
	return p.ask(obs, offered, &r)
}

func (p *FailoverPlanner) Strategize(obs Observation, offered []Objective, reason string) (Plan, error) {
	return p.askPlan(obs, offered, reason, nil)
}

func (p *FailoverPlanner) StrategizeRetry(obs Observation, offered []Objective, reason string, r Retry) (Plan, error) {
	return p.askPlan(obs, offered, reason, &r)
}

func (p *FailoverPlanner) askPlan(obs Observation, offered []Objective, reason string, retry *Retry) (Plan, error) {
	active := p.Primary
	p.lastBackend, p.lastModel = "primary", p.Primary.Model
	plan, err, transport := p.callPlan(active, obs, offered, reason, retry)
	if transport && p.Fallback != nil {
		p.failovers++
		if p.Primary.Log != nil {
			fmt.Fprintf(p.Primary.Log,
				"  llm route: primary %s at %s had a strategist transport failure; using fallback %s at %s for this call\n",
				p.Primary.Model, p.Primary.BaseURL, p.Fallback.Model, p.Fallback.BaseURL)
		}
		active = p.Fallback
		p.lastBackend, p.lastModel = "fallback", p.Fallback.Model
		p.syncContext(active)
		plan, err, transport = p.callPlan(active, obs, offered, reason, retry)
	}
	if err != nil && transport {
		return Plan{}, fmt.Errorf("%w: %v", ErrTransport, err)
	}
	return plan, err
}

func (p *FailoverPlanner) callPlan(active *LLMPlanner, obs Observation, offered []Objective, reason string, retry *Retry) (Plan, error, bool) {
	beforeTransport := active.Health.Transport
	start := time.Now()
	var (
		plan Plan
		err  error
	)
	if retry == nil {
		plan, err = active.Strategize(obs, offered, reason)
	} else {
		plan, err = active.StrategizeRetry(obs, offered, reason, *retry)
	}
	if p.OnCall != nil {
		p.OnCall(LLMCall{
			Observation: obs,
			Offered:     len(offered),
			Plan:        plan,
			Strategic:   true,
			Err:         err,
			Duration:    time.Since(start),
		})
	}
	return plan, err, active.Health.Transport > beforeTransport
}

func (p *FailoverPlanner) ask(obs Observation, offered []Objective, retry *Retry) (Objective, error) {
	active := p.Primary
	p.lastBackend, p.lastModel = "primary", p.Primary.Model
	o, err, transport := p.call(active, obs, offered, retry)
	if transport && p.Fallback != nil {
		p.failovers++
		if p.Primary.Log != nil {
			fmt.Fprintf(p.Primary.Log,
				"  llm route: primary %s at %s had a transport failure; using fallback %s at %s for this call\n",
				p.Primary.Model, p.Primary.BaseURL, p.Fallback.Model, p.Fallback.BaseURL)
		}
		active = p.Fallback
		p.lastBackend, p.lastModel = "fallback", p.Fallback.Model
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
	return LLMRoute{Backend: p.lastBackend, Model: p.lastModel, Failovers: p.failovers}
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
