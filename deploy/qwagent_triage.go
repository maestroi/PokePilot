package deploy

import "strings"

type TriageIssue struct {
	Status          string `json:"status"`
	Resolution      string `json:"resolution"`
	OccurrenceCount int64  `json:"occurrence_count"`
	FixedRevision   string `json:"fixed_revision"`
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
// evidence. repairedKeys have a merged [triage:key] PR but the representative
// failure was produced by a revision that predates that merge, so they are
// fixed-or-waiting-for-deploy and must not be offered again. regressedKeys have
// a representative failure from a revision containing the merged repair; that
// concrete post-fix recurrence overrides stale resolved issue metadata.
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
		if !found || g.Count > best.Count {
			best = g
			found = true
		}
	}
	return best, found
}
