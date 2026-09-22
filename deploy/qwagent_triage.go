package deploy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

func DecodeTriageGroups(raw []byte) ([]TriageGroup, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty triage payload")
	}
	if trimmed[0] != '[' && trimmed[0] != '{' {
		return nil, fmt.Errorf("triage payload is not JSON (got %q)", jsonPreview(trimmed))
	}
	if trimmed[0] == '[' {
		var groups []TriageGroup
		if err := json.Unmarshal(trimmed, &groups); err != nil {
			return nil, err
		}
		return groups, nil
	}
	var envelope struct {
		Groups []TriageGroup `json:"groups"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return nil, err
	}
	if envelope.Groups == nil && !bytes.Contains(trimmed, []byte(`"groups"`)) {
		return nil, fmt.Errorf("triage JSON object missing groups")
	}
	return envelope.Groups, nil
}

func jsonPreview(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 80 {
		trimmed = trimmed[:80]
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, string(trimmed))
}

type TriageIssue struct {
	IssueNumber          int64  `json:"issue_number"`
	Status               string `json:"status"`
	Resolution           string `json:"resolution"`
	OccurrenceCount      int64  `json:"occurrence_count"`
	FixedRevision        string `json:"fixed_revision"`
	LastObservedRevision string `json:"last_observed_revision,omitempty"`
	CircuitOpen          bool   `json:"circuit_open,omitempty"`
	CircuitKind          string `json:"circuit_kind,omitempty"`
	CircuitCount         int    `json:"circuit_count,omitempty"`
}

type TriageGroup struct {
	Pattern     string       `json:"pattern"`
	Key         string       `json:"key"`
	Fingerprint string       `json:"fingerprint"`
	Count       int          `json:"count"`
	Example     string       `json:"example"`
	RunIDs      []string     `json:"run_ids"`
	Issue       *TriageIssue `json:"issue,omitempty"`
}

func (g TriageGroup) RunID() string {
	if len(g.RunIDs) == 0 {
		return ""
	}
	return g.RunIDs[0]
}

func Actionable(g TriageGroup) bool {
	if g.Issue == nil {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(g.Issue.Status))
	resolution := strings.ToLower(strings.TrimSpace(g.Issue.Resolution))
	switch status {
	case "open", "reopened", "investigating", "in_progress", "in-progress", "todo", "backlog":
		return true
	}
	if resolution != "" {
		return false
	}
	switch status {
	case "resolved", "closed", "fixed", "done", "completed":
		return false
	}
	return true
}

func Claimed(key string, prTitles []string) bool {
	if key == "" {
		return false
	}
	marker := "[triage:" + key + "]"
	for _, title := range prTitles {
		if strings.Contains(title, marker) {
			return true
		}
	}
	return false
}

func keyed(keys []string, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	for _, candidate := range keys {
		if strings.TrimSpace(candidate) == key {
			return true
		}
	}
	return false
}

// Pick keeps the original picker contract for callers that only know about
// open PR claims. The unattended loop uses PickWithLocalState so merged PRs
// can stand in for Agent Orchestrator while it is unavailable.
func Pick(groups []TriageGroup, prTitles []string) (TriageGroup, bool) {
	return PickWithLocalState(groups, prTitles, nil, nil)
}

// PickWithLocalState combines the remote issue lifecycle with local GitHub
// evidence. repairedKeys have a merged [triage:key] PR but the fingerprint's
// last_observed_revision does not contain that merge, so they stay fixed.
// regressedKeys were observed again on a revision that contains the merged
// repair. A later attempt of the same run is not that observation.
func PickWithLocalState(groups []TriageGroup, prTitles, repairedKeys, regressedKeys []string) (TriageGroup, bool) {
	best := TriageGroup{}
	found := false
	for _, g := range groups {
		key := strings.TrimSpace(g.Key)
		if key == "" || Claimed(key, prTitles) || keyed(repairedKeys, key) {
			continue
		}
		if !Actionable(g) && !keyed(regressedKeys, key) {
			continue
		}
		gCircuit := g.Issue != nil && g.Issue.CircuitOpen
		bestCircuit := best.Issue != nil && best.Issue.CircuitOpen
		if !found || (gCircuit && !bestCircuit) || (gCircuit == bestCircuit && g.Count > best.Count) {
			best = g
			found = true
		}
	}
	return best, found
}

// RepairObservation is one merged [triage:key] repair and the revision that
// last produced that fingerprint. ObservedRevision is not the run's latest
// finish: a later attempt can fail for a different reason on a newer build.
type RepairObservation struct {
	Key              string `json:"key"`
	MergeSHA         string `json:"merge_sha"`
	ObservedRevision string `json:"observed_revision"`
}

// ClassifyRepairs marks a merged repair as a regression only when git can
// prove the fingerprint's observed revision contains the merge commit.
// A missing revision, a missing object, or a failed ancestry check stays
// repaired so a closed issue is not investigated again.
func ClassifyRepairs(repo string, rows []RepairObservation) (repaired, regressed []string) {
	for _, row := range rows {
		key := strings.TrimSpace(row.Key)
		if key == "" {
			continue
		}
		if repairRegressed(repo, row) {
			regressed = append(regressed, key)
			continue
		}
		repaired = append(repaired, key)
	}
	sort.Strings(repaired)
	sort.Strings(regressed)
	return repaired, regressed
}

func repairRegressed(repo string, row RepairObservation) bool {
	observed := strings.TrimSpace(row.ObservedRevision)
	merge := strings.TrimSpace(row.MergeSHA)
	if observed == "" || merge == "" || strings.TrimSpace(repo) == "" {
		return false
	}
	cmd := exec.Command("git", "-C", repo, "merge-base", "--is-ancestor", merge, observed)
	return cmd.Run() == nil
}

var triageKeyPattern = regexp.MustCompile(`\[triage:([^\]]+)\]`)

func TriageKeyFromTitle(title string) string {
	match := triageKeyPattern.FindStringSubmatch(title)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

type PullCheck struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

func (c PullCheck) Pending() bool {
	switch strings.ToUpper(strings.TrimSpace(c.Status)) {
	case "", "COMPLETED":
		return false
	default:
		return true
	}
}

func (c PullCheck) Failed() bool {
	switch strings.ToUpper(strings.TrimSpace(c.Conclusion)) {
	case "FAILURE", "TIMED_OUT", "STARTUP_FAILURE", "ERROR":
		return true
	default:
		return false
	}
}

type OpenPullRequest struct {
	Number  int         `json:"number"`
	Title   string      `json:"title"`
	HeadRef string      `json:"headRefName"`
	URL     string      `json:"url"`
	Checks  []PullCheck `json:"checks,omitempty"`
}

func (pr OpenPullRequest) FailingChecks() string {
	names := make([]string, 0, len(pr.Checks))
	for _, check := range pr.Checks {
		if !check.Failed() {
			continue
		}
		name := strings.TrimSpace(check.Name)
		if name == "" {
			name = "check"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// PickOwnPRFailure returns the oldest open triage PR whose checks have
// finished and failed. Pending checks stay claimed. Green checks stay
// claimed. Pull requests without a [triage:key] marker are not this loop's
// repairs.
func PickOwnPRFailure(prs []OpenPullRequest) (OpenPullRequest, bool) {
	best := OpenPullRequest{}
	found := false
	for _, pr := range prs {
		if !ownPRFailed(pr) {
			continue
		}
		if !found || pr.Number < best.Number {
			best = pr
			found = true
		}
	}
	return best, found
}

func ownPRFailed(pr OpenPullRequest) bool {
	if TriageKeyFromTitle(pr.Title) == "" {
		return false
	}
	failed := false
	for _, check := range pr.Checks {
		if check.Pending() {
			return false
		}
		if check.Failed() {
			failed = true
		}
	}
	return failed
}

func DecodePullRequests(raw []byte) ([]OpenPullRequest, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty pull request payload")
	}
	var rows []struct {
		Number  int              `json:"number"`
		Title   string           `json:"title"`
		HeadRef string           `json:"headRefName"`
		URL     string           `json:"url"`
		Rollup  []map[string]any `json:"statusCheckRollup"`
		Checks  []PullCheck      `json:"checks"`
	}
	if err := json.Unmarshal(trimmed, &rows); err != nil {
		return nil, err
	}
	out := make([]OpenPullRequest, 0, len(rows))
	for _, row := range rows {
		pr := OpenPullRequest{
			Number:  row.Number,
			Title:   row.Title,
			HeadRef: row.HeadRef,
			URL:     row.URL,
		}
		if len(row.Checks) > 0 {
			pr.Checks = row.Checks
		} else {
			pr.Checks = checksFromRollup(row.Rollup)
		}
		out = append(out, pr)
	}
	return out, nil
}

func checksFromRollup(items []map[string]any) []PullCheck {
	out := make([]PullCheck, 0, len(items))
	for _, item := range items {
		name, _ := item["name"].(string)
		if name == "" {
			name, _ = item["context"].(string)
		}
		if state, ok := item["state"].(string); ok && strings.TrimSpace(state) != "" && item["conclusion"] == nil {
			status := "COMPLETED"
			conclusion := strings.ToUpper(strings.TrimSpace(state))
			if conclusion == "PENDING" {
				status = "PENDING"
				conclusion = ""
			}
			out = append(out, PullCheck{Name: name, Status: status, Conclusion: conclusion})
			continue
		}
		status, _ := item["status"].(string)
		conclusion, _ := item["conclusion"].(string)
		out = append(out, PullCheck{Name: name, Status: status, Conclusion: conclusion})
	}
	return out
}
