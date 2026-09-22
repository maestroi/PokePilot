package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

const (
	runActivityKeep       = 240
	runActivitySummaryCap = 220
	runActivityDetailCap  = 700
	skillActivityInterval = 20 * time.Second
)

// runActivityEvent is the operator-facing causal story of a run. It is
// intentionally compact and source-tagged so the console can distinguish what
// the LLM chose from what deterministic gameplay/recovery machinery did.
type runActivityEvent struct {
	Source          string `json:"source"`
	Kind            string `json:"kind"`
	At              int64  `json:"at,omitempty"`
	Frame           uint64 `json:"frame,omitempty"`
	Round           int    `json:"round,omitempty"`
	Attempt         int    `json:"attempt,omitempty"`
	RecoveryAttempt int    `json:"recovery_attempt,omitempty"`
	Summary         string `json:"summary"`
	Detail          string `json:"detail,omitempty"`
}

func activityText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "…"
}

// appendRunActivityLocked adds one meaningful event to the bounded run story.
// Caller owns w.mu (or is otherwise the sole mutator of the tile).
func appendRunActivityLocked(t *Tile, event runActivityEvent) {
	if t == nil {
		return
	}
	event.Source = strings.TrimSpace(event.Source)
	event.Kind = strings.TrimSpace(event.Kind)
	event.Summary = activityText(event.Summary, runActivitySummaryCap)
	event.Detail = activityText(event.Detail, runActivityDetailCap)
	if event.Source == "" {
		event.Source = "system"
	}
	if event.Kind == "" {
		event.Kind = "event"
	}
	if event.Summary == "" {
		return
	}
	if event.At == 0 {
		event.At = time.Now().Unix()
	}
	if event.Frame == 0 {
		event.Frame = t.Frame
	}
	if event.Attempt == 0 {
		event.Attempt = t.Attempts + 1
	}
	if event.RecoveryAttempt == 0 && t.RecoveryAttempts > 0 {
		event.RecoveryAttempt = t.RecoveryAttempts
	}
	if len(t.Activity) > 0 {
		last := t.Activity[len(t.Activity)-1]
		// Heartbeats can repeat the same semantic state for many seconds. Do
		// not turn that into timeline noise.
		if last.Source == event.Source && last.Kind == event.Kind && last.Summary == event.Summary &&
			last.Detail == event.Detail && last.Attempt == event.Attempt {
			return
		}
	}
	t.Activity = append(t.Activity, event)
	if len(t.Activity) > runActivityKeep {
		copy(t.Activity, t.Activity[len(t.Activity)-runActivityKeep:])
		t.Activity = t.Activity[:runActivityKeep]
	}
}

func copyRunActivity(events []runActivityEvent) []runActivityEvent {
	if len(events) == 0 {
		return nil
	}
	return append([]runActivityEvent(nil), events...)
}

// durableRunActivity is the infrequent part safe for the production control
// plane singleton. LLM/skill heartbeat breadcrumbs remain RAM-live (and are
// captured in the history catalog when the run settles) so they do not turn
// every planner/trace change into a PostgreSQL control-plane write.
func durableRunActivity(events []runActivityEvent) []runActivityEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]runActivityEvent, 0, len(events))
	for _, event := range events {
		switch event.Source {
		case "recovery", "milestone", "system":
			out = append(out, event)
		}
	}
	return out
}

func appendHeartbeatActivityLocked(t *Tile, hb farm.Heartbeat, now time.Time, previousStatus, previousQuestion, previousDecision, previousTrace string, previousPlayer *farm.Player) {
	if previousStatus != statusRunning {
		appendRunActivityLocked(t, runActivityEvent{
			Source: "system", Kind: "attempt_start", At: now.Unix(), Frame: hb.Frame,
			Summary: fmt.Sprintf("Attempt %d started", t.Attempts+1),
			Detail:  fmt.Sprintf("runner %s", activityText(hb.Version, 80)),
		})
	}

	if hb.Question != "" && hb.Question != previousQuestion && hb.Decision == "" {
		round := 0
		if hb.Stats != nil {
			round = hb.Stats.Round
		}
		appendRunActivityLocked(t, runActivityEvent{
			Source: "llm", Kind: "planning", At: now.Unix(), Frame: hb.Frame, Round: round,
			Summary: "LLM planning",
			Detail:  hb.Question,
		})
	}
	if hb.Decision != "" && hb.Decision != previousDecision {
		round := 0
		if hb.Stats != nil {
			round = hb.Stats.Round
		}
		appendRunActivityLocked(t, runActivityEvent{
			Source: "llm", Kind: "decision", At: now.Unix(), Frame: hb.Frame, Round: round,
			Summary: hb.Decision,
			Detail:  "Planner selected the next objective.",
		})
	}

	if hb.Trace != "" && hb.Trace != previousTrace {
		lastSkillAt := int64(0)
		for index := len(t.Activity) - 1; index >= 0; index-- {
			if t.Activity[index].Source == "skill" {
				lastSkillAt = t.Activity[index].At
				break
			}
		}
		if lastSkillAt == 0 || now.Unix()-lastSkillAt >= int64(skillActivityInterval/time.Second) {
			appendRunActivityLocked(t, runActivityEvent{
				Source: "skill", Kind: "execution", At: now.Unix(), Frame: hb.Frame,
				Summary: hb.Trace,
				Detail:  "Deterministic gameplay execution.",
			})
		}
	}

	appendPlayerMilestonesLocked(t, previousPlayer, hb.Player, now, hb.Frame)
}

func appendPlayerMilestonesLocked(t *Tile, before, after *farm.Player, now time.Time, frame uint64) {
	if after == nil {
		return
	}
	badges := map[string]struct{}{}
	milestones := map[string]struct{}{}
	if before != nil {
		for _, badge := range before.Badges {
			badges[badge] = struct{}{}
		}
		for _, milestone := range before.Milestones {
			milestones[milestone] = struct{}{}
		}
	}
	for _, badge := range after.Badges {
		if _, known := badges[badge]; known {
			continue
		}
		appendRunActivityLocked(t, runActivityEvent{
			Source: "milestone", Kind: "badge", At: now.Unix(), Frame: frame,
			Summary: fmt.Sprintf("%s Badge obtained", activityText(badge, 80)),
		})
	}
	for _, milestone := range after.Milestones {
		if _, known := milestones[milestone]; known {
			continue
		}
		appendRunActivityLocked(t, runActivityEvent{
			Source: "milestone", Kind: "progress", At: now.Unix(), Frame: frame,
			Summary: activityText(milestone, 160),
		})
	}
}
