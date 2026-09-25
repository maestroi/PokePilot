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
	defaultJevDecisionBaseURL = "https://api.typesafe.ai/v1"
	defaultJevDecisionModel   = "jev-latest"
	defaultJevDecisionBackend = "jev"
	jevDecisionQuestionName   = "decision"
	jevDecisionContract       = "typesafe-systemone-choice-v1"
)

// JevDecisionPromptHash identifies the stable request mapping used by the
// ROM-free evaluator. Jev does not use a generative system prompt, so the hash
// tracks the typed System One contract instead.
func JevDecisionPromptHash() string {
	return PromptHash("TypeSafe Jev System One", "", "", jevDecisionContract)
}

// JevDecisionEngine adapts TypeSafe's hosted System One choice API to the
// backend-neutral DecisionEngine interface. Generic agent code never depends on
// Jev-specific request or response types.
type JevDecisionEngine struct {
	BaseURL string
	Model   string
	Token   string
	Backend string
	Client  *http.Client
	Timeout time.Duration
}

func NewJevDecisionEngineFromEnv() *JevDecisionEngine {
	baseURL := strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_URL"))
	if baseURL == "" {
		baseURL = defaultJevDecisionBaseURL
	}
	model := strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_MODEL"))
	if model == "" {
		model = defaultJevDecisionModel
	}
	token := strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY"))
	}
	return &JevDecisionEngine{
		BaseURL: baseURL,
		Model:   model,
		Token:   token,
		Backend: defaultJevDecisionBackend,
	}
}

type jevChoiceQuestion struct {
	Type         string         `json:"type"`
	Instructions string         `json:"instructions,omitempty"`
	Criteria     map[string]any `json:"criteria"`
}

type jevSystemOneRequest struct {
	Model     string                       `json:"model"`
	State     any                          `json:"state"`
	Questions map[string]jevChoiceQuestion `json:"questions"`
}

type jevChoiceAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

type jevSystemOneResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]jevChoiceAnswer `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func jevDecisionState(raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return map[string]any{}, nil
	}
	var state any
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("%w: Jev state is not valid JSON: %v", ErrInvalidDecision, err)
	}
	switch state.(type) {
	case string, []any, map[string]any:
		return state, nil
	default:
		// System One accepts string/object/array state. Preserve scalar state by
		// wrapping it instead of rejecting an otherwise valid DecisionRequest.
		return map[string]any{"value": state}, nil
	}
}

func jevDecisionInstructions(req DecisionRequest) string {
	question := strings.TrimSpace(req.Question)
	instructions := strings.TrimSpace(req.Instructions)
	if instructions == "" {
		return question
	}
	return question + "\n\n" + instructions
}

func jevDecisionCriteria(choices []DecisionChoice) map[string]any {
	criteria := make(map[string]any, len(choices))
	for _, choice := range choices {
		label := strings.TrimSpace(choice.Label)
		description := strings.TrimSpace(choice.Description)
		switch {
		case label != "" && description != "":
			criteria[choice.ID] = label + ": " + description
		case description != "":
			criteria[choice.ID] = description
		case label != "":
			criteria[choice.ID] = label
		default:
			criteria[choice.ID] = nil
		}
	}
	return criteria
}

func jevSystemOneEndpoint(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/systemone") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/systemone"
	}
	return baseURL + "/v1/systemone"
}

func (e *JevDecisionEngine) Decide(ctx context.Context, decision DecisionRequest) (resp DecisionResponse, err error) {
	started := time.Now()
	resp.Backend = strings.TrimSpace(e.Backend)
	if resp.Backend == "" {
		resp.Backend = defaultJevDecisionBackend
	}
	resp.Model = strings.TrimSpace(e.Model)
	defer func() {
		resp.Duration = time.Since(started)
	}()

	if err := ValidateDecisionRequest(decision); err != nil {
		return resp, err
	}
	state, err := jevDecisionState(decision.State)
	if err != nil {
		return resp, err
	}
	model := strings.TrimSpace(e.Model)
	if model == "" {
		model = defaultJevDecisionModel
		resp.Model = model
	}
	baseURL := strings.TrimSpace(e.BaseURL)
	if baseURL == "" {
		baseURL = defaultJevDecisionBaseURL
	}

	wire := jevSystemOneRequest{
		Model: model,
		State: state,
		Questions: map[string]jevChoiceQuestion{
			jevDecisionQuestionName: {
				Type:         "choice",
				Instructions: jevDecisionInstructions(decision),
				Criteria:     jevDecisionCriteria(decision.Choices),
			},
		},
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return resp, fmt.Errorf("agent: Jev decision marshal request: %w", err)
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
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, jevSystemOneEndpoint(baseURL), bytes.NewReader(body))
	if err != nil {
		return resp, fmt.Errorf("agent: Jev decision build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if e.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.Token)
	}

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return resp, fmt.Errorf("agent: Jev decision POST: %w", err)
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, 2<<20))
	if err != nil {
		return resp, fmt.Errorf("agent: Jev decision read response: %w", err)
	}
	resp.Usage.OutputBytes = len(raw)
	resp.Raw = strings.TrimSpace(string(raw))
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return resp, fmt.Errorf("agent: Jev decision HTTP %s: %s", httpResp.Status, strings.TrimSpace(string(raw)))
	}

	var envelope jevSystemOneResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return resp, fmt.Errorf("%w: Jev reply is not System One JSON: %v", ErrInvalidDecision, err)
	}
	if strings.TrimSpace(envelope.Model) != "" {
		// TypeSafe explicitly allows aliases such as jev-latest to resolve to a
		// different concrete model name. Record what actually answered.
		resp.Model = strings.TrimSpace(envelope.Model)
	}
	resp.Usage.PromptTokens = envelope.Usage.InputTokens
	resp.Usage.CompletionTokens = envelope.Usage.OutputTokens

	answer, ok := envelope.Answers[jevDecisionQuestionName]
	if !ok {
		return resp, fmt.Errorf("%w: Jev response missing %q answer", ErrInvalidDecision, jevDecisionQuestionName)
	}
	if answer.Type != "choice" {
		return resp, fmt.Errorf("%w: Jev answer type %q, want choice", ErrInvalidDecision, answer.Type)
	}
	resp.Choice = answer.Choice
	resp.Confidence = answer.Confidence
	resp.Probabilities = answer.Probabilities
	return ValidateDecisionResponse(decision, resp)
}
