// Package producer turns durable, spectator-safe semantic timelines into
// evidence-backed editorial decisions. It never reads gameplay model internals
// or renders/publishes media.
package producer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

const (
	SchemaVersion = 1
	PolicyVersion = 1
	PromptVersion = "evidence-story-v1"
	maxModelBytes = 64 << 10
)

type PublicationClass string

const (
	FullEpisode  PublicationClass = "full_episode"
	ShortOnly    PublicationClass = "short_only"
	ArchiveOnly  PublicationClass = "archive_only"
	DoNotPublish PublicationClass = "do_not_publish"
)

type EvidenceRef struct {
	Attempt     int    `json:"attempt"`
	EventID     string `json:"event_id"`
	Frame       uint64 `json:"frame"`
	TimestampMS int64  `json:"timestamp_ms"`
}

type Beat struct {
	Kind     string        `json:"kind"`
	Summary  string        `json:"summary"`
	Evidence []EvidenceRef `json:"evidence"`
}

type Score struct {
	Value  int    `json:"value"`
	Reason string `json:"reason"`
}

type Scores struct {
	Novelty              Score  `json:"novelty"`
	Drama                Score  `json:"drama"`
	ChallengeDifficulty  Score  `json:"challenge_difficulty"`
	FailureAndRecovery   Score  `json:"failure_and_recovery"`
	HistoricalImportance *Score `json:"historical_importance,omitempty"`
	RecentSimilarity     *Score `json:"recent_similarity,omitempty"`
}

// HistoricalRun must come from explicitly identified durable statistics. A
// model cannot create or amend these records. Published controls whether the
// run participates in repetitive-content comparisons.
type HistoricalRun struct {
	RunID      string   `json:"run_id"`
	Source     string   `json:"source"`
	Goal       string   `json:"goal"`
	DurationMS int64    `json:"duration_ms"`
	Status     string   `json:"status"`
	EventTypes []string `json:"event_types,omitempty"`
	Published  bool     `json:"published"`
}

type Comparison struct {
	Metric       string   `json:"metric"`
	Sources      []string `json:"sources"`
	BaselineRuns []string `json:"baseline_runs"`
	SampleSize   int      `json:"sample_size"`
	Current      int64    `json:"current"`
	Reference    int64    `json:"reference"`
	Detail       string   `json:"detail"`
}

type Policy struct {
	Version           int `json:"version"`
	FullMinEvents     int `json:"full_min_events"`
	SimilarityCeiling int `json:"similarity_ceiling"`
}

func DefaultPolicy() Policy {
	return Policy{Version: PolicyVersion, FullMinEvents: 4, SimilarityCeiling: 85}
}

type Input struct {
	RunID     string               `json:"run_id,omitempty"`
	Timelines []farm.MediaTimeline `json:"timelines"`
	History   []HistoricalRun      `json:"history,omitempty"`
	Incomplete bool             `json:"incomplete,omitempty"`
}

// EventFact is the only event vocabulary sent to an editorial model. It does
// not contain raw prompts, hidden reasoning, ROM data, or failure dump bytes.
type EventFact struct {
	EvidenceRef
	Type      string `json:"type"`
	Summary   string `json:"summary,omitempty"`
	Objective string `json:"objective,omitempty"`
}

type EvidencePack struct {
	RunID      string       `json:"run_id"`
	Status     string       `json:"status,omitempty"`
	Goal       string       `json:"goal,omitempty"`
	Planner    string       `json:"planner,omitempty"`
	DurationMS int64        `json:"duration_ms"`
	Events     []EventFact  `json:"events"`
	History    []Comparison `json:"verified_history,omitempty"`
	Incomplete bool         `json:"incomplete,omitempty"`
}

// Candidate is the model-owned creative part. Scores, provenance, and the
// maximum permitted publication class are decided by deterministic code.
type Candidate struct {
	PrimaryStory     string           `json:"primary_story"`
	SecondaryStories []string         `json:"secondary_stories,omitempty"`
	Beats            []Beat           `json:"beats"`
	Publication      PublicationClass `json:"publication_class"`
}

// Model is intentionally independent of the gameplay LLM and transport.
type Model interface {
	Identity() string
	Generate(context.Context, EvidencePack) ([]byte, error)
}

type Assessment struct {
	Version       int              `json:"version"`
	RunID         string           `json:"run_id"`
	InputSHA256   string           `json:"input_sha256"`
	Policy        Policy           `json:"policy"`
	PromptVersion string           `json:"prompt_version"`
	Model         string           `json:"model"`
	PrimaryStory  string           `json:"primary_story,omitempty"`
	Secondary     []string         `json:"secondary_stories,omitempty"`
	Beats         []Beat           `json:"beats"`
	Scores        Scores           `json:"scores"`
	Comparisons   []Comparison     `json:"historical_comparisons,omitempty"`
	Publication   PublicationClass `json:"publication_class"`
	Reason        string           `json:"publication_reason"`
	Hash          string           `json:"hash"`
}

func normalizedPolicy(p Policy) (Policy, error) {
	if p == (Policy{}) {
		p = DefaultPolicy()
	}
	if p.Version != PolicyVersion || p.FullMinEvents < 2 || p.FullMinEvents > 100 ||
		p.SimilarityCeiling < 1 || p.SimilarityCeiling > 100 {
		return p, fmt.Errorf("producer: unsupported policy: %+v", p)
	}
	return p, nil
}

func evidence(in Input) (EvidencePack, string, error) {
	pack := EvidencePack{RunID: strings.TrimSpace(in.RunID), Incomplete: in.Incomplete}
	timelines := make([]farm.MediaTimeline, len(in.Timelines))
	seenAttempts := make(map[int]bool)
	for i, raw := range in.Timelines {
		t := raw.Normalized()
		if t.Attempt == 0 {
			t.Attempt = i + 1 // legacy single-attempt artifacts
		}
		if err := t.Validate(); err != nil {
			return pack, "", err
		}
		if pack.RunID == "" {
			pack.RunID = t.Run.RunID
		}
		if t.Run.RunID != pack.RunID || seenAttempts[t.Attempt] {
			return pack, "", errors.New("producer: mismatched run ID or duplicate attempt")
		}
		seenAttempts[t.Attempt] = true
		timelines[i] = t
	}
	if pack.RunID == "" {
		return pack, "", errors.New("producer: run ID is required")
	}
	sort.Slice(timelines, func(i, j int) bool { return timelines[i].Attempt < timelines[j].Attempt })
	for _, t := range timelines {
		pack.DurationMS += int64(math.Round(float64(t.EndFrame) * 1000 / t.FramesPerSecond))
		if pack.Goal == "" {
			pack.Goal = t.Run.Goal
		}
		if pack.Planner == "" {
			pack.Planner = t.Run.Planner
		}
		if t.Run.Status != "" {
			pack.Status = t.Run.Status
		}
		for _, event := range t.Events {
			pack.Events = append(pack.Events, EventFact{
				EvidenceRef: EvidenceRef{Attempt: t.Attempt, EventID: event.ID, Frame: event.Frame, TimestampMS: event.TimestampMS},
				Type:        strings.TrimSpace(event.Type), Summary: compact(event.Summary, 280), Objective: compact(event.Objective, 160),
			})
		}
	}
	sort.SliceStable(pack.Events, func(i, j int) bool {
		if pack.Events[i].Attempt != pack.Events[j].Attempt {
			return pack.Events[i].Attempt < pack.Events[j].Attempt
		}
		if pack.Events[i].Frame != pack.Events[j].Frame {
			return pack.Events[i].Frame < pack.Events[j].Frame
		}
		return pack.Events[i].EventID < pack.Events[j].EventID
	})
	// A historical comparison without a durable source is never accepted.
	history := append([]HistoricalRun(nil), in.History...)
	sort.Slice(history, func(i, j int) bool {
		if history[i].RunID != history[j].RunID {
			return history[i].RunID < history[j].RunID
		}
		return history[i].Source < history[j].Source
	})
	seen := make(map[string]bool)
	for _, run := range history {
		if run.RunID == "" || run.Source == "" || run.DurationMS < 0 ||
			run.RunID == pack.RunID || seen[run.RunID] {
			return pack, "", fmt.Errorf("producer: historical run %q lacks unique, sourced statistics", run.RunID)
		}
		seen[run.RunID] = true
	}
	pack.History = comparisons(pack, history)
	// The hash includes raw normalized timelines (not just selected beats)
	// and the provenance of every historical row. Reassessment is immutable.
	bytes, err := json.Marshal(struct {
		RunID string `json:"run_id"`
		Timelines []farm.MediaTimeline `json:"timelines"`
		History []HistoricalRun `json:"history"`
		Incomplete bool `json:"incomplete"`
	}{pack.RunID, timelines, history, in.Incomplete})
	if err != nil { return pack, "", err }
	sum := sha256.Sum256(bytes)
	return pack, hex.EncodeToString(sum[:]), nil
}

func comparisons(pack EvidencePack, history []HistoricalRun) []Comparison {
	var peers []HistoricalRun
	for _, run := range history {
		if strings.EqualFold(strings.TrimSpace(run.Goal), strings.TrimSpace(pack.Goal)) && pack.Goal != "" && run.Status == "done" && run.DurationMS > 0 {
			peers = append(peers, run)
		}
	}
	var out []Comparison
	if len(peers) > 0 && pack.DurationMS > 0 && pack.Status == "done" {
		var sum int64
		c := Comparison{Metric: "duration_ms", Current: pack.DurationMS, SampleSize: len(peers)}
		for _, peer := range peers {
			sum += peer.DurationMS
			c.BaselineRuns = append(c.BaselineRuns, peer.RunID)
			c.Sources = append(c.Sources, peer.Source)
		}
		c.Reference = sum / int64(len(peers))
		c.Detail = "Compared with sourced completed-history duration observations for the same goal; no record claim"
		out = append(out, c)
	}
	// Only published runs, and only explicit sets of observed event types,
	// are meaningful for an editorial-repetition comparison.
	current := make(map[string]bool)
	for _, e := range pack.Events {
		current[e.Type] = true
	}
	var maxSimilarity int64 = -1
	var closest HistoricalRun
	for _, peer := range history {
		if !peer.Published || len(peer.EventTypes) == 0 || len(current) == 0 {
			continue
		}
		union := make(map[string]bool)
		peerTypes := make(map[string]bool)
		intersection := 0
		for kind := range current {
			union[kind] = true
		}
		for _, kind := range peer.EventTypes {
			peerTypes[kind] = true
		}
		for kind := range peerTypes {
			union[kind] = true
			if current[kind] {
				intersection++
			}
		}
		percent := int64(100 * intersection / len(union))
		if percent > maxSimilarity {
			maxSimilarity, closest = percent, peer
		}
	}
	if maxSimilarity >= 0 {
		out = append(out, Comparison{
			Metric:  "recent_published_event_type_similarity_percent",
			Current: maxSimilarity, Reference: 100, SampleSize: 1,
			BaselineRuns: []string{closest.RunID}, Sources: []string{closest.Source},
			Detail: "Jaccard similarity of observed semantic event types (not a claim about visual similarity)",
		})
	}
	return out
}

func compact(v string, max int) string {
	v = strings.TrimSpace(v)
	if len(v) > max {
		return v[:max]
	}
	return v
}

func decodeCandidate(data []byte) (Candidate, error) {
	var c Candidate
	if len(data) == 0 || len(data) > maxModelBytes {
		return c, errors.New("producer: invalid model response size")
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("producer: malformed model JSON: %w", err)
	}
	if d.Decode(new(any)) != io.EOF {
		return c, errors.New("producer: model returned trailing JSON")
	}
	return c, nil
}

func ValidateCandidate(c Candidate, pack EvidencePack) error {
	if len(c.PrimaryStory) > 800 || len(c.SecondaryStories) > 5 || len(c.Beats) > 16 {
		return errors.New("producer: oversized narrative")
	}
	for _, s := range c.SecondaryStories {
		if strings.TrimSpace(s) == "" || len(s) > 400 {
			return errors.New("producer: invalid secondary story")
		}
	}
	if !validClass(c.Publication) {
		return errors.New("producer: unknown publication class")
	}
	available := make(map[string]EventFact, len(pack.Events))
	for _, e := range pack.Events {
		available[fmt.Sprintf("%d/%s", e.Attempt, e.EventID)] = e
	}
	for i, beat := range c.Beats {
		if strings.TrimSpace(beat.Kind) == "" || len(beat.Kind) > 40 ||
			strings.TrimSpace(beat.Summary) == "" || len(beat.Summary) > 500 ||
			len(beat.Evidence) == 0 || len(beat.Evidence) > 8 {
			return fmt.Errorf("producer: beat %d has invalid shape or no evidence", i)
		}
		for _, ref := range beat.Evidence {
			event, ok := available[fmt.Sprintf("%d/%s", ref.Attempt, ref.EventID)]
			if !ok || ref.Frame != event.Frame || ref.TimestampMS != event.TimestampMS {
				return fmt.Errorf("producer: beat %d cites missing or mismatched event %q", i, ref.EventID)
			}
		}
	}
	if len(pack.Events) == 0 && (len(c.Beats) > 0 || c.PrimaryStory != "") {
		return errors.New("producer: story without event telemetry")
	}
	if (len(c.Beats) > 0) != (strings.TrimSpace(c.PrimaryStory) != "") {
		return errors.New("producer: story and selected beats must appear together")
	}
	if c.Publication == FullEpisode && len(c.Beats) < 2 {
		return errors.New("producer: full episode requires at least two cited beats")
	}
	if c.Publication == FullEpisode || c.Publication == ShortOnly {
		if len(c.Beats) == 0 || strings.TrimSpace(c.PrimaryStory) == "" {
			return errors.New("producer: publishable content requires evidence-backed story")
		}
	}
	return nil
}

func validClass(c PublicationClass) bool {
	switch c {
	case FullEpisode, ShortOnly, ArchiveOnly, DoNotPublish:
		return true
	}
	return false
}

func scores(pack EvidencePack) Scores {
	var badge, battle, catch, failure, recovery, milestone int
	for _, e := range pack.Events {
		switch e.Type {
		case "badge_acquired":
			badge++
		case "gym_battle", "boss_battle":
			battle++
		case "rare_catch", "important_catch", "evolution":
			catch++
		case "blackout", "failure_terminal", "planner_stall", "objective_failed":
			failure++
		case "planner_recovery", "failure_recovered":
			recovery++
		case "milestone", "run_finished":
			milestone++
		}
	}
	out := Scores{
		Novelty:             Score{Value: clamp(18*catch + 8*badge), Reason: "Observed rare catches, evolutions, and badges; does not imply a record"},
		Drama:               Score{Value: clamp(20*failure + 13*recovery + 10*battle), Reason: "Observed failures, recoveries, and major battles"},
		ChallengeDifficulty: Score{Value: clamp(12*battle + 10*failure + 4*badge), Reason: "Observed battles and failures; no unsupported difficulty claim"},
		FailureAndRecovery:  Score{Value: clamp(22*failure + 20*recovery), Reason: "Observed failure/recovery events"},
	}
	for _, c := range pack.History {
		switch c.Metric {
		case "duration_ms":
			value := 35
			if c.Current < c.Reference {
				value = 70
			}
			out.HistoricalImportance = &Score{Value: value, Reason: "Sourced same-goal duration comparison; not a record"}
		case "recent_published_event_type_similarity_percent":
			out.RecentSimilarity = &Score{Value: int(c.Current), Reason: "Sourced published-run event-type Jaccard similarity"}
		}
	}
	return out
}

func clamp(value int) int {
	if value < 0 { return 0 }
	if value > 100 { return 100 }
	return value
}

func rank(c PublicationClass) int {
	switch c { case FullEpisode: return 3; case ShortOnly: return 2; case ArchiveOnly: return 1 }
	return 0
}

func publicationCap(pack EvidencePack, score Scores, policy Policy) (PublicationClass, string) {
	if len(pack.Events) == 0 { return DoNotPublish, "No durable event evidence; nothing can be supported editorially" }
	if pack.Incomplete { return ArchiveOnly, "One or more run attempts are missing durable semantic timelines" }
	switch pack.Status {
	case "done", "failed", "error", "stuck", "budget", "cancelled", "canceled", "timeout":
	default:
		return ArchiveOnly, "Run has no verified terminal status"
	}
	types := make(map[string]bool)
	significant := 0
	for _, e := range pack.Events {
		switch e.Type {
		case "run_finished", "run_failed", "gym_battle", "boss_battle", "badge_acquired",
			"blackout", "rare_catch", "important_catch", "evolution", "planner_recovery",
			"failure_recovered", "failure_terminal", "milestone":
			significant++
			types[e.Type] = true
		}
	}
	if significant == 0 {
		return ArchiveOnly, "No distinct editorially significant evidence"
	}
	if score.RecentSimilarity != nil && score.RecentSimilarity.Value >= policy.SimilarityCeiling {
		return ArchiveOnly, "Highly repetitive of a sourced published run"
	}
	if significant >= policy.FullMinEvents && len(types) >= 2 {
		return FullEpisode, "Enough distinct, evidenced milestones for episode consideration"
	}
	return ShortOnly, "Evidence supports a clip, but not a full episode"
}

func rulesCandidate(pack EvidencePack, cap PublicationClass) Candidate {
	c := Candidate{Publication: cap}
	if cap == DoNotPublish || len(pack.Events) == 0 {
		return c
	}
	var selected []EventFact
	seen := make(map[string]bool)
	for _, e := range pack.Events {
		switch e.Type {
		case "run_finished", "run_failed", "badge_acquired", "gym_battle", "boss_battle",
			"blackout", "rare_catch", "important_catch", "evolution", "planner_recovery",
			"failure_recovered", "failure_terminal", "milestone":
		default:
			continue
		}
		if !seen[e.Type] {
			selected = append(selected, e)
			seen[e.Type] = true
		}
		if len(selected) == 8 {
			break
		}
	}
	for _, e := range selected {
		summary := e.Summary
		if summary == "" {
			summary = strings.ReplaceAll(e.Type, "_", " ")
		}
		c.Beats = append(c.Beats, Beat{Kind: e.Type, Summary: summary, Evidence: []EvidenceRef{e.EvidenceRef}})
	}
	if len(c.Beats) == 0 {
		return c
	}
	if pack.Goal != "" {
		c.PrimaryStory = "Run goal: " + compact(pack.Goal, 240) + "; observed: " + compact(c.Beats[0].Summary, 240)
	} else {
		c.PrimaryStory = "Observed run event: " + compact(c.Beats[0].Summary, 240)
	}
	return c
}

// Assess validates every selected model beat against exact durable event IDs,
// applies a conservative publication ceiling, and returns a content-addressed,
// reproducible assessment. Nil model uses an explicitly identified rules-only
// producer so local/offline re-evaluation still works.
func Assess(ctx context.Context, in Input, model Model, policy Policy) (Assessment, error) {
	policy, err := normalizedPolicy(policy)
	if err != nil {
		return Assessment{}, err
	}
	pack, inputHash, err := evidence(in)
	if err != nil {
		return Assessment{}, err
	}
	s := scores(pack)
	cap, reason := publicationCap(pack, s, policy)
	identity := "rules-v1"
	promptVersion := "none"
	var candidate Candidate
	if model != nil && len(pack.Events) > 0 {
		identity = strings.TrimSpace(model.Identity())
		if identity == "" {
			return Assessment{}, errors.New("producer: model identity is required")
		}
		promptVersion = PromptVersion
		raw, err := model.Generate(ctx, pack)
		if err != nil {
			return Assessment{}, fmt.Errorf("producer: model: %w", err)
		}
		candidate, err = decodeCandidate(raw)
		if err != nil {
			return Assessment{}, err
		}
	} else {
		candidate = rulesCandidate(pack, cap)
	}
	if err := ValidateCandidate(candidate, pack); err != nil {
		return Assessment{}, err
	}
	publication := candidate.Publication
	if rank(publication) > rank(cap) {
		publication = cap
	}
	if publication != candidate.Publication {
		reason = "Publication downgraded: " + reason
	} else if rank(publication) < rank(cap) {
		reason = "Producer declined publication or selected a smaller format"
	}
	out := Assessment{
		Version: SchemaVersion, RunID: pack.RunID, InputSHA256: inputHash,
		Policy: policy, PromptVersion: promptVersion, Model: identity,
		PrimaryStory: strings.TrimSpace(candidate.PrimaryStory), Secondary: candidate.SecondaryStories,
		Beats: candidate.Beats, Scores: s, Comparisons: pack.History,
		Publication: publication, Reason: reason,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return Assessment{}, err
	}
	hash := sha256.Sum256(b)
	out.Hash = hex.EncodeToString(hash[:])
	return out, nil
}

// Validate repeats the deterministic proof for persisted assessments. A
// different timeline, model/prompt/policy, or changed evidence must be assessed
// anew rather than silently treating stale stories as current.
func Validate(out Assessment, in Input) error {
	if out.Version != SchemaVersion { return errors.New("producer: unsupported assessment version") }
	policy, err := normalizedPolicy(out.Policy)
	if err != nil { return err }
	pack, fingerprint, err := evidence(in)
	if err != nil { return err }
	if pack.RunID != out.RunID || fingerprint != out.InputSHA256 { return errors.New("producer: source timeline/history changed") }
	if err := ValidateCandidate(Candidate{
		PrimaryStory: out.PrimaryStory, SecondaryStories: out.Secondary,
		Beats: out.Beats, Publication: out.Publication,
	}, pack); err != nil { return err }
	if !validClass(out.Publication) || out.Model == "" { return errors.New("producer: invalid assessment identity") }
	if !reflect.DeepEqual(out.Scores, scores(pack)) || !reflect.DeepEqual(out.Comparisons, pack.History) {
		return errors.New("producer: deterministic scores or history were changed")
	}
	cap, _ := publicationCap(pack, scores(pack), policy)
	if rank(out.Publication) > rank(cap) { return errors.New("producer: unsafe publication class") }
	if out.Model == "rules-v1" && out.PromptVersion != "none" { return errors.New("producer: rules identity mismatch") }
	if out.Model != "rules-v1" && out.PromptVersion != PromptVersion { return errors.New("producer: prompt identity mismatch") }
	return VerifyChecksum(out)
}

// VerifyChecksum is usable by immutable artifact readers that do not have
// the original timeline available. Full Validate should still be used when
// the source artifacts can be loaded.
func VerifyChecksum(out Assessment) error {
	hash := out.Hash
	out.Hash = ""
	b, err := json.Marshal(out)
	if err != nil { return err }
	sum := sha256.Sum256(b)
	if hash != hex.EncodeToString(sum[:]) { return errors.New("producer: assessment content hash mismatch") }
	return nil
}
