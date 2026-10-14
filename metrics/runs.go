package metrics

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Run duration buckets span seconds to two days. Farm runs are hours long;
// the latency buckets on DurationBuckets would pile every run into +Inf.
var RunDurationBuckets = []float64{
	30, 60, 120, 300, 600, 900,
	1800, 3600, 7200, 14400, 28800, 57600, 86400, 172800,
}

// RunRoundBuckets count planner rounds observed when a run becomes terminal.
var RunRoundBuckets = []float64{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000}

// RunFrameBuckets count emulator frames observed when a run becomes terminal.
var RunFrameBuckets = []float64{1e3, 1e4, 3e4, 1e5, 3e5, 1e6, 2e6, 5e6, 1e7, 2e7}

// Terminal result labels. Raw finish reasons stay out of Prometheus.
const (
	ResultCompleted = "completed"
	ResultFailed    = "failed"
	ResultAborted   = "aborted"
)

// MaxBadgeMilestones is the largest badge index published as badge_N.
// Sixteen covers a full Johto plus Kanto badge set.
const MaxBadgeMilestones = 16

const stoppingFloor = "started"

var gameLabel = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var storyMilestone = map[string]string{
	"pokédex":         "pokedex",
	"pokedex":         "pokedex",
	"mt. moon fossil": "mt_moon_fossil",
	"s.s. ticket":     "ss_ticket",
	"hm01":            "hm01",
	"silph scope":     "silph_scope",
	"poké flute":      "poke_flute",
	"poke flute":      "poke_flute",
	"hm03 surf":       "surf",
	"hm04 strength":   "strength",
	"card key":        "card_key",
	"silph rescue":    "silph_rescue",
	"secret key":      "secret_key",
	"champion":        "champion",
	"hall of fame":    "hall_of_fame",
}

var milestoneRank map[string]int

func init() {
	order := []string{"starter"}
	for n := 1; n <= MaxBadgeMilestones; n++ {
		order = append(order, fmt.Sprintf("badge_%d", n))
	}
	order = append(order,
		"pokedex", "mt_moon_fossil", "ss_ticket", "hm01", "silph_scope", "poke_flute",
		"surf", "strength", "card_key", "silph_rescue", "secret_key",
		"champion", "hall_of_fame", "completed",
	)
	milestoneRank = make(map[string]int, len(order))
	for i, id := range order {
		milestoneRank[id] = i
	}
}

// NormalizeGame clamps a game id to a bounded label. Empty and unsafe values
// become "unknown". An empty game is not rewritten to a specific cartridge.
func NormalizeGame(game string) string {
	game = strings.ToLower(strings.TrimSpace(game))
	if game == "" || len(game) > 40 || !gameLabel.MatchString(game) {
		return "unknown"
	}
	return game
}

// NormalizeMode clamps a planner/mode string to the small set the farm
// actually leases. Anything else is "other".
func NormalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "llm", "scripted", "policy", "launch":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "other"
	}
}

// RunResult maps a wall finish reason to a bounded result label.
// cancelled is the wall's cooperative-cancel flag, which settles some
// finishes as aborted even when the runner's reason string differs.
func RunResult(reason string, cancelled bool) string {
	switch {
	case reason == "done":
		return ResultCompleted
	case cancelled || reason == "cancelled":
		return ResultAborted
	default:
		return ResultFailed
	}
}

// KnownMilestone reports whether id is part of the funnel vocabulary.
// "started" is a stopping-point floor, not a funnel event.
func KnownMilestone(id string) bool {
	_, ok := milestoneRank[id]
	return ok
}

// BadgeMilestone returns badge_N for a 1-based badge count inside the cap.
func BadgeMilestone(n int) (string, bool) {
	if n < 1 || n > MaxBadgeMilestones {
		return "", false
	}
	return fmt.Sprintf("badge_%d", n), true
}

// StoryMilestone maps a player milestone label the runner already emits onto
// a funnel id. Unknown labels are dropped so a new string cannot open a series.
func StoryMilestone(label string) (string, bool) {
	id, ok := storyMilestone[strings.ToLower(strings.TrimSpace(label))]
	return id, ok
}

// FurthestMilestone is the highest funnel stage in ids, or "started" when
// the run ended before any stage.
func FurthestMilestone(ids []string) string {
	best := stoppingFloor
	bestRank := -1
	for _, id := range ids {
		rank, ok := milestoneRank[id]
		if !ok || rank <= bestRank {
			continue
		}
		best, bestRank = id, rank
	}
	return best
}

// RunFinish is one terminal observation. Result must be a Result* constant.
// Duration is recorded only when HasDuration is set; a zero QueuedAt is
// missing data, not a zero-length run.
type RunFinish struct {
	Game        string
	Mode        string
	Result      string
	Milestone   string
	Duration    time.Duration
	HasDuration bool
	Rounds      int
	Frames      uint64
	At          time.Time
}

// Runs publishes the run-lifecycle series for one process.
type Runs struct {
	mu          sync.Mutex
	last        map[string]float64
	started     *prometheus.CounterVec
	completed   *prometheus.CounterVec
	failed      *prometheus.CounterVec
	aborted     *prometheus.CounterVec
	active      *prometheus.GaugeVec
	duration    *prometheus.HistogramVec
	rounds      *prometheus.HistogramVec
	frames      *prometheus.HistogramVec
	lastSuccess *prometheus.GaugeVec
	milestones  *prometheus.CounterVec
	stopping    *prometheus.CounterVec
}

// NewRuns registers the run-lifecycle collectors on reg.
func NewRuns(reg *Registry) (*Runs, error) {
	if reg == nil {
		return nil, fmt.Errorf("metrics: nil registry")
	}
	s := &Runs{last: map[string]float64{}}
	var err error
	s.started, err = reg.NewCounterVec("pokepilot_runs_started_total",
		"Runs accepted onto the wall, including a manual re-queue of a finished run. Retries of the same campaign are not starts.",
		[]string{"game", "mode"})
	if err != nil {
		return nil, err
	}
	s.completed, err = reg.NewCounterVec("pokepilot_runs_completed_total",
		"Runs that reached a terminal success (finish reason done).",
		[]string{"game", "mode"})
	if err != nil {
		return nil, err
	}
	s.failed, err = reg.NewCounterVec("pokepilot_runs_failed_total",
		"Runs that ended terminal without success or cancellation. A retry that re-queues the same run is not a failure.",
		[]string{"game", "mode"})
	if err != nil {
		return nil, err
	}
	s.aborted, err = reg.NewCounterVec("pokepilot_runs_aborted_total",
		"Runs ended by operator or worker cancellation.",
		[]string{"game", "mode"})
	if err != nil {
		return nil, err
	}
	s.active, err = reg.NewGaugeVec("pokepilot_runs_active",
		"Runs currently leased or running. Rebuilt from restored tiles after a process restart.",
		[]string{"game"})
	if err != nil {
		return nil, err
	}
	s.duration, err = reg.NewHistogramVec("pokepilot_run_duration_seconds",
		"Wall-clock seconds from queue to a terminal result.",
		[]string{"game", "result"}, RunDurationBuckets)
	if err != nil {
		return nil, err
	}
	s.rounds, err = reg.NewHistogramVec("pokepilot_run_rounds",
		"Planner rounds observed on a run that reached a terminal result.",
		[]string{"game", "result"}, RunRoundBuckets)
	if err != nil {
		return nil, err
	}
	s.frames, err = reg.NewHistogramVec("pokepilot_run_frames",
		"Emulator frames observed on a run that reached a terminal result.",
		[]string{"game", "result"}, RunFrameBuckets)
	if err != nil {
		return nil, err
	}
	s.lastSuccess, err = reg.NewGaugeVec("pokepilot_last_successful_run_timestamp_seconds",
		"Unix time of the newest terminal success seen for a game. Restored from wall state and the history catalog after a restart, and never moved backwards.",
		[]string{"game"})
	if err != nil {
		return nil, err
	}
	s.milestones, err = reg.NewCounterVec("pokepilot_run_milestones_total",
		"Runs that newly reached a funnel stage. Each stage is counted once per campaign, including across retries.",
		[]string{"game", "milestone"})
	if err != nil {
		return nil, err
	}
	s.stopping, err = reg.NewCounterVec("pokepilot_run_stopping_milestone_total",
		"Terminal runs, labeled by the furthest funnel stage reached. started means the run ended before any stage.",
		[]string{"game", "result", "milestone"})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// MustRuns is NewRuns for process startup.
func MustRuns(reg *Registry) *Runs {
	s, err := NewRuns(reg)
	if err != nil {
		panic(err)
	}
	return s
}

// Started counts one newly queued campaign.
func (s *Runs) Started(game, mode string) {
	if s == nil {
		return
	}
	s.started.WithLabelValues(NormalizeGame(game), NormalizeMode(mode)).Inc()
}

// MarkActive counts one run that became leased or running.
func (s *Runs) MarkActive(game string) {
	if s == nil {
		return
	}
	s.active.WithLabelValues(NormalizeGame(game)).Inc()
}

// ClearActive removes one run from the active gauge.
func (s *Runs) ClearActive(game string) {
	if s == nil {
		return
	}
	s.active.WithLabelValues(NormalizeGame(game)).Dec()
}

// ReplaceActive sets the active gauge from restored tiles. It does not
// increment started/completed/failed/aborted.
func (s *Runs) ReplaceActive(counts map[string]int) {
	if s == nil {
		return
	}
	s.active.Reset()
	for game, n := range counts {
		if n <= 0 {
			continue
		}
		s.active.WithLabelValues(NormalizeGame(game)).Set(float64(n))
	}
}

// Milestone counts one newly reached funnel stage. Callers dedupe per campaign.
func (s *Runs) Milestone(game, id string) {
	if s == nil || !KnownMilestone(id) {
		return
	}
	s.milestones.WithLabelValues(NormalizeGame(game), id).Inc()
}

// NoteSuccess raises the last-success gauge when unixSeconds is newer.
func (s *Runs) NoteSuccess(game string, unixSeconds float64) {
	if s == nil || unixSeconds <= 0 {
		return
	}
	game = NormalizeGame(game)
	s.mu.Lock()
	defer s.mu.Unlock()
	if unixSeconds <= s.last[game] {
		return
	}
	s.last[game] = unixSeconds
	s.lastSuccess.WithLabelValues(game).Set(unixSeconds)
}

// Finished records one terminal transition. A second call is a second run;
// callers must not invoke it for a duplicate finish of the same campaign.
func (s *Runs) Finished(obs RunFinish) {
	if s == nil {
		return
	}
	game := NormalizeGame(obs.Game)
	mode := NormalizeMode(obs.Mode)
	switch obs.Result {
	case ResultCompleted:
		s.completed.WithLabelValues(game, mode).Inc()
	case ResultAborted:
		s.aborted.WithLabelValues(game, mode).Inc()
	default:
		obs.Result = ResultFailed
		s.failed.WithLabelValues(game, mode).Inc()
	}
	if obs.HasDuration && obs.Duration >= 0 {
		s.duration.WithLabelValues(game, obs.Result).Observe(obs.Duration.Seconds())
	}
	if obs.Rounds > 0 {
		s.rounds.WithLabelValues(game, obs.Result).Observe(float64(obs.Rounds))
	}
	if obs.Frames > 0 {
		s.frames.WithLabelValues(game, obs.Result).Observe(float64(obs.Frames))
	}
	milestone := obs.Milestone
	if milestone != stoppingFloor && !KnownMilestone(milestone) {
		milestone = stoppingFloor
	}
	s.stopping.WithLabelValues(game, obs.Result, milestone).Inc()
	if obs.Result == ResultCompleted && !obs.At.IsZero() {
		s.NoteSuccess(game, float64(obs.At.Unix()))
	}
}
