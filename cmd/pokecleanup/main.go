package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	failurePatternCap = 128
	deleteConcurrency = 3
	failureMarker     = "failure-id:"
)

var (
	failureHexRE = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	failureNumRE = regexp.MustCompile(`\d+`)
	// failureMarkerRE keeps the base32 identity a triage group embeds. A
	// composed objective-failure pattern truncates it, so the shorter prefix
	// is still an identity — 12 characters of a 64-character marker is far
	// more than enough to be unique among live groups.
	failureMarkerRE = regexp.MustCompile(`failure-id:([a-p]{12,64})`)
	// failureMapSuffixRE strips the map discriminator pokewall appends after
	// normalizing, which no normalized run detail can carry.
	failureMapSuffixRE = regexp.MustCompile(`\s\|\smap=[0-9a-fA-F]{2}$`)
)

// failureReasons are the reasons that mean a run stopped on a failure. A
// cleanly finished run that still carries an old failure detail is not
// cleanup evidence and must never be deleted.
var failureReasons = map[string]bool{
	"error":  true,
	"lost":   true,
	"failed": true,
	"stuck":  true,
}

type triageGroup struct {
	Pattern string `json:"pattern"`
	Detail  string `json:"detail"`
	Key     string `json:"key"`
	Count   int    `json:"count"`
	Example string `json:"example"`
	Issue   *struct {
		IssueNumber int64  `json:"issue_number"`
		Status      string `json:"status"`
		Resolution  string `json:"resolution"`
	} `json:"issue,omitempty"`
}

type cleanupRun struct {
	RunID           string `json:"run_id"`
	Status          string `json:"status"`
	Reason          string `json:"reason"`
	Detail          string `json:"detail"`
	EndedAt         int64  `json:"ended_at"`
	ResumeProtected bool   `json:"resume_protected"`
}

type cleanupDashboard struct {
	Runs []cleanupRun `json:"runs"`
}

type deleteFailure struct {
	RunID string
	Err   error
}

type deleteProgress struct {
	Completed int
	Total     int
	Deleted   int
	Failed    int
}

func normalizeFailureDetail(detail string) string {
	s := failureHexRE.ReplaceAllString(detail, "<hex>")
	s = failureNumRE.ReplaceAllString(s, "<n>")
	if len(s) > failurePatternCap {
		s = s[:failurePatternCap]
	}
	return s
}

// groupIdentity is every identity a triage group advertises: the failure-id
// markers it carries and the normalized patterns it may equal.
type groupIdentity struct {
	tokens   []string
	patterns []string
}

func failureGroupIdentity(texts ...string) groupIdentity {
	var identity groupIdentity
	seenToken := map[string]bool{}
	seenPattern := map[string]bool{}
	for _, raw := range texts {
		text := strings.TrimSpace(raw)
		if text == "" {
			continue
		}
		for _, pattern := range []string{text, strings.TrimSpace(failureMapSuffixRE.ReplaceAllString(text, ""))} {
			if pattern == "" || seenPattern[pattern] {
				continue
			}
			seenPattern[pattern] = true
			identity.patterns = append(identity.patterns, pattern)
		}
		for _, match := range failureMarkerRE.FindAllStringSubmatch(text, -1) {
			if seenToken[match[1]] {
				continue
			}
			seenToken[match[1]] = true
			identity.tokens = append(identity.tokens, match[1])
		}
	}
	return identity
}

func runMatchesGroup(run cleanupRun, identity groupIdentity) bool {
	detail := strings.TrimSpace(run.Detail)
	if detail == "" {
		return false
	}
	for _, token := range identity.tokens {
		if strings.Contains(detail, failureMarker+token) {
			return true
		}
	}
	normalized := normalizeFailureDetail(detail)
	for _, pattern := range identity.patterns {
		if pattern == normalized {
			return true
		}
	}
	return false
}

// matchingRuns selects the finished runs of one triage group. The group's
// pattern may be a composed objective-failure pattern
// (normalizeDetail(objective + " | " + error)[:128] + " | map=xx") whose
// length no normalized run detail can equal, so the shared failure-id marker
// is the identity; groups without one fall back to normalized-pattern
// equality. Runs a live resume lineage still needs are never candidates.
func matchingRuns(runs []cleanupRun, group triageGroup) []cleanupRun {
	identity := failureGroupIdentity(group.Pattern, group.Detail, group.Example)
	matched := make([]cleanupRun, 0)
	for _, run := range runs {
		if run.Status != "done" || run.ResumeProtected {
			continue
		}
		if !failureReasons[strings.ToLower(strings.TrimSpace(run.Reason))] {
			continue
		}
		if runMatchesGroup(run, identity) {
			matched = append(matched, run)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].EndedAt < matched[j].EndedAt })
	return matched
}

func getJSON(ctx context.Context, client *http.Client, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 16<<10))
		return fmt.Errorf("GET %s returned %s: %s", endpoint, res.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", endpoint, err)
	}
	return nil
}

func selectTriageGroup(groups []triageGroup, key string, issueNumber int64) (triageGroup, error) {
	key = strings.TrimSpace(key)
	if key != "" && issueNumber > 0 {
		return triageGroup{}, errors.New("use either -key or -issue, not both")
	}
	if key == "" && issueNumber <= 0 {
		return triageGroup{}, errors.New("triage key or issue number is required")
	}
	for _, candidate := range groups {
		if key != "" && candidate.Key == key {
			return candidate, nil
		}
		if issueNumber > 0 && candidate.Issue != nil && candidate.Issue.IssueNumber == issueNumber {
			return candidate, nil
		}
	}
	if issueNumber > 0 {
		return triageGroup{}, fmt.Errorf("issue #%d has no triage failure group", issueNumber)
	}
	if _, err := fmt.Sscan(key, new(int64)); err == nil && len(key) < 16 {
		return triageGroup{}, fmt.Errorf("triage key %q not found; if %q is an issue number, use -issue %s", key, key, key)
	}
	return triageGroup{}, fmt.Errorf("triage key %q not found", key)
}

func planCleanup(ctx context.Context, client *http.Client, base, key string, issueNumber int64) (triageGroup, []cleanupRun, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return triageGroup{}, nil, errors.New("base URL is required")
	}

	var groups []triageGroup
	if err := getJSON(ctx, client, base+"/v1/triage", &groups); err != nil {
		return triageGroup{}, nil, err
	}
	group, err := selectTriageGroup(groups, key, issueNumber)
	if err != nil {
		return triageGroup{}, nil, err
	}
	if group.Pattern == "" {
		return triageGroup{}, nil, fmt.Errorf("triage key %q has no failure pattern", group.Key)
	}

	var dashboard cleanupDashboard
	if err := getJSON(ctx, client, base+"/v1/dashboard?status=done", &dashboard); err != nil {
		return triageGroup{}, nil, err
	}
	return group, matchingRuns(dashboard.Runs, group), nil
}

func deleteOne(ctx context.Context, client *http.Client, base, runID string) error {
	endpoint := strings.TrimRight(base, "/") + "/v1/runs/" + url.PathEscape(runID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 16<<10))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = res.Status
		}
		return errors.New(message)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

func deleteRuns(ctx context.Context, client *http.Client, base string, runs []cleanupRun, concurrency int, onProgress func(deleteProgress)) ([]string, []deleteFailure) {
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(runs) && len(runs) > 0 {
		concurrency = len(runs)
	}
	jobs := make(chan cleanupRun)
	var mu sync.Mutex
	deleted := make([]string, 0, len(runs))
	failures := make([]deleteFailure, 0)
	completed := 0
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for run := range jobs {
				err := deleteOne(ctx, client, base, run.RunID)
				mu.Lock()
				if err != nil {
					failures = append(failures, deleteFailure{RunID: run.RunID, Err: err})
				} else {
					deleted = append(deleted, run.RunID)
				}
				completed++
				if onProgress != nil {
					onProgress(deleteProgress{Completed: completed, Total: len(runs), Deleted: len(deleted), Failed: len(failures)})
				}
				mu.Unlock()
			}
		}()
	}
	for _, run := range runs {
		select {
		case jobs <- run:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return deleted, append(failures, deleteFailure{Err: ctx.Err()})
		}
	}
	close(jobs)
	wg.Wait()
	sort.Strings(deleted)
	sort.SliceStable(failures, func(i, j int) bool { return failures[i].RunID < failures[j].RunID })
	return deleted, failures
}

func issueState(group triageGroup) string {
	if group.Issue == nil {
		return "unlinked"
	}
	if resolution := strings.TrimSpace(group.Issue.Resolution); resolution != "" {
		return resolution
	}
	if status := strings.TrimSpace(group.Issue.Status); status != "" {
		return status
	}
	return "linked"
}

func run(ctx context.Context, client *http.Client, base, key string, issueNumber int64, apply bool, out io.Writer) error {
	group, runs, err := planCleanup(ctx, client, base, key, issueNumber)
	if err != nil {
		return err
	}
	issueLabel := ""
	if group.Issue != nil && group.Issue.IssueNumber > 0 {
		issueLabel = fmt.Sprintf(" · issue #%d", group.Issue.IssueNumber)
	}
	fmt.Fprintf(out, "triage %s%s · %s · %d recorded occurrence(s)\n", group.Key, issueLabel, issueState(group), group.Count)
	fmt.Fprintf(out, "pattern: %s\n", group.Pattern)
	fmt.Fprintf(out, "matching finished runs: %d\n", len(runs))
	for _, run := range runs {
		fmt.Fprintf(out, "  %s\n", run.RunID)
	}
	if len(runs) == 0 {
		return nil
	}
	if !apply {
		fmt.Fprintln(out, "dry run only; rerun with -yes to permanently delete these runs and their artifacts")
		return nil
	}

	deleted, failures := deleteRuns(ctx, client, base, runs, deleteConcurrency, func(progress deleteProgress) {
		fmt.Fprintf(out, "\rDeleting %d / %d · %d deleted", progress.Completed, progress.Total, progress.Deleted)
		if progress.Failed > 0 {
			fmt.Fprintf(out, " · %d failed", progress.Failed)
		}
	})
	fmt.Fprintln(out)
	fmt.Fprintf(out, "deleted: %d\n", len(deleted))
	if len(failures) == 0 {
		return nil
	}
	for _, failure := range failures {
		if failure.RunID == "" {
			fmt.Fprintf(out, "  cleanup stopped: %v\n", failure.Err)
		} else {
			fmt.Fprintf(out, "  failed %s: %v\n", failure.RunID, failure.Err)
		}
	}
	return fmt.Errorf("%d run deletion(s) failed", len(failures))
}

func main() {
	base := flag.String("url", "https://admin.rompilot.app", "private RomPilot operator URL")
	key := flag.String("key", "", "triage failure key to clean")
	issueNumber := flag.Int64("issue", 0, "Agent Orchestrator issue number whose triage group should be cleaned")
	yes := flag.Bool("yes", false, "permanently delete matching finished runs and their S3/replay artifacts")
	flag.Parse()

	client := &http.Client{Timeout: 35 * time.Second}
	if err := run(context.Background(), client, *base, *key, *issueNumber, *yes, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "pokecleanup:", err)
		os.Exit(1)
	}
}
