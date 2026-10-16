// Package editplan defines the trusted boundary between creative editorial
// choices and deterministic media rendering. It never executes model text.
package editplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/media/producer"
)

const (
	SchemaVersion = 1
	PolicyVersion = 1
	RenderVersion = "ffmpeg-typed-v1"
	MaxSegments   = 256
)

type Profile string

const (
	Episode Profile = "episode"
	Short   Profile = "short"
)

type Action string

const (
	Keep Action = "keep"
	Cut  Action = "cut"
	Card Action = "card"
	Pad  Action = "pad"
)

type Segment struct {
	Action       Action   `json:"action"`
	Attempt      int      `json:"attempt,omitempty"`
	StartMS      int64    `json:"start_ms,omitempty"`
	EndMS        int64    `json:"end_ms,omitempty"`
	Speed        int      `json:"speed,omitempty"`
	DurationMS   int64    `json:"duration_ms,omitempty"` // card/pad only
	EventIDs     []string `json:"event_ids,omitempty"`
	Overlay      string   `json:"overlay,omitempty"`
	Caption      string   `json:"caption,omitempty"`
	NarrationCue string   `json:"narration_cue,omitempty"` // cue, NOT synthesized audio
	Chapter      string   `json:"chapter,omitempty"`
	Transition   string   `json:"transition,omitempty"` // cut or fade-to-black
}

type Source struct {
	Attempt    int   `json:"attempt"`
	DurationMS int64 `json:"duration_ms"`
}

type Plan struct {
	Version        int       `json:"version"`
	PolicyVersion  int       `json:"policy_version"`
	RenderVersion  string    `json:"render_version"`
	RunID          string    `json:"run_id"`
	AssessmentHash string    `json:"assessment_hash"`
	Profile        Profile   `json:"profile"`
	Sources        []Source  `json:"sources"`
	SourceHash     string    `json:"source_hash"`
	Segments       []Segment `json:"segments"`
	DurationMS     int64     `json:"duration_ms"`
	Hash           string    `json:"hash"`
}

func sourceData(timelines []farm.MediaTimeline) ([]Source, map[int]farm.MediaTimeline, string, error) {
	sources := make([]Source, 0, len(timelines))
	byAttempt := make(map[int]farm.MediaTimeline, len(timelines))
	for _, raw := range timelines {
		t := raw.Normalized()
		if t.Attempt == 0 && len(timelines) == 1 {
			t.Attempt = 1
		}
		if t.Attempt < 1 || byAttempt[t.Attempt].Run.RunID != "" {
			return nil, nil, "", errors.New("editplan: invalid or duplicate attempt")
		}
		if err := t.Validate(); err != nil {
			return nil, nil, "", err
		}
		if t.EndFrame > math.MaxInt64/1000 {
			return nil, nil, "", errors.New("editplan: source duration overflow")
		}
		ms := int64(math.Ceil(float64(t.EndFrame+1) * 1000 / t.FramesPerSecond))
		if ms <= 0 {
			return nil, nil, "", errors.New("editplan: invalid source duration")
		}
		sources = append(sources, Source{t.Attempt, ms})
		byAttempt[t.Attempt] = t
	}
	if len(sources) == 0 {
		return nil, nil, "", errors.New("editplan: source timeline required")
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Attempt < sources[j].Attempt })
	raw, err := json.Marshal(timelinesCanonical(sources, byAttempt))
	if err != nil {
		return nil, nil, "", err
	}
	sum := sha256.Sum256(raw)
	return sources, byAttempt, hex.EncodeToString(sum[:]), nil
}
func timelinesCanonical(sources []Source, by map[int]farm.MediaTimeline) []farm.MediaTimeline {
	out := make([]farm.MediaTimeline, 0, len(sources))
	for _, s := range sources {
		out = append(out, by[s.Attempt])
	}
	return out
}

func ValidProfile(p Profile) bool { return p == Episode || p == Short }
func speedOK(speed int) bool {
	return speed == 1 || speed == 2 || speed == 4 || speed == 8 || speed == 16
}
func textOK(s string, max int) bool { return len(s) <= max && !strings.ContainsAny(s, "\x00\r") }

func duration(segments []Segment) int64 {
	var n int64
	for _, seg := range segments {
		switch seg.Action {
		case Keep:
			n += int64(math.Ceil(float64(seg.EndMS-seg.StartMS) / float64(seg.Speed)))
		case Card, Pad:
			n += seg.DurationMS
		}
	}
	return n
}

func Validate(p Plan, assessment producer.Assessment, timelines []farm.MediaTimeline) error {
	if p.Version != SchemaVersion || p.PolicyVersion != PolicyVersion || p.RenderVersion != RenderVersion || !ValidProfile(p.Profile) {
		return errors.New("editplan: unsupported plan schema/policy/renderer/profile")
	}
	if err := producer.VerifyChecksum(assessment); err != nil {
		return fmt.Errorf("editplan: assessment: %w", err)
	}
	if assessment.Publication == producer.DoNotPublish || assessment.Publication == producer.ArchiveOnly {
		return errors.New("editplan: producer declined public edits")
	}
	if assessment.Publication == producer.ShortOnly && p.Profile != Short {
		return errors.New("editplan: assessment only permits a short")
	}
	if p.AssessmentHash != assessment.Hash || p.RunID != assessment.RunID || p.RunID == "" {
		return errors.New("editplan: mismatched producer assessment")
	}
	sources, by, sourceHash, err := sourceData(timelines)
	if err != nil {
		return err
	}
	if p.SourceHash != sourceHash || len(p.Sources) != len(sources) {
		return errors.New("editplan: stale source timeline fingerprint")
	}
	for i := range sources {
		if sources[i] != p.Sources[i] || by[sources[i].Attempt].Run.RunID != p.RunID {
			return errors.New("editplan: source duration/run mismatch")
		}
	}
	if len(p.Segments) == 0 || len(p.Segments) > MaxSegments {
		return errors.New("editplan: invalid segment count")
	}
	events := make(map[string]farm.MediaEvent)
	for _, t := range by {
		for _, e := range t.Events {
			events[fmt.Sprintf("%d/%s", t.Attempt, e.ID)] = e
		}
	}
	var lastAttempt int
	var lastEnd int64
	var played int
	var outputMS int64
	covered := make(map[string]bool)
	for i, s := range p.Segments {
		if !textOK(s.Overlay, 180) || !textOK(s.Caption, 240) || !textOK(s.NarrationCue, 600) || !textOK(s.Chapter, 100) {
			return fmt.Errorf("editplan: segment %d has unsafe or oversized text", i)
		}
		if s.Transition != "" && s.Transition != "cut" && s.Transition != "fade" {
			return fmt.Errorf("editplan: segment %d has unsupported transition", i)
		}
		switch s.Action {
		case Keep, Cut:
			t, ok := by[s.Attempt]
			if !ok {
				return fmt.Errorf("editplan: segment %d references unknown attempt", i)
			}
			maxMS := int64(math.Ceil(float64(t.EndFrame+1) * 1000 / t.FramesPerSecond))
			if s.StartMS < 0 || s.EndMS <= s.StartMS || s.EndMS > maxMS {
				return fmt.Errorf("editplan: segment %d exceeds source bounds", i)
			}
			if s.Attempt < lastAttempt || (s.Attempt == lastAttempt && s.StartMS < lastEnd) {
				return fmt.Errorf("editplan: overlapping or unordered segment %d", i)
			}
			lastAttempt, lastEnd = s.Attempt, s.EndMS
			if s.DurationMS != 0 {
				return fmt.Errorf("editplan: segment %d has extraneous duration", i)
			}
			if s.Action == Cut {
				if s.Speed != 0 || s.Overlay != "" || s.Caption != "" || s.NarrationCue != "" || s.Chapter != "" || len(s.EventIDs) > 0 {
					return fmt.Errorf("editplan: cut %d cannot carry render instructions", i)
				}
				continue
			}
			if !speedOK(s.Speed) {
				return fmt.Errorf("editplan: segment %d uses unsupported speed", i)
			}
			outputMS += int64(math.Ceil(float64(s.EndMS-s.StartMS) / float64(s.Speed)))
			played++
			for _, id := range s.EventIDs {
				event, found := events[fmt.Sprintf("%d/%s", s.Attempt, id)]
				if !found || event.TimestampMS < s.StartMS || event.TimestampMS > s.EndMS {
					return fmt.Errorf("editplan: segment %d has missing or out-of-window evidence %q", i, id)
				}
				if s.Speed != 1 {
					return fmt.Errorf("editplan: cited event %q must play real-time", id)
				}
				covered[fmt.Sprintf("%d/%s", s.Attempt, id)] = true
			}
		case Card, Pad:
			if s.Attempt != 0 || s.StartMS != 0 || s.EndMS != 0 || s.Speed != 0 || len(s.EventIDs) > 0 ||
				s.DurationMS < 250 || s.DurationMS > 15000 {
				return fmt.Errorf("editplan: invalid generated card/padding %d", i)
			}
			if s.Action == Pad && (s.Overlay != "" || s.Caption != "" || s.NarrationCue != "" || s.Chapter != "") {
				return fmt.Errorf("editplan: padding %d cannot contain text", i)
			}
			outputMS += s.DurationMS
		default:
			return fmt.Errorf("editplan: unknown action on segment %d", i)
		}
	}
	if played == 0 || outputMS > 90*60*1000 || p.DurationMS != outputMS {
		return errors.New("editplan: invalid or excessive output duration")
	}
	// Every story beat needs at least one cited event preserved at 1x in the
	// corresponding timeline. Manual editors cannot silently drop the climax.
	for _, beat := range assessment.Beats {
		ok := false
		for _, ref := range beat.Evidence {
			if covered[fmt.Sprintf("%d/%s", ref.Attempt, ref.EventID)] {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("editplan: missing real-time evidence for beat %q", beat.Kind)
		}
	}
	hash := p.Hash
	p.Hash = ""
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	check := sha256.Sum256(raw)
	if hash != hex.EncodeToString(check[:]) {
		return errors.New("editplan: content hash mismatch")
	}
	return nil
}

// Seal computes derived fields and a content hash for typed authoring tools;
// Validate MUST still be called with the authoritative assessment/timelines.
func Seal(p Plan) Plan {
	p.Version, p.PolicyVersion, p.RenderVersion = SchemaVersion, PolicyVersion, RenderVersion
	p.DurationMS = duration(p.Segments)
	p.Hash = ""
	data, _ := json.Marshal(p)
	check := sha256.Sum256(data)
	p.Hash = hex.EncodeToString(check[:])
	return p
}

func Initialize(runID string, assessment producer.Assessment, profile Profile, timelines []farm.MediaTimeline) (Plan, error) {
	sources, _, hash, err := sourceData(timelines)
	if err != nil {
		return Plan{}, err
	}
	return Plan{RunID: runID, AssessmentHash: assessment.Hash, Profile: profile, Sources: sources, SourceHash: hash}, nil
}

type Policy struct {
	PreRollMS    int64
	PostRollMS   int64
	SkipAfterMS  int64
	RoutineSpeed int
}

func DefaultPolicy() Policy {
	return Policy{PreRollMS: 5000, PostRollMS: 8000, SkipAfterMS: 30000, RoutineSpeed: 8}
}

type window struct {
	attempt    int
	start, end int64
	eventIDs   []string
	caption    string
}

// Build creates a reviewable draft with deterministic real-time story windows,
// accelerated navigation and explicitly skipped long stretches. No model can
// output encoder filters or commands.
func Build(a producer.Assessment, timelines []farm.MediaTimeline, profile Profile) (Plan, error) {
	p, err := Initialize(a.RunID, a, profile, timelines)
	if err != nil {
		return Plan{}, err
	}
	if !ValidProfile(profile) {
		return Plan{}, errors.New("editplan: unsupported profile")
	}
	pol := DefaultPolicy()
	by := make(map[int]farm.MediaTimeline)
	for _, t := range timelines {
		t = t.Normalized()
		if t.Attempt == 0 {
			t.Attempt = 1
		}
		by[t.Attempt] = t
	}
	var windows []window
	for _, beat := range a.Beats {
		if len(beat.Evidence) == 0 {
			continue
		}
		ref := beat.Evidence[0]
		t, ok := by[ref.Attempt]
		if !ok {
			return Plan{}, errors.New("editplan: missing beat attempt")
		}
		var event farm.MediaEvent
		found := false
		for _, e := range t.Events {
			if e.ID == ref.EventID {
				event, found = e, true
				break
			}
		}
		if !found || event.Frame != ref.Frame || event.TimestampMS != ref.TimestampMS {
			return Plan{}, errors.New("editplan: producer referenced missing timeline evidence")
		}
		start := event.TimestampMS
		if event.StartFrame > 0 && event.StartFrame < event.Frame {
			start = int64(math.Round(float64(event.StartFrame) * 1000 / t.FramesPerSecond))
		}
		start -= pol.PreRollMS
		if start < 0 {
			start = 0
		}
		maxMS := int64(math.Ceil(float64(t.EndFrame+1) * 1000 / t.FramesPerSecond))
		end := event.TimestampMS + pol.PostRollMS
		if end > maxMS {
			end = maxMS
		}
		if end <= start {
			continue
		}
		windows = append(windows, window{ref.Attempt, start, end, []string{event.ID}, beat.Summary})
	}
	sort.Slice(windows, func(i, j int) bool {
		if windows[i].attempt != windows[j].attempt {
			return windows[i].attempt < windows[j].attempt
		}
		return windows[i].start < windows[j].start
	})
	merged := make([]window, 0, len(windows))
	for _, w := range windows {
		n := len(merged)
		if n > 0 && merged[n-1].attempt == w.attempt && merged[n-1].end >= w.start {
			if w.end > merged[n-1].end {
				merged[n-1].end = w.end
			}
			merged[n-1].eventIDs = append(merged[n-1].eventIDs, w.eventIDs...)
		} else {
			merged = append(merged, w)
		}
	}
	if profile == Short && len(merged) > 2 {
		// Short intentionally only uses at most two windows, but all selected
		// beats must remain. Reject rather than silently dropping evidence.
		return Plan{}, errors.New("editplan: too many distinct story windows for a single short; use episode profile or revise the assessment")
	}
	if len(merged) == 0 {
		return Plan{}, errors.New("editplan: no evidence-backed story windows")
	}
	p.Segments = append(p.Segments, Segment{Action: Card, DurationMS: 2000, Overlay: "POKEPILOT", Caption: "Run highlights", Transition: "cut"})
	for _, source := range p.Sources {
		attemptWindows := []window{}
		for _, w := range merged {
			if w.attempt == source.Attempt {
				attemptWindows = append(attemptWindows, w)
			}
		}
		cursor := int64(0)
		for _, w := range attemptWindows {
			if w.start > cursor {
				if profile == Short || w.start-cursor > pol.SkipAfterMS {
					p.Segments = append(p.Segments, Segment{Action: Cut, Attempt: source.Attempt, StartMS: cursor, EndMS: w.start})
				} else {
					p.Segments = append(p.Segments, Segment{Action: Keep, Attempt: source.Attempt, StartMS: cursor, EndMS: w.start, Speed: pol.RoutineSpeed})
				}
			}
			caption := strings.TrimSpace(w.caption)
			if len(caption) > 220 {
				caption = caption[:220]
			}
			p.Segments = append(p.Segments, Segment{Action: Keep, Attempt: source.Attempt, StartMS: w.start, EndMS: w.end, Speed: 1,
				EventIDs: w.eventIDs, Caption: caption, NarrationCue: caption, Chapter: caption, Transition: "fade"})
			cursor = w.end
		}
		if cursor < source.DurationMS {
			p.Segments = append(p.Segments, Segment{Action: Cut, Attempt: source.Attempt, StartMS: cursor, EndMS: source.DurationMS})
		}
	}
	p.Segments = append(p.Segments, Segment{Action: Card, DurationMS: 1500, Overlay: "POKEPILOT", Caption: "Run recap"})
	p = Seal(p)
	if err := Validate(p, a, timelines); err != nil {
		return Plan{}, err
	}
	return p, nil
}

// OffsetMS maps an attempt-local source timestamp to a rendered full-replay
// timestamp. All prior attempts are concatenated in source order.
func OffsetMS(sources []Source, attempt int, at int64) (int64, error) {
	var offset int64
	for _, s := range sources {
		if s.Attempt == attempt {
			return offset + at, nil
		}
		offset += s.DurationMS
	}
	return 0, errors.New("editplan: unknown attempt")
}
