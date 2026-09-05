package farm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ResumeCheckpoint is the durable state the wall offers when a leased run is
// continuing after its previous worker disappeared. State is always present;
// Knowledge is present for LLM objective checkpoints so agent.Run can restore
// both emulator state and learned run context from the same boundary.
type ResumeCheckpoint struct {
	Attempt   int       `json:"attempt"`
	State     Artifact  `json:"state"`
	Knowledge *Artifact `json:"knowledge,omitempty"`
}

// ResumeCheckpoint asks the wall for the latest safe checkpoint belonging to
// the immediately previous attempt. A nil checkpoint means this lease should
// start fresh. Resume lookup is deliberately best-effort at the runner: an
// unavailable checkpoint must never wedge the farm.
func (c *Client) ResumeCheckpoint(ctx context.Context, runID string, attempt int) (*ResumeCheckpoint, error) {
	body, err := json.Marshal(struct {
		RunID   string `json:"run_id"`
		Attempt int    `json:"attempt"`
		Resume  bool   `json:"resume"`
	}{RunID: runID, Attempt: attempt, Resume: true})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.runURL(runID, "checkpoint"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("farm: resume checkpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, responseError("resume checkpoint", resp)
	}
	var cp ResumeCheckpoint
	if err := json.NewDecoder(resp.Body).Decode(&cp); err != nil {
		return nil, fmt.Errorf("farm: resume checkpoint: decode: %w", err)
	}
	arts := []Artifact{cp.State}
	if cp.Knowledge != nil {
		arts = append(arts, *cp.Knowledge)
	}
	if err := ValidateFinishArtifacts(FinishReport{Artifacts: arts}); err != nil {
		return nil, fmt.Errorf("farm: resume checkpoint: invalid artifact: %w", err)
	}
	return &cp, nil
}
