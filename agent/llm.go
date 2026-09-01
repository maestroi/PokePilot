package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Typed rejection errors. A reply that fails envelope verification (wrong
// model, non-stop finish) or content parsing is REJECTED: an error, never a
// guess. Run classifies the rejection (classifyRetry) and re-asks the same
// round only when the re-ask can differ from the ask it repeats (Retry):
// a wrong-shaped reply is re-asked with the rejection quoted back and a
// raised temperature, a "length" truncation with a larger max_tokens, and
// a model mismatch is never re-asked at all. Exhausting MaxReplyRetries
// asks is a clean stop rather than a silent wrong answer.
var (
	ErrModelMismatch = errors.New("agent: llm planner: model mismatch")
	ErrNotFinished   = errors.New("agent: llm planner: reply did not finish cleanly")
)

// IsLengthTruncation reports whether err is a finish_reason "length"
// rejection: the reply was cut off at the completion budget, so a re-ask of
// the same request truncates again at the same token — only a larger
// max_tokens changes the outcome.
func IsLengthTruncation(err error) bool {
	return errors.Is(err, ErrNotFinished) && strings.Contains(err.Error(), `finish_reason "length"`)
}

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

	// ReplyLog is PromptLog's other half: it receives the reply content
	// verbatim, as the server sent it, once per call — including replies
	// that are about to be rejected for shape, which are the ones worth
	// reading. Written after the POST returns, so a prompt logged with no
	// reply beside it is a call that never came back.
	ReplyLog io.Writer

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

	// NoThink asks the server to render the chat template with thinking
	// turned OFF (chat_template_kwargs {"enable_thinking": false}), for a
	// reasoning model that will not stop thinking about a menu.
	//
	// MEASURED 2026-08-31, qwen3.8-27b on llama.cpp, one 16-objective menu:
	// thinking on, 47.1s and 4096 completion tokens, truncated mid-thought
	// and REJECTED as finish_reason "length"; thinking off, 0.88s and 22
	// tokens, a clean {"choice": 1, "intent": ...}. Fifty times faster and
	// the difference between a run and a stall. (Qwen's "/no_think" prompt
	// suffix did nothing on this build: 44.8s, truncated. The template
	// argument is what this server honours.)
	//
	// Off by default, so the request is byte-identical for every existing
	// caller and prior measurement. A server that does not know the field
	// ignores it. POKEPILOT_LLM_NO_THINK=1 sets it.
	NoThink bool

	// MaxTokens caps one reply's completion tokens. Zero means
	// maxReplyTokens. Raise it for a reasoning model: the think block is
	// spent from this budget, and a cap that truncates it is reported as
	// finish_reason "length" and rejected, which reads as a broken model
	// rather than a short leash. POKEPILOT_LLM_MAX_TOKENS sets it.
	MaxTokens int

	// Timeout bounds one POST to the model. Zero means the 60s default. A
	// large model (ablation A) can take well over a minute per call, so
	// badgerun raises it via POKEPILOT_LLM_TIMEOUT rather than editing code.
	Timeout time.Duration

	// Health counts what happened to this planner's replies, per run. A
	// scoreboard row collected with a dozen transport errors or nine
	// fallback parses is not comparable to one with none; the counters are
	// what make that visible, so every row can carry the conditions it was
	// collected under. badgerun reads these off the planner at the end of
	// each run.
	Health LLMHealth

	// modelOmittedLogged is the once-per-run gate for the "server did not
	// report a model" log line: visible, but not repeated every call.
	modelOmittedLogged bool
}

// LLMHealth counts the conditions a run's replies were collected under,
// per planner (one planner per run). A bad sweep must not look like a bad
// model: Transport errors are server problems, Rejected is reply-shape
// problems, Fallbacks marks replies that did not honour the requested JSON
// schema and had to be parsed as plain text.
type LLMHealth struct {
	Transport int // POST failures, timeouts, non-200, unreadable or malformed envelope
	Rejected  int // replies rejected for shape: model mismatch, finish_reason != stop, unparseable content
	Fallbacks int // replies parsed by the plain-text fallback path
	// PromptTokens and CompletionTokens are what the run has spent, summed
	// over every reply whose server reported usage. A server that omits
	// usage leaves them at zero, which is why they are counters and not an
	// average: zero means "not reported", never "free".
	PromptTokens     int
	CompletionTokens int
}

// maxReplyTokens caps the reply so a model that will not stop cannot burn the
// whole request timeout. It is NOT a way to keep replies short: a reasoning
// model emits a <think> block before the answer (see thinkRe), so a tight cap
// truncates it mid-thought, the server reports finish_reason "length", the
// reply is rejected as ErrNotFinished. The retry then doubles the budget
// (classifyRetry, capped at maxRetryTokens) rather than re-asking the same
// request, and only if the doubled budget still truncates does the round
// burn the rest of MaxReplyRetries. Keep it well above a think block plus
// {"choice": N}.
//
// 512 is sized for a small model that barely thinks. MEASURED 2026-08-31
// against a local qwen3.8-27b: 439 completion tokens to choose between TWO
// objectives, nearly all of it reasoning. A real round's menu would blow
// straight through this and stop the run on round 1, so a bigger model
// raises it with POKEPILOT_LLM_MAX_TOKENS rather than by editing code —
// the same seam POKEPILOT_LLM_TIMEOUT is for.
const maxReplyTokens = 512

// maxRetryTokens caps a length-truncation retry's raised budget: the point
// is to clear the think block, not to remove the cap that keeps a runaway
// reply from eating the request timeout.
const maxRetryTokens = 8192

// Usage reports the tokens this planner's model calls have spent, summed
// over EVERY call including rejected re-asks: the usage is folded in right
// after each ask returns, before the reply is verified, so a re-ask costs a
// full prompt and the total says so. Zero means the server never reported
// usage, which is "not reported", never "free".
func (p *LLMPlanner) Usage() (prompt, completion int) {
	return p.Health.PromptTokens, p.Health.CompletionTokens
}

// PromptHash is the comparability marker for the prompt a planner sends:
// the first 8 hex chars of SHA-256 over the four values the request is
// built from — the base system prompt, the goal, the extra system text and
// the reply schema — joined with NUL separators so no field boundary can be
// reinterpreted as another field's content.
//
// It is a MARKER, not a version scheme: there is no registry and no mapping
// of hashes to slices. Two rows with different hashes are not comparable,
// and the run's prompts.txt holds the prompt verbatim, which is the record
// of what a hash meant. It is deliberately not a hand-maintained version
// constant: a constant someone forgets to bump asserts a comparability that
// does not hold, which is worse than no marker at all.
func PromptHash(system, goal, extraSystem, schema string) string {
	h := sha256.Sum256([]byte(system + "\x00" + goal + "\x00" + extraSystem + "\x00" + schema))
	return hex.EncodeToString(h[:])[:8]
}

// PromptHash returns the hash of the prompt THIS planner sends, computed
// from the same values ask() builds the request from: the rendered system
// prompt (base + goal + extra) and the reply schema, marshalled the same
// way it enters the request. Any change to any of the four — including a
// prompt edit like S10-1's argument annotations — changes the hash, which
// is the point: the hash only ever changes when the prompt does.
func (p *LLMPlanner) PromptHash() string {
	schema, err := json.Marshal(choiceSchema)
	if err != nil {
		return "" // choiceSchema is static plain data; this cannot happen
	}
	return PromptHash(llmSystemPrompt, p.Goal, p.ExtraSystem, string(schema))
}

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
	if v := os.Getenv("POKEPILOT_LLM_NO_THINK"); v != "" && v != "0" {
		p.NoThink = true
	}
	if v := os.Getenv("POKEPILOT_LLM_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.MaxTokens = n
		}
	}
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
// answer. It is NextRetry with a zero Retry.
func (p *LLMPlanner) Next(obs Observation, offered []Objective) (Objective, error) {
	return p.NextRetry(obs, offered, Retry{})
}

// NextRetry is Next for a round that was already asked once: r describes
// how this ask DIFFERS from the one it repeats (see Retry). A retry that
// re-asked the identical question of a temperature-0 model obtained the
// identical bytes (MEASURED by S9-12), so r must change the request in the
// way the rejection class needs: r.Feedback is appended to the user prompt
// verbatim so the model sees exactly what it did wrong ("level argument 12
// does not apply to go to route 1") and can correct it; r.Temperature
// overrides the sampling temperature so a deterministic sampler can emit
// different bytes at all; r.MaxTokensFactor multiplies the effective
// max_tokens so a "length" truncation is retried with the larger budget it
// was rejected for. The observation itself is unchanged — a malformed
// reply says nothing about the world, only about the shape of the answer.
// The reply is asked for as a JSON object ({"choice": N, plus the
// arguments of the chosen objective when it has any) via
// response_format/json_schema, so a well-behaved server cannot emit a
// bare number wrapped in prose. Before any of that is parsed, the
// envelope is verified: a reply whose model field names a DIFFERENT model
// than the one requested is a typed error (ErrModelMismatch), and a
// finish_reason other than "stop" (truncation, content filter) is a typed
// rejection (ErrNotFinished). A server that rejects or ignores the schema
// still works: a non-JSON reply that looks like an answer (short,
// substantially just the number) falls back to the text path. The schema
// is an optimisation, not the safety mechanism — WithArgs and Validate
// check every value against its stated range either way.
func (p *LLMPlanner) NextRetry(obs Observation, offered []Objective, r Retry) (Objective, error) {
	if len(offered) == 0 {
		return Objective{}, fmt.Errorf("agent: llm planner: nothing was offered")
	}
	picked := "" // filled in below; the deferred log line reports it
	start := time.Now()
	res, err := p.ask(obs, offered, r.Feedback, r.Temperature, r.MaxTokensFactor)
	took := time.Since(start)
	if err != nil {
		return Objective{}, err
	}
	reply := strings.TrimSpace(res.Content)
	if res.Usage != nil {
		p.Health.PromptTokens += res.Usage.PromptTokens
		p.Health.CompletionTokens += res.Usage.CompletionTokens
	}
	defer func() {
		// Logged after the reply has resolved, so the line reports what it
		// actually became rather than what it looked like.
		if p.Log != nil {
			usage := ""
			if res.Usage != nil {
				usage = fmt.Sprintf(", tokens %d prompt/%d completion",
					res.Usage.PromptTokens, res.Usage.CompletionTokens)
			}
			fmt.Fprintf(p.Log, "  llm: %d offered, %s%s, reply %q -> %s\n",
				len(offered), took.Round(10*time.Millisecond), usage, snippet([]byte(reply)), picked)
		}
	}()
	if res.Model != "" && res.Model != p.Model {
		// The ablation question is "does the bigger model solve this?". If
		// the server ignored the model field, loaded one model, or the env
		// var is wrong, comparing a model to itself would be read as "not
		// capacity" — a false negative on the central experiment. So this
		// is a hard error naming both sides, never a warn-and-continue.
		p.Health.Rejected++
		return Objective{}, fmt.Errorf("%w: requested %q but %q answered", ErrModelMismatch, p.Model, res.Model)
	}
	if res.Model == "" && !p.modelOmittedLogged {
		// Some OpenAI-compatible servers omit the field entirely; that is
		// not an error (failing there would break working setups), but it
		// means which model answered is UNVERIFIED, so say so once.
		p.modelOmittedLogged = true
		if p.Log != nil {
			fmt.Fprintln(p.Log, "  llm: server did not report a model field; cannot verify which model answered")
		}
	}
	if res.FinishReason != "" && res.FinishReason != "stop" {
		// "length" means the reply was cut off: a truncated JSON that still
		// parses is a silent wrong answer. Any non-stop stop reason is a
		// REJECTED reply, not one to be parsed.
		p.Health.Rejected++
		return Objective{}, fmt.Errorf("%w: finish_reason %q", ErrNotFinished, res.FinishReason)
	}
	o, usedFallback, err := resolveReply(offered, reply)
	if usedFallback {
		p.Health.Fallbacks++
	}
	if err != nil {
		p.Health.Rejected++
		picked = "rejected: " + err.Error()
		return Objective{}, fmt.Errorf("agent: llm planner: %w", err)
	}
	picked = o.String()
	return o, nil
}

// resolveReply turns a raw model reply into an offered objective. A reply
// that parses as a JSON object with an integer "choice" takes the schema
// path: the choice indexes the offered list and any argument fields are
// applied (and range-checked) by WithArgs. Anything else — a server that
// ignored response_format — takes the fallback text path, which is kept
// because those servers must keep working, but is now gated: the reply
// must LOOK like an answer (looksLikeAnswer), or it is rejected.
//
// The second return value says whether the fallback path was taken, so the
// caller can count how many replies did not honour the requested schema.
func resolveReply(offered []Objective, reply string) (Objective, bool, error) {
	if cr, ok := parseChoiceReply(reply); ok {
		base, err := Chosen(offered, strconv.Itoa(*cr.Choice))
		if err != nil {
			return Objective{}, false, err
		}
		o, err := WithArgs(base, ReplyArgs{
			Level:    cr.Level,
			Species:  cr.Species,
			Item:     cr.Item,
			Quantity: cr.Quantity,
			Flee:     cr.Flee,
			Intent:   cr.Intent,
		})
		if err != nil {
			return Objective{}, false, err
		}
		return o, false, nil
	}
	if !looksLikeAnswer(reply) {
		return Objective{}, true, fmt.Errorf("reply does not look like an answer: %s", snippet([]byte(reply)))
	}
	n, ok := answerInt(reply)
	if !ok {
		return Objective{}, true, fmt.Errorf("no number in reply %q", reply)
	}
	o, err := Chosen(offered, n)
	if err != nil {
		return Objective{}, true, err
	}
	return o, true, nil
}

// choiceReply is the schema-shaped reply: the choice is required, the
// argument fields are optional and only meaningful for the kind they
// belong to (WithArgs enforces that). Flee is no longer in the schema —
// a conforming server cannot emit it — but the field stays so a reply from
// a server that ignores response_format and still carries "flee" is parsed
// here and rejected by WithArgs, never silently dropped.
type choiceReply struct {
	Choice   *int   `json:"choice"`
	Level    *int   `json:"level"`
	Species  string `json:"species"`
	Item     string `json:"item"`
	Quantity *int   `json:"quantity"`
	Flee     *bool  `json:"flee"`
	Intent   string `json:"intent"`
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

// looksLikeAnswer is the fallback gate (S7-3). A non-JSON reply is
// accepted only when it substantially IS the number: after stripping
// reasoning blocks and trimming, at most 12 characters long, containing
// exactly one integer, with every remaining character whitespace or
// punctuation. "2", " 2.", "(5)" pass; "rate limited, retry in 5 seconds"
// (letters) and "Option 1 is tempting, but 3 is better." (two integers,
// letters) do not. The rule exists because the old fallback took the last
// integer ANYWHERE in the reply: an HTTP-200 body of prose that merely
// contains a digit became a game action. A long prose reply containing a
// digit is an unhealthy response, not a choice.
func looksLikeAnswer(s string) bool {
	s = strings.TrimSpace(thinkRe.ReplaceAllString(s, " "))
	if len(s) > 12 {
		return false
	}
	if n := len(intRe.FindAllString(s, -1)); n != 1 {
		return false
	}
	rest := intRe.ReplaceAllString(s, "")
	for _, r := range rest {
		if !strings.ContainsRune(" \t\n.,:;!?()[]{}\"'-", r) {
			return false
		}
	}
	return true
}

// The reply is constrained with a single call, not two. A whole-output
// schema forbids visible reasoning (the answer must come immediately), and
// that is the deliberate trade: at ~260 ms per call against a multi-minute
// run, one constrained call halves the planning latency, and the choice is
// bounded — an index into a short offered list plus at most one argument.
// If S6-11's diagnosis shows the model needs to think out loud before
// choosing, the answer is a free pre-call followed by this constrained
// one; nothing here has to change for that.
const llmSystemPrompt = `You are choosing the next objective for a Pokemon Red player. Prefer an objective that makes NEW progress: repeating what you just did wastes the run. The run has a limited number of rounds and each objective costs one — the observation's "RoundsLeft" is how many you have left, this one included: most small talk does not advance your goal, so spend rounds on objectives that move toward it. Reply with ONLY a JSON object: {"choice": N} where N is the number of your choice. Every objective is already complete as written — the level, species, item and quantity are part of the sentence you are picking, so send no other fields. Travelling objectives are offered twice: the plain one FIGHTS wild battles on the way, and the one ending in ", fleeing wild battles" RUNS them instead — pick the variant you want by its number. Also include "intent": one short sentence (at most 200 bytes) saying what this choice is in service of. It will be read back to you on the next round's observation as "Intent", with "IntentAge" — how many rounds it has gone unchanged — so state it honestly and change it only when your purpose changes. Do not explain.`

// llmUserPrompt renders the observation as compact JSON, then the offered
// objectives as a 1-based numbered list of their String() forms.
// The model is asked for the index, not the sentence: Chosen accepts a
// bare index, and small models emit indices far more reliably than exact
// sentences.
//
// The whole Observation is the prompt's game knowledge — including the
// lead's moves, the bag, the recent dialogue and the round history, which
// is what makes "fight Brock now or train first?" answerable. If a field
// is not rendered here it might as well not exist.
func llmUserPrompt(obs Observation, offered []Objective) string {
	// Compact, not indented: indentation is pure prompt cost. MEASURED
	// 2026-08-29 on a live run — latency scales with prompt length, 5-7
	// offered took ~6-7s per call and 13-15 took ~21-23s.
	obsJSON, err := json.Marshal(obs)
	if err != nil {
		obsJSON = []byte("{}") // Observation is plain data; this cannot happen
	}
	var b strings.Builder
	b.WriteString("Observation:\n")
	b.Write(obsJSON)
	b.WriteString("\n\nOffered objectives:\n")
	for i, o := range offered {
		// Note is what this run has already done with this objective (see
		// Offer's annotate): "(done 6x)", "(failed 3x)". It rides on the
		// LINE BEING CHOSEN because the same counts sitting elsewhere in
		// the prompt were demonstrably skipped — the model re-picked an
		// objective while its own failure record named it three lines
		// above. String() does not include it, so nothing that identifies
		// an objective changes.
		if o.Note != "" {
			fmt.Fprintf(&b, "%d: %s  %s\n", i+1, o, o.Note)
			continue
		}
		fmt.Fprintf(&b, "%d: %s\n", i+1, o)
	}
	return b.String()
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Messages    []chatMessage `json:"messages"`
	// ChatTemplateKwargs are arguments for the server's chat template. Only
	// ever set from NoThink; omitted otherwise, so the request a server sees
	// is unchanged unless someone asked for this.
	ChatTemplateKwargs map[string]any  `json:"chat_template_kwargs,omitempty"`
	ResponseFormat     *responseFormat `json:"response_format,omitempty"`
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

// choiceSchema is the requested reply shape: an index, and a sentence
// saying what it is for. NOTHING ELSE.
//
// Every offered objective is already CONCRETE when Offer builds it — the
// catch names its species, the train names its target level, the buy names
// its item and quantity, the use-item names its slot. So an argument in the
// reply was never information the menu lacked; it was only ever a way to
// OVERRIDE the offered value, and that override has now ended three runs:
//
//	"flee": true attached to starters and talk (S10-1)
//	{"choice": 1, "level": 1, "species": "Charmander"} on a starter menu
//	{"choice": 2, "level": 7} on "use a POTION on party slot 0"
//
// The pattern is always the same. A small model handed an optional field
// FILLS IT IN, attaches it to whatever it picked, WithArgs correctly
// rejects it as inapplicable, and at temperature 0 the rejection feedback
// produces a byte-identical reply — so the round burns MaxReplyRetries and
// the run stops. Narrowing the schema per round fixed only the case where
// NOTHING offered took an argument; the moment one objective on the menu
// carries a level, the model can staple that level to a different one.
//
// The constrained decoder forbids what the schema omits, so omitting all of
// them leaves exactly the question the model answers reliably: a number.
// This is the flee lesson applied to the rest — the choice lives in the
// MENU, made as an index.
//
// The COST, stated plainly: the planner can no longer aim training at an
// arbitrary level. It gets the offered target (the lead's level plus
// trainStep) and can pick training again to climb further. If aiming
// higher in one round turns out to matter, the answer is another menu
// entry, not another reply field.
//
// WithArgs is unchanged and still range-checks every argument that arrives:
// a server that ignores response_format can still send one, and it is still
// rejected rather than silently dropped. The schema is an optimisation, not
// the safety mechanism.
var choiceSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"choice": map[string]any{"type": "integer"},
		"intent": map[string]any{
			"type":        "string",
			"description": "One short sentence, at most 200 bytes: what this choice is in service of. It is read back to you next round as the observation's Intent field, with IntentAge — how many rounds it has gone unchanged.",
		},
	},
	"required": []string{"choice"},
}

// chatChoice carries the message plus finish_reason: "length" means the
// reply was cut off mid-generation, and a truncated JSON that still parses
// is a silent wrong answer, so Next rejects any non-stop reason.
type chatChoice struct {
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// chatResponse carries the model field: which model actually ANSWERED. It
// is verified against the requested model in Next (a mismatch is a typed
// error) because an ablation that compares a model to itself would report
// "not capacity" for what is a serving bug. An empty Model means the
// server omitted the field, which is legal but unverified — logged once.
type chatResponse struct {
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   *chatUsage   `json:"usage,omitempty"`
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

// chatResult is the parsed reply: the content plus the envelope facts
// (which model answered, how the generation stopped) that Next verifies
// before trusting the content.
type chatResult struct {
	Content      string
	Model        string // empty when the server omitted the field
	FinishReason string // empty when the server omitted the field
	Usage        *chatUsage
}

// ask performs the one POST to {BaseURL}/chat/completions and returns the
// first choice's content plus the envelope model and finish_reason. Every
// failure mode — transport error, timeout, non-200 status, unparseable
// body, empty choices — is an error naming what happened, and each one
// increments Health.Transport. When feedback is non-empty (a re-ask after
// a rejected reply) it is appended to the user prompt as rejection
// feedback; temperature (nil means 0) and maxTokensFactor (0 or 1 means
// the planner's own budget) are the request-level differences a retry may
// carry; see NextRetry.
func (p *LLMPlanner) ask(obs Observation, offered []Objective, feedback string, temperature *float64, maxTokensFactor int) (chatResult, error) {
	client := p.Client
	if client == nil {
		timeout := p.Timeout
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	system := p.systemPrompt()
	user := llmUserPrompt(obs, offered)
	if feedback != "" {
		user += "\n\nYour previous reply was rejected: " + feedback +
			"\nReply again with ONLY a JSON object naming one of the offered objectives and only arguments that apply to it."
	}
	if p.PromptLog != nil {
		// The prompt hash rides on EVERY entry rather than a one-off header
		// line: prompts.txt is read by tailing and grepping it, and a header
		// scrolled past a thousand rounds ago is the runnote problem again.
		fmt.Fprintf(p.PromptLog, "=== prompt (model %s, prompt %s) ===\n[system]\n%s\n[user]\n%s\n",
			p.Model, p.PromptHash(), system, user)
	}
	transportErr := func(format string, args ...any) (chatResult, error) {
		p.Health.Transport++
		return chatResult{}, fmt.Errorf("agent: llm planner: "+format, args...)
	}
	temp := 0.0
	if temperature != nil {
		temp = *temperature
	}
	maxTokens := p.MaxTokens
	if maxTokens <= 0 {
		maxTokens = maxReplyTokens
	}
	if maxTokensFactor > 1 {
		if mt := maxTokens * maxTokensFactor; mt < maxRetryTokens {
			maxTokens = mt
		} else {
			maxTokens = maxRetryTokens
		}
	}
	var templateKwargs map[string]any
	if p.NoThink {
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
			JSONSchema: &jsonSchema{Name: "objective_choice", Strict: false, Schema: choiceSchema},
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
		// A timeout lands here as a context deadline error; it counts as a
		// transport failure, which is what it is — the POKEPILOT_LLM_TIMEOUT
		// comment "was killing runs spuriously" is this counter in disguise.
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
