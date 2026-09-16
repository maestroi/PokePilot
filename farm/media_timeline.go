package farm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	MediaTimelineVersion      = 1
	MediaTimelineArtifactName = "media-timeline.json"
	GameBoyFramesPerSecond    = 59.7275
)

// MediaTimeline is the sparse, durable semantic companion to run.gbrun.
// Frames are relative to the recording start, so the same artifact can drive
// offline replay, live presentation, and later editorial consumers without
// depending on wall-clock timing or re-running gameplay.
type MediaTimeline struct {
	Version          int             `json:"version"`
	Run              MediaRunSummary `json:"run"`
	Attempt          int             `json:"attempt,omitempty"`
	SourceStartFrame uint64          `json:"source_start_frame,omitempty"`
	EndFrame         uint64          `json:"end_frame"`
	FramesPerSecond  float64         `json:"frames_per_second"`
	Snapshots        []MediaSnapshot `json:"snapshots,omitempty"`
	Events           []MediaEvent    `json:"events,omitempty"`
}

type MediaRunSummary struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status,omitempty"`
	Goal    string `json:"goal,omitempty"`
	Planner string `json:"planner,omitempty"`
}

type MediaLocation struct {
	Map   uint8  `json:"map"`
	Name  string `json:"name,omitempty"`
	Place string `json:"place,omitempty"`
	X     uint8  `json:"x"`
	Y     uint8  `json:"y"`
}

type MediaPlannerState struct {
	Intent       string `json:"intent,omitempty"`
	PlanGoal     string `json:"plan_goal,omitempty"`
	PlanStep     int    `json:"plan_step,omitempty"`
	ReplanReason string `json:"replan_reason,omitempty"`
}

type MediaSnapshot struct {
	Frame       uint64            `json:"frame"`
	TimestampMS int64             `json:"timestamp_ms"`
	Round       int               `json:"round,omitempty"`
	Objective   string            `json:"objective,omitempty"`
	Outcome     string            `json:"outcome,omitempty"`
	Location    MediaLocation     `json:"location"`
	Player      *Player           `json:"player,omitempty"`
	Planner     MediaPlannerState `json:"planner,omitempty"`
	Events      []string          `json:"story_events,omitempty"`
}

type MediaEvent struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Frame       uint64            `json:"frame"`
	TimestampMS int64             `json:"timestamp_ms"`
	Round       int               `json:"round,omitempty"`
	Objective   string            `json:"objective,omitempty"`
	Summary     string            `json:"summary,omitempty"`
	Evidence    string            `json:"evidence,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type MediaFrameContext struct {
	Frame       uint64          `json:"frame"`
	TimestampMS int64           `json:"timestamp_ms"`
	Run         MediaRunSummary `json:"run"`
	Snapshot    *MediaSnapshot  `json:"snapshot,omitempty"`
	Events      []MediaEvent    `json:"events,omitempty"`
}

func (t MediaTimeline) Normalized() MediaTimeline {
	out := t
	if out.Version == 0 {
		out.Version = MediaTimelineVersion
	}
	if out.FramesPerSecond <= 0 {
		out.FramesPerSecond = GameBoyFramesPerSecond
	}
	out.Run.RunID = strings.TrimSpace(out.Run.RunID)
	out.Snapshots = append([]MediaSnapshot(nil), t.Snapshots...)
	for i := range out.Snapshots {
		out.Snapshots[i].Events = append([]string(nil), out.Snapshots[i].Events...)
		out.Snapshots[i].Player = cloneMediaPlayer(out.Snapshots[i].Player)
	}
	out.Events = append([]MediaEvent(nil), t.Events...)
	for i := range out.Events {
		out.Events[i].Metadata = cloneStringMap(out.Events[i].Metadata)
	}

	sort.SliceStable(out.Snapshots, func(i, j int) bool {
		if out.Snapshots[i].Frame != out.Snapshots[j].Frame {
			return out.Snapshots[i].Frame < out.Snapshots[j].Frame
		}
		if out.Snapshots[i].Round != out.Snapshots[j].Round {
			return out.Snapshots[i].Round < out.Snapshots[j].Round
		}
		return out.Snapshots[i].Objective < out.Snapshots[j].Objective
	})
	sort.SliceStable(out.Events, func(i, j int) bool {
		a, b := out.Events[i], out.Events[j]
		if a.Frame != b.Frame {
			return a.Frame < b.Frame
		}
		if a.Round != b.Round {
			return a.Round < b.Round
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Evidence != b.Evidence {
			return a.Evidence < b.Evidence
		}
		return a.Objective < b.Objective
	})

	for i := range out.Snapshots {
		out.Snapshots[i].TimestampMS = mediaTimestampMS(out.Snapshots[i].Frame, out.FramesPerSecond)
	}
	for i := range out.Events {
		out.Events[i].TimestampMS = mediaTimestampMS(out.Events[i].Frame, out.FramesPerSecond)
		out.Events[i].ID = mediaEventID(out.Run.RunID, out.Attempt, out.Events[i])
	}
	return out
}

func (t MediaTimeline) Validate() error {
	if t.Version != MediaTimelineVersion {
		return fmt.Errorf("farm: media timeline version %d, want %d", t.Version, MediaTimelineVersion)
	}
	if strings.TrimSpace(t.Run.RunID) == "" {
		return fmt.Errorf("farm: media timeline has empty run_id")
	}
	if t.FramesPerSecond <= 0 || math.IsNaN(t.FramesPerSecond) || math.IsInf(t.FramesPerSecond, 0) {
		return fmt.Errorf("farm: media timeline has invalid frames_per_second %v", t.FramesPerSecond)
	}
	var previous uint64
	for i, snapshot := range t.Snapshots {
		if snapshot.Frame > t.EndFrame {
			return fmt.Errorf("farm: media snapshot %d frame %d exceeds end frame %d", i, snapshot.Frame, t.EndFrame)
		}
		if i > 0 && snapshot.Frame < previous {
			return fmt.Errorf("farm: media snapshots are not ordered at %d", i)
		}
		previous = snapshot.Frame
	}
	seen := make(map[string]struct{}, len(t.Events))
	previous = 0
	for i, event := range t.Events {
		if strings.TrimSpace(event.Type) == "" {
			return fmt.Errorf("farm: media event %d has empty type", i)
		}
		if event.Frame > t.EndFrame {
			return fmt.Errorf("farm: media event %d frame %d exceeds end frame %d", i, event.Frame, t.EndFrame)
		}
		if i > 0 && event.Frame < previous {
			return fmt.Errorf("farm: media events are not ordered at %d", i)
		}
		if event.ID == "" {
			return fmt.Errorf("farm: media event %d has empty id", i)
		}
		if _, exists := seen[event.ID]; exists {
			return fmt.Errorf("farm: duplicate media event id %q", event.ID)
		}
		seen[event.ID] = struct{}{}
		previous = event.Frame
	}
	return nil
}

func NewMediaTimelineArtifact(t MediaTimeline) (Artifact, error) {
	t = t.Normalized()
	if err := t.Validate(); err != nil {
		return Artifact{}, err
	}
	data, err := json.Marshal(t)
	if err != nil {
		return Artifact{}, fmt.Errorf("farm: encode media timeline: %w", err)
	}
	sum := sha256.Sum256(data)
	return Artifact{
		Name:      MediaTimelineArtifactName,
		MediaType: "application/json",
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}, nil
}

func DecodeMediaTimeline(data []byte) (MediaTimeline, error) {
	var timeline MediaTimeline
	if err := json.Unmarshal(data, &timeline); err != nil {
		return MediaTimeline{}, fmt.Errorf("farm: decode media timeline: %w", err)
	}
	timeline = timeline.Normalized()
	if err := timeline.Validate(); err != nil {
		return MediaTimeline{}, err
	}
	return timeline, nil
}

// ContextAtFrame returns the most recent semantic snapshot at or before frame,
// plus events whose anchor is exactly that frame. A nil snapshot means no
// semantic state had been recorded yet; callers can still render raw replay.
func (t MediaTimeline) ContextAtFrame(frame uint64) MediaFrameContext {
	ctx := MediaFrameContext{
		Frame:       frame,
		TimestampMS: mediaTimestampMS(frame, t.FramesPerSecond),
		Run:         t.Run,
	}
	idx := sort.Search(len(t.Snapshots), func(i int) bool { return t.Snapshots[i].Frame > frame }) - 1
	if idx >= 0 {
		snapshot := t.Snapshots[idx]
		ctx.Snapshot = &snapshot
	}
	start := sort.Search(len(t.Events), func(i int) bool { return t.Events[i].Frame >= frame })
	for i := start; i < len(t.Events) && t.Events[i].Frame == frame; i++ {
		ctx.Events = append(ctx.Events, t.Events[i])
	}
	return ctx
}

// EventsBetween returns events in (afterFrame, throughFrame]. It is the useful
// primitive for renderers that advance in batches and must not miss an event
// merely because no video sample landed on the exact semantic frame.
func (t MediaTimeline) EventsBetween(afterFrame, throughFrame uint64) []MediaEvent {
	if throughFrame < afterFrame {
		return nil
	}
	start := sort.Search(len(t.Events), func(i int) bool { return t.Events[i].Frame > afterFrame })
	end := sort.Search(len(t.Events), func(i int) bool { return t.Events[i].Frame > throughFrame })
	return append([]MediaEvent(nil), t.Events[start:end]...)
}

func mediaTimestampMS(frame uint64, fps float64) int64 {
	if fps <= 0 {
		fps = GameBoyFramesPerSecond
	}
	return int64(math.Round(float64(frame) * 1000 / fps))
}

func mediaEventID(runID string, attempt int, event MediaEvent) string {
	h := sha256.New()
	for _, part := range []string{
		runID,
		strconv.Itoa(attempt),
		event.Type,
		strconv.FormatUint(event.Frame, 10),
		strconv.Itoa(event.Round),
		event.Objective,
		event.Evidence,
	} {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return "evt-" + hex.EncodeToString(h.Sum(nil)[:12])
}

func cloneMediaPlayer(in *Player) *Player {
	if in == nil {
		return nil
	}
	out := *in
	out.Badges = append([]string(nil), in.Badges...)
	out.Party = append([]PartyMon(nil), in.Party...)
	out.Bag = append([]BagItem(nil), in.Bag...)
	out.Milestones = append([]string(nil), in.Milestones...)
	return &out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
