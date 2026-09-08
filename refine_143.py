from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    s = p.read_text()
    if old not in s:
        raise SystemExit(f"missing anchor in {path}: {old[:120]!r}")
    if s.count(old) != 1:
        raise SystemExit(f"non-unique anchor in {path}: {s.count(old)}")
    p.write_text(s.replace(old, new, 1))


# Run, not a concrete LLM implementation, is the final authority on plan
# legality. This also makes custom/decorator strategists safe by construction.
replace_once("agent/plan.go",
'''func strategizeWithRetries(log io.Writer, round int, p StrategicPlanner, obs Observation, offered []Objective, reason string) (Plan, error, int) {
\tplan, err := p.Strategize(obs, offered, reason)
\tfp, canRetry := p.(StrategicFeedbackPlanner)
\tretries := 0
\tfor err != nil && !errors.Is(err, ErrDone) && canRetry && retries < MaxReplyRetries-1 {''',
'''func strategizeWithRetries(log io.Writer, round int, p StrategicPlanner, obs Observation, offered []Objective, reason string) (Plan, error, int) {
\tplan, err := p.Strategize(obs, offered, reason)
\tif err == nil {
\t\tplan, err = validateStrategicPlan(plan, offered, round)
\t}
\tfp, canRetry := p.(StrategicFeedbackPlanner)
\tretries := 0
\tfor err != nil && !errors.Is(err, ErrDone) && canRetry && retries < MaxReplyRetries-1 {''')
replace_once("agent/plan.go",
'''\t\tretries++
\t\tif log != nil {
\t\t\tfmt.Fprintf(log, "round %d: strategist reply rejected (ask %d of %d): %v; re-ask differs by %s\\n",
\t\t\t\tround, retries+1, MaxReplyRetries, err, r.describe())
\t\t}
\t\tplan, err = fp.StrategizeRetry(obs, offered, reason, r)
\t}
\treturn plan, err, retries''',
'''\t\tretries++
\t\tif IsLengthTruncation(err) {
\t\t\t// Unlike the cheap chooser, strategic planning starts at 8192
\t\t\t// tokens. Make the third ask genuinely larger than the second.
\t\t\tr.MaxTokensFactor = 1 << retries
\t\t\tif r.MaxTokensFactor < 2 {
\t\t\t\tr.MaxTokensFactor = 2
\t\t\t}
\t\t}
\t\tif log != nil {
\t\t\tfmt.Fprintf(log, "round %d: strategist reply rejected (ask %d of %d): %v; re-ask differs by %s\\n",
\t\t\t\tround, retries+1, MaxReplyRetries, err, r.describe())
\t\t}
\t\tplan, err = fp.StrategizeRetry(obs, offered, reason, r)
\t\tif err == nil {
\t\t\tplan, err = validateStrategicPlan(plan, offered, round)
\t\t}
\t}
\treturn plan, err, retries''')

# A newly learned concrete wall is more informative than a generic failure
# already queued from the objective that exposed it.
replace_once("agent/plan.go",
'''func (r *runPlanning) request(reason string) {
\tif reason == "" || r.pending != "" {
\t\treturn
\t}
\tr.pending = reason
}''',
'''func (r *runPlanning) request(reason string) {
\tif reason == "" {
\t\treturn
\t}
\tif r.pending == "" || (r.pending == "objective_failed" && reason != "objective_failed") {
\t\tr.pending = reason
\t}
}''')

# Extract recoverable failure escalation so the structured trigger policy is
# ROM-free testable and never depends on raw error strings.
insert = r'''
func recoverableFailureReplan(seen map[string]bool, obj Objective, result ObjectiveResult, blackedOut, retreated bool, consecutive, maxConsecutive int) (reason, key string, terminal bool) {
    cause := string(result.Cause)
    if cause == "" {
        cause = string(result.Outcome)
    }
    key = obj.String() + "|" + cause
    if seen[key] || consecutive > maxConsecutive {
        return "", key, true
    }
    reason = "objective_failed"
    if blackedOut {
        reason = "blackout"
    } else if retreated {
        reason = "train_retreat"
    }
    return reason, key, false
}

// replanOnce converts a watchdog edge into one strategic replan opportunity.
// The second edge before observable progress is terminal.
func replanOnce(escalated *bool) bool {
    if escalated == nil || *escalated {
        return false
    }
    *escalated = true
    return true
}

'''
text = Path("agent/plan.go").read_text()
anchor = "func (r *runPlanning) success(fromPlan bool) {"
if anchor not in text:
    raise SystemExit("missing plan helper insertion anchor")
Path("agent/plan.go").write_text(text.replace(anchor, insert + anchor, 1))

# Use the testable helpers in Run.
replace_once("agent/run.go",
'''\t\tif stagnantRounds >= stagnationAfter {
\t\t\tif planning.hasStrategist(p) {
\t\t\t\tif stagnationEscalated {
\t\t\t\t\tres.Stop = StopStuck''',
'''\t\tif stagnantRounds >= stagnationAfter {
\t\t\tif planning.hasStrategist(p) {
\t\t\t\tif !replanOnce(&stagnationEscalated) {
\t\t\t\t\tres.Stop = StopStuck''')
replace_once("agent/run.go",
'''\t\t\t\tplanning.request("stagnation")
\t\t\t\tstagnationEscalated = true
\t\t\t\tlastMajorProgressRound = completedRounds''',
'''\t\t\t\tplanning.request("stagnation")
\t\t\t\tlastMajorProgressRound = completedRounds''')
replace_once("agent/run.go",
'''\t\t\tif planning.hasStrategist(p) {
\t\t\t\tconsecFailures++
\t\t\t\tcause := string(objectiveResult.Cause)
\t\t\t\tif cause == "" {
\t\t\t\t\tcause = string(objectiveResult.Outcome)
\t\t\t\t}
\t\t\t\tkey := obj.String() + "|" + cause
\t\t\t\tif failureEscalated[key] || consecFailures > maxConsecFailures {
\t\t\t\t\tres.Stop, res.Err = StopFailed, execErr
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t\tfailureEscalated[key] = true
\t\t\t\treason := "objective_failed"
\t\t\t\tif blackedOut {
\t\t\t\t\treason = "blackout"
\t\t\t\t} else if retreated {
\t\t\t\t\treason = "train_retreat"
\t\t\t\t}
\t\t\t\tplanning.request(reason)''',
'''\t\t\tif planning.hasStrategist(p) {
\t\t\t\tconsecFailures++
\t\t\t\treason, key, terminal := recoverableFailureReplan(
\t\t\t\t\tfailureEscalated, obj, objectiveResult, blackedOut, retreated, consecFailures, maxConsecFailures,
\t\t\t\t)
\t\t\t\tif terminal {
\t\t\t\t\tres.Stop, res.Err = StopFailed, execErr
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t\tfailureEscalated[key] = true
\t\t\t\tplanning.request(reason)''')
replace_once("agent/run.go",
'''\t\tif stuck >= stuckAfter {
\t\t\tif planning.hasStrategist(p) {
\t\t\t\tif stuckEscalated {
\t\t\t\t\tres.Stop = StopStuck''',
'''\t\tif stuck >= stuckAfter {
\t\t\tif planning.hasStrategist(p) {
\t\t\t\tif !replanOnce(&stuckEscalated) {
\t\t\t\t\tres.Stop = StopStuck''')
replace_once("agent/run.go",
'''\t\t\t\tplanning.request("stuck")
\t\t\t\tstuckEscalated = true
\t\t\t\tstuck = 0''',
'''\t\t\t\tplanning.request("stuck")
\t\t\t\tstuck = 0''')

# Remove the environment-sensitive real-ROM integration; the same semantics
# are covered directly by runPlanning without relying on which places happen
# to be offered from a particular checkpoint.
Path("agent/run_plan_test.go").unlink(missing_ok=True)

# Strengthen pure tier/replan tests.
p = Path("agent/plan_test.go")
s = p.read_text()
s += r'''

func TestRunPlanningUsesCheapChooserWithoutStrategist(t *testing.T) {
    offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
    cheap := &cheapPlanningTestPlanner{}
    r := newRunPlanning(Plan{})
    obj, fromPlan, err, _ := r.choose(nil, 1, cheap, Observation{Round: 1}, offered)
    if err != nil || fromPlan || obj.String() != "go to route 1" {
        t.Fatalf("obj=%q fromPlan=%v err=%v", obj.String(), fromPlan, err)
    }
    if cheap.calls != 1 || r.Stats.FastCalls != 1 {
        t.Fatalf("cheap calls=%d stats=%+v", cheap.calls, r.Stats)
    }
}

type cheapPlanningTestPlanner struct{ calls int }

func (p *cheapPlanningTestPlanner) Next(_ Observation, offered []Objective) (Objective, error) {
    p.calls++
    return offered[0], nil
}

func TestRecoverableFailureReplansOnceThenStopsOnSameStructuredCause(t *testing.T) {
    seen := map[string]bool{}
    obj := Objective{Kind: KindGoTo, Place: "route 1"}
    result := ObjectiveResult{Outcome: OutcomeBlocked, Cause: FailureCauseID("route_prerequisite_missing")}
    reason, key, terminal := recoverableFailureReplan(seen, obj, result, false, false, 1, 3)
    if terminal || reason != "objective_failed" || key == "" {
        t.Fatalf("first = reason %q key %q terminal %v", reason, key, terminal)
    }
    seen[key] = true
    _, _, terminal = recoverableFailureReplan(seen, obj, result, false, false, 2, 3)
    if !terminal {
        t.Fatal("same objective/cause after a strategic replan did not become terminal")
    }
}

func TestRecoverableFailureNamesBlackoutAndRetreatReplans(t *testing.T) {
    obj := Objective{Kind: KindTrain, Level: 12}
    result := ObjectiveResult{Outcome: OutcomeBlocked, Cause: FailureCauseID("battle_blackout")}
    reason, _, terminal := recoverableFailureReplan(map[string]bool{}, obj, result, true, false, 1, 3)
    if terminal || reason != "blackout" {
        t.Fatalf("blackout = %q terminal=%v", reason, terminal)
    }
    reason, _, terminal = recoverableFailureReplan(map[string]bool{}, obj, result, false, true, 1, 3)
    if terminal || reason != "train_retreat" {
        t.Fatalf("retreat = %q terminal=%v", reason, terminal)
    }
}

func TestReplanOnceStopsSecondWatchdogEdgeUntilProgressReset(t *testing.T) {
    escalated := false
    if !replanOnce(&escalated) {
        t.Fatal("first watchdog edge did not allow replan")
    }
    if replanOnce(&escalated) {
        t.Fatal("second watchdog edge allowed another replan without progress")
    }
    escalated = false
    if !replanOnce(&escalated) {
        t.Fatal("progress reset did not restore one replan opportunity")
    }
}

func TestRunPlanningValidatesCustomStrategistOutput(t *testing.T) {
    offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
    p := &rawStrategist{plan: Plan{Goal: "cheat", Steps: []string{"1"}}}
    r := newRunPlanning(Plan{})
    if _, _, err, _ := r.choose(nil, 1, p, Observation{Round: 1}, offered); err == nil {
        t.Fatal("Run accepted a custom strategist plan made of a menu index")
    }
}

type rawStrategist struct{ plan Plan }
func (p *rawStrategist) Next(_ Observation, offered []Objective) (Objective, error) { return offered[0], nil }
func (p *rawStrategist) Strategize(_ Observation, _ []Objective, _ string) (Plan, error) { return p.plan, nil }
'''
p.write_text(s)

# HTTP-level strategist request test: NoThink on the cheap planner must not
# leak into the strategy call; it also gets the large default token budget.
Path("agent/llm_plan_request_test.go").write_text(r'''package agent

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "testing"
)

type strategicRoundTrip func(*http.Request) (*http.Response, error)
func (f strategicRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStrategistUsesThinkingModeAndLargeBudget(t *testing.T) {
    var request map[string]any
    client := &http.Client{Transport: strategicRoundTrip(func(r *http.Request) (*http.Response, error) {
        data, err := io.ReadAll(r.Body)
        if err != nil { t.Fatal(err) }
        if err := json.Unmarshal(data, &request); err != nil { t.Fatal(err) }
        body := `{"model":"test-model","choices":[{"message":{"content":"{\\"goal\\":\\"go north\\",\\"steps\\":[\\"go to route 1\\"]}"},"finish_reason":"stop"}]}`
        return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}, nil
    })}
    p := &LLMPlanner{BaseURL: "http://unused", Model: "test-model", Client: client, NoThink: true, MaxTokens: 512}
    offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
    if _, err := p.Strategize(Observation{Round: 3}, offered, "initial"); err != nil {
        t.Fatalf("Strategize: %v", err)
    }
    if got := int(request["max_tokens"].(float64)); got < strategicReplyTokens {
        t.Fatalf("max_tokens=%d, want >=%d", got, strategicReplyTokens)
    }
    if _, present := request["chat_template_kwargs"]; present {
        t.Fatalf("strategist inherited cheap NoThink request: %+v", request["chat_template_kwargs"])
    }
    rf := request["response_format"].(map[string]any)
    schema := rf["json_schema"].(map[string]any)
    if schema["name"] != "objective_plan" {
        t.Fatalf("schema name=%v, want objective_plan", schema["name"])
    }
}
''')

# Make the watch plan position human-oriented (1/N while active).
replace_once("emu/watch.go",
'''row('plan', s.plan_goal ? ((s.plan_step || 0) + '/' + (s.plan_steps ? s.plan_steps.length : 0) + ' ' + s.plan_goal) : 'none')''',
'''row('plan', s.plan_goal ? (Math.min((s.plan_step || 0) + 1, (s.plan_steps ? s.plan_steps.length : 0)) + '/' + (s.plan_steps ? s.plan_steps.length : 0) + ' ' + s.plan_goal) : 'none')''')
