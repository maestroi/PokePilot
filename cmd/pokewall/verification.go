package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

const (
	defaultIssueVerificationEvery = 30 * time.Second
	defaultIssueVerificationRuns  = 10
	defaultIssueVerificationQuiet = 6 * time.Hour
)

const (
	verificationVerifying = "verifying"
	verificationVerified  = "verified"
	verificationRegressed = "regressed"
)

type verificationTile struct {
	RunID    string
	Planner  string
	Starter  string
	Goal     string
	QueuedAt time.Time
	EndedAt  time.Time
	Attempts int
	Finished bool
	Reason   string
	Detail   string
}

// RunIssueVerification keeps local proof that a remotely fixed issue stayed
// fixed in later farm runs. Agent Orchestrator remains the issue-lifecycle
// authority; this loop only annotates IssueLink with post-fix evidence.
func (w *Wall) RunIssueVerification(every time.Duration) {
	if every <= 0 {
		every = defaultIssueVerificationEvery
	}
	w.refreshIssueVerifications(time.Now())
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for now := range ticker.C {
		w.refreshIssueVerifications(now)
	}
}

func (w *Wall) refreshIssueVerifications(now time.Time) {
	links, tiles := w.verificationSnapshot()
	changed := false
	for key, link := range links {
		next, ok := w.refreshOneIssueVerification(link, tiles, now)
		if !ok || next == link {
			continue
		}
		w.mu.Lock()
		cur, exists := w.issueLinks[key]
		// Do not overwrite a newer status-sync result or a newly reported
		// recurrence with a snapshot computed outside the wall lock.
		if exists && cur.IssueID == link.IssueID && cur.FixedRevision == link.FixedRevision && cur.Status == link.Status && cur.Resolution == link.Resolution && cur.OccurrenceCount == link.OccurrenceCount {
			copyIssueVerification(&cur, next)
			w.issueLinks[key] = cur
			changed = true
		}
		w.mu.Unlock()
	}
	if changed {
		w.saveState()
	}
}

func (w *Wall) verificationSnapshot() (map[string]IssueLink, []verificationTile) {
	w.mu.Lock()
	defer w.mu.Unlock()
	links := copyIssueLink(w.issueLinks)
	tiles := make([]verificationTile, 0, len(w.order))
	for _, id := range w.order {
		t := w.tiles[id]
		if t == nil {
			continue
		}
		tiles = append(tiles, verificationTile{
			RunID: t.RunID, Planner: t.Planner, Starter: t.Starter, Goal: t.Goal,
			QueuedAt: t.QueuedAt, EndedAt: t.EndedAt, Attempts: t.Attempts,
			Finished: t.Finished, Reason: t.Reason, Detail: t.Detail,
		})
	}
	return links, tiles
}

func (w *Wall) refreshOneIssueVerification(link IssueLink, tiles []verificationTile, now time.Time) (IssueLink, bool) {
	if link.IssueID == "" {
		return link, false
	}
	fixed := issueFixedForVerification(link)
	if !fixed {
		if link.VerificationRevision != "" && issueStatusActive(link.Status) && link.VerificationState != verificationRegressed {
			link.VerificationState = verificationRegressed
			link.VerificationCleanRuns = 0
			link.VerificationLastCleanRun = ""
			link.VerificationLastCleanAt = 0
			link.VerificationVerifiedAt = 0
			return link, true
		}
		return link, false
	}

	fixedRevision := strings.TrimSpace(link.FixedRevision)
	if fixedRevision == "" {
		return link, false
	}
	if link.VerificationRevision != fixedRevision {
		link = resetIssueVerification(link, fixedRevision, now)
		if baseline, ok := findVerificationTile(tiles, link.LastObservedRun); ok {
			link.VerificationPlanner = baseline.Planner
			link.VerificationStarter = baseline.Starter
			link.VerificationGoal = baseline.Goal
			if report, err := w.loadVerificationFinish(baseline); err == nil && report.ProgressFinal != nil {
				link.VerificationHasBaseline = true
				link.VerificationBaselineBadges = report.ProgressFinal.Badges
				link.VerificationBaselineEvents = report.ProgressFinal.Events
				link.VerificationBaselineMaps = report.ProgressFinal.Maps
			}
		}
	}
	// A recurrence on the same fixed revision is a failed verification. Wait
	// for Agent Orchestrator to record a new fixed revision before retrying.
	if link.VerificationState == verificationRegressed {
		return link, true
	}

	cleanRuns := 0
	lastCleanRun := ""
	var lastCleanAt int64
	for _, tile := range tiles {
		if !verificationTileMatches(link, tile) {
			continue
		}
		report, err := w.loadVerificationFinish(tile)
		if err != nil {
			continue
		}
		if report.RunnerVersion == "" {
			// Legacy dumps cannot prove which runner build was exercised.
			continue
		}
		if verificationReportReproduces(link, report) {
			link.VerificationState = verificationRegressed
			link.VerificationCleanRuns = 0
			link.VerificationLastCleanRun = ""
			link.VerificationLastCleanAt = 0
			link.VerificationVerifiedAt = 0
			return link, true
		}
		if !verificationReportHadOpportunity(link, report) {
			continue
		}
		cleanRuns++
		ended := tile.EndedAt.Unix()
		if ended > lastCleanAt {
			lastCleanAt = ended
			lastCleanRun = tile.RunID
		}
	}

	link.VerificationCleanRuns = cleanRuns
	link.VerificationLastCleanRun = lastCleanRun
	link.VerificationLastCleanAt = lastCleanAt
	if link.VerificationRequiredRuns <= 0 {
		link.VerificationRequiredRuns = defaultIssueVerificationRuns
	}
	if link.VerificationQuietSeconds <= 0 {
		link.VerificationQuietSeconds = int64(defaultIssueVerificationQuiet / time.Second)
	}
	if cleanRuns >= link.VerificationRequiredRuns && now.Unix()-link.VerificationStartedAt >= link.VerificationQuietSeconds {
		link.VerificationState = verificationVerified
		if link.VerificationVerifiedAt == 0 {
			link.VerificationVerifiedAt = now.Unix()
		}
	} else {
		link.VerificationState = verificationVerifying
		link.VerificationVerifiedAt = 0
	}
	return link, true
}

func resetIssueVerification(link IssueLink, revision string, now time.Time) IssueLink {
	link.VerificationState = verificationVerifying
	link.VerificationRevision = revision
	link.VerificationStartedAt = now.Unix()
	link.VerificationOccurrenceCount = link.OccurrenceCount
	link.VerificationRequiredRuns = defaultIssueVerificationRuns
	link.VerificationCleanRuns = 0
	link.VerificationQuietSeconds = int64(defaultIssueVerificationQuiet / time.Second)
	link.VerificationVerifiedAt = 0
	link.VerificationLastCleanRun = ""
	link.VerificationLastCleanAt = 0
	link.VerificationPlanner = ""
	link.VerificationStarter = ""
	link.VerificationGoal = ""
	link.VerificationHasBaseline = false
	link.VerificationBaselineBadges = 0
	link.VerificationBaselineEvents = 0
	link.VerificationBaselineMaps = 0
	return link
}

func copyIssueVerification(dst *IssueLink, src IssueLink) {
	dst.VerificationState = src.VerificationState
	dst.VerificationRevision = src.VerificationRevision
	dst.VerificationStartedAt = src.VerificationStartedAt
	dst.VerificationOccurrenceCount = src.VerificationOccurrenceCount
	dst.VerificationRequiredRuns = src.VerificationRequiredRuns
	dst.VerificationCleanRuns = src.VerificationCleanRuns
	dst.VerificationQuietSeconds = src.VerificationQuietSeconds
	dst.VerificationVerifiedAt = src.VerificationVerifiedAt
	dst.VerificationLastCleanRun = src.VerificationLastCleanRun
	dst.VerificationLastCleanAt = src.VerificationLastCleanAt
	dst.VerificationPlanner = src.VerificationPlanner
	dst.VerificationStarter = src.VerificationStarter
	dst.VerificationGoal = src.VerificationGoal
	dst.VerificationHasBaseline = src.VerificationHasBaseline
	dst.VerificationBaselineBadges = src.VerificationBaselineBadges
	dst.VerificationBaselineEvents = src.VerificationBaselineEvents
	dst.VerificationBaselineMaps = src.VerificationBaselineMaps
}

func issueFixedForVerification(link IssueLink) bool {
	if ignoredResolution(strings.ToLower(strings.TrimSpace(link.Resolution))) {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(link.Status))
	resolution := strings.ToLower(strings.TrimSpace(link.Resolution))
	fixedStatus := status == "resolved" || status == "closed" || status == "fixed" || status == "fixed-applied" || status == "fixed_applied"
	fixedResolution := resolution == "fixed" || resolution == "fixed-applied" || resolution == "fixed_applied"
	return (fixedStatus || fixedResolution) && strings.TrimSpace(link.FixedRevision) != ""
}

func issueStatusActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "open", "actionable", "investigating", "reopened", "in_progress", "in-progress":
		return true
	default:
		return false
	}
}

func findVerificationTile(tiles []verificationTile, runID string) (verificationTile, bool) {
	for _, tile := range tiles {
		if tile.RunID == runID {
			return tile, true
		}
	}
	return verificationTile{}, false
}

func verificationTileMatches(link IssueLink, tile verificationTile) bool {
	if !tile.Finished || tile.RunID == "" || tile.RunID == link.LastObservedRun {
		return false
	}
	if tile.QueuedAt.IsZero() || tile.QueuedAt.Unix() < link.VerificationStartedAt {
		return false
	}
	if link.VerificationPlanner != "" && tile.Planner != link.VerificationPlanner {
		return false
	}
	if link.VerificationStarter != "" && tile.Starter != link.VerificationStarter {
		return false
	}
	if link.VerificationGoal != "" && tile.Goal != link.VerificationGoal {
		return false
	}
	return true
}

func verificationReportHadOpportunity(link IssueLink, report farm.FinishReport) bool {
	if !link.VerificationHasBaseline {
		return strings.EqualFold(strings.TrimSpace(report.Reason), "done")
	}
	p := report.ProgressFinal
	if p == nil {
		return false
	}
	if p.Badges > link.VerificationBaselineBadges {
		return true
	}
	if p.Badges < link.VerificationBaselineBadges {
		return false
	}
	if p.Events > link.VerificationBaselineEvents {
		return true
	}
	if p.Events < link.VerificationBaselineEvents {
		return false
	}
	return p.Maps > link.VerificationBaselineMaps
}

func verificationReportReproduces(link IssueLink, report farm.FinishReport) bool {
	if report.Detail == "" || (report.Reason != "error" && report.Reason != "lost") {
		return false
	}
	key, _ := failureIdentity(normalizeDetail(report.Detail))
	return key != "" && key == verificationKeyForLink(link)
}

func verificationKeyForLink(link IssueLink) string {
	// Structured fingerprints already carry a stable key marker in Detail, but
	// IssueLink intentionally stores only the full fingerprint. The caller's
	// map key is unavailable here, so derive the legacy key from the full
	// sha256 fingerprint when possible.
	const prefix = "sha256:"
	fp := strings.ToLower(strings.TrimSpace(link.Fingerprint))
	if strings.HasPrefix(fp, prefix) && len(fp) >= len(prefix)+16 {
		return fp[len(prefix) : len(prefix)+16]
	}
	return ""
}

func (w *Wall) loadVerificationFinish(tile verificationTile) (farm.FinishReport, error) {
	if w.dumpsDir == "" {
		return farm.FinishReport{}, os.ErrNotExist
	}
	attempt := tile.Attempts
	if attempt < 1 {
		attempt = 1
	}
	path := filepath.Join(w.dumpsDir, safeDumpName(tile.RunID))
	if attempt > 1 {
		path = filepath.Join(w.dumpsDir, safeBase(tile.RunID)+"-attempt-"+itoa(attempt)+".json")
	}
	data, err := os.ReadFile(path)
	if err != nil && attempt > 1 && errors.Is(err, os.ErrNotExist) {
		path = filepath.Join(w.dumpsDir, safeDumpName(tile.RunID))
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return farm.FinishReport{}, err
	}
	var report farm.FinishReport
	if err := json.Unmarshal(data, &report); err != nil {
		return farm.FinishReport{}, err
	}
	return report, nil
}

func itoa(v int) string {
	// strconv.Itoa without adding another import to call sites that only need
	// this tiny path component.
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
