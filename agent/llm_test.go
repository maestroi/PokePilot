package agent_test

import (
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

// TestLLMPlannerStripsProse: the model wraps the number in prose; the
// answer still wins.
func TestLLMPlannerStripsProse(t *testing.T) {
	srv := startModelServer(t, `{"choices":[{"message":{"content":"I choose 2 because it is closer."}}]}`, nil)
	offered := llmOffered()

	got, err := llmPlanner(srv).Next(llmObs(), offered)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got != offered[1] {
		t.Fatalf("Next = %s, want %s", got, offered[1])
	}
}

// TestLLMPlannerAnswerWinsOverScratchWork: a model that reasons out loud
// mentions numbers it then rejects. The ANSWER is the last integer, not
// the first — taking the first silently picks a rejected option, which is
// a legal objective and therefore an invisible wrong choice.
func TestLLMPlannerAnswerWinsOverScratchWork(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply string
		want  int // index into offered
	}{
		{"weighs two options", "Option 1 is tempting, but 3 is better.", 2},
		{"thinks out loud first", "<think>maybe 1? no, 2 is closer</think>\n3", 2},
		{"bare number", "2", 1},
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
		{"unknown species", `{"choice":5,"species":"snorlax"}`, "unknown species"},
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
// POKEPILOT_LLM_URL at a non-schema server does not break the run.
func TestLLMPlannerIgnoresSchemaFallback(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply string
		want  int // index into offered
	}{
		{"bare number", "2", 1},
		{"prose around the number", "I choose 2 because it is closer.", 1},
		{"number after rejected options", "Option 1 is tempting, but 3 is better.", 2},
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
	// arrive escaped: assert on the escaped form.
	for _, want := range []string{
		`\"LeadMoves\"`,
		`\"Power\": 35`,
		`\"Type\": \"normal\"`,
		`\"Bag\"`,
		`pokeball`,
		`\"RecentDialogue\"`,
		`\"History\"`,
		`take the charmander starter`,
		`\"Objective\": \"take the charmander starter\"`,
		`\"Outcome\": \"done\"`,
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

// TestLLMPlannerPromptCarriesHistory: the planner is otherwise a pure
// function of the observation, so at temperature 0 an objective that
// returns the player to a place they have been loops forever (measured:
// oak's lab -> pallet town -> oak's lab for 21 rounds). The second call
// must show the model what the first one chose.
func TestLLMPlannerPromptCarriesHistory(t *testing.T) {
	var body string
	srv := startModelServer(t, `{"choices":[{"message":{"content":"1"}}]}`, &body)
	offered := llmOffered()
	p := llmPlanner(srv)

	if _, err := p.Next(llmObs(), offered); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if strings.Contains(body, "Already done this run") {
		t.Fatalf("first prompt claims history before anything was chosen\nbody: %s", body)
	}
	if _, err := p.Next(llmObs(), offered); err != nil {
		t.Fatalf("Next: %v", err)
	}
	for _, want := range []string{"Already done this run", "- go to pallet town"} {
		if !strings.Contains(body, want) {
			t.Errorf("second prompt does not contain %q\nbody: %s", want, body)
		}
	}
}
