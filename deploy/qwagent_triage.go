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

func Pick(groups []TriageGroup, prTitles []string) (TriageGroup, bool) {
	best := TriageGroup{}
	found := false
	for _, g := range groups {
		if strings.TrimSpace(g.Key) == "" || !Actionable(g) || Claimed(g.Key, prTitles) {
			continue
		}
		if !found || g.Count > best.Count {
			best = g
			found = true
		}
	}
	return best, found
}
