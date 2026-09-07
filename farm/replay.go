package farm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ReplaySource records where an explicitly queued repro run starts. Unlike a
// worker-loss resume, this provenance belongs to a new run and survives code
// changes between the source failure and the verification run.
type ReplaySource struct {
	SourceRunID  string `json:"source_run_id"`
	SourceAttempt int    `json:"source_attempt"`
	Checkpoint   string `json:"checkpoint"`
	CreatedAt    int64  `json:"created_at,omitempty"`
}

// ReplayRequest asks the wall to queue a fresh run using one checkpoint from
// an existing run. Empty Checkpoint means the newest replay-safe checkpoint.
type ReplayRequest struct {
	Checkpoint string `json:"checkpoint,omitempty"`
	Attempt    int    `json:"attempt,omitempty"`
}

// ReplayQueued is returned after the wall has durably recorded replay
// provenance and queued the verification run.
type ReplayQueued struct {
	RunID  string       `json:"run_id"`
	Source ReplaySource `json:"source"`
}

// ReplayCheckpoint asks whether this leased run was explicitly queued as a
// repro. Ordinary runs receive 204 and return nil. Replay runs receive the
// exact pinned checkpoint chosen when they were queued.
func (c *Client) ReplayCheckpoint(ctx context.Context, runID string) (*ResumeCheckpoint, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.runURL(runID, "repro-checkpoint"), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("farm: replay checkpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, responseError("replay checkpoint", resp)
	}
	return decodeReplayCheckpoint(resp)
}

// FetchCheckpoint downloads a replay-safe checkpoint directly from a source
// run. It is used by local repro tooling; "latest" selects the newest safe
// boundary for the requested attempt.
func (c *Client) FetchCheckpoint(ctx context.Context, runID string, attempt int, checkpoint string) (*ResumeCheckpoint, error) {
	endpoint := fmt.Sprintf("%s/v1/runs/%s/checkpoint", c.BaseURL, url.PathEscape(runID))
	q := url.Values{}
	if attempt > 0 {
		q.Set("attempt", strconv.Itoa(attempt))
	}
	if checkpoint != "" {
		q.Set("name", checkpoint)
	}
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("farm: fetch checkpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, responseError("fetch checkpoint", resp)
	}
	return decodeReplayCheckpoint(resp)
}

func decodeReplayCheckpoint(resp *http.Response) (*ResumeCheckpoint, error) {
	var cp ResumeCheckpoint
	if err := json.NewDecoder(resp.Body).Decode(&cp); err != nil {
		return nil, fmt.Errorf("farm: replay checkpoint: decode: %w", err)
	}
	arts := []Artifact{cp.State}
	if cp.Knowledge != nil {
		arts = append(arts, *cp.Knowledge)
	}
	if err := ValidateFinishArtifacts(FinishReport{Artifacts: arts}); err != nil {
		return nil, fmt.Errorf("farm: replay checkpoint: invalid artifact: %w", err)
	}
	return &cp, nil
}
