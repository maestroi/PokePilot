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
	defaultLLMBaseURL = "http://192.168.50.81:8002/v1"
	defaultLLMModel   = "qwen3.8-27b"
)

// LLMPlanner asks an OpenAI-compatible chat endpoint to choose one of
// the offered objectives. It can only ever return an objective that was
// offered — see Chosen.
type LLMPlanner struct {
	BaseURL string       // default http://192.168.50.81:8002/v1
	Model   string       // default qwen3.8-27b
	Client  *http.Client // nil means a client with a sane timeout
}

// NewLLMPlanner returns an LLMPlanner with the defaults, overridden by
// POKEPILOT_LLM_URL and POKEPILOT_LLM_MODEL when set.
func NewLLMPlanner() *LLMPlanner {
	p := &LLMPlanner{BaseURL: defaultLLMBaseURL, Model: defaultLLMModel}
	if v := os.Getenv("POKEPILOT_LLM_URL"); v != "" {
		p.BaseURL = v
	}
	if v := os.Getenv("POKEPILOT_LLM_MODEL"); v != "" {
		p.Model = v
	}
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
	reply, err := p.ask(obs, offered)
	if err != nil {
		return Objective{}, err
	}
	reply = strings.TrimSpace(reply)
	n, ok := firstInt(reply)
	if !ok {
		return Objective{}, fmt.Errorf("agent: llm planner: no number in reply %q", reply)
	}
	return Chosen(offered, n)
}

const llmSystemPrompt = "You are choosing the next objective for a Pokemon Red player. Reply with ONLY the number of your choice. Do not explain."

// llmUserPrompt renders the observation as indented JSON, then the
// offered objectives as a 1-based numbered list of their String() forms.
// The model is asked for the index, not the sentence: Chosen accepts a
// bare index, and small models emit indices far more reliably than exact
// sentences.
func llmUserPrompt(obs Observation, offered []Objective) string {
	obsJSON, err := json.MarshalIndent(obs, "", "  ")
	if err != nil {
		obsJSON = []byte("{}") // Observation is plain data; this cannot happen
	}
	var b strings.Builder
	b.WriteString("Observation:\n")
	b.Write(obsJSON)
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
			{Role: "user", Content: llmUserPrompt(obs, offered)},
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

var firstIntRe = regexp.MustCompile(`-?\d+`)

// firstInt returns the first integer that appears in s, as a string.
// Prose, code fences, and trailing punctuation around it do not matter.
func firstInt(s string) (string, bool) {
	m := firstIntRe.FindString(s)
	return m, m != ""
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
