package agent_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/skill"
)

// The LLM tests stand up an httptest server and point BaseURL at it: no
// ROM, no inference server, no network beyond loopback. They must run
// with POKEMON_RED_ROM unset and with nothing listening on the default
// model URL.

func llmObs() agent.Observation {
	return agent.Observation{
		Map:            0x28,
		MapName:        "OAKS_LAB",
		X:              5,
		Y:              6,
		Facing:         "down",
		Controllable:   true,
		PartyCount:     1,
		Party:          []agent.PartyMon{{Species: 1, Level: 5, HP: 20, MaxHP: 20}},
		Badges:         []string{},
		Money:          3000,
		Events:         []string{"got a starter"},
		LeadMoves:      []agent.Move{{Power: 35, Type: "normal"}, {Power: 0, Type: "normal"}},
		Bag:            []agent.Item{{Name: "pokeball", Quantity: 5}},
		RecentDialogue: []string{"OAK: Oh! You're awake! It's been a while, huh?"},
		History:        []agent.RoundRecord{{Objective: "take the charmander starter", Outcome: "done"}},
	}
}

// llmOffered deliberately does not start with a bare KindStarter: the
// zero Objective is equal to {Kind: KindStarter}, and the out-of-range
// test must be able to tell "no objective" apart from offered[0]. It also
// carries one argument per kind (a starter, a level, a species) so the
// reply-argument tests have something to aim at.
func llmOffered() []agent.Objective {
	return []agent.Objective{
		{Kind: agent.KindGoTo, Place: "pallet town"},
		{Kind: agent.KindStarter, Starter: skill.StarterCharmander},
		{Kind: agent.KindTrain, Level: 10},
		{Kind: agent.KindTalk, X: 3, Y: 1},
		{Kind: agent.KindCatch, Species: 0x7B}, // CATERPIE
	}
}

// startModelServer stands up an OpenAI-compatible chat endpoint that
// always answers with reply. It fails the test on the wrong path or
// Content-Type, and stores the raw request body in *capture when
// capture is non-nil.
func startModelServer(t *testing.T, reply string, capture *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("POST %s, want /chat/completions", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type %q, want application/json", ct)
		}
		if capture != nil {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
			} else {
				*capture = string(body)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func llmPlanner(srv *httptest.Server) *agent.LLMPlanner {
	return &agent.LLMPlanner{BaseURL: srv.URL, Model: "qwen3.8-27b"}
}

// TestLLMPlannerPicksOfferedObjective is the happy path: the model
// answers "2" and the second offered objective comes back. It also
// asserts the request body carried the model name and the numbered
// list, because that body is the prompt a human will tune.
func TestLLMPlannerPicksOfferedObjective(t *testing.T) {
	var body string
	srv := startModelServer(t, `{"choices":[{"message":{"content":"2"}}]}`, &body)
	offered := llmOffered()

	got, err := llmPlanner(srv).Next(llmObs(), offered)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got != offered[1] {
		t.Fatalf("Next = %s, want %s", got, offered[1])
	}
	for _, want := range []string{
		`"model":"qwen3.8-27b"`,
		`"temperature":0`,
		`"role":"system"`,
		`"role":"user"`,
		"1: go to pallet town",
		"2: take the charmander starter",
		"3: train the lead to level 10",
		"4: talk at (3,1)",
		"5: catch a CATERPIE here",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("request body does not contain %q\nbody: %s", want, body)
		}
	}
}

// TestLLMPlannerProseWithDigitIsRejected: an HTTP-200 body of prose that
// merely contains a digit is an unhealthy response, not a choice. Under
// the old fallback "rate limited, retry in 5 seconds" became choice 5 —
// a server error turning straight into a game action. The tightened rule
// (short, substantially just the number) rejects it.
func TestLLMPlannerProseWithDigitIsRejected(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply string
	}{
		{"rate limit message", "rate limited, retry in 5 seconds"},
		{"apology with a number", "I choose 2 because it is closer."},
		{"weighs two options out loud", "Option 1 is tempting, but 3 is better."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := startModelServer(t, fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, tc.reply), nil)

			got, err := llmPlanner(srv).Next(llmObs(), llmOffered())
			if err == nil {
				t.Fatalf("Next = %s, want an error for prose reply %q", got, tc.reply)
			}
			if got != (agent.Objective{}) {
				t.Fatalf("Next = %s, must not resolve to any objective", got)
			}
		})
	}
}

// TestLLMPlannerAnswerWinsOverScratchWork: a model that reasons out loud
// in a CLOSED block then answers with the bare number still works — the
// scratch work is stripped before the fallback gate, leaving exactly the
// answer. (Prose outside the block is rejected; see
// TestLLMPlannerProseWithDigitIsRejected.)
func TestLLMPlannerAnswerWinsOverScratchWork(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply string
		want  int // index into offered
	}{
		{"thinks out loud first", "<think>maybe 1? no, 2 is closer</think>\n3", 2},
		{"bare number", "2", 1},
		{"number with a trailing period", "2.", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, tc.reply)
			srv := startModelServer(t, body, nil)
			offered := llmOffered()

			got, err := llmPlanner(srv).Next(llmObs(), offered)
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			if got != offered[tc.want] {
				t.Fatalf("Next = %s, want %s", got, offered[tc.want])
			}
		})
	}
}

// TestLLMPlannerTruncatedThoughtIsError: an unclosed reasoning block
// means the reply was cut off before the answer. Every number in it is
// scratch work, so there is nothing to pick, and picking one anyway is
// precisely the silent wrong choice this parsing exists to prevent.
func TestLLMPlannerTruncatedThoughtIsError(t *testing.T) {
	srv := startModelServer(t, `{"choices":[{"message":{"content":"<think>1 looks wrong, 3 it is"}}]}`, nil)
	offered := llmOffered()

	got, err := llmPlanner(srv).Next(llmObs(), offered)
	if err == nil {
		t.Fatalf("Next = %s, want an error for a truncated reply", got)
	}
	if got == offered[0] {
		t.Fatalf("Next = %s, must not fall back to the first offered", got)
	}
}

// TestLLMPlannerOutOfRangeIsError: "7" with three offered is an error,
// and the result is NOT offered[0] — no silent fallback.
func TestLLMPlannerOutOfRangeIsError(t *testing.T) {
	srv := startModelServer(t, `{"choices":[{"message":{"content":"7"}}]}`, nil)
	offered := llmOffered()

	got, err := llmPlanner(srv).Next(llmObs(), offered)
	if err == nil {
		t.Fatalf("Next = %s, want error for index 7 of 3", got)
	}
	if got == offered[0] {
		t.Fatalf("Next = %s, must not fall back to the first offered", got)
	}
}

// TestLLMPlannerHallucinationIsError: an objective name that was never
// offered is an error carrying the raw reply, not a guess.
func TestLLMPlannerHallucinationIsError(t *testing.T) {
	srv := startModelServer(t, `{"choices":[{"message":{"content":"go to viridian city"}}]}`, nil)

	_, err := llmPlanner(srv).Next(llmObs(), llmOffered())
	if err == nil {
		t.Fatal("Next = objective, want error for a hallucinated objective")
	}
	if !strings.Contains(err.Error(), "go to viridian city") {
		t.Errorf("error does not carry the raw reply: %v", err)
	}
}

// TestLLMPlannerHTTPError: a non-200 response is an error naming the
// status.
func TestLLMPlannerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	_, err := llmPlanner(srv).Next(llmObs(), llmOffered())
	if err == nil {
		t.Fatal("Next = objective, want error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error does not mention the status: %v", err)
	}
}

// TestLLMPlannerBadJSON: a body that will not parse is an error.
func TestLLMPlannerBadJSON(t *testing.T) {
	srv := startModelServer(t, `{"choices": [`, nil)

	_, err := llmPlanner(srv).Next(llmObs(), llmOffered())
	if err == nil {
		t.Fatal("Next = objective, want error for malformed JSON")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "json") {
		t.Errorf("error does not name the JSON failure: %v", err)
	}
}

// TestLLMPlannerEmptyChoices: an empty choices array is an error.
func TestLLMPlannerEmptyChoices(t *testing.T) {
	srv := startModelServer(t, `{"choices":[]}`, nil)

	_, err := llmPlanner(srv).Next(llmObs(), llmOffered())
	if err == nil {
		t.Fatal("Next = objective, want error for an empty choices array")
	}
	if !strings.Contains(err.Error(), "choices") {
		t.Errorf("error does not name the empty choices: %v", err)
	}
}

// TestNewLLMPlannerEnv: POKEPILOT_LLM_URL and POKEPILOT_LLM_MODEL
// override the defaults.
func TestNewLLMPlannerEnv(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_URL", "http://example.com/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "some-model")

	p := agent.NewLLMPlanner()
	if p.BaseURL != "http://example.com/v1" {
		t.Errorf("BaseURL = %q, want the POKEPILOT_LLM_URL value", p.BaseURL)
	}
	if p.Model != "some-model" {
		t.Errorf("Model = %q, want the POKEPILOT_LLM_MODEL value", p.Model)
	}
}

// TestLLMPlannerSendsSchema: the request asks for a json_schema reply
// shape, so a well-behaved server cannot emit a bare number in prose.
func TestLLMPlannerSendsSchema(t *testing.T) {
	var body string
	srv := startModelServer(t, `{"choices":[{"message":{"content":"1"}}]}`, &body)

	if _, err := llmPlanner(srv).Next(llmObs(), llmOffered()); err != nil {
		t.Fatalf("Next: %v", err)
	}
	for _, want := range []string{
		`"response_format"`,
		`"type":"json_schema"`,
		`"choice"`,
		`"required":["choice"]`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("request body does not contain %q\nbody: %s", want, body)
		}
	}
}

// TestLLMPlannerSchemaReplyWithArgs: a schema-shaped reply carries the
// choice AND the argument; the argument overrides the offered objective's
// default and comes back on the returned objective.
func TestLLMPlannerSchemaReplyWithArgs(t *testing.T) {
	srv := startModelServer(t, `{"choices":[{"message":{"content":"{\"choice\":3,\"level\":12}"}}]}`, nil)
	offered := llmOffered()

	got, err := llmPlanner(srv).Next(llmObs(), offered)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := agent.Objective{Kind: agent.KindTrain, Level: 12}
	if got != want {
		t.Fatalf("Next = %s, want %s (level override applied)", got, want)
	}
}

// TestLLMPlannerSchemaReplyBareChoice: a schema-shaped reply with only the
// choice selects the offered objective unchanged, argument and all.
func TestLLMPlannerSchemaReplyBareChoice(t *testing.T) {
	srv := startModelServer(t, `{"choices":[{"message":{"content":"{\"choice\":5}"}}]}`, nil)
	offered := llmOffered()

	got, err := llmPlanner(srv).Next(llmObs(), offered)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got != offered[4] {
		t.Fatalf("Next = %s, want %s", got, offered[4])
	}
}

// TestLLMPlannerOutOfRangeArgRejected: an argument outside its stated
// range is REJECTED with a typed error — never clamped to 100, never
// best-matched. This is the safety mechanism; the schema only makes such
// replies less likely, so this must hold regardless of it.
func TestLLMPlannerOutOfRangeArgRejected(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply string
		want  string
	}{
		{"level above range", `{"choice":3,"level":500}`, "out of range"},
		{"level zero", `{"choice":3,"level":0}`, "out of range"},
		{"unknown species", `{"choice":5,"species":"mewthree"}`, "unknown species"},
		{"quantity above range", `{"choice":5,"quantity":150}`, "does not apply"},
		{"choice out of range", `{"choice":7}`, "out of range"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, tc.reply)
			srv := startModelServer(t, body, nil)

			got, err := llmPlanner(srv).Next(llmObs(), llmOffered())
			if err == nil {
				t.Fatalf("Next = %s, want an error for %s", got, tc.reply)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not name the problem (%q): %v", tc.want, err)
			}
		})
	}
}

// TestLLMPlannerIgnoresSchemaFallback: a server that ignores
// response_format answers in plain text; the existing parsing path must
// still yield a valid objective. This is the guarantee that pointing
// POKEPILOT_LLM_URL at a non-schema server does not break the run. The
// fallback is now gated (looksLikeAnswer), so only replies that
// substantially ARE the number get through.
func TestLLMPlannerIgnoresSchemaFallback(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply string
		want  int // index into offered
	}{
		{"bare number", "2", 1},
		{"padded number", "  3 ", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := startModelServer(t, fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, tc.reply), nil)
			offered := llmOffered()

			got, err := llmPlanner(srv).Next(llmObs(), offered)
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			if got != offered[tc.want] {
				t.Fatalf("Next = %s, want %s", got, offered[tc.want])
			}
		})
	}
}

// TestLLMPlannerModelMismatchIsRejected: the reply's model field names a
// DIFFERENT model than the one requested. This is the test ablation A's
// validity rests on: if the server ignores the model field or has one
// model loaded, the ablation compares a model to itself and reports "not
// capacity" — a false negative on the central experiment. The error must
// name BOTH models.
func TestLLMPlannerModelMismatchIsRejected(t *testing.T) {
	srv := startModelServer(t,
		`{"model":"qwen3.5-4b","choices":[{"message":{"content":"2"}}]}`, nil)

	got, err := llmPlanner(srv).Next(llmObs(), llmOffered())
	if err == nil {
		t.Fatalf("Next = %s, want a model-mismatch error", got)
	}
	for _, want := range []string{"qwen3.8-27b", "qwen3.5-4b"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %v", want, err)
		}
	}
	if got != (agent.Objective{}) {
		t.Fatalf("Next = %s, must not resolve to any objective", got)
	}
}

// TestLLMPlannerMissingModelIsAccepted: some OpenAI-compatible servers
// omit the model field entirely. That is NOT an error — failing there
// would break working setups — but it means which model answered is
// unverified, so the planner says so once in the run log.
func TestLLMPlannerMissingModelIsAccepted(t *testing.T) {
	var logBuf bytes.Buffer
	srv := startModelServer(t, `{"choices":[{"message":{"content":"2"}}]}`, nil)
	p := llmPlanner(srv)
	p.Log = &logBuf
	offered := llmOffered()

	got, err := p.Next(llmObs(), offered)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got != offered[1] {
		t.Fatalf("Next = %s, want %s", got, offered[1])
	}
	// A second call must not repeat the line.
	if _, err := p.Next(llmObs(), offered); err != nil {
		t.Fatalf("second Next: %v", err)
	}
	if n := strings.Count(logBuf.String(), "did not report a model"); n != 1 {
		t.Errorf("run log mentions the omitted model %d times, want exactly once:\n%s", n, logBuf.String())
	}
}

// TestLLMPlannerLogReportsUsage catches silent prompt growth: llama.cpp's
// OpenAI-compatible envelope reports input and output token counts, and the
// per-call log must expose both beside the latency.
func TestLLMPlannerLogReportsUsage(t *testing.T) {
	var logBuf bytes.Buffer
	srv := startModelServer(t,
		`{"model":"qwen3.8-27b","choices":[{"message":{"content":"{\"choice\":1}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":321,"completion_tokens":7,"total_tokens":328}}`, nil)
	p := llmPlanner(srv)
	p.Log = &logBuf

	if _, err := p.Next(llmObs(), llmOffered()); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if want := "tokens 321 prompt/7 completion"; !strings.Contains(logBuf.String(), want) {
		t.Fatalf("run log does not contain %q:\n%s", want, logBuf.String())
	}
}

// TestLLMPlannerFinishReason: finish_reason "length" means the reply was
// cut off — a truncated JSON that still parses would be a silent wrong
// answer. Any non-stop reason is a rejection; "stop" (and an omitted
// field) are accepted.
func TestLLMPlannerFinishReason(t *testing.T) {
	for _, tc := range []struct {
		name      string
		reason    string // empty means the field is absent
		wantErr   bool
		wantIndex int // valid only when !wantErr
	}{
		{"stop with bare number", "stop", false, 1},
		{"stop with valid JSON", "stop", false, 2},
		{"length rejects even a parseable reply", "length", true, 0},
		{"content_filter rejects", "content_filter", true, 0},
		{"omitted finish_reason accepted", "", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := "2"
			if tc.wantIndex == 2 {
				content = `{"choice":3,"level":12}`
			}
			body := fmt.Sprintf(`{"choices":[{"message":{"content":%q}`, content)
			if tc.reason != "" {
				body += `,"finish_reason":"` + tc.reason + `"`
			}
			body += `}]}`
			srv := startModelServer(t, body, nil)

			got, err := llmPlanner(srv).Next(llmObs(), llmOffered())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Next = %s, want a rejection for finish_reason %q", got, tc.reason)
				}
				if !strings.Contains(err.Error(), tc.reason) {
					t.Errorf("error does not name the finish_reason: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			if tc.wantIndex == 2 {
				want := agent.Objective{Kind: agent.KindTrain, Level: 12}
				if got != want {
					t.Fatalf("Next = %s, want %s", got, want)
				}
				return
			}
			if got != llmOffered()[tc.wantIndex] {
				t.Fatalf("Next = %s, want offered[%d]", got, tc.wantIndex)
			}
		})
	}
}

// TestLLMPlannerHealthCounts: the per-run counters that keep a bad sweep
// from looking like a bad model. One transport error (HTTP 500), one
// reply rejected for shape (model mismatch), one fallback parse — each
// lands in its own bucket.
func TestLLMPlannerHealthCounts(t *testing.T) {
	srv500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	t.Cleanup(srv500.Close)
	planner := llmPlanner(srv500)
	if _, err := planner.Next(llmObs(), llmOffered()); err == nil {
		t.Fatal("Next: want a transport error")
	}
	// model mismatch (rejected for shape)
	srvMismatch := startModelServer(t, `{"model":"other-model","choices":[{"message":{"content":"2"}}]}`, nil)
	planner.BaseURL = srvMismatch.URL
	if _, err := planner.Next(llmObs(), llmOffered()); err == nil {
		t.Fatal("Next: want a model-mismatch error")
	}
	// bare-number fallback (accepted, counted as a fallback use)
	srvPlain := startModelServer(t, `{"choices":[{"message":{"content":"2"}}]}`, nil)
	planner.BaseURL = srvPlain.URL
	if _, err := planner.Next(llmObs(), llmOffered()); err != nil {
		t.Fatalf("Next: %v", err)
	}
	h := planner.Health
	if h.Transport != 1 || h.Rejected != 1 || h.Fallbacks != 1 {
		t.Errorf("Health = %+v, want {Transport:1 Rejected:1 Fallbacks:1}", h)
	}
}

// TestLLMPlannerPromptCarriesMovesAndHistory: the fields that make
// "fight Brock now or train first?" answerable must actually reach the
// prompt. A field added to Observation but never rendered into it changes
// nothing for the model, so this asserts on the request body, not the
// struct.
func TestLLMPlannerPromptCarriesMovesAndHistory(t *testing.T) {
	var body string
	srv := startModelServer(t, `{"choices":[{"message":{"content":"1"}}]}`, &body)

	if _, err := llmPlanner(srv).Next(llmObs(), llmOffered()); err != nil {
		t.Fatalf("Next: %v", err)
	}
	// The observation JSON is embedded in a chat message, so its quotes
	// arrive escaped: assert on the escaped form. It is marshalled COMPACT
	// (no space after the colon) — indentation is prompt cost the model
	// gains nothing from.
	for _, want := range []string{
		`\"LeadMoves\"`,
		`\"Power\":35`,
		`\"Type\":\"normal\"`,
		`\"Bag\"`,
		`pokeball`,
		`\"RecentDialogue\"`,
		`\"History\"`,
		`take the charmander starter`,
		`\"Objective\":\"take the charmander starter\"`,
		`\"Outcome\":\"done\"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("request body does not contain %q", want)
		}
	}
}

// TestLLMPlannerGoalInSystemPrompt: with Goal set, the task statement
// reaches the model as a line above everything else in the system prompt.
func TestLLMPlannerGoalInSystemPrompt(t *testing.T) {
	var body string
	srv := startModelServer(t, `{"choices":[{"message":{"content":"1"}}]}`, &body)
	p := llmPlanner(srv)
	p.Goal = "Earn the Boulder Badge."

	if _, err := p.Next(llmObs(), llmOffered()); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !strings.Contains(body, `Your goal: Earn the Boulder Badge.`) {
		t.Errorf("system prompt does not carry the goal\nbody: %s", body)
	}
}

// TestLLMPlannerGoalDoesNotChangeParsing: a goal is not an argument. The
// same reply and the same offered list must resolve to the same objective
// with or without a Goal — the goal changes what the model is told, never
// how its reply is read.
func TestLLMPlannerGoalDoesNotChangeParsing(t *testing.T) {
	for _, goal := range []string{"", "Earn the Boulder Badge."} {
		srv := startModelServer(t, `{"choices":[{"message":{"content":"{\"choice\":3,\"level\":12}"}}]}`, nil)
		p := llmPlanner(srv)
		p.Goal = goal

		got, err := p.Next(llmObs(), llmOffered())
		if err != nil {
			t.Fatalf("Next (goal %q): %v", goal, err)
		}
		want := agent.Objective{Kind: agent.KindTrain, Level: 12}
		if got != want {
			t.Fatalf("Next (goal %q) = %s, want %s", goal, got, want)
		}
	}
}

// TestLLMPlannerPromptUsesObservationHistoryOnly catches duplicated run
// memory: Observation.History already carries objectives and their outcomes,
// including failure text. Repeating objective-only strings under a second
// "Already done" section spends prompt tokens while discarding that outcome.
func TestLLMPlannerPromptUsesObservationHistoryOnly(t *testing.T) {
	var body string
	srv := startModelServer(t, `{"choices":[{"message":{"content":"1"}}]}`, &body)
	offered := llmOffered()
	p := llmPlanner(srv)

	for call := 1; call <= 2; call++ {
		if _, err := p.Next(llmObs(), offered); err != nil {
			t.Fatalf("Next call %d: %v", call, err)
		}
		if strings.Contains(body, "Already done this run") {
			t.Fatalf("call %d duplicated observation history under an Already done section\nbody: %s", call, body)
		}
		for _, want := range []string{`\"History\"`, `\"Outcome\":\"done\"`} {
			if !strings.Contains(body, want) {
				t.Errorf("call %d prompt does not contain observation history field %q\nbody: %s", call, want, body)
			}
		}
	}
}

// TestLLMPlannerRejectionCarriedIntoReprompt is the feedback half of S7-4:
// a reply whose argument does not apply to the chosen objective is
// rejected, and the re-ask (NextFeedback) must carry the rejection text
// into the next prompt. A retry that re-asks the identical question
// teaches the model nothing and just burns the budget three times as fast;
// quoting the rejection back is what makes the retry a correction instead
// of a repeat. The strict rejection itself must survive: the first reply
// is still an error, never coerced into the bare choice.
func TestLLMPlannerRejectionCarriedIntoReprompt(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		switch len(bodies) {
		case 1:
			// level does not apply to the first offered objective (go to).
			fmt.Fprint(w, `{"model":"qwen3.8-27b","choices":[{"message":{"role":"assistant","content":"{\"choice\":1,\"level\":12}"}, "finish_reason":"stop"}]}`)
		default:
			fmt.Fprint(w, `{"model":"qwen3.8-27b","choices":[{"message":{"role":"assistant","content":"{\"choice\":1}"}, "finish_reason":"stop"}]}`)
		}
	}))
	t.Cleanup(srv.Close)
	p := llmPlanner(srv)
	offered := llmOffered()

	// First ask: the superfluous argument is rejected, not coerced away.
	_, err := p.Next(llmObs(), offered)
	if err == nil {
		t.Fatalf("Next accepted {\"choice\":1,\"level\":12} for a go-to objective; the strict rejection must survive")
	}
	const want = "level argument 12 does not apply to go to pallet town"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Err = %v, want the rejection naming %q", err, want)
	}

	// Re-ask with the rejection quoted back: the model corrects it.
	got, err := p.NextFeedback(llmObs(), offered, err.Error())
	if err != nil {
		t.Fatalf("NextFeedback: %v", err)
	}
	if got.Kind != agent.KindGoTo || got.Place != "pallet town" {
		t.Fatalf("NextFeedback = %s, want the corrected bare choice", got)
	}

	// The rejection text is in the SECOND prompt and only there.
	if len(bodies) != 2 {
		t.Fatalf("%d requests, want 2 (initial + one re-ask)", len(bodies))
	}
	if strings.Contains(bodies[0], "rejected") {
		t.Errorf("first prompt already carries rejection feedback:\n%s", bodies[0])
	}
	for _, want := range []string{"was rejected", want} {
		if !strings.Contains(bodies[1], want) {
			t.Errorf("second prompt does not contain %q\nbody: %s", want, bodies[1])
		}
	}
	if p.Health.Rejected != 1 {
		t.Errorf("Health.Rejected = %d, want 1 (the first reply was rejected)", p.Health.Rejected)
	}
}
