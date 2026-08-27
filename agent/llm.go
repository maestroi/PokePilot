package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
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
	return p
}

// Next posts the observation and the offered objectives to the model and
// returns the offered objective the model picked. It never guesses: a
// reply that does not resolve to one of the offered objectives is an
// error, so the run loop can stop on it instead of acting on a wrong
// answer.
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
		// Logged after Chosen has run, so the line reports what the reply
		// actually resolved to rather than what it looked like.
		if p.Log != nil {
			fmt.Fprintf(p.Log, "  llm: %d offered, %s, reply %q -> %s\n",
				len(offered), took.Round(10*time.Millisecond), snippet([]byte(reply)), picked)
		}
	}()
	n, ok := answerInt(reply)
	if !ok {
		picked = "no number in the reply"
		return Objective{}, fmt.Errorf("agent: llm planner: no number in reply %q", reply)
	}
	o, err := Chosen(offered, n)
	if err != nil {
		picked = "not an offered objective"
		return Objective{}, err
	}
	picked = o.String()
	p.recent = append(p.recent, picked)
	if len(p.recent) > recentCap {
		p.recent = p.recent[len(p.recent)-recentCap:]
	}
	return o, nil
}

const llmSystemPrompt = "You are choosing the next objective for a Pokemon Red player. Prefer an objective that makes NEW progress: repeating what you just did wastes the run. Reply with ONLY the number of your choice. Do not explain."

// llmUserPrompt renders the observation as indented JSON, then the
// offered objectives as a 1-based numbered list of their String() forms.
// The model is asked for the index, not the sentence: Chosen accepts a
// bare index, and small models emit indices far more reliably than exact
// sentences.
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
	Model       string        `json:"model"`
	Temperature float64       `json:"temperature"`
	Messages    []chatMessage `json:"messages"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// ask performs the one POST to {BaseURL}/chat/completions and returns
// choices[0].message.content. Every failure mode — transport error,
// non-200 status, unparseable body, empty choices — is an error naming
// what happened.
func (p *LLMPlanner) ask(obs Observation, offered []Objective) (string, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	reqBody, err := json.Marshal(chatRequest{
		Model:       p.Model,
		Temperature: 0,
		Messages: []chatMessage{
			{Role: "system", Content: llmSystemPrompt},
			{Role: "user", Content: llmUserPrompt(obs, offered, p.recent)},
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
