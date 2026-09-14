package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// The catalog projection is deliberately allowed to finish independently of
	// a browser request. A cold production catalog has thousands of historical
	// rows; tying the refresh to req.Context meant a proxy/browser disconnect
	// cancelled the only refresh before it could ever populate the cache.
	outcomeStatsTimeout     = 2 * time.Minute
	outcomeStatsCacheTTL    = 30 * time.Second
	outcomeStatsInitialWait = 2 * time.Second
	outcomeStatsRetryDelay  = 5 * time.Second
)

type outcomeStatsCache struct {
	sync.Mutex
	payload      []byte
	refreshAfter time.Time
	stale        bool
	refreshing   bool
	ready        chan struct{}
	lastErr      error
}

// outcomesStatsHandler reads the wall's narrow, streaming outcomes feed rather
// than the full dashboard. Refresh work is detached from the browser request:
// once one poll starts a catalog scan, later polls either use the last snapshot
// or briefly report a warming snapshot while the same background scan finishes.
// This prevents a slow cold scan from turning the three-second browser poll into
// an endless chain of cancelled scans and 502s.
func outcomesStatsHandler(wallBase string) http.HandlerFunc {
	return outcomesStatsHandlerWithPolicyAndWait(
		wallBase,
		outcomeStatsTimeout,
		outcomeStatsCacheTTL,
		outcomeStatsInitialWait,
	)
}

func outcomesStatsHandlerWithPolicy(wallBase string, timeout, cacheTTL time.Duration) http.HandlerFunc {
	wait := outcomeStatsInitialWait
	if timeout > 0 && timeout < wait {
		wait = timeout
	}
	return outcomesStatsHandlerWithPolicyAndWait(wallBase, timeout, cacheTTL, wait)
}

func outcomesStatsHandlerWithPolicyAndWait(wallBase string, timeout, cacheTTL, initialWait time.Duration) http.HandlerFunc {
	client := &http.Client{Timeout: timeout}
	var cache outcomeStatsCache

	startRefreshLocked := func() chan struct{} {
		if cache.refreshing {
			return cache.ready
		}
		ready := make(chan struct{})
		cache.refreshing = true
		cache.ready = ready

		go func() {
			payload, err := loadOutcomeStats(client, wallBase, timeout)
			now := time.Now()

			cache.Lock()
			defer cache.Unlock()
			if err == nil {
				cache.payload = payload
				cache.refreshAfter = now.Add(cacheTTL)
				cache.stale = false
				cache.lastErr = nil
			} else {
				cache.stale = len(cache.payload) > 0
				cache.lastErr = err
				delay := cacheTTL
				if delay <= 0 {
					delay = outcomeStatsRetryDelay
				}
				cache.refreshAfter = now.Add(delay)
			}
			cache.refreshing = false
			close(ready)
			cache.ready = nil
		}()
		return ready
	}

	return func(res http.ResponseWriter, req *http.Request) {
		now := time.Now()

		cache.Lock()
		payload := append([]byte(nil), cache.payload...)
		stale := cache.stale
		refreshAfter := cache.refreshAfter
		lastErr := cache.lastErr

		// A fresh snapshot never waits on the wall.
		if len(payload) > 0 && now.Before(refreshAfter) && !stale {
			cache.Unlock()
			writeOutcomeStats(res, payload, false)
			return
		}

		var ready chan struct{}
		if cache.refreshing {
			ready = cache.ready
		} else if refreshAfter.IsZero() || !now.Before(refreshAfter) {
			ready = startRefreshLocked()
		}
		cache.Unlock()

		// Once any good snapshot exists, never make a browser poll wait for a
		// refresh. The previous value is explicitly marked stale until the
		// background scan replaces it.
		if len(payload) > 0 {
			writeOutcomeStats(res, payload, true)
			return
		}

		// A recent failed cold refresh gets a short backoff instead of spawning a
		// new full catalog scan on every three-second browser poll.
		if ready == nil && lastErr != nil {
			writeUnreachable(res)
			return
		}

		// Give a fast/cached wall a chance to answer the first request normally.
		// If it is genuinely cold, release the browser after a small bounded wait
		// while the detached scan continues to completion in the background.
		if ready != nil && initialWait > 0 {
			timer := time.NewTimer(initialWait)
			defer timer.Stop()
			select {
			case <-ready:
				cache.Lock()
				payload = append([]byte(nil), cache.payload...)
				stale = cache.stale
				lastErr = cache.lastErr
				cache.Unlock()
				if len(payload) > 0 {
					writeOutcomeStats(res, payload, stale)
					return
				}
				if lastErr != nil {
					writeUnreachable(res)
					return
				}
			case <-timer.C:
				// The refresh owns a background context, so returning here does not
				// cancel it. A later poll will pick up the completed snapshot.
			}
		}

		writeOutcomeStatsWarming(res)
	}
}

func loadOutcomeStats(client *http.Client, wallBase string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	up, err := http.NewRequestWithContext(ctx, http.MethodGet, wallBase+"/v1/outcomes", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(up)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("outcomes status %d", resp.StatusCode)
	}

	stats, err := summarizeOutcomeStream(json.NewDecoder(resp.Body))
	if err != nil {
		return nil, err
	}
	return json.Marshal(stats)
}

func writeOutcomeStats(res http.ResponseWriter, payload []byte, stale bool) {
	res.Header().Set("Content-Type", "application/json")
	res.Header().Set("Cache-Control", "no-store")
	if stale {
		res.Header().Set("X-PokePilot-Stats-Stale", "true")
	}
	_, _ = res.Write(payload)
}

func writeOutcomeStatsWarming(res http.ResponseWriter) {
	payload, err := json.Marshal(farmOutcomeStats{})
	if err != nil {
		writeUnreachable(res)
		return
	}
	res.Header().Set("X-PokePilot-Stats-Warming", "true")
	writeOutcomeStats(res, payload, true)
}

func summarizeOutcomeStream(dec *json.Decoder) (farmOutcomeStats, error) {
	var out farmOutcomeStats
	reasons := map[string]int{}
	badges := make([]int, 9)
	groups := map[string]*endlessExperimentStats{}
	profiles := map[string]*llmProfileAccumulator{}
	models := map[string]*llmModelStats{}
	var llmWeights llmWeightSums

	start, err := dec.Token()
	if err != nil || start != json.Delim('{') {
		return out, fmt.Errorf("bad outcomes object")
	}
	foundRuns := false
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return out, err
		}
		key, _ := keyToken.(string)
		if key != "runs" {
			var discard any
			if err := dec.Decode(&discard); err != nil {
				return out, err
			}
			continue
		}
		foundRuns = true
		open, err := dec.Token()
		if err != nil || open != json.Delim('[') {
			return out, fmt.Errorf("bad outcomes runs array")
		}
		for dec.More() {
			var run statsRun
			if err := dec.Decode(&run); err != nil {
				return out, err
			}
			addOutcomeRun(&out, reasons, badges, groups, profiles, models, &llmWeights, run)
		}
		if closeToken, err := dec.Token(); err != nil || closeToken != json.Delim(']') {
			return out, fmt.Errorf("bad outcomes runs terminator")
		}
	}
	if _, err := dec.Token(); err != nil {
		return out, err
	}
	if !foundRuns {
		return out, fmt.Errorf("outcomes response has no runs")
	}

	finalizeLLMStats(&out.LLM, llmWeights, profiles, models)
	out.TerminalReasons = sortedCounts(reasons)
	out.BadgeDistribution = make([]badgeBucket, 0, len(badges))
	for i, count := range badges {
		out.BadgeDistribution = append(out.BadgeDistribution, badgeBucket{Badges: i, Count: count})
	}
	out.EndlessExperiments = make([]endlessExperimentStats, 0, len(groups))
	for _, group := range groups {
		out.EndlessExperiments = append(out.EndlessExperiments, *group)
	}
	sort.Slice(out.EndlessExperiments, func(i, j int) bool {
		a, b := out.EndlessExperiments[i], out.EndlessExperiments[j]
		if a.CompletedAttempts != b.CompletedAttempts {
			return a.CompletedAttempts > b.CompletedAttempts
		}
		return a.Key < b.Key
	})
	return out, nil
}

func addOutcomeRun(
	out *farmOutcomeStats,
	reasons map[string]int,
	badges []int,
	groups map[string]*endlessExperimentStats,
	profiles map[string]*llmProfileAccumulator,
	models map[string]*llmModelStats,
	llmWeights *llmWeightSums,
	run statsRun,
) {
	attempts := completedAttempts(run)
	out.CompletedAttempts += attempts
	out.RetryableFailureAttempts += retryableFailures(run, attempts)
	addLLMStats(&out.LLM, llmWeights, profiles, models, run)

	settled := isSettled(run)
	if settled {
		out.SettledRuns++
		reason := strings.TrimSpace(run.Reason)
		if reason == "" {
			reason = "unknown"
		}
		reasons[reason]++
	}
	if settled && run.Player != nil {
		n := len(run.Player.Badges)
		if n > 8 {
			n = 8
		}
		out.UsableProgressRuns++
		badges[n]++
		if n > 0 {
			out.AtLeastOneBadge++
		}
		if n > out.BestBadges {
			out.BestBadges = n
		}
	}
	if settled && goalTracked(run.Stats) {
		out.GoalTrackedRuns++
		if run.Stats.GoalComplete {
			out.GoalWins++
		}
	}

	if run.Endless {
		key := endlessKey(run)
		group := groups[key]
		if group == nil {
			group = &endlessExperimentStats{
				Key: key, Planner: run.Planner, Starter: run.Starter, Goal: run.Goal,
				LLMProfile: run.LLMProfile, MaxRounds: run.MaxRounds, MaxFrames: run.MaxFrames,
				RandomSeed: run.RandomSeed,
			}
			groups[key] = group
		}
		addRunToEndless(group, run, attempts)
	}
}
