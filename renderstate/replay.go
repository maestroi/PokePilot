package renderstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// ReplayTimelineVersion versions the derived semantic replay sidecar. It is
// independent of RenderState.SchemaVersion so the compact storage format can
// evolve without changing the live renderer wire contract.
const ReplayTimelineVersion = 1

// ReplayTimeline is a seekable, derived presentation artifact generated from a
// deterministic recording. Layer grids are deduplicated because they dominate
// RenderState size and usually remain unchanged while actors move.
type ReplayTimeline struct {
	Version       int            `json:"version"`
	SchemaVersion int            `json:"schema_version"`
	RunID         string         `json:"run_id,omitempty"`
	FramesPerSec  float64        `json:"frames_per_second"`
	DurationMS    int64          `json:"duration_ms"`
	LayerSets     [][]TileLayer  `json:"layer_sets,omitempty"`
	Samples       []ReplaySample `json:"samples"`
}

// ReplaySample stores one authoritative semantic observation at a replay time.
// LayerSet is 1-based; zero means the state had no semantic layers.
type ReplaySample struct {
	AtMS     int64       `json:"at_ms"`
	Attempt  int         `json:"attempt,omitempty"`
	LayerSet int         `json:"layer_set,omitempty"`
	State    RenderState `json:"state"`
}

// ReplayTimelineBuilder incrementally compacts RenderState samples while a
// deterministic recording is replayed.
type ReplayTimelineBuilder struct {
	timeline   ReplayTimeline
	layerIndex map[string]int
}

func NewReplayTimelineBuilder(runID string, fps float64) *ReplayTimelineBuilder {
	return &ReplayTimelineBuilder{
		timeline: ReplayTimeline{
			Version:       ReplayTimelineVersion,
			SchemaVersion: SchemaVersion,
			RunID:         runID,
			FramesPerSec:  fps,
		},
		layerIndex: make(map[string]int),
	}
}

func (b *ReplayTimelineBuilder) Append(atMS int64, attempt int, state RenderState) error {
	if b == nil {
		return fmt.Errorf("renderstate: replay timeline builder is nil")
	}
	if err := Validate(state); err != nil {
		return fmt.Errorf("renderstate: replay sample: %w", err)
	}
	if atMS < 0 {
		return fmt.Errorf("renderstate: replay sample time cannot be negative")
	}
	if n := len(b.timeline.Samples); n > 0 && atMS < b.timeline.Samples[n-1].AtMS {
		return fmt.Errorf("renderstate: replay samples are not ordered")
	}

	layerSet := 0
	if len(state.Layers) > 0 {
		data, err := json.Marshal(state.Layers)
		if err != nil {
			return fmt.Errorf("renderstate: encode replay layers: %w", err)
		}
		sum := sha256.Sum256(data)
		key := hex.EncodeToString(sum[:])
		if existing, ok := b.layerIndex[key]; ok {
			layerSet = existing
		} else {
			b.timeline.LayerSets = append(b.timeline.LayerSets, state.Layers)
			layerSet = len(b.timeline.LayerSets)
			b.layerIndex[key] = layerSet
		}
		state.Layers = nil
	}

	b.timeline.Samples = append(b.timeline.Samples, ReplaySample{
		AtMS:     atMS,
		Attempt:  attempt,
		LayerSet: layerSet,
		State:    state,
	})
	return nil
}

func (b *ReplayTimelineBuilder) Build(durationMS int64) (ReplayTimeline, error) {
	if b == nil {
		return ReplayTimeline{}, fmt.Errorf("renderstate: replay timeline builder is nil")
	}
	out := b.timeline
	out.DurationMS = durationMS
	if err := out.Validate(); err != nil {
		return ReplayTimeline{}, err
	}
	return out, nil
}

func (t ReplayTimeline) Validate() error {
	if t.Version != ReplayTimelineVersion {
		return fmt.Errorf("renderstate: replay timeline version %d, want %d", t.Version, ReplayTimelineVersion)
	}
	if t.SchemaVersion != SchemaVersion {
		return fmt.Errorf("renderstate: replay schema version %d, want %d", t.SchemaVersion, SchemaVersion)
	}
	if t.FramesPerSec <= 0 || math.IsNaN(t.FramesPerSec) || math.IsInf(t.FramesPerSec, 0) {
		return fmt.Errorf("renderstate: replay frames_per_second is invalid")
	}
	if t.DurationMS < 0 {
		return fmt.Errorf("renderstate: replay duration cannot be negative")
	}
	var previous int64 = -1
	for i, sample := range t.Samples {
		if sample.AtMS < previous {
			return fmt.Errorf("renderstate: replay samples are not ordered at %d", i)
		}
		if sample.AtMS < 0 || sample.AtMS > t.DurationMS {
			return fmt.Errorf("renderstate: replay sample %d time %d is outside duration %d", i, sample.AtMS, t.DurationMS)
		}
		if sample.LayerSet < 0 || sample.LayerSet > len(t.LayerSets) {
			return fmt.Errorf("renderstate: replay sample %d layer_set %d is invalid", i, sample.LayerSet)
		}
		if len(sample.State.Layers) != 0 {
			return fmt.Errorf("renderstate: replay sample %d contains inline layers", i)
		}
		state := sample.State
		if sample.LayerSet > 0 {
			state.Layers = t.LayerSets[sample.LayerSet-1]
		}
		if err := Validate(state); err != nil {
			return fmt.Errorf("renderstate: replay sample %d: %w", i, err)
		}
		previous = sample.AtMS
	}
	return nil
}

// StateAtMS returns the latest authoritative state at or before the requested
// replay time. Times before the first sample clamp to the first sample; times
// after the last sample clamp to the last. The returned state has its semantic
// layer grid materialized and is directly consumable by normal renderers.
func (t ReplayTimeline) StateAtMS(atMS int64) (RenderState, bool) {
	if len(t.Samples) == 0 {
		return RenderState{}, false
	}
	idx := sort.Search(len(t.Samples), func(i int) bool { return t.Samples[i].AtMS > atMS }) - 1
	if idx < 0 {
		idx = 0
	}
	sample := t.Samples[idx]
	state := sample.State
	if sample.LayerSet > 0 && sample.LayerSet <= len(t.LayerSets) {
		state.Layers = t.LayerSets[sample.LayerSet-1]
	}
	return state, true
}
