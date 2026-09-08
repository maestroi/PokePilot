from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    s = p.read_text()
    if old not in s:
        raise SystemExit(f"missing replacement anchor in {path}: {old[:120]!r}")
    if s.count(old) != 1:
        raise SystemExit(f"replacement anchor not unique in {path}: {old[:120]!r} ({s.count(old)})")
    p.write_text(s.replace(old, new, 1))


def insert_before(path, anchor, text):
    replace_once(path, anchor, text + anchor)


Path("agent/plan.go").write_text(r'''package agent

import (
    "errors"
    "fmt"
    "io"
    "strconv"
    "strings"
)

const (
    MaxPlanSteps = 10
    PlanGoalCap  = 200
    PlanStepCap  = 320
)

// Plan is the strategist's bounded, multi-round commitment. Steps are exact
// Objective.String() sentences, never menu indexes: Offer is rebuilt every
// round and indexes are not stable across observations.
type Plan struct {
    Goal  string   `json:"goal,omitempty"`
    Steps []string `json:"steps,omitempty"`
    Step  int      `json:"step,omitempty"`
    Round int      `json:"round,omitempty"`
}

func (p Plan) Active() bool { return p.Step >= 0 && p.Step < len(p.Steps) }

func (p Plan) clone() Plan {
    p.Steps = append([]string(nil), p.Steps...)
    return p
}

// validateStoredPlan checks only the durable shape. A resumed step may be
// stale by design; Run resolves it against the freshly rebuilt menu and skips
// it rather than treating an old checkpoint as corrupt.
func validateStoredPlan(p Plan) error {
    if len(p.Goal) > PlanGoalCap {
        return fmt.Errorf("plan goal is %d bytes, over cap %d", len(p.Goal), PlanGoalCap)
    }
    if len(p.Steps) > MaxPlanSteps {
        return fmt.Errorf("plan has %d steps, over cap %d", len(p.Steps), MaxPlanSteps)
    }
    if p.Step < 0 || p.Step > len(p.Steps) {
        return fmt.Errorf("plan step %d is outside 0..%d", p.Step, len(p.Steps))
    }
    for i, step := range p.Steps {
        if strings.TrimSpace(step) == "" {
            return fmt.Errorf("plan step %d is empty", i)
        }
        if len(step) > PlanStepCap {
            return fmt.Errorf("plan step %d is %d bytes, over cap %d", i, len(step), PlanStepCap)
        }
    }
    return nil
}

// validateStrategicPlan canonicalizes a fresh model plan against the menu the
// strategist actually saw. This is intentionally exact: the model may select
// only deterministic objectives already exposed by Offer, and the runtime
// never fuzzy-matches or invents a missing action.
func validateStrategicPlan(p Plan, offered []Objective, round int) (Plan, error) {
    p.Goal = strings.TrimSpace(p.Goal)
    if p.Goal == "" {
        return Plan{}, errors.New("agent: strategist: plan goal is empty")
    }
    if len(p.Goal) > PlanGoalCap {
        return Plan{}, fmt.Errorf("agent: strategist: plan goal is %d bytes, over cap %d", len(p.Goal), PlanGoalCap)
    }
    if len(p.Steps) == 0 {
        return Plan{}, errors.New("agent: strategist: plan has no steps")
    }
    if len(p.Steps) > MaxPlanSteps {
        return Plan{}, fmt.Errorf("agent: strategist: plan has %d steps, over cap %d", len(p.Steps), MaxPlanSteps)
    }
    canonical := make([]string, 0, len(p.Steps))
    for i, raw := range p.Steps {
        step := strings.TrimSpace(raw)
        if step == "" {
            return Plan{}, fmt.Errorf("agent: strategist: plan step %d is empty", i+1)
        }
        if len(step) > PlanStepCap {
            return Plan{}, fmt.Errorf("agent: strategist: plan step %d is %d bytes, over cap %d", i+1, len(step), PlanStepCap)
        }
        if _, err := strconv.Atoi(step); err == nil {
            return Plan{}, fmt.Errorf("agent: strategist: plan step %d is menu index %q; steps must be objective sentences", i+1, step)
        }
        obj, err := Chosen(offered, step)
        if err != nil {
            return Plan{}, fmt.Errorf("agent: strategist: plan step %d does not resolve: %w", i+1, err)
        }
        canonical = append(canonical, obj.String())
    }
    return Plan{Goal: p.Goal, Steps: canonical, Step: 0, Round: round}, nil
}

// StrategicPlanner is optional. Scripted and simple test planners retain the
// historical one-step loop; real LLM planners implement this interface and
// let Run add the zero-call plan tier without changing Planner itself.
type StrategicPlanner interface {
    Strategize(obs Observation, offered []Objective, reason string) (Plan, error)
}

// StrategicFeedbackPlanner is the strategist equivalent of FeedbackPlanner:
// rejected plan-shaped replies can be re-asked only when the request changes.
type StrategicFeedbackPlanner interface {
    StrategizeRetry(obs Observation, offered []Objective, reason string, r Retry) (Plan, error)
}

func strategizeWithRetries(log io.Writer, round int, p StrategicPlanner, obs Observation, offered []Objective, reason string) (Plan, error, int) {
    plan, err := p.Strategize(obs, offered, reason)
    fp, canRetry := p.(StrategicFeedbackPlanner)
    retries := 0
    for err != nil && !errors.Is(err, ErrDone) && canRetry && retries < MaxReplyRetries-1 {
        r, retryable := classifyRetry(err)
        if !retryable {
            if log != nil {
                fmt.Fprintf(log, "round %d: strategist reply rejected and not retried: %v\n", round, err)
            }
            break
        }
        retries++
        if log != nil {
            fmt.Fprintf(log, "round %d: strategist reply rejected (ask %d of %d): %v; re-ask differs by %s\n",
                round, retries+1, MaxReplyRetries, err, r.describe())
        }
        plan, err = fp.StrategizeRetry(obs, offered, reason, r)
    }
    return plan, err, retries
}

// resolvePlanStep advances past stale steps and returns the first sentence
// that still resolves against the current menu. Resolution failure is a skip,
// not a failure: a prerequisite may have disappeared because the step is
// already satisfied.
func resolvePlanStep(plan *Plan, offered []Objective) (Objective, int, bool) {
    if plan == nil {
        return Objective{}, 0, false
    }
    skipped := 0
    for plan.Step < len(plan.Steps) {
        obj, err := Chosen(offered, plan.Steps[plan.Step])
        if err == nil {
            return obj, skipped, true
        }
        plan.Step++
        skipped++
    }
    return Objective{}, skipped, false
}

// PlanningStats is run-owned telemetry for the three planning tiers. Counts
// are logical planner operations; endpoint-level retry/failover counts remain
// in LLMStats/LLMHealth where they already live.
type PlanningStats struct {
    Plan             Plan           `json:"plan,omitempty"`
    StrategicCalls   int            `json:"strategic_calls,omitempty"`
    FastCalls        int            `json:"fast_calls,omitempty"`
    PlanExecutions   int            `json:"plan_executions,omitempty"`
    StepsSkipped     int            `json:"steps_skipped,omitempty"`
    LastReplanReason string         `json:"last_replan_reason,omitempty"`
    ReplanReasons    map[string]int `json:"replan_reasons,omitempty"`
}

func (s PlanningStats) clone() PlanningStats {
    s.Plan = s.Plan.clone()
    if s.ReplanReasons != nil {
        cp := make(map[string]int, len(s.ReplanReasons))
        for k, v := range s.ReplanReasons {
            cp[k] = v
        }
        s.ReplanReasons = cp
    }
    return s
}

// PlanningObserver lets decorators publish run-owned plan state without
// turning telemetry into another planner or introducing a dependency on farm.
type PlanningObserver interface {
    ObservePlanning(PlanningStats)
}

func notifyPlanning(p Planner, stats PlanningStats) {
    if o, ok := p.(PlanningObserver); ok {
        o.ObservePlanning(stats.clone())
    }
}

type runPlanning struct {
    Plan        Plan
    Stats       PlanningStats
    pending     string
    strategized bool
}

func newRunPlanning(plan Plan) *runPlanning {
    r := &runPlanning{Plan: plan.clone(), strategized: plan.Goal != "" || len(plan.Steps) != 0}
    r.Stats.ReplanReasons = map[string]int{}
    r.sync()
    return r
}

func (r *runPlanning) hasStrategist(p Planner) bool {
    _, ok := p.(StrategicPlanner)
    return ok
}

func (r *runPlanning) request(reason string) {
    if reason == "" || r.pending != "" {
        return
    }
    r.pending = reason
}

func (r *runPlanning) sync() {
    r.Stats.Plan = r.Plan.clone()
}

func (r *runPlanning) snapshot() PlanningStats {
    r.sync()
    return r.Stats.clone()
}

func (r *runPlanning) install(plan Plan, reason string) {
    r.Plan = plan.clone()
    r.strategized = true
    r.pending = ""
    r.Stats.StrategicCalls++
    r.Stats.LastReplanReason = reason
    if r.Stats.ReplanReasons == nil {
        r.Stats.ReplanReasons = map[string]int{}
    }
    r.Stats.ReplanReasons[reason]++
    r.sync()
}

// choose implements the tier order: strategist when a replan is required,
// then a zero-call plan step, then the legacy cheap chooser only when no
// strategist is available. A strategist-produced plan is guaranteed to have
// at least one currently resolvable step by validateStrategicPlan.
func (r *runPlanning) choose(log io.Writer, round int, p Planner, obs Observation, offered []Objective) (Objective, bool, error, int) {
    sp, strategic := p.(StrategicPlanner)
    for {
        if strategic && (r.pending != "" || !r.Plan.Active()) {
            reason := r.pending
            if reason == "" {
                if r.strategized {
                    reason = "plan_exhausted"
                } else {
                    reason = "initial"
                }
            }
            plan, err, retries := strategizeWithRetries(log, round, sp, obs, offered, reason)
            if err != nil {
                return Objective{}, false, err, retries
            }
            r.install(plan, reason)
            obj, skipped, ok := resolvePlanStep(&r.Plan, offered)
            r.Stats.StepsSkipped += skipped
            r.sync()
            if !ok {
                return Objective{}, false, errors.New("agent: strategist returned a plan with no resolvable step") , retries
            }
            r.Stats.PlanExecutions++
            r.sync()
            return obj, true, nil, retries
        }

        if r.Plan.Active() {
            obj, skipped, ok := resolvePlanStep(&r.Plan, offered)
            r.Stats.StepsSkipped += skipped
            r.sync()
            if ok {
                r.Stats.PlanExecutions++
                r.sync()
                return obj, true, nil, 0
            }
            if strategic {
                r.request("plan_exhausted")
                continue
            }
        }

        r.Stats.FastCalls++
        obj, err, retries := planWithRetries(log, round, p, obs, offered)
        r.sync()
        return obj, false, err, retries
    }
}

func (r *runPlanning) success(fromPlan bool) {
    if fromPlan && r.Plan.Step < len(r.Plan.Steps) {
        r.Plan.Step++
    }
    r.sync()
}
''')

Path("agent/plan_test.go").write_text(r'''package agent

import (
    "errors"
    "testing"
)

func TestResolvePlanStepSkipsStaleSentences(t *testing.T) {
    offered := []Objective{
        {Kind: KindGoTo, Place: "pallet town"},
        {Kind: KindGoTo, Place: "route 1"},
    }
    plan := Plan{Goal: "go north", Steps: []string{"heal the party here", "go to route 1"}}
    obj, skipped, ok := resolvePlanStep(&plan, offered)
    if !ok || skipped != 1 || obj.String() != "go to route 1" || plan.Step != 1 {
        t.Fatalf("resolve = obj %q skipped %d ok %v step %d", obj.String(), skipped, ok, plan.Step)
    }
}

func TestValidateStrategicPlanRequiresObjectiveSentences(t *testing.T) {
    offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
    if _, err := validateStrategicPlan(Plan{Goal: "north", Steps: []string{"1"}}, offered, 4); err == nil {
        t.Fatal("menu index was accepted as a durable plan step")
    }
    got, err := validateStrategicPlan(Plan{Goal: "north", Steps: []string{"GO TO ROUTE 1"}}, offered, 4)
    if err != nil {
        t.Fatalf("sentence plan: %v", err)
    }
    if got.Steps[0] != "go to route 1" || got.Round != 4 || got.Step != 0 {
        t.Fatalf("canonical plan = %+v", got)
    }
}

type planningTestPlanner struct {
    strategic int
    fast      int
    plans     []Plan
}

func (p *planningTestPlanner) Next(_ Observation, offered []Objective) (Objective, error) {
    p.fast++
    if len(offered) == 0 {
        return Objective{}, ErrDone
    }
    return offered[0], nil
}

func (p *planningTestPlanner) Strategize(_ Observation, offered []Objective, _ string) (Plan, error) {
    p.strategic++
    if len(p.plans) == 0 {
        return Plan{}, errors.New("no test plan")
    }
    plan := p.plans[0]
    p.plans = p.plans[1:]
    return validateStrategicPlan(plan, offered, 1)
}

func TestRunPlanningExecutesPlanWithoutFastCalls(t *testing.T) {
    offered := []Objective{
        {Kind: KindGoTo, Place: "pallet town"},
        {Kind: KindGoTo, Place: "route 1"},
    }
    p := &planningTestPlanner{plans: []Plan{{Goal: "north", Steps: []string{"go to pallet town", "go to route 1"}}}}
    r := newRunPlanning(Plan{})

    first, fromPlan, err, _ := r.choose(nil, 1, p, Observation{Round: 1}, offered)
    if err != nil || !fromPlan || first.String() != "go to pallet town" {
        t.Fatalf("first = %q fromPlan=%v err=%v", first.String(), fromPlan, err)
    }
    r.success(true)
    second, fromPlan, err, _ := r.choose(nil, 2, p, Observation{Round: 2}, offered)
    if err != nil || !fromPlan || second.String() != "go to route 1" {
        t.Fatalf("second = %q fromPlan=%v err=%v", second.String(), fromPlan, err)
    }
    if p.strategic != 1 || p.fast != 0 {
        t.Fatalf("calls strategic=%d fast=%d, want 1/0", p.strategic, p.fast)
    }
    if r.Stats.PlanExecutions != 2 {
        t.Fatalf("plan executions = %d, want 2", r.Stats.PlanExecutions)
    }
}

func TestRunPlanningReplansWhenRequested(t *testing.T) {
    offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
    p := &planningTestPlanner{plans: []Plan{
        {Goal: "first", Steps: []string{"go to route 1"}},
        {Goal: "recover", Steps: []string{"go to route 1"}},
    }}
    r := newRunPlanning(Plan{})
    if _, _, err, _ := r.choose(nil, 1, p, Observation{Round: 1}, offered); err != nil {
        t.Fatal(err)
    }
    r.request("objective_failed")
    if _, _, err, _ := r.choose(nil, 2, p, Observation{Round: 2}, offered); err != nil {
        t.Fatal(err)
    }
    if p.strategic != 2 || r.Stats.ReplanReasons["objective_failed"] != 1 || r.Plan.Goal != "recover" {
        t.Fatalf("planner=%d stats=%+v plan=%+v", p.strategic, r.Stats, r.Plan)
    }
}
''')

Path("agent/memory_plan_test.go").write_text(r'''package agent

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
)

func TestCheckpointMemoryRestoresPlanAdditively(t *testing.T) {
    dir := t.TempDir()
    statePath := filepath.Join(dir, "round-001.state")
    if err := os.WriteFile(statePath, []byte("state-not-read-here"), 0o644); err != nil {
        t.Fatal(err)
    }
    k := NewKnowledge(map[uint8][]uint8{})
    plan := Plan{Goal: "reach pewter", Steps: []string{"go to route 1", "go to viridian city"}, Step: 1, Round: 7}
    if err := writeMemoryFile(statePath, k, "", 0, plan); err != nil {
        t.Fatal(err)
    }
    got := LoadCheckpointMemory(statePath, map[uint8][]uint8{}, nil)
    if got.Plan.Goal != plan.Goal || got.Plan.Step != 1 || len(got.Plan.Steps) != 2 {
        t.Fatalf("restored plan = %+v, want %+v", got.Plan, plan)
    }
}

func TestCheckpointMemoryVersionFourWithoutPlanLoadsEmptyPlan(t *testing.T) {
    dir := t.TempDir()
    statePath := filepath.Join(dir, "round-001.state")
    if err := os.WriteFile(statePath, []byte("state-not-read-here"), 0o644); err != nil {
        t.Fatal(err)
    }
    legacy := map[string]any{
        "version": memoryVersion,
        "visited": []uint8{},
        "places": []string{},
        "completed": []Completion{},
        "talked": []talkedKey{},
    }
    data, _ := json.Marshal(legacy)
    if err := os.WriteFile(knowledgePathForState(statePath), data, 0o644); err != nil {
        t.Fatal(err)
    }
    got := LoadCheckpointMemory(statePath, map[uint8][]uint8{}, nil)
    if got.Plan.Goal != "" || len(got.Plan.Steps) != 0 || got.Plan.Step != 0 {
        t.Fatalf("legacy checkpoint restored non-empty plan: %+v", got.Plan)
    }
}
''')

# Memory: add Plan without bumping v4 so older v4 checkpoint JSON decodes to zero Plan.
replace_once("agent/memory.go",
'''\tFailures  []Failure `json:"failures,omitempty"`
\tIntent    string    `json:"intent,omitempty"`
\tIntentAge int       `json:"intent_age,omitempty"`
}''',
'''\tFailures  []Failure `json:"failures,omitempty"`
\tIntent    string    `json:"intent,omitempty"`
\tIntentAge int       `json:"intent_age,omitempty"`
\tPlan      Plan      `json:"plan,omitempty"`
}''')
replace_once("agent/memory.go",
'''func encodeMemoryFile(k *Knowledge, intent string, intentAge int) ([]byte, error) {
\tmem := memoryFile{Version: memoryVersion, Intent: intent, IntentAge: intentAge}''',
'''func encodeMemoryFile(k *Knowledge, intent string, intentAge int, plans ...Plan) ([]byte, error) {
\tmem := memoryFile{Version: memoryVersion, Intent: intent, IntentAge: intentAge}
\tif len(plans) > 0 {
\t\tmem.Plan = plans[0].clone()
\t}''')
replace_once("agent/memory.go",
'''func writeMemoryFile(statePath string, k *Knowledge, intent string, intentAge int) error {
\tdata, err := encodeMemoryFile(k, intent, intentAge)''',
'''func writeMemoryFile(statePath string, k *Knowledge, intent string, intentAge int, plans ...Plan) error {
\tdata, err := encodeMemoryFile(k, intent, intentAge, plans...)''')
replace_once("agent/memory.go",
'''type ResumedMemory struct {
\tKnowledge *Knowledge
\tIntent    string
\tIntentAge int
}''',
'''type ResumedMemory struct {
\tKnowledge *Knowledge
\tIntent    string
\tIntentAge int
\tPlan      Plan
}''')
replace_once("agent/memory.go",
'''\tif len(mem.Intent) > IntentCap {
\t\t// An over-cap intent cannot have come from a WithArgs that passed
\t\t// validation; the file is not what we wrote.
\t\tlogMemory(log, "knowledge file beside %s carries an intent of %d bytes, over the cap of %d; starting with empty knowledge",
\t\t\tstatePath, len(mem.Intent), IntentCap)
\t\treturn empty
\t}
\tk := NewKnowledge(adjacency)
\tk.restore(mem)
\treturn ResumedMemory{Knowledge: k, Intent: mem.Intent, IntentAge: mem.IntentAge}''',
'''\tif len(mem.Intent) > IntentCap {
\t\t// An over-cap intent cannot have come from a WithArgs that passed
\t\t// validation; the file is not what we wrote.
\t\tlogMemory(log, "knowledge file beside %s carries an intent of %d bytes, over the cap of %d; starting with empty knowledge",
\t\t\tstatePath, len(mem.Intent), IntentCap)
\t\treturn empty
\t}
\tif err := validateStoredPlan(mem.Plan); err != nil {
\t\tlogMemory(log, "knowledge file beside %s carries an invalid plan (%v); starting with empty knowledge", statePath, err)
\t\treturn empty
\t}
\tk := NewKnowledge(adjacency)
\tk.restore(mem)
\treturn ResumedMemory{Knowledge: k, Intent: mem.Intent, IntentAge: mem.IntentAge, Plan: mem.Plan.clone()}''')

# LLM strategist: keep chooser schema untouched, add a separate plan schema and request mode.
replace_once("agent/llm.go",
'''const maxRetryTokens = 8192''',
'''const maxRetryTokens = 8192

const (
\tstrategicReplyTokens = 8192
\tstrategicRetryTokens = 32768
\tstrategicTimeout     = 2 * time.Minute
)''')
insert_before("agent/llm.go", "// NewLLMPlanner returns an LLMPlanner", r'''// StrategicPromptHash is the comparability marker for the strategist prompt.
// It is deliberately separate from PromptHash because the two tiers have
// different instructions and reply schemas.
func (p *LLMPlanner) StrategicPromptHash() string {
    schema, err := json.Marshal(planSchema)
    if err != nil {
        return ""
    }
    return PromptHash(strategicSystemPrompt, p.Goal, p.ExtraSystem, string(schema))
}

''')
insert_before("agent/llm.go", "// resolveReply turns a raw model reply", r'''// Strategize asks the same endpoint for an ordered, bounded plan. Thinking is
// intentionally enabled for this request even when the cheap chooser runs
// with NoThink=true, and the strategist receives a larger completion/time
// budget so a reasoning block cannot make planning unusable by construction.
func (p *LLMPlanner) Strategize(obs Observation, offered []Objective, reason string) (Plan, error) {
    return p.StrategizeRetry(obs, offered, reason, Retry{})
}

func (p *LLMPlanner) StrategizeRetry(obs Observation, offered []Objective, reason string, r Retry) (Plan, error) {
    if len(offered) == 0 {
        return Plan{}, fmt.Errorf("agent: strategist: nothing was offered")
    }
    start := time.Now()
    res, err := p.askPlan(obs, offered, reason, r.Feedback, r.Temperature, r.MaxTokensFactor)
    took := time.Since(start)
    if err != nil {
        return Plan{}, err
    }
    reply := strings.TrimSpace(res.Content)
    if res.Usage != nil {
        p.Health.PromptTokens += res.Usage.PromptTokens
        p.Health.CompletionTokens += res.Usage.CompletionTokens
    }
    if p.Log != nil {
        usage := ""
        if res.Usage != nil {
            usage = fmt.Sprintf(", tokens %d prompt/%d completion", res.Usage.PromptTokens, res.Usage.CompletionTokens)
        }
        fmt.Fprintf(p.Log, "  strategist: %d offered, %s%s, reply %q\n", len(offered), took.Round(10*time.Millisecond), usage, snippet([]byte(reply)))
    }
    if res.Model != "" && res.Model != p.Model {
        p.Health.Rejected++
        return Plan{}, fmt.Errorf("%w: requested %q but %q answered", ErrModelMismatch, p.Model, res.Model)
    }
    if res.Model == "" && !p.modelOmittedLogged {
        p.modelOmittedLogged = true
        if p.Log != nil {
            fmt.Fprintln(p.Log, "  llm: server did not report a model field; cannot verify which model answered")
        }
    }
    if res.FinishReason != "" && res.FinishReason != "stop" {
        p.Health.Rejected++
        return Plan{}, fmt.Errorf("%w: finish_reason %q", ErrNotFinished, res.FinishReason)
    }
    var raw Plan
    if err := json.Unmarshal([]byte(thinkRe.ReplaceAllString(reply, "")), &raw); err != nil {
        p.Health.Rejected++
        return Plan{}, fmt.Errorf("agent: strategist: reply is not a plan JSON object: %w", err)
    }
    plan, err := validateStrategicPlan(raw, offered, obs.Round)
    if err != nil {
        p.Health.Rejected++
        return Plan{}, err
    }
    return plan, nil
}

''')
insert_before("agent/llm.go", "type chatMessage struct", r'''const strategicSystemPrompt = `You are the strategic planner for a deterministic game-playing runtime. Build a short multi-round plan toward the run goal from the current Observation. Deterministic code owns legality, navigation, battles, menus, and execution; you only sequence semantic objectives. Every plan step MUST be copied exactly as an objective sentence from the current Offered objectives. Never use menu indexes, never invent an unavailable action, and never infer that a prerequisite is satisfied unless Observation says so. RouteBlockages and Requirements are explicit evidence for prerequisite planning. Reply with ONLY JSON: {"goal":"one short strategic purpose","steps":["exact objective sentence", ...]}. Use at most 10 steps. Do not explain.`

func (p *LLMPlanner) strategicSystemMessage() string {
    s := strategicSystemPrompt + p.ExtraSystem
    if p.Goal != "" {
        s = "Your goal: " + p.Goal + "\n\n" + s
    }
    return s
}

func strategicUserPrompt(obs Observation, offered []Objective, reason string) string {
    obsJSON, err := json.Marshal(obs)
    if err != nil {
        obsJSON = []byte("{}")
    }
    var b strings.Builder
    b.WriteString("Replan reason: ")
    if strings.TrimSpace(reason) == "" {
        b.WriteString("initial")
    } else {
        b.WriteString(reason)
    }
    b.WriteString("\nObservation:\n")
    b.Write(obsJSON)
    b.WriteString("\n\nOffered objectives (copy the sentence after the number, not the number):\n")
    for i, o := range offered {
        if o.Note != "" {
            fmt.Fprintf(&b, "%d: %s  %s\n", i+1, o, o.Note)
        } else {
            fmt.Fprintf(&b, "%d: %s\n", i+1, o)
        }
    }
    return b.String()
}

''')
insert_before("agent/llm.go", "// chatChoice carries", r'''var planSchema = map[string]any{
    "type": "object",
    "properties": map[string]any{
        "goal": map[string]any{"type": "string"},
        "steps": map[string]any{
            "type": "array",
            "minItems": 1,
            "maxItems": MaxPlanSteps,
            "items": map[string]any{"type": "string"},
        },
    },
    "required": []string{"goal", "steps"},
}

''')

# Replace the transport function with a shared request primitive and a strategic mode.
llm = Path("agent/llm.go").read_text()
start = llm.index("func (p *LLMPlanner) ask(obs Observation")
end = llm.index("\nvar (\n\tintRe", start)
new_ask = r'''func (p *LLMPlanner) ask(obs Observation, offered []Objective, feedback string, temperature *float64, maxTokensFactor int) (chatResult, error) {
    system := p.systemPrompt()
    user := llmUserPrompt(obs, offered)
    if feedback != "" {
        user += "\n\nYour previous reply was rejected: " + feedback +
            "\nReply again with ONLY a JSON object naming one of the offered objectives and only arguments that apply to it."
    }
    maxTokens := p.MaxTokens
    if maxTokens <= 0 {
        maxTokens = maxReplyTokens
    }
    return p.askRequest(system, user, "objective_choice", choiceSchema, p.NoThink, maxTokens, maxRetryTokens, p.Timeout, p.PromptHash(), temperature, maxTokensFactor)
}

func (p *LLMPlanner) askPlan(obs Observation, offered []Objective, reason, feedback string, temperature *float64, maxTokensFactor int) (chatResult, error) {
    system := p.strategicSystemMessage()
    user := strategicUserPrompt(obs, offered, reason)
    if feedback != "" {
        user += "\n\nYour previous plan reply was rejected: " + feedback +
            "\nReturn ONLY corrected plan JSON, using exact objective sentences from the offered list."
    }
    maxTokens := p.MaxTokens
    if maxTokens < strategicReplyTokens {
        maxTokens = strategicReplyTokens
    }
    timeout := p.Timeout
    if timeout < strategicTimeout {
        timeout = strategicTimeout
    }
    return p.askRequest(system, user, "objective_plan", planSchema, false, maxTokens, strategicRetryTokens, timeout, p.StrategicPromptHash(), temperature, maxTokensFactor)
}

func (p *LLMPlanner) askRequest(system, user, schemaName string, schema map[string]any, noThink bool, baseMaxTokens, retryCap int, timeout time.Duration, promptHash string, temperature *float64, maxTokensFactor int) (chatResult, error) {
    client := p.Client
    if client == nil {
        if timeout <= 0 {
            timeout = 60 * time.Second
        }
        client = &http.Client{Timeout: timeout}
    }
    if p.PromptLog != nil {
        fmt.Fprintf(p.PromptLog, "=== prompt (model %s, prompt %s) ===\n[system]\n%s\n[user]\n%s\n",
            p.Model, promptHash, system, user)
    }
    transportErr := func(format string, args ...any) (chatResult, error) {
        p.Health.Transport++
        return chatResult{}, fmt.Errorf("agent: llm planner: "+format, args...)
    }
    temp := 0.0
    if temperature != nil {
        temp = *temperature
    }
    maxTokens := baseMaxTokens
    if maxTokensFactor > 1 {
        if mt := maxTokens * maxTokensFactor; mt < retryCap {
            maxTokens = mt
        } else {
            maxTokens = retryCap
        }
    }
    var templateKwargs map[string]any
    if noThink {
        templateKwargs = map[string]any{"enable_thinking": false}
    }
    reqBody, err := json.Marshal(chatRequest{
        Model:       p.Model,
        Temperature: temp,
        MaxTokens:   maxTokens,
        Messages: []chatMessage{
            {Role: "system", Content: system},
            {Role: "user", Content: user},
        },
        ChatTemplateKwargs: templateKwargs,
        ResponseFormat: &responseFormat{
            Type:       "json_schema",
            JSONSchema: &jsonSchema{Name: schemaName, Strict: false, Schema: schema},
        },
    })
    if err != nil {
        return transportErr("encode request: %w", err)
    }
    url := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
    req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
    if err != nil {
        return transportErr("build request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")
    if p.Token != "" {
        req.Header.Set("Authorization", "Bearer "+p.Token)
    }
    resp, err := client.Do(req)
    if err != nil {
        return transportErr("POST %s: %w", url, err)
    }
    defer resp.Body.Close()
    data, err := io.ReadAll(resp.Body)
    if err != nil {
        return transportErr("read reply: %w", err)
    }
    if resp.StatusCode != http.StatusOK {
        return transportErr("model returned HTTP %s: %s", resp.Status, snippet(data))
    }
    var cr chatResponse
    if err := json.Unmarshal(data, &cr); err != nil {
        return transportErr("reply is not valid JSON: %v", err)
    }
    if len(cr.Choices) == 0 {
        return transportErr("reply has no choices: %s", snippet(data))
    }
    if p.ReplyLog != nil {
        fmt.Fprintf(p.ReplyLog, "=== reply (model %s, finish %s) ===\n%s\n",
            cr.Model, cr.Choices[0].FinishReason, cr.Choices[0].Message.Content)
    }
    return chatResult{
        Content:      cr.Choices[0].Message.Content,
        Model:        cr.Model,
        FinishReason: cr.Choices[0].FinishReason,
        Usage:        cr.Usage,
    }, nil
}
'''
Path("agent/llm.go").write_text(llm[:start] + new_ask + llm[end:])

Path("agent/llm_plan_test.go").write_text(r'''package agent

import "testing"

func TestPlanSchemaIsSeparateFromChoiceSchema(t *testing.T) {
    choice := choiceSchema["properties"].(map[string]any)
    if _, ok := choice["steps"]; ok {
        t.Fatal("strategist fields leaked into the chooser schema")
    }
    props := planSchema["properties"].(map[string]any)
    if _, ok := props["goal"]; !ok {
        t.Fatal("plan schema missing goal")
    }
    if _, ok := props["steps"]; !ok {
        t.Fatal("plan schema missing steps")
    }
}

func TestStrategicPlanRejectsUnavailableFutureAction(t *testing.T) {
    offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
    _, err := validateStrategicPlan(Plan{Goal: "reach pewter", Steps: []string{"go to route 1", "go to pewter city"}}, offered, 1)
    if err == nil {
        t.Fatal("strategist invented an unavailable future action")
    }
}
''')

# Failover forwards strategist asks through the same transport-only pinning path.
replace_once("agent/failover.go",
'''type LLMCall struct {
\tObservation Observation
\tOffered     int
\tObjective   Objective
\tErr         error
\tDuration    time.Duration
}''',
'''type LLMCall struct {
\tObservation Observation
\tOffered     int
\tObjective   Objective
\tPlan        Plan
\tStrategic   bool
\tErr         error
\tDuration    time.Duration
}''')
insert_before("agent/failover.go", "func (p *FailoverPlanner) ask(obs Observation", r'''func (p *FailoverPlanner) Strategize(obs Observation, offered []Objective, reason string) (Plan, error) {
    return p.askPlan(obs, offered, reason, nil)
}

func (p *FailoverPlanner) StrategizeRetry(obs Observation, offered []Objective, reason string, r Retry) (Plan, error) {
    return p.askPlan(obs, offered, reason, &r)
}

func (p *FailoverPlanner) askPlan(obs Observation, offered []Objective, reason string, retry *Retry) (Plan, error) {
    active := p.active
    p.syncContext(active)
    plan, err, transport := p.callPlan(active, obs, offered, reason, retry)
    if transport && active == p.Primary && p.Fallback != nil {
        p.failovers++
        p.active = p.Fallback
        p.backend = "fallback"
        if p.Primary.Log != nil {
            fmt.Fprintf(p.Primary.Log,
                "  llm route: primary %s at %s had a strategist transport failure; pinning fallback %s at %s for the rest of the run\n",
                p.Primary.Model, p.Primary.BaseURL, p.Fallback.Model, p.Fallback.BaseURL)
        }
        active = p.Fallback
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

''')

# Run: add planning result, restore/persist plan, tier selection, and escalation-before-stop behavior.
replace_once("agent/run.go",
'''\tProgressEarly *Progress
\tProgressFinal *Progress
}''',
'''\tProgressEarly *Progress
\tProgressFinal *Progress
\t// Planning is the run-owned three-tier planner telemetry and final plan.
\tPlanning PlanningStats
}''')
replace_once("agent/run.go",
'''\tintent, intentAge := "", 0
\tif budget.ResumeFrom != "" {''',
'''\tintent, intentAge := "", 0
\tresumedPlan := Plan{}
\tif budget.ResumeFrom != "" {''')
replace_once("agent/run.go",
'''\t\tknown = mem.Knowledge
\t\tintent, intentAge = mem.Intent, mem.IntentAge
\t}''',
'''\t\tknown = mem.Knowledge
\t\tintent, intentAge = mem.Intent, mem.IntentAge
\t\tresumedPlan = mem.Plan.clone()
\t}''')
replace_once("agent/run.go",
'''\tlast := Observe(m, romData)
\t// The early progress sample:''',
'''\tlast := Observe(m, romData)
\tplanning := newRunPlanning(resumedPlan)
\tnotifyPlanning(p, planning.snapshot())
\tknownRequirementCount := len(known.Requirements)
\tobservedBadges, observedEvents := len(last.Badges), len(last.Events)
\tstuckEscalated, stagnationEscalated := false, false
\tfailureEscalated := map[string]bool{}
\t// The early progress sample:''')
replace_once("agent/run.go",
'''\t\tnoteObservation(known, last)
\t\t// Every map the last objective actually walked through,''',
'''\t\tnoteObservation(known, last)
\t\tif len(known.Requirements) > knownRequirementCount {
\t\t\tplanning.request("new_requirement")
\t\t\tknownRequirementCount = len(known.Requirements)
\t\t}
\t\tif round > 1 && len(last.Badges) > observedBadges {
\t\t\tplanning.request("badge_changed")
\t\t}
\t\tif round > 1 && len(last.Events) > observedEvents {
\t\t\tplanning.request("story_changed")
\t\t}
\t\tobservedBadges, observedEvents = len(last.Badges), len(last.Events)
\t\t// Every map the last objective actually walked through,''')
replace_once("agent/run.go",
'''\t\tcurrentMajorProgress := majorProgressMarkOf(last, known)
\t\tif majorProgress.absorb(currentMajorProgress) {
\t\t\tlastMajorProgressRound = round - 1
\t\t\tif budget.Log != nil && round > 1 {
\t\t\t\tfmt.Fprintf(budget.Log, "round %d: major progress -> %s\\n", round-1, majorProgress)
\t\t\t}
\t\t}
\t\tcompletedRounds := round - 1
\t\tstagnantRounds := completedRounds - lastMajorProgressRound''',
'''\t\tcurrentMajorProgress := majorProgressMarkOf(last, known)
\t\tif majorProgress.absorb(currentMajorProgress) {
\t\t\tlastMajorProgressRound = round - 1
\t\t\tstagnationEscalated = false
\t\t\tif budget.Log != nil && round > 1 {
\t\t\t\tfmt.Fprintf(budget.Log, "round %d: major progress -> %s\\n", round-1, majorProgress)
\t\t\t}
\t\t}
\t\tcompletedRounds := round - 1
\t\tstagnantRounds := completedRounds - lastMajorProgressRound
\t\tif stagnantRounds >= stagnationAfter {
\t\t\tif planning.hasStrategist(p) {
\t\t\t\tif stagnationEscalated {
\t\t\t\t\tres.Stop = StopStuck
\t\t\t\t\tif budget.Log != nil {
\t\t\t\t\t\tfmt.Fprintf(budget.Log, "stagnation watchdog recurred after strategic replan: %d rounds without major progress; high-water mark: %s\\n", stagnantRounds, majorProgress)
\t\t\t\t\t}
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t\tplanning.request("stagnation")
\t\t\t\tstagnationEscalated = true
\t\t\t\tlastMajorProgressRound = completedRounds
\t\t\t} else {
\t\t\t\tres.Stop = StopStuck
\t\t\t\tif budget.Log != nil {
\t\t\t\t\tfmt.Fprintf(budget.Log, "stagnation watchdog: %d rounds without major progress; high-water mark: %s\\n", stagnantRounds, majorProgress)
\t\t\t\t}
\t\t\t\tbreak
\t\t\t}
\t\t}''')
replace_once("agent/run.go",
'''\t\tobj, err, retries := planWithRetries(budget.Log, round, p, last, now)
\t\tres.ReplyRetries += retries''',
'''\t\tobj, fromPlan, err, retries := planning.choose(budget.Log, round, p, last, now)
\t\tres.ReplyRetries += retries
\t\tnotifyPlanning(p, planning.snapshot())''')
replace_once("agent/run.go",
'''\t\tif stagnantRounds >= stagnationAfter {
\t\t\tres.Stop = StopStuck
\t\t\tif budget.Log != nil {
\t\t\t\tfmt.Fprintf(budget.Log, "stagnation watchdog: %d rounds without major progress; high-water mark: %s\\n",
\t\t\t\t\tstagnantRounds, majorProgress)
\t\t\t}
\t\t\tbreak
\t\t}

''', '')
replace_once("agent/run.go",
'''\t\tif ring != nil {
\t\t\tif err := ring.write(m, round, obj, known, intent, intentAge); err != nil {''',
'''\t\tif ring != nil {
\t\t\tif err := ring.write(m, round, obj, known, intent, intentAge, planning.Plan); err != nil {''')

# Strategic recoverable failures replan once per structured objective/cause before terminal failure.
insert_before("agent/run.go", "\t\t\tif blackedOut || retreated {", r'''            if planning.hasStrategist(p) {
                consecFailures++
                cause := string(objectiveResult.Cause)
                if cause == "" {
                    cause = string(objectiveResult.Outcome)
                }
                key := obj.String() + "|" + cause
                if failureEscalated[key] || consecFailures > maxConsecFailures {
                    res.Stop, res.Err = StopFailed, execErr
                    break
                }
                failureEscalated[key] = true
                reason := "objective_failed"
                if blackedOut {
                    reason = "blackout"
                } else if retreated {
                    reason = "train_retreat"
                }
                planning.request(reason)
                notifyPlanning(p, planning.snapshot())
                lastFailObj, lastFailErr = obj.String(), execErr.Error()
                if m.FrameCount()-startFrame >= uint64(budget.MaxFrames) {
                    res.Stop = StopBudget
                    break
                }
                continue
            }
''')
replace_once("agent/run.go",
'''\t\tres.Outcomes = append(res.Outcomes, objectiveResult)
\t\tres.Completed = append(res.Completed, obj)
\t\tknown.Done(obj)''',
'''\t\tres.Outcomes = append(res.Outcomes, objectiveResult)
\t\tres.Completed = append(res.Completed, obj)
\t\tplanning.success(fromPlan)
\t\tnotifyPlanning(p, planning.snapshot())
\t\tknown.Done(obj)''')
replace_once("agent/run.go",
'''\t\tconsecFailures = 0
\t\tlastFailObj, lastFailErr = "", ""''',
'''\t\tconsecFailures = 0
\t\tfailureEscalated = map[string]bool{}
\t\tlastFailObj, lastFailErr = "", ""''')
replace_once("agent/run.go",
'''\t\tif sameProgress(before, last) {
\t\t\tstuck++
\t\t} else {
\t\t\tstuck = 0
\t\t}
\t\tif stuck >= stuckAfter {
\t\t\tres.Stop = StopStuck
\t\t\tbreak
\t\t}''',
'''\t\tif sameProgress(before, last) {
\t\t\tstuck++
\t\t} else {
\t\t\tstuck = 0
\t\t\tstuckEscalated = false
\t\t}
\t\tif stuck >= stuckAfter {
\t\t\tif planning.hasStrategist(p) {
\t\t\t\tif stuckEscalated {
\t\t\t\t\tres.Stop = StopStuck
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t\tplanning.request("stuck")
\t\t\t\tstuckEscalated = true
\t\t\t\tstuck = 0
\t\t\t\tnotifyPlanning(p, planning.snapshot())
\t\t\t} else {
\t\t\t\tres.Stop = StopStuck
\t\t\t\tbreak
\t\t\t}
\t\t}''')
replace_once("agent/run.go",
'''\tres.Final = last
\tif deterministicGoal {''',
'''\tres.Final = last
\tres.Planning = planning.snapshot()
\tif deterministicGoal {''')
replace_once("agent/run.go",
'''func (c *checkpointRing) write(m *emu.Emu, round int, obj Objective, k *Knowledge, intent string, intentAge int) error {''',
'''func (c *checkpointRing) write(m *emu.Emu, round int, obj Objective, k *Knowledge, intent string, intentAge int, plans ...Plan) error {''')
replace_once("agent/run.go",
'''\tif err := writeMemoryFile(path, k, intent, intentAge); err != nil {''',
'''\tif err := writeMemoryFile(path, k, intent, intentAge, plans...); err != nil {''')

# Update obsolete strategy comment now that Run owns Plan in addition to Intent.
replace_once("agent/strategy.go",
'''// It deliberately stores no planner-authored strategy text: Observation.Intent
// and IntentAge are the single planner-owned strategic memory carried by Run.''',
'''// It deliberately stores no planner-authored strategy text. Observation.Intent
// remains chooser-authored context; the persistent multi-step Plan is owned and
// advanced separately by Run.''')

# Farm telemetry fields.
replace_once("farm/spec.go",
'''\tIntent    string `json:"intent"`
\tIntentAge int    `json:"intent_age"`

\t// Goal* is present only when LLMPlanner.Goal opted into the structured''',
'''\tIntent    string `json:"intent"`
\tIntentAge int    `json:"intent_age"`

\tStrategicCalls   int            `json:"strategic_calls,omitempty"`
\tFastCalls        int            `json:"fast_calls,omitempty"`
\tPlanExecutions   int            `json:"plan_executions,omitempty"`
\tStepsSkipped     int            `json:"steps_skipped,omitempty"`
\tPlanGoal         string         `json:"plan_goal,omitempty"`
\tPlanSteps        []string       `json:"plan_steps,omitempty"`
\tPlanStep         int            `json:"plan_step,omitempty"`
\tPlanRound        int            `json:"plan_round,omitempty"`
\tLastReplanReason string         `json:"last_replan_reason,omitempty"`
\tReplanReasons    map[string]int `json:"replan_reasons,omitempty"`
\tStrategicSeconds float64        `json:"strategic_seconds,omitempty"`

\t// Goal* is present only when LLMPlanner.Goal opted into the structured''')

# statsPlanner forwards strategist and publishes run-owned planning telemetry.
replace_once("cmd/pokepilot/stats.go",
'''\ts.router = agent.NewFailoverPlanner(inner, fallback)
\ts.router.OnCall = func(call agent.LLMCall) {
\t\ts.record(call.Observation, call.Offered, call.Objective, call.Err, call.Duration)
\t}
\treturn s''',
'''\ts.router = agent.NewFailoverPlanner(inner, fallback)
\ts.router.OnCall = s.recordCall
\treturn s''')
insert_before("cmd/pokepilot/stats.go", "// prepareRunContext is the one per-ask seam", r'''func (s *statsPlanner) Strategize(obs agent.Observation, offered []agent.Objective, reason string) (agent.Plan, error) {
    s.prepareRunContext(obs)
    return s.router.Strategize(obs, offered, reason)
}

func (s *statsPlanner) StrategizeRetry(obs agent.Observation, offered []agent.Objective, reason string, r agent.Retry) (agent.Plan, error) {
    s.prepareRunContext(obs)
    return s.router.StrategizeRetry(obs, offered, reason, r)
}

func (s *statsPlanner) ObservePlanning(p agent.PlanningStats) {
    s.stats.PlanGoal = p.Plan.Goal
    s.stats.PlanSteps = append([]string(nil), p.Plan.Steps...)
    s.stats.PlanStep = p.Plan.Step
    s.stats.PlanRound = p.Plan.Round
    s.stats.PlanExecutions = p.PlanExecutions
    s.stats.StepsSkipped = p.StepsSkipped
    s.stats.LastReplanReason = p.LastReplanReason
    s.stats.ReplanReasons = make(map[string]int, len(p.ReplanReasons))
    for k, v := range p.ReplanReasons {
        s.stats.ReplanReasons[k] = v
    }
    s.publish()
}

''')
# Replace record implementation with branching call record while retaining test helper signature.
stats = Path("cmd/pokepilot/stats.go").read_text()
start = stats.index("func (s *statsPlanner) record(obs agent.Observation")
end = stats.index("\n// publishSnapshot", start)
new_record = r'''func (s *statsPlanner) record(obs agent.Observation, offered int, o agent.Objective, err error, took time.Duration) {
    s.recordCall(agent.LLMCall{Observation: obs, Offered: offered, Objective: o, Err: err, Duration: took})
}

func (s *statsPlanner) recordCall(call agent.LLMCall) {
    obs, offered, o, err, took := call.Observation, call.Offered, call.Objective, call.Err, call.Duration
    s.stats.Calls++
    s.offered += offered
    s.elapsed += took
    s.stats.LastSeconds = took.Seconds()
    s.stats.AvgOffered = float64(s.offered) / float64(s.stats.Calls)
    s.stats.AvgSeconds = s.elapsed.Seconds() / float64(s.stats.Calls)
    s.stats.Round, s.stats.RoundsLeft = obs.Round, obs.RoundsLeft
    s.stats.Intent, s.stats.IntentAge = obs.Intent, obs.IntentAge
    route := s.router.Route()
    s.stats.Backend, s.stats.Model, s.stats.Failovers = route.Backend, route.Model, route.Failovers

    h := s.router.Health()
    s.stats.PromptTokens, s.stats.CompletionTokens = h.PromptTokens, h.CompletionTokens
    s.stats.Transport, s.stats.Fallbacks = h.Transport, h.Fallbacks

    if call.Strategic {
        s.stats.StrategicCalls++
        s.stats.StrategicSeconds += took.Seconds()
        if err != nil {
            s.stats.Rejected++
        }
        s.publish()
        return
    }

    s.stats.FastCalls++
    if err != nil {
        s.stats.Rejected++
    } else {
        s.stats.Rounds++
        name := o.String()
        if s.counts[name] > 0 {
            s.stats.Repeats++
        }
        s.counts[name]++
        s.stats.Choices = rankChoices(s.counts)
    }
    s.publish()
}
'''
Path("cmd/pokepilot/stats.go").write_text(stats[:start] + new_record + stats[end:])

# reportingPlanner must preserve optional strategist and planning observer interfaces.
insert_before("cmd/pokepilot/farm.go", "func (p reportingPlanner) ask(obs agent.Observation", r'''func (p reportingPlanner) Strategize(obs agent.Observation, offered []agent.Objective, reason string) (agent.Plan, error) {
    return p.strategize(obs, offered, reason, agent.Retry{})
}

func (p reportingPlanner) StrategizeRetry(obs agent.Observation, offered []agent.Objective, reason string, r agent.Retry) (agent.Plan, error) {
    return p.strategize(obs, offered, reason, r)
}

func (p reportingPlanner) strategize(obs agent.Observation, offered []agent.Objective, reason string, r agent.Retry) (agent.Plan, error) {
    q := "STRATEGY (" + reason + ")\n" + planQuestion(offered)
    if p.snap != nil {
        p.snap.storePlan(q, "")
    }
    sp := p.inner.(agent.StrategicPlanner)
    var (
        plan agent.Plan
        err  error
    )
    if r != (agent.Retry{}) {
        plan, err = p.inner.(agent.StrategicFeedbackPlanner).StrategizeRetry(obs, offered, reason, r)
    } else {
        plan, err = sp.Strategize(obs, offered, reason)
    }
    if err == nil && p.snap != nil {
        p.snap.storePlan(q, plan.Goal+": "+strings.Join(plan.Steps, " -> "))
    }
    return plan, err
}

func (p reportingPlanner) ObservePlanning(stats agent.PlanningStats) {
    if o, ok := p.inner.(agent.PlanningObserver); ok {
        o.ObservePlanning(stats)
    }
}

''')

# badgerun wrapper forwards strategist calls and counts them as model calls.
insert_before("cmd/badgerun/main.go", "func (b *badgePlanner) Next(obs agent.Observation", r'''func (b *badgePlanner) Strategize(obs agent.Observation, offered []agent.Objective, reason string) (agent.Plan, error) {
    return b.StrategizeRetry(obs, offered, reason, agent.Retry{})
}

func (b *badgePlanner) StrategizeRetry(obs agent.Observation, offered []agent.Objective, reason string, r agent.Retry) (agent.Plan, error) {
    b.calls++
    if obs.BlackedOut && !b.sawBlackout {
        b.blackouts++
    }
    b.sawBlackout = obs.BlackedOut
    if b.framesToBadge == 0 && hasBadge(obs, b.badge) {
        b.framesToBadge = b.m.FrameCount()
        return agent.Plan{}, agent.ErrDone
    }
    return b.inner.StrategizeRetry(obs, offered, reason, r)
}

''')

# Watch page: surface plan tier/position without changing game UI behavior.
replace_once("emu/watch.go",
'''    row('think', s.last_seconds.toFixed(1) + 's / ' + s.avg_seconds.toFixed(1) + 's avg') +
    row('offered', s.avg_offered.toFixed(1) + ' avg') +''',
'''    row('think', s.last_seconds.toFixed(1) + 's / ' + s.avg_seconds.toFixed(1) + 's avg') +
    row('tiers', (s.strategic_calls || 0) + ' strategic / ' + (s.fast_calls || 0) + ' fast / ' + (s.plan_executions || 0) + ' zero-call') +
    row('plan', s.plan_goal ? ((s.plan_step || 0) + '/' + (s.plan_steps ? s.plan_steps.length : 0) + ' ' + s.plan_goal) : 'none') +
    row('replan', s.last_replan_reason || '—') +
    row('offered', s.avg_offered.toFixed(1) + ' avg') +''')

# Add an agent-level run test with a strategist wrapper; fixture-backed when ROM is available.
Path("agent/run_plan_test.go").write_text(r'''package agent_test

import (
    "testing"

    "github.com/maestroi/pokepilot/agent"
)

type strategicScriptPlanner struct {
    plans      []agent.Plan
    fastCalls  int
    planCalls  int
}

func (p *strategicScriptPlanner) Next(_ agent.Observation, offered []agent.Objective) (agent.Objective, error) {
    p.fastCalls++
    if len(offered) == 0 {
        return agent.Objective{}, agent.ErrDone
    }
    return offered[0], nil
}

func (p *strategicScriptPlanner) Strategize(_ agent.Observation, offered []agent.Objective, _ string) (agent.Plan, error) {
    p.planCalls++
    if len(p.plans) == 0 {
        return agent.Plan{}, agent.ErrDone
    }
    plan := p.plans[0]
    p.plans = p.plans[1:]
    return plan, nil
}

// This fixture-backed integration pins the zero-call tier through Run itself:
// one strategist call creates two legal starter-room steps, and the second
// round must not ask the cheap chooser again. It naturally skips with the
// rest of run_test when no prepared ROM fixture is available.
func TestRunExecutesPersistentPlanWithoutChooserCall(t *testing.T) {
    e := loadFixture(t)
    p := &strategicScriptPlanner{plans: []agent.Plan{{
        Goal: "leave the bedroom",
        Steps: []string{"take the charmander starter", "go to pallet town"},
        Round: 1,
    }}}
    res := agent.Run(e, e.ROM(), p, agent.Budget{MaxRounds: 2, MaxFrames: 10_000_000})
    if res.Stop != agent.StopBudget && res.Stop != agent.StopDone {
        t.Fatalf("stop=%v err=%v", res.Stop, res.Err)
    }
    if p.planCalls != 1 || p.fastCalls != 0 {
        t.Fatalf("calls strategist=%d chooser=%d, want 1/0", p.planCalls, p.fastCalls)
    }
    if res.Planning.PlanExecutions != 2 {
        t.Fatalf("plan executions=%d, want 2", res.Planning.PlanExecutions)
    }
}
''')
