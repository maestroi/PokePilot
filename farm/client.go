package farm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Client is the runner's only knowledge of the wall: four HTTP calls.
// No Docker, no Swarm.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// Version is this build's identity (git SHA), stamped onto pings and
	// heartbeats so the wall can show which build each worker runs.
	Version string
}

// NewClient builds a Client with a default *http.Client.
func NewClient(baseURL string) *Client {
	return &Client{BaseURL: baseURL, HTTP: http.DefaultClient}
}

// Lease asks the wall for the next spec. A 204 response means none is
// ready yet; Lease returns (nil, nil) so the caller can wait and retry.
func (c *Client) Lease(ctx context.Context) (*Spec, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/lease", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("farm: lease: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("farm: lease: status %d", resp.StatusCode)
	}
	var spec Spec
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		return nil, fmt.Errorf("farm: lease: decode: %w", err)
	}
	return &spec, nil
}

// Heartbeat reports progress for an in-flight run and returns the wall's
// reply, which may ask for a cooperative cancel.
func (c *Client) Heartbeat(ctx context.Context, hb Heartbeat) (HeartbeatReply, error) {
	var reply HeartbeatReply
	hb.Version = c.Version
	body, err := json.Marshal(hb)
	if err != nil {
		return reply, err
	}
	url := fmt.Sprintf("%s/v1/runs/%s/heartbeat", c.BaseURL, hb.RunID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return reply, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return reply, fmt.Errorf("farm: heartbeat: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return reply, fmt.Errorf("farm: heartbeat: status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return reply, fmt.Errorf("farm: heartbeat: decode: %w", err)
	}
	return reply, nil
}

// Ping advertises this worker's watch addresses while it is between runs,
// so the wall's grid shows idle capacity as well as in-flight runs. The
// wall treats it as presence only; a failure means the wall is
// unreachable, which the lease call right after reports loudly.
func (c *Client) Ping(ctx context.Context, addrs []string) error {
	body, err := json.Marshal(WorkerPing{Addrs: addrs, Version: c.Version})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/workers", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("farm: ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("farm: ping: status %d", resp.StatusCode)
	}
	return nil
}

// Finish reports why a leased run ended. It is the last call the runner
// makes before leasing again.
func (c *Client) Finish(ctx context.Context, report FinishReport) error {
	if err := ValidateFinishArtifacts(report); err != nil {
		return err
	}
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/v1/runs/%s/finish", c.BaseURL, report.RunID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("farm: finish: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("farm: finish: status %d", resp.StatusCode)
	}
	return nil
}

// Checkpoint uploads one in-flight checkpoint. It accepts plain artifact
// bytes and never receives an emulator handle.
func (c *Client) Checkpoint(ctx context.Context, report CheckpointReport) error {
	if err := ValidateFinishArtifacts(FinishReport{Artifacts: report.Artifacts}); err != nil {
		return err
	}
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/v1/runs/%s/checkpoint", c.BaseURL, report.RunID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("farm: checkpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("farm: checkpoint: status %d", resp.StatusCode)
	}
	return nil
}
