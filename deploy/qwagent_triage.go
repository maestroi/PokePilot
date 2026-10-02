package deploy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
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
	// FixedRevision is the issue system's resolution baseline: the default
	// branch head when the issue was closed. It covers fixes that landed
	// after the [triage:key] PR or without its marker (a manual close naming
	// a later commit), so it is the same rule pokeissues uses to reopen.
	FixedRevision string `json:"fixed_revision,omitempty"`
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
	contains := func(rev string) bool {
		return exec.Command("git", "-C", repo, "merge-base", "--is-ancestor", rev, observed).Run() == nil
	}
	if !contains(merge) {
		return false
	}
	fixed := strings.TrimSpace(row.FixedRevision)
	return fixed == "" || contains(fixed)
}

var triageKeyPattern = regexp.MustCompile(`\[triage:([^\]]+)\]`)

func TriageKeyFromTitle(title string) string {
	match := triageKeyPattern.FindStringSubmatch(title)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

// AssignedIssue is an open generated farm issue with at least one assignee.
type AssignedIssue struct {
	Number     int64
	Key        string
	Assignees  []string
	AssignedAt time.Time // latest assignment; zero when unknown
}

// DecodeAssignedIssues reads the GraphQL issue listing in qwagent-triage.sh
// and keeps assigned issues that carry a generated "Triage key" line.
func DecodeAssignedIssues(raw []byte) ([]AssignedIssue, error) {
	var resp struct {
		Data struct {
			Repository struct {
				Issues struct {
					Nodes []struct {
						Number    int64  `json:"number"`
						Body      string `json:"body"`
						Assignees struct {
							Nodes []struct {
								Login string `json:"login"`
							} `json:"nodes"`
						} `json:"assignees"`
						TimelineItems struct {
							Nodes []struct {
								CreatedAt time.Time `json:"createdAt"`
							} `json:"nodes"`
						} `json:"timelineItems"`
					} `json:"nodes"`
				} `json:"issues"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode assigned issues: %w (%s)", err, jsonPreview(raw))
	}
	var out []AssignedIssue
	for _, n := range resp.Data.Repository.Issues.Nodes {
		issue := AssignedIssue{Number: n.Number, Key: issueTriageKey(n.Body)}
		for _, a := range n.Assignees.Nodes {
			issue.Assignees = append(issue.Assignees, a.Login)
		}
		if items := n.TimelineItems.Nodes; len(items) > 0 {
			issue.AssignedAt = items[len(items)-1].CreatedAt
		}
		if issue.Key != "" && len(issue.Assignees) > 0 {
			out = append(out, issue)
		}
	}
	return out, nil
}

func issueTriageKey(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "- **Triage key:**") {
			continue
		}
		if parts := strings.Split(line, "`"); len(parts) >= 3 {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

// IssueClaims splits assigned farm issues into live claims and leaked ones.
// An assignment is the fixer's cross-machine claim, but nothing expires it: a
// hard-killed attempt skips its release, and a fixer PR closed unmerged leaves
// the claim it kept. So an assignment holds only while a [triage:key] PR is
// open or it is younger than ttl; anything older parks the failure forever.
// ponytail: a hand fix with no PR after ttl loses its claim; open a draft PR.
func IssueClaims(issues []AssignedIssue, prTitles []string, now time.Time, ttl time.Duration) (claimed []string, stale []AssignedIssue) {
	for _, issue := range issues {
		if Claimed(issue.Key, prTitles) || issue.AssignedAt.IsZero() || now.Sub(issue.AssignedAt) < ttl {
			claimed = append(claimed, issue.Key)
			continue
		}
		stale = append(stale, issue)
	}
	return claimed, stale
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
	// Mergeable is GitHub's MERGEABLE, CONFLICTING, or UNKNOWN.
	Mergeable string `json:"mergeable,omitempty"`
}

// mergeConflictCheck is the repair instruction for a PR that cannot merge.
const mergeConflictCheck = "merge conflict with main (merge origin/main into the branch, resolve, re-test, push)"

func (pr OpenPullRequest) conflicting() bool {
	return strings.EqualFold(strings.TrimSpace(pr.Mergeable), "CONFLICTING")
}

func (pr OpenPullRequest) FailingChecks() string {
	names := make([]string, 0, len(pr.Checks)+1)
	if pr.conflicting() {
		names = append(names, mergeConflictCheck)
	}
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
	// A conflicting PR never merges, and GitHub may never run its checks, so
	// it must not wait for them: repair it like a red check.
	if pr.conflicting() {
		return true
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
		Number    int              `json:"number"`
		Title     string           `json:"title"`
		HeadRef   string           `json:"headRefName"`
		URL       string           `json:"url"`
		Rollup    []map[string]any `json:"statusCheckRollup"`
		Checks    []PullCheck      `json:"checks"`
		Mergeable string           `json:"mergeable"`
	}
	if err := json.Unmarshal(trimmed, &rows); err != nil {
		return nil, err
	}
	out := make([]OpenPullRequest, 0, len(rows))
	for _, row := range rows {
		pr := OpenPullRequest{
			Number:    row.Number,
			Title:     row.Title,
			HeadRef:   row.HeadRef,
			URL:       row.URL,
			Mergeable: row.Mergeable,
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
