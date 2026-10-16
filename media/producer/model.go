package producer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ChatModel speaks an OpenAI-compatible chat-completions HTTP endpoint. It
// receives ONLY the explicit EvidencePack, never private planner messages,
// chain-of-thought, screenshots, or emulator state.
type ChatModel struct {
	Endpoint   string
	ModelID    string
	Deployment string
	APIKey     string
	Client     *http.Client
}

func (m ChatModel) Identity() string {
	if m.Deployment != "" {
		return "chat:" + m.ModelID + ":" + m.Deployment
	}
	return "chat:" + m.ModelID
}

const storySystemPrompt = `You are the editorial producer of a recorded autonomous game run.
Use ONLY the supplied structured run metadata, semantic events, and verified history.
Never invent quotes, world records, achievements, emotional reactions, unseen gameplay, or historical comparisons.
Every selected beat MUST cite one or more exact evidence references with attempt, event_id, frame, timestamp_ms.
Classify as full_episode, short_only, archive_only, or do_not_publish. Prefer no publication when repetitive or uninteresting.
Beats can have kinds: hook, setup, escalation, crisis, turning_point, climax, result, or other.
Return ONLY one JSON object with this exact shape:
{"primary_story":"", "secondary_stories":[], "beats":[{"kind":"hook","summary":"", "evidence":[{"attempt":1,"event_id":"evt-...","frame":123,"timestamp_ms":2060}]}],"publication_class":"archive_only"}.
Do not describe a story unless specific evidence supports it. If telemetry is empty, return an empty story/beats and do_not_publish.`

func (m ChatModel) Generate(ctx context.Context, pack EvidencePack) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(m.Endpoint))
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") ||
		u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("producer: configured chat endpoint must be an absolute HTTP(S) URL without credentials/query")
	}
	if strings.TrimSpace(m.ModelID) == "" {
		return nil, errors.New("producer: chat model ID is required")
	}
	// Bound prompt size, while sampling the whole run's chronology rather than
	// sending an arbitrary prefix of a multi-hour recording.
	if len(pack.Events) > 128 {
		reduced := make([]EventFact, 0, 128)
		for i := 0; i < 128; i++ {
			reduced = append(reduced, pack.Events[i*(len(pack.Events)-1)/127])
		}
		pack.Events = reduced
	}
	safe, err := json.Marshal(pack)
	if err != nil {
		return nil, err
	}
	requestBody, err := json.Marshal(map[string]any{
		"model":       m.ModelID,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": storySystemPrompt},
			{"role": "user", "content": string(safe)},
		},
	})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if m.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+m.APIKey)
	}
	client := m.Client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("producer: chat request: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxModelBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxModelBytes {
		return nil, errors.New("producer: chat response too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Never leak a provider response that might contain a credential,
		// model context, or untrusted diagnostic data.
		return nil, fmt.Errorf("producer: chat endpoint returned %s", response.Status)
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("producer: chat envelope: %w", err)
	}
	if len(envelope.Choices) == 0 {
		return nil, errors.New("producer: chat returned no choices")
	}
	return []byte(envelope.Choices[0].Message.Content), nil
}
