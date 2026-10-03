package operatorapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type CheckResult struct {
	Name    string `json:"name"` // stable id, e.g. "disk:vm-swarm-worker-03:/"
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Grace   int    `json:"grace"`            // consecutive bad observations before paging
	RunID   string `json:"run_id,omitempty"` // set when the check concerns one run
	Link    string `json:"link,omitempty"`
}

type DiskFree struct {
	Mount  string `json:"mount"`
	FreeGB int    `json:"free_gb"`
}

type FixerSummary struct {
	PaidStarts24h int      `json:"paid_starts_24h"`
	FreeStarts24h int      `json:"free_starts_24h"`
	PaidCap       int      `json:"paid_cap"`
	Attempts24h   int      `json:"attempts_24h"`
	PRs24h        int      `json:"prs_24h"`
	BlockedKeys   []string `json:"blocked_keys,omitempty"`
}

type NodeReport struct {
	Node  string        `json:"node"`
	At    int64         `json:"at"`
	Disks []DiskFree    `json:"disks"`
	Fixer *FixerSummary `json:"fixer,omitempty"`
}

type SwarmNode struct {
	Hostname      string `json:"hostname"`
	Manager       bool   `json:"manager"`
	Status        string `json:"status"`         // ready, down, ...
	ManagerStatus string `json:"manager_status"` // reachable, unreachable, leader ("" for workers)
}

type ServiceState struct {
	Name        string `json:"name"`
	Running     int    `json:"running"`
	Desired     int    `json:"desired"`
	UpdateState string `json:"update_state,omitempty"`
}

type PullRequest struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	CreatedAt int64  `json:"created_at"`
}

type OpsSnapshot struct {
	At          int64          `json:"at"`
	Checks      []CheckResult  `json:"checks"`
	Nodes       []SwarmNode    `json:"nodes"`
	Services    []ServiceState `json:"services"`
	Disks       []NodeReport   `json:"disks"`
	Fixer       *FixerSummary  `json:"fixer,omitempty"`
	FrozenUntil int64          `json:"frozen_until,omitempty"`
	TriagePRs   []PullRequest  `json:"triage_prs,omitempty"`
	Merged24h   int            `json:"merged_24h"`  // -1 = unknown
	FarmOpened  int            `json:"farm_opened"` // -1 = unknown
	FarmClosed  int            `json:"farm_closed"` // -1 = unknown
	// Warm is false while the watcher has not yet read Docker once and heard
	// from every ready node (or run 15 minutes). A cold snapshot may lack
	// checks that are merely unknown yet, so the bot must not resolve
	// absent checks from it.
	Warm bool `json:"warm"`
}

// PostOps sends v to url with the shared bearer token.
func PostOps(ctx context.Context, client *http.Client, url, token string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("POST %s: %s", url, res.Status)
	}
	return nil
}

// OpsAuthorized reports whether req carries the shared bearer token.
func OpsAuthorized(req *http.Request, token string) bool {
	if token == "" {
		return false
	}
	got := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

// ReadSecretFile returns the trimmed contents of path, or "" when unset/unreadable.
func ReadSecretFile(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
