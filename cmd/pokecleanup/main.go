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
)

var (
	failureHexRE = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	failureNumRE = regexp.MustCompile(`\d+`)
)

type triageGroup struct {
	Pattern string `json:"pattern"`
	Key     string `json:"key"`
	Count   int    `json:"count"`
	Example string `json:"example"`
	Issue   *struct {
		Status     string `json:"status"`
		Resolution string `json:"resolution"`
	} `json:"issue,omitempty"`
}

type cleanupRun struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Detail  string `json:"detail"`
	EndedAt int64  `json:"ended_at"`
}

type cleanupDashboard struct {
	Runs []cleanupRun `json:"runs"`
}

type deleteFailure struct {
	RunID string
	Err   error
}

func normalizeFailureDetail(detail string) string {
	s := failureHexRE.ReplaceAllString(detail, "<hex>")
	s = failureNumRE.ReplaceAllString(s, "<n>")
	if len(s) > failurePatternCap {
		s = s[:failurePatternCap]
	}
	return s
}

func matchingRuns(runs []cleanupRun, pattern string) []cleanupRun {
	if pattern == "" {
		return nil
	}
	matched := make([]cleanupRun, 0)
	for _, run := range runs {
		if run.Status != "done" || (run.Reason != "error" && run.Reason != "lost") || run.Detail == "" {
			continue
		}
		if normalizeFailureDetail(run.Detail) == pattern {
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

func planCleanup(ctx context.Context, client *http.Client, base, key string) (triageGroup, []cleanupRun, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	key = strings.TrimSpace(key)
	if base == "" {
		return triageGroup{}, nil, errors.New("base URL is required")
	}
	if key == "" {
		return triageGroup{}, nil, errors.New("triage key is required")
	}

	var groups []triageGroup
	if err := getJSON(ctx, client, base+"/v1/triage", &groups); err != nil {
		return triageGroup{}, nil, err
	}
	var group triageGroup
	found := false
	for _, candidate := range groups {
		if candidate.Key == key {
			group = candidate
			found = true
			break
		}
	}
	if !found {
		return triageGroup{}, nil, fmt.Errorf("triage key %q not found", key)
	}
	if group.Pattern == "" {
		return triageGroup{}, nil, fmt.Errorf("triage key %q has no failure pattern", key)
	}

	var dashboard cleanupDashboard
	if err := getJSON(ctx, client, base+"/v1/dashboard?status=done", &dashboard); err != nil {
		return triageGroup{}, nil, err
	}
	return group, matchingRuns(dashboard.Runs, group.Pattern), nil
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

func deleteRuns(ctx context.Context, client *http.Client, base string, runs []cleanupRun, concurrency int) ([]string, []deleteFailure) {
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
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for run := range jobs {
				if err := deleteOne(ctx, client, base, run.RunID); err != nil {
					mu.Lock()
					failures = append(failures, deleteFailure{RunID: run.RunID, Err: err})
					mu.Unlock()
					continue
				}
				mu.Lock()
				deleted = append(deleted, run.RunID)
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

func run(ctx context.Context, client *http.Client, base, key string, apply bool, out io.Writer) error {
	group, runs, err := planCleanup(ctx, client, base, key)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "triage %s · %s · %d recorded occurrence(s)\n", group.Key, issueState(group), group.Count)
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

	deleted, failures := deleteRuns(ctx, client, base, runs, deleteConcurrency)
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
	base := flag.String("url", "https://pokemon.labstack.cc", "private PokePilot operator URL")
	key := flag.String("key", "", "triage failure key to clean")
	yes := flag.Bool("yes", false, "permanently delete matching finished runs and their S3/replay artifacts")
	flag.Parse()

	client := &http.Client{Timeout: 35 * time.Second}
	if err := run(context.Background(), client, *base, *key, *yes, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "pokecleanup:", err)
		os.Exit(1)
	}
}
