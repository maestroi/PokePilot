package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLLMBaseURL = "http://192.168.50.204:8000/v1"
	defaultLLMModel   = "qwen3.5-4b"
)

// LLMPlanner asks an OpenAI-compatible chat endpoint to choose one of
// the offered objectives. It can only ever return an objective that was
// offered — see Chosen.
type LLMPlanner struct {
	BaseURL string       // default http://192.168.50.204:8000/v1
	Model   string       // default qwen3.5-4b
	Token   string       // bearer token; empty means no Authorization header
	Client  *http.Client // nil means a client with a sane timeout
	Log     io.Writer    // one line per call; nil means no logging

	// PromptLog, when set, receives every prompt verbatim (system and user
	// messages) before it is sent. It exists for the record: badgerun keeps
	// it so a scored run can show exactly what the model was told.
	PromptLog io.Writer

	// Goal is the task statement the run is trying to achieve — for
	// example "Earn the Boulder Badge.". It is a GOAL, not a solution:
	// it names the task and nothing else. It must never say which starter
	// to take, which Pokemon to catch, or which type beats which — that is
	// the answer the experiment measures whether the model derives on its
	// own. When non-empty it is rendered into the system prompt above
	// everything else; when empty the prompt is byte-identical to the one
	// without a Goal, so prior measurements stay comparable. badgerun sets
	// it from -goal; it is a run parameter, not a constant.
	Goal string

	// ExtraSystem is appended to the system prompt. Empty by default. It is
	// the seam badgerun's -inject-fact diagnostic uses (one injected fact,
	// default off): the fact being injected is the thing being measured, so
	// this must never be on in a baseline measurement.
	ExtraSystem string

	// Timeout bounds one POST to the model. Zero means the 60s default. A
	// large model (ablation A) can take well over a minute per call, so
	// badgerun raises it via POKEPILOT_LLM_TIMEOUT rather than editing code.
	Timeout time.Duration

	// recent is what this planner has already chosen, oldest first. It
	// exists because Next is otherwise a pure function of the
	// observation: at temperature 0 the same observation yields the same
	// choice forever, so any objective that returns the player to a
	// place they have been is an infinite loop by construction (measured:
	// lab -> pallet -> lab for 21 rounds). The history breaks the tie.
	recent []string
}

// recentCap is how many past choices the prompt carries: enough to show
// a two- or three-step oscillation, short enough not to crowd out the
// observation.
const recentCap = 6

// NewLLMPlanner returns an LLMPlanner with the defaults, overridden by
// POKEPILOT_LLM_URL and POKEPILOT_LLM_MODEL when set. The bearer
// token comes from llm_token (the name used in .env).
func NewLLMPlanner() *LLMPlanner {
	p := &LLMPlanner{BaseURL: defaultLLMBaseURL, Model: defaultLLMModel}
	if v := os.Getenv("POKEPILOT_LLM_URL"); v != "" {
		p.BaseURL = v
	}
	if v := os.Getenv("POKEPILOT_LLM_MODEL"); v != "" {
		p.Model = v
	}
	p.Token = os.Getenv("llm_token")
	if v := os.Getenv("POKEPILOT_LLM_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			p.Timeout = d
		}
	}
	return p
}

// Next posts the observation and the offered objectives to the model and
// returns the offered objective the model picked. It never guesses: a
// reply that does not resolve to one of the offered objectives is an
// error, so the run loop can stop on it instead of acting on a wrong
// answer.
//
// The reply is asked for as a JSON object ({"choice": N, plus the
// arguments of the chosen objective when it has any) via
// response_format/json_schema, so a well-behaved server cannot emit a
// bare number wrapped in prose. A server that rejects or ignores the
// schema still works: a reply without a parseable "choice" falls back to
// the existing text path (last integer wins). The schema is an
// optimisation, not the safety mechanism — WithArgs and Validate check
// every value against its stated range either way.
func (p *LLMPlanner) Next(obs Observation, offered []Objective) (Objective, error) {
	if len(offered) == 0 {
		return Objective{}, fmt.Errorf("agent: llm planner: nothing was offered")
	}
	picked := "" // filled in below; the deferred log line reports it
	start := time.Now()
	reply, err := p.ask(obs, offered)
	took := time.Since(start)
	if err != nil {
		return Objective{}, err
	}
	reply = strings.TrimSpace(reply)
	defer func() {
		// Logged after the reply has resolved, so the line reports what it
		// actually became rather than what it looked like.
		if p.Log != nil {
			fmt.Fprintf(p.Log, "  llm: %d offered, %s, reply %q -> %s\n",
				len(offered), took.Round(10*time.Millisecond), snippet([]byte(reply)), picked)
		}
	}()
	o, err := resolveReply(offered, reply)
	if err != nil {
		picked = "rejected: " + err.Error()
		return Objective{}, fmt.Errorf("agent: llm planner: %w", err)
	}
	picked = o.String()
	p.recent = append(p.recent, picked)
	if len(p.recent) > recentCap {
		p.recent = p.recent[len(p.recent)-recentCap:]
	}
	return o, nil
}

// resolveReply turns a raw model reply into an offered objective. A reply
// that parses as a JSON object with an integer "choice" takes the schema
// path: the choice indexes the offered list and any argument fields are
// applied (and range-checked) by WithArgs. Anything else — a bare number,
// prose, or a server that ignored response_format — takes the fallback
// text path, where the last integer wins.
func resolveReply(offered []Objective, reply string) (Objective, error) {
	if cr, ok := parseChoiceReply(reply); ok {
		base, err := Chosen(offered, strconv.Itoa(*cr.Choice))
		if err != nil {
			return Objective{}, err
		}
		return WithArgs(base, ReplyArgs{
			Level:    cr.Level,
			Species:  cr.Species,
			Item:     cr.Item,
			Quantity: cr.Quantity,
		})
	}
	n, ok := answerInt(reply)
	if !ok {
		return Objective{}, fmt.Errorf("no number in reply %q", reply)
	}
	return Chosen(offered, n)
}

// choiceReply is the schema-shaped reply: the choice is required, the
// argument fields are optional and only meaningful for the kind they
// belong to (WithArgs enforces that).
type choiceReply struct {
	Choice   *int   `json:"choice"`
	Level    *int   `json:"level"`
	Species  string `json:"species"`
	Item     string `json:"item"`
	Quantity *int   `json:"quantity"`
}

// parseChoiceReply decodes a schema-shaped reply. ok is false when the
// reply is not a JSON object with an integer "choice" — that is the
// fallback signal, not an error: servers that ignore response_format
// still answer in plain text, and those replies must keep working.
func parseChoiceReply(s string) (choiceReply, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return choiceReply{}, false
	}
	var r choiceReply
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return choiceReply{}, false
	}
	if r.Choice == nil {
		return choiceReply{}, false
	}
	return r, true
}

// The reply is constrained with a single call, not two. A whole-output
// schema forbids visible reasoning (the answer must come immediately), and
// that is the deliberate trade: at ~260 ms per call against a multi-minute
// run, one constrained call halves the planning latency, and the choice is
// bounded — an index into a short offered list plus at most one argument.
// If S6-11's diagnosis shows the model needs to think out loud before
// choosing, the answer is a free pre-call followed by this constrained
// one; nothing here has to change for that.
const llmSystemPrompt = `You are choosing the next objective for a Pokemon Red player. Prefer an objective that makes NEW progress: repeating what you just did wastes the run. Reply with ONLY a JSON object: {"choice": N} where N is the number of your choice, plus the arguments of that objective when it has any ("level", "species", "item", "quantity"). Do not explain.`

// llmUserPrompt renders the observation as indented JSON, then the
// offered objectives as a 1-based numbered list of their String() forms.
// The model is asked for the index, not the sentence: Chosen accepts a
// bare index, and small models emit indices far more reliably than exact
// sentences.
//
// The whole Observation is the prompt's game knowledge — including the
// lead's moves, the bag, the recent dialogue and the round history, which
// is what makes "fight Brock now or train first?" answerable. If a field
// is not rendered here it might as well not exist.
func llmUserPrompt(obs Observation, offered []Objective, recent []string) string {
	obsJSON, err := json.MarshalIndent(obs, "", "  ")
	if err != nil {
		obsJSON = []byte("{}") // Observation is plain data; this cannot happen
	}
	var b strings.Builder
	b.WriteString("Observation:\n")
	b.Write(obsJSON)
	if len(recent) > 0 {
		b.WriteString("\n\nAlready done this run, oldest first:\n")
		for _, r := range recent {
			b.WriteString("- " + r + "\n")
		}
		b.WriteString("Do not simply undo the last one.")
	}
	b.WriteString("\n\nOffered objectives:\n")
	for i, o := range offered {
		fmt.Fprintf(&b, "%d: %s\n", i+1, o)
	}
	return b.String()
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Temperature    float64         `json:"temperature"`
	Messages       []chatMessage   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

// responseFormat asks a server that supports structured output (llama.cpp
// does: type "json_schema") to constrain the whole reply. Servers that do
// not support it either ignore the field or reject the request; the
// fallback path in resolveReply covers the first, and a rejection is a
// visible HTTP error naming the status.
type responseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *jsonSchema `json:"json_schema,omitempty"`
}

type jsonSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

// choiceSchema is the requested reply shape. "choice" is the only required
// field; the argument fields are optional because most offered objectives
// carry no argument, and strict mode (which would require all of them)
// would make a bare {"choice": N} malformed.
var choiceSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"choice":   map[string]any{"type": "integer"},
		"level":    map[string]any{"type": "integer"},
		"species":  map[string]any{"type": "string"},
		"item":     map[string]any{"type": "string"},
		"quantity": map[string]any{"type": "integer"},
	},
	"required": []string{"choice"},
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// systemPrompt renders the full system message. With a Goal it is a
// single line above everything else — the task statement, and nothing
// that looks like strategy. Without one the result is byte-identical to
// the pre-Goal prompt (llmSystemPrompt + ExtraSystem), so every existing
// caller and measurement is unchanged.
func (p *LLMPlanner) systemPrompt() string {
	s := llmSystemPrompt + p.ExtraSystem
	if p.Goal != "" {
		s = "Your goal: " + p.Goal + "\n\n" + s
	}
	return s
}

// ask performs the one POST to {BaseURL}/chat/completions and returns
// choices[0].message.content. Every failure mode — transport error,
// non-200 status, unparseable body, empty choices — is an error naming
// what happened.
func (p *LLMPlanner) ask(obs Observation, offered []Objective) (string, error) {
	client := p.Client
	if client == nil {
		timeout := p.Timeout
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	system := p.systemPrompt()
	user := llmUserPrompt(obs, offered, p.recent)
	if p.PromptLog != nil {
		fmt.Fprintf(p.PromptLog, "=== prompt (model %s) ===\n[system]\n%s\n[user]\n%s\n", p.Model, system, user)
	}
	reqBody, err := json.Marshal(chatRequest{
		Model:       p.Model,
		Temperature: 0,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		ResponseFormat: &responseFormat{
			Type:       "json_schema",
			JSONSchema: &jsonSchema{Name: "objective_choice", Strict: false, Schema: choiceSchema},
		},
	})
	if err != nil {
		return "", fmt.Errorf("agent: llm planner: encode request: %w", err)
	}
	url := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("agent: llm planner: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("agent: llm planner: POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("agent: llm planner: read reply: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("agent: llm planner: model returned HTTP %s: %s", resp.Status, snippet(data))
	}
	var cr chatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return "", fmt.Errorf("agent: llm planner: reply is not valid JSON: %v", err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("agent: llm planner: reply has no choices: %s", snippet(data))
	}
	return cr.Choices[0].Message.Content, nil
}

var (
	intRe = regexp.MustCompile(`-?\d+`)
	// A reasoning model emits its scratch work before the answer, and an
	// unclosed block means the reply was cut off mid-thought.
	thinkRe = regexp.MustCompile(`(?is)<(think|thinking|reasoning)>.*?(</(think|thinking|reasoning)>|$)`)
)

// answerInt returns the model's chosen index from s, as a string.
//
// The LAST integer wins, not the first, and reasoning blocks are dropped
// before looking. A small model answers "2" and either rule works; a
// larger or reasoning model says "3 is tempting, but 6 is better", or
// thinks out loud first, and taking the first integer silently picks the
// number it rejected. That failure is invisible — a legal objective, the
// wrong one — and it would show up as a planning failure when comparing
// models, which is exactly the comparison this has to survive.
//
// A reply whose index is out of range is still an error, never a guess,
// so the worst case here is a clean stop rather than a wrong action.
func answerInt(s string) (string, bool) {
	s = thinkRe.ReplaceAllString(s, " ")
	m := intRe.FindAllString(s, -1)
	if len(m) == 0 {
		return "", false
	}
	return m[len(m)-1], true
}

// snippet renders a reply body for an error message, trimmed and capped.
func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "(empty body)"
	}
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
