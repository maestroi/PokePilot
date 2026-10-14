package highlight

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

const (
	PlanVersion   = 1
	PolicyVersion = 2
)

type Rule struct {
	EventType  string `json:"event_type"`
	Priority   int    `json:"priority"`
	PreRollMS  int64  `json:"pre_roll_ms"`
	PostRollMS int64  `json:"post_roll_ms"`
}

type Policy struct {
	Version          int    `json:"version"`
	TargetDurationMS int64  `json:"target_duration_ms"`
	MergeGapMS       int64  `json:"merge_gap_ms"`
	Rules            []Rule `json:"rules"`
}

type EventRef struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Summary  string `json:"summary,omitempty"`
	Frame    uint64 `json:"frame"`
	Priority int    `json:"priority"`
}

type Window struct {
	Attempt    int        `json:"attempt"`
	Index      int        `json:"index"`
	StartFrame uint64     `json:"start_frame"`
	EndFrame   uint64     `json:"end_frame"`
	StartMS    int64      `json:"start_ms"`
	EndMS      int64      `json:"end_ms"`
	Priority   int        `json:"priority"`
	Events     []EventRef `json:"events"`
}

type Plan struct {
	Version          int      `json:"version"`
	PolicyVersion    int      `json:"policy_version"`
	TargetDurationMS int64    `json:"target_duration_ms"`
	DurationMS       int64    `json:"duration_ms"`
	Windows          []Window `json:"windows"`
	Hash             string   `json:"hash"`
}

func DefaultPolicy() Policy {
	return Policy{
		Version: PolicyVersion,
		// The reel keeps whole fights, so the cap is only a safety bound for
		// pathological runs, not an edit target.
		TargetDurationMS: 60 * 60 * 1000,
		MergeGapMS:       3000,
		Rules: []Rule{
			{EventType: "run_finished", Priority: 110, PreRollMS: 20_000, PostRollMS: 5_000},
			{EventType: "run_failed", Priority: 105, PreRollMS: 20_000, PostRollMS: 5_000},
			{EventType: "boss_battle", Priority: 104, PreRollMS: 10_000, PostRollMS: 10_000},
			{EventType: "badge_acquired", Priority: 100, PreRollMS: 20_000, PostRollMS: 15_000},
			{EventType: "gym_battle", Priority: 95, PreRollMS: 25_000, PostRollMS: 15_000},
			{EventType: "rare_catch", Priority: 88, PreRollMS: 12_000, PostRollMS: 15_000},
			{EventType: "evolution", Priority: 85, PreRollMS: 12_000, PostRollMS: 12_000},
			{EventType: "important_catch", Priority: 82, PreRollMS: 12_000, PostRollMS: 15_000},
			{EventType: "blackout", Priority: 80, PreRollMS: 15_000, PostRollMS: 15_000},
			{EventType: "planner_recovery", Priority: 72, PreRollMS: 12_000, PostRollMS: 12_000},
			{EventType: "failure_recovered", Priority: 70, PreRollMS: 12_000, PostRollMS: 12_000},
			{EventType: "failure_terminal", Priority: 68, PreRollMS: 12_000, PostRollMS: 8_000},
			{EventType: "milestone", Priority: 55, PreRollMS: 8_000, PostRollMS: 8_000},
			{EventType: "checkpoint", Priority: 50, PreRollMS: 8_000, PostRollMS: 8_000},
		},
	}
}

func Build(timelines []farm.MediaTimeline, policy Policy) Plan {
	policy = normalizePolicy(policy)
	rules := make(map[string]Rule, len(policy.Rules))
	for _, rule := range policy.Rules {
		rules[rule.EventType] = rule
	}
	var candidates []Window
	for _, raw := range timelines {
		timeline := raw.Normalized()
		fps := timeline.FramesPerSecond
		if fps <= 0 {
			fps = farm.GameBoyFramesPerSecond
		}
		for _, event := range timeline.Events {
			rule, ok := rules[strings.TrimSpace(event.Type)]
			if !ok {
				continue
			}
			pre, post := msToFrames(rule.PreRollMS, fps), msToFrames(rule.PostRollMS, fps)
			// Spanning events (a whole battle) roll back from where they began,
			// not from the frame they resolved.
			anchor := event.Frame
			if event.StartFrame > 0 && event.StartFrame < anchor {
				anchor = event.StartFrame
			}
			start := uint64(0)
			if anchor > pre {
				start = anchor - pre
			}
			end := event.Frame + post
			if end > timeline.EndFrame {
				end = timeline.EndFrame
			}
			if end < start {
				continue
			}
			candidates = append(candidates, Window{
				Attempt: timeline.Attempt, StartFrame: start, EndFrame: end,
				StartMS: frameMS(start, fps), EndMS: frameMS(end+1, fps), Priority: rule.Priority,
				Events: []EventRef{{ID: event.ID, Type: event.Type, Summary: event.Summary, Frame: event.Frame, Priority: rule.Priority}},
			})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Attempt != candidates[j].Attempt {
			return candidates[i].Attempt < candidates[j].Attempt
		}
		if candidates[i].StartFrame != candidates[j].StartFrame {
			return candidates[i].StartFrame < candidates[j].StartFrame
		}
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		return firstEventID(candidates[i]) < firstEventID(candidates[j])
	})
	merged := mergeWindows(candidates, policy.MergeGapMS)
	selected := capWindows(merged, policy.TargetDurationMS)
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Attempt != selected[j].Attempt {
			return selected[i].Attempt < selected[j].Attempt
		}
		if selected[i].StartFrame != selected[j].StartFrame {
			return selected[i].StartFrame < selected[j].StartFrame
		}
		return firstEventID(selected[i]) < firstEventID(selected[j])
	})
	var duration int64
	for i := range selected {
		selected[i].Index = i
		duration += windowDuration(selected[i])
		sort.SliceStable(selected[i].Events, func(a, b int) bool {
			if selected[i].Events[a].Frame != selected[i].Events[b].Frame {
				return selected[i].Events[a].Frame < selected[i].Events[b].Frame
			}
			if selected[i].Events[a].Priority != selected[i].Events[b].Priority {
				return selected[i].Events[a].Priority > selected[i].Events[b].Priority
			}
			return selected[i].Events[a].ID < selected[i].Events[b].ID
		})
	}
	out := Plan{Version: PlanVersion, PolicyVersion: policy.Version, TargetDurationMS: policy.TargetDurationMS, DurationMS: duration, Windows: selected}
	out.Hash = hashPlan(out)
	return out
}

func normalizePolicy(policy Policy) Policy {
	defaults := DefaultPolicy()
	if policy.Version == 0 {
		policy.Version = PolicyVersion
	}
	if policy.TargetDurationMS <= 0 {
		policy.TargetDurationMS = defaults.TargetDurationMS
	}
	if policy.MergeGapMS < 0 {
		policy.MergeGapMS = 0
	}
	policy.Rules = append([]Rule(nil), policy.Rules...)
	if len(policy.Rules) == 0 {
		policy.Rules = defaults.Rules
	}
	for i := range policy.Rules {
		policy.Rules[i].EventType = strings.TrimSpace(policy.Rules[i].EventType)
		if policy.Rules[i].PreRollMS < 0 {
			policy.Rules[i].PreRollMS = 0
		}
		if policy.Rules[i].PostRollMS < 0 {
			policy.Rules[i].PostRollMS = 0
		}
	}
	sort.SliceStable(policy.Rules, func(i, j int) bool {
		if policy.Rules[i].EventType != policy.Rules[j].EventType {
			return policy.Rules[i].EventType < policy.Rules[j].EventType
		}
		return policy.Rules[i].Priority > policy.Rules[j].Priority
	})
	return policy
}

func mergeWindows(in []Window, gapMS int64) []Window {
	var out []Window
	for _, next := range in {
		if len(out) == 0 {
			out = append(out, next)
			continue
		}
		last := &out[len(out)-1]
		if last.Attempt != next.Attempt || next.StartMS > last.EndMS+gapMS {
			out = append(out, next)
			continue
		}
		if next.StartFrame < last.StartFrame {
			last.StartFrame, last.StartMS = next.StartFrame, next.StartMS
		}
		if next.EndFrame > last.EndFrame {
			last.EndFrame, last.EndMS = next.EndFrame, next.EndMS
		}
		if next.Priority > last.Priority {
			last.Priority = next.Priority
		}
		last.Events = append(last.Events, next.Events...)
	}
	return out
}

func capWindows(in []Window, targetMS int64) []Window {
	if targetMS <= 0 {
		return nil
	}
	ranked := append([]Window(nil), in...)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Priority != ranked[j].Priority {
			return ranked[i].Priority > ranked[j].Priority
		}
		if ranked[i].Attempt != ranked[j].Attempt {
			return ranked[i].Attempt < ranked[j].Attempt
		}
		if ranked[i].StartFrame != ranked[j].StartFrame {
			return ranked[i].StartFrame < ranked[j].StartFrame
		}
		return firstEventID(ranked[i]) < firstEventID(ranked[j])
	})
	var out []Window
	var used int64
	for _, window := range ranked {
		duration := windowDuration(window)
		if duration <= 0 || used+duration > targetMS {
			continue
		}
		out = append(out, window)
		used += duration
	}
	return out
}

func windowDuration(window Window) int64 {
	if window.EndMS <= window.StartMS {
		return 0
	}
	return window.EndMS - window.StartMS
}

func msToFrames(ms int64, fps float64) uint64 {
	if ms <= 0 {
		return 0
	}
	return uint64(math.Round(float64(ms) * fps / 1000))
}

func frameMS(frame uint64, fps float64) int64 {
	if fps <= 0 {
		fps = farm.GameBoyFramesPerSecond
	}
	return int64(math.Round(float64(frame) * 1000 / fps))
}

func firstEventID(window Window) string {
	if len(window.Events) == 0 {
		return ""
	}
	return window.Events[0].ID
}

func hashPlan(plan Plan) string {
	plan.Hash = ""
	data, err := json.Marshal(plan)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Chapter is one YouTube-style chapter marker in a rendered reel.
type Chapter struct {
	StartMS int64  `json:"start_ms"`
	Title   string `json:"title"`
}

// ChapterTitle names a window after its most important event.
func ChapterTitle(window Window) string {
	var top EventRef
	for i, event := range window.Events {
		if i == 0 || event.Priority > top.Priority {
			top = event
		}
	}
	summary := strings.Join(strings.Fields(top.Summary), " ")
	if top.Type == "badge_acquired" && summary != "" {
		return clipTitle(strings.TrimPrefix(summary, "acquired "))
	}
	label := strings.ReplaceAll(strings.TrimSpace(top.Type), "_", " ")
	if label == "" {
		label = "highlight"
	}
	label = strings.ToUpper(label[:1]) + label[1:]
	if summary != "" {
		label += ": " + summary
	}
	return clipTitle(label)
}

// YouTubeChapters adapts chapters to YouTube's rules: the first starts at
// 0:00 (absorbing an intro) and each lasts at least 10s (a short one folds
// into its predecessor). Fewer than three chapters means YouTube shows none.
func YouTubeChapters(chapters []Chapter, totalMS int64) []Chapter {
	const minMS = 10_000
	var out []Chapter
	for i, chapter := range chapters {
		end := totalMS
		if i+1 < len(chapters) {
			end = chapters[i+1].StartMS
		}
		if len(out) == 0 {
			chapter.StartMS = 0
		} else if end-chapter.StartMS < minMS {
			continue
		}
		out = append(out, chapter)
	}
	// Folding can leave the previous chapter short only at the very end.
	for len(out) > 1 && totalMS-out[len(out)-1].StartMS < minMS {
		out = out[:len(out)-1]
	}
	if len(out) < 3 {
		return nil
	}
	return out
}

// FormatChapters renders chapters as YouTube description lines ("1:02:03 Title").
func FormatChapters(chapters []Chapter) string {
	var out strings.Builder
	for _, chapter := range chapters {
		total := chapter.StartMS / 1000
		h, m, s := total/3600, (total%3600)/60, total%60
		if h > 0 {
			fmt.Fprintf(&out, "%d:%02d:%02d %s\n", h, m, s, chapter.Title)
		} else {
			fmt.Fprintf(&out, "%d:%02d %s\n", m, s, chapter.Title)
		}
	}
	return out.String()
}

func clipTitle(value string) string {
	if runes := []rune(value); len(runes) > 60 {
		return string(runes[:59]) + "…"
	}
	return value
}
