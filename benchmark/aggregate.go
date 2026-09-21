package benchmark

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type Distribution struct {
	Count  int     `json:"count"`
	Median float64 `json:"median"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

type Aggregate struct {
	Version             int                     `json:"version"`
	Runs                int                     `json:"runs"`
	Completed           int                     `json:"completed"`
	CompletionRate      float64                 `json:"completion_rate"`
	MedianFrames        uint64                  `json:"median_frames_completed,omitempty"`
	MinFrames           uint64                  `json:"min_frames_completed,omitempty"`
	MaxFrames           uint64                  `json:"max_frames_completed,omitempty"`
	MedianWallSeconds   float64                 `json:"median_wall_seconds_completed,omitempty"`
	FailureFingerprints map[string]int          `json:"failure_fingerprints,omitempty"`
	MilestoneFrames     map[string]Distribution `json:"milestone_frames_since_previous,omitempty"`
	Counters            map[string]Distribution `json:"counters,omitempty"`
	ModelCalls          Distribution            `json:"model_calls"`
	StrategistCalls     Distribution            `json:"strategist_calls"`
}

func Summarize(results []Result) Aggregate {
	agg := Aggregate{
		Version: ResultVersion, Runs: len(results),
		FailureFingerprints: map[string]int{},
		MilestoneFrames:     map[string]Distribution{},
		Counters:            map[string]Distribution{},
	}
	var completedFrames []float64
	var completedWalls []float64
	milestones := map[string][]float64{}
	counters := map[string][]float64{}
	var modelCalls, strategistCalls []float64
	for _, result := range results {
		if result.Outcome == "completed" {
			agg.Completed++
			completedFrames = append(completedFrames, float64(result.Frames))
			completedWalls = append(completedWalls, result.WallSeconds)
		}
		for _, failure := range result.Failures {
			key := failure.Fingerprint
			if key == "" {
				key = failure.Cause
			}
			if key == "" {
				key = "unknown"
			}
			agg.FailureFingerprints[key]++
		}
		for _, split := range result.Milestones {
			if split.ID == "fresh_start" || split.ID == "checkpoint_start" {
				continue
			}
			milestones[split.ID] = append(milestones[split.ID], float64(split.FramesSincePrevious))
		}
		for name, value := range result.Counters {
			counters[name] = append(counters[name], float64(value))
		}
		modelCalls = append(modelCalls, float64(result.Model.Calls))
		strategistCalls = append(strategistCalls, float64(result.Model.StrategistCalls))
	}
	if agg.Runs > 0 {
		agg.CompletionRate = float64(agg.Completed) / float64(agg.Runs)
	}
	if len(completedFrames) > 0 {
		d := distribution(completedFrames)
		agg.MedianFrames = uint64(math.Round(d.Median))
		agg.MinFrames = uint64(math.Round(d.Min))
		agg.MaxFrames = uint64(math.Round(d.Max))
		agg.MedianWallSeconds = distribution(completedWalls).Median
	}
	for id, values := range milestones {
		agg.MilestoneFrames[id] = distribution(values)
	}
	for name, values := range counters {
		agg.Counters[name] = distribution(values)
	}
	agg.ModelCalls = distribution(modelCalls)
	agg.StrategistCalls = distribution(strategistCalls)
	return agg
}

func distribution(values []float64) Distribution {
	if len(values) == 0 {
		return Distribution{}
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	median := cp[len(cp)/2]
	if len(cp)%2 == 0 {
		median = (cp[len(cp)/2-1] + cp[len(cp)/2]) / 2
	}
	return Distribution{Count: len(cp), Median: median, Min: cp[0], Max: cp[len(cp)-1]}
}

func CompareText(baseline, candidate Aggregate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-24s %14s %14s %12s\n", "", "BASELINE", "CANDIDATE", "CHANGE")
	fmt.Fprintf(&b, "%-24s %14s %14s %12s\n", "Completion",
		fmt.Sprintf("%d/%d", baseline.Completed, baseline.Runs),
		fmt.Sprintf("%d/%d", candidate.Completed, candidate.Runs),
		completionChange(baseline, candidate))
	fmt.Fprintf(&b, "%-24s %14s %14s %12s\n", "Median frames",
		formatUint(baseline.MedianFrames), formatUint(candidate.MedianFrames),
		percentChange(float64(baseline.MedianFrames), float64(candidate.MedianFrames)))
	fmt.Fprintf(&b, "%-24s %14s %14s %12s\n", "Median wall",
		formatSeconds(baseline.MedianWallSeconds), formatSeconds(candidate.MedianWallSeconds),
		percentChange(baseline.MedianWallSeconds, candidate.MedianWallSeconds))
	fmt.Fprintf(&b, "%-24s %14.1f %14.1f %12s\n", "Strategist calls",
		baseline.StrategistCalls.Median, candidate.StrategistCalls.Median,
		percentChange(baseline.StrategistCalls.Median, candidate.StrategistCalls.Median))

	if candidate.CompletionRate < baseline.CompletionRate {
		fmt.Fprintf(&b, "\nRELIABILITY REGRESSION: completion rate %.1f%% -> %.1f%%.\n",
			100*baseline.CompletionRate, 100*candidate.CompletionRate)
	}

	type splitChange struct {
		id        string
		pct       float64
		base      float64
		candidate float64
	}
	var changes []splitChange
	for id, base := range baseline.MilestoneFrames {
		cand, ok := candidate.MilestoneFrames[id]
		if !ok || base.Median == 0 {
			continue
		}
		changes = append(changes, splitChange{id: id, pct: (cand.Median - base.Median) / base.Median * 100, base: base.Median, candidate: cand.Median})
	}
	sort.Slice(changes, func(i, j int) bool { return math.Abs(changes[i].pct) > math.Abs(changes[j].pct) })
	if len(changes) > 0 {
		b.WriteString("\nLargest split changes (median emulator frames):\n")
		for i, change := range changes {
			if i >= 8 {
				break
			}
			fmt.Fprintf(&b, "  %-28s %12.0f -> %-12.0f %+.1f%%\n", change.id, change.base, change.candidate, change.pct)
		}
	}

	counterNames := []string{"wild_battles", "encounters", "local_navigation_replans", "replans", "repel_uses", "strategist_calls", "fast_planner_calls", "typed_decision_calls", "typed_decision_failures", "successful_recoveries", "blackouts"}
	var wroteCounters bool
	for _, name := range counterNames {
		base, bok := baseline.Counters[name]
		cand, cok := candidate.Counters[name]
		if !bok && !cok {
			continue
		}
		if !wroteCounters {
			b.WriteString("\nCounters (median):\n")
			wroteCounters = true
		}
		fmt.Fprintf(&b, "  %-28s %12.1f -> %-12.1f %s\n", name, base.Median, cand.Median, percentChange(base.Median, cand.Median))
	}

	if len(candidate.FailureFingerprints) > 0 || len(baseline.FailureFingerprints) > 0 {
		b.WriteString("\nFailure fingerprints:\n")
		keys := map[string]bool{}
		for key := range baseline.FailureFingerprints {
			keys[key] = true
		}
		for key := range candidate.FailureFingerprints {
			keys[key] = true
		}
		var ordered []string
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			fmt.Fprintf(&b, "  %-52s %3d -> %3d\n", key, baseline.FailureFingerprints[key], candidate.FailureFingerprints[key])
		}
	}

	if min(baseline.Runs, candidate.Runs) < 30 {
		b.WriteString("\nDescriptive comparison only; sample sizes are too small to claim statistical significance.\n")
	}
	return b.String()
}

func completionChange(base, candidate Aggregate) string {
	if base.Runs == 0 || candidate.Runs == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%+.1f pp", (candidate.CompletionRate-base.CompletionRate)*100)
}

func percentChange(base, candidate float64) string {
	if base == 0 {
		if candidate == 0 {
			return "0.0%"
		}
		return "n/a"
	}
	return fmt.Sprintf("%+.1f%%", (candidate-base)/base*100)
}

func formatUint(value uint64) string {
	if value == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d", value)
}

func formatSeconds(value float64) string {
	if value == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1fs", value)
}
