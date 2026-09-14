package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// outcomesStatsHandler reads the wall's narrow, streaming outcomes feed rather
// than the full dashboard. Aggregation stays O(group/profile/model cardinality)
// instead of O(total historical runs).
func outcomesStatsHandler(wallBase string) http.HandlerFunc {
	client := &http.Client{Timeout: proxyTimeout}
	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout)
		defer cancel()

		up, err := http.NewRequestWithContext(ctx, http.MethodGet, wallBase+"/v1/outcomes", nil)
		if err != nil {
			writeUnreachable(res)
			return
		}
		resp, err := client.Do(up)
		if err != nil {
			writeUnreachable(res)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			writeUnreachable(res)
			return
		}

		stats, err := summarizeOutcomeStream(json.NewDecoder(resp.Body))
		if err != nil {
			writeUnreachable(res)
			return
		}
		res.Header().Set("Content-Type", "application/json")
		res.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(res).Encode(stats)
	}
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
