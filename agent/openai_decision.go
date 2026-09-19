package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultDecisionBackend = "system-one-local"
	defaultDecisionTokens  = 512
	defaultDecisionTimeout = 60 * time.Second
)

const decisionSystemPrompt = `You are a typed decision function inside a deterministic game-playing runtime. Choose exactly one declared choice from structured state. Deterministic code owns legality, navigation, controller safety, menus, and execution; never invent actions outside the declared choices. Return ONLY JSON with "choice" and "probabilities". probabilities must contain every declared choice exactly once, contain no other choice, use values from 0 to 1, and represent your confidence distribution. The selected choice must have the highest probability. Do not explain.`

func DecisionPromptHash() string {
	return PromptHash(decisionSystemPrompt, "", "", "typed-choice-probabilities-v1")
}

// OpenAIDecisionEngine is the local System One-style experiment. It reuses an
// OpenAI-compatible /chat/completions endpoint but narrows the task to a strict
// enum plus an explicit probability distribution.
type OpenAIDecisionEngine struct {
	BaseURL   string
	Model     string
	Token     string
	Backend   string
	Client    *http.Client
	Timeout   time.Duration
	MaxTokens int
}

func NewOpenAIDecisionEngineFromEnv() *OpenAIDecisionEngine {
	baseURL := strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("POKEPILOT_LLM_URL"))
	}
	if baseURL == "" {
		baseURL = defaultLLMBaseURL
	}
	model := strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_MODEL"))
	if model == "" {
		model = strings.TrimSpace(os.Getenv("POKEPILOT_LLM_MODEL"))
	}
	if model == "" {
		model = defaultLLMModel
	}
	token := os.Getenv("POKEPILOT_DECISION_TOKEN")
	if token == "" {
		token = os.Getenv("llm_token")
	}
	return &OpenAIDecisionEngine{
		BaseURL: baseURL,
		Model:   model,
		Token:   token,
		Backend: defaultDecisionBackend,
	}
}

type decisionWireProbability struct {
	Choice      string  `json:"choice"`
	Probability float64 `json:"probability"`
}

type decisionWireResponse struct {
	Choice        string                    `json:"choice"`
	Probabilities []decisionWireProbability `json:"probabilities"`
}

type decisionChatEnvelope struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage,omitempty"`
}

func decisionSchema(req DecisionRequest) map[string]any {
	enum := make([]string, 0, len(req.Choices))
	for _, choice := range req.Choices {
		enum = append(enum, choice.ID)
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"choice", "probabilities"},
		"properties": map[string]any{
			"choice": map[string]any{
				"type": "string",
				"enum": enum,
			},
			"probabilities": map[string]any{
				"type":     "array",
				"minItems": len(enum),
				"maxItems": len(enum),
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"choice", "probability"},
					"properties": map[string]any{
						"choice": map[string]any{
							"type": "string",
							"enum": enum,
						},
						"probability": map[string]any{
							"type":    "number",
							"minimum": 0,
							"maximum": 1,
						},
					},
				},
			},
		},
	}
}

func decisionUserPrompt(req DecisionRequest) string {
	b, err := json.Marshal(req)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (e *OpenAIDecisionEngine) Decide(ctx context.Context, decision DecisionRequest) (resp DecisionResponse, err error) {
	started := time.Now()
	resp.Backend = strings.TrimSpace(e.Backend)
	if resp.Backend == "" {
		resp.Backend = defaultDecisionBackend
	}
	resp.Model = strings.TrimSpace(e.Model)
	defer func() {
		resp.Duration = time.Since(started)
	}()

	if err := ValidateDecisionRequest(decision); err != nil {
		return resp, err
	}
	baseURL := strings.TrimSpace(e.BaseURL)
	if baseURL == "" {
		baseURL = defaultLLMBaseURL
	}
	model := strings.TrimSpace(e.Model)
	if model == "" {
		model = defaultLLMModel
		resp.Model = model
	}
	maxTokens := e.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultDecisionTokens
	}
	noThink := map[string]any{"enable_thinking": false}
	userPrompt := decisionUserPrompt(decision)
	wire := chatRequest{
		Model:       model,
		Temperature: 0,
		MaxTokens:   maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: decisionSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ChatTemplateKwargs: noThink,
		ExtraBody: map[string]any{
			"chat_template_kwargs": noThink,
		},
		ResponseFormat: &responseFormat{
			Type: "json_schema",
			JSONSchema: &jsonSchema{
				Name:   "pokepilot_typed_decision",
				Strict: true,
				Schema: decisionSchema(decision),
			},
		},
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return resp, fmt.Errorf("agent: typed decision marshal request: %w", err)
	}
	resp.Usage.InputBytes = len(body)

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = defaultDecisionTimeout
	}
	client := e.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return resp, fmt.Errorf("agent: typed decision build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if e.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.Token)
	}

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return resp, fmt.Errorf("agent: typed decision POST: %w", err)
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, 2<<20))
	if err != nil {
		return resp, fmt.Errorf("agent: typed decision read response: %w", err)
	}
	resp.Usage.OutputBytes = len(raw)
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return resp, fmt.Errorf("agent: typed decision HTTP %s: %s", httpResp.Status, strings.TrimSpace(string(raw)))
	}

	var envelope decisionChatEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return resp, fmt.Errorf("agent: typed decision decode envelope: %w", err)
	}
	if envelope.Model != "" && envelope.Model != model {
		return resp, fmt.Errorf("%w: requested %q but %q answered", ErrModelMismatch, model, envelope.Model)
	}
	if len(envelope.Choices) == 0 {
		return resp, fmt.Errorf("%w: response contained no choices", ErrInvalidDecision)
	}
	if reason := envelope.Choices[0].FinishReason; reason != "" && reason != "stop" {
		return resp, fmt.Errorf("%w: finish_reason %q", ErrNotFinished, reason)
	}
	if envelope.Usage != nil {
		resp.Usage.PromptTokens = envelope.Usage.PromptTokens
		resp.Usage.CompletionTokens = envelope.Usage.CompletionTokens
	}

	content := strings.TrimSpace(envelope.Choices[0].Message.Content)
	resp.Raw = content
	var typed decisionWireResponse
	if err := json.Unmarshal([]byte(content), &typed); err != nil {
		return resp, fmt.Errorf("%w: reply is not typed decision JSON: %v", ErrInvalidDecision, err)
	}
	resp.Choice = typed.Choice
	resp.Probabilities = make(map[string]float64, len(typed.Probabilities))
	for _, probability := range typed.Probabilities {
		if _, exists := resp.Probabilities[probability.Choice]; exists {
			return resp, fmt.Errorf("%w: duplicate probability for choice %q", ErrInvalidDecision, probability.Choice)
		}
		resp.Probabilities[probability.Choice] = probability.Probability
	}
	return ValidateDecisionResponse(decision, resp)
}
