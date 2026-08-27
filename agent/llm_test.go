package agent_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

// The LLM tests stand up an httptest server and point BaseURL at it: no
// ROM, no inference server, no network beyond loopback. They must run
// with POKEMON_RED_ROM unset and with nothing listening on the default
// model URL.

func llmObs() agent.Observation {
	return agent.Observation{
		Map:          0x28,
		MapName:      "OAKS_LAB",
		X:            5,
		Y:            6,
		Facing:       "down",
		Controllable: true,
		PartyCount:   1,
		Party:        []agent.PartyMon{{Species: 1, Level: 5, HP: 20, MaxHP: 20}},
		Badges:       []string{},
		Money:        3000,
		Events:       []string{"got a starter"},
	}
}

// llmOffered deliberately does not start with a bare KindStarter: the
// zero Objective is equal to {Kind: KindStarter}, and the out-of-range
// test must be able to tell "no objective" apart from offered[0].
func llmOffered() []agent.Objective {
	return []agent.Objective{
		{Kind: agent.KindGoTo, Place: "pallet town"},
		{Kind: agent.KindStarter},
		{Kind: agent.KindTalk, X: 3, Y: 1},
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
		"2: take a starter",
		"3: talk at (3,1)",
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
