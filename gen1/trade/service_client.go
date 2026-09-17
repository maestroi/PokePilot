package trade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultServiceHTTPTimeout = 10 * time.Second
const serviceStatusPollInterval = 50 * time.Millisecond
const maxServiceResponseBytes = 64 << 10

// SessionRequest is the stable control-plane request for one virtual trader
// endpoint. Session is also the broker session id the emulator must join.
type SessionRequest struct {
	Session string `json:"session"`
	RunID   string `json:"run_id,omitempty"`
	Game    string `json:"game,omitempty"`
	Policy  string `json:"policy,omitempty"`
	Species string `json:"species,omitempty"`
	Level   int    `json:"level,omitempty"`
}

// SessionStatus is the service-owned lifecycle for one virtual trader peer.
type SessionStatus struct {
	Session   string    `json:"session"`
	RunID     string    `json:"run_id,omitempty"`
	Game      string    `json:"game"`
	Policy    string    `json:"policy"`
	Species   string    `json:"species"`
	Level     int       `json:"level"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Error     string    `json:"error,omitempty"`
}

// ServiceClient provisions lightweight virtual peers. It never handles link
// traffic itself; after WaitReady succeeds the emulator joins Session through
// GomeBoy's broker transport.
type ServiceClient struct {
	BaseURL string
	Client  *http.Client
}

func (c ServiceClient) StartSession(ctx context.Context, req SessionRequest) (SessionStatus, error) {
	var out SessionStatus
	if strings.TrimSpace(req.Session) == "" {
		return out, fmt.Errorf("gen1 trade service: session is required")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return out, fmt.Errorf("gen1 trade service: encode session: %w", err)
	}
	status, data, err := c.do(ctx, http.MethodPost, "/v1/sessions", body)
	if err != nil {
		return out, err
	}
	if status != http.StatusAccepted {
		return out, serviceHTTPError(status, data)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("gen1 trade service: decode session response: %w", err)
	}
	return out, nil
}

func (c ServiceClient) Session(ctx context.Context, id string) (SessionStatus, error) {
	var out SessionStatus
	status, data, err := c.do(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(id), nil)
	if err != nil {
		return out, err
	}
	if status != http.StatusOK {
		return out, serviceHTTPError(status, data)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("gen1 trade service: decode session status: %w", err)
	}
	return out, nil
}

// WaitReady waits until the service has registered its virtual peer with the
// broker. "running" is deliberately stronger than POST acceptance: it closes
// the race where the emulator could become the broker's first peer and clock
// bytes before the virtual endpoint had joined.
func (c ServiceClient) WaitReady(ctx context.Context, id string) (SessionStatus, error) {
	for {
		status, err := c.Session(ctx, id)
		if err != nil {
			return SessionStatus{}, err
		}
		switch status.Status {
		case "running":
			return status, nil
		case "starting":
			// keep polling
		case "error", "expired", "cancelled", "done":
			if status.Error != "" {
				return status, fmt.Errorf("gen1 trade service: session %s %s: %s", id, status.Status, status.Error)
			}
			return status, fmt.Errorf("gen1 trade service: session %s became %s before emulator attachment", id, status.Status)
		default:
			return status, fmt.Errorf("gen1 trade service: session %s has unknown status %q", id, status.Status)
		}

		timer := time.NewTimer(serviceStatusPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return status, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c ServiceClient) DeleteSession(ctx context.Context, id string) error {
	status, data, err := c.do(ctx, http.MethodDelete, "/v1/sessions/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	if status == http.StatusAccepted || status == http.StatusOK || status == http.StatusNotFound {
		return nil
	}
	return serviceHTTPError(status, data)
}

func (c ServiceClient) do(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return 0, nil, fmt.Errorf("gen1 trade service: base URL is required")
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("gen1 trade service: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: defaultServiceHTTPTimeout}
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("gen1 trade service: %s %s: %w", method, path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxServiceResponseBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("gen1 trade service: read response: %w", err)
	}
	return res.StatusCode, data, nil
}

func serviceHTTPError(status int, data []byte) error {
	var envelope struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &envelope) == nil && strings.TrimSpace(envelope.Error) != "" {
		return fmt.Errorf("gen1 trade service: HTTP %d: %s", status, strings.TrimSpace(envelope.Error))
	}
	text := strings.TrimSpace(string(data))
	if len(text) > 240 {
		text = text[:237] + "..."
	}
	if text == "" {
		return fmt.Errorf("gen1 trade service: HTTP %d", status)
	}
	return fmt.Errorf("gen1 trade service: HTTP %d: %s", status, text)
}
