package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const WatchdogReproVersion = 1
const WatchdogReproArtifactName = "watchdog-repro.json"

const (
	WatchdogBoundaryRound     = "round_boundary"
	WatchdogBoundaryObjective = "successful_objective"
)

// WatchdogRepro is a ROM-free snapshot of the policy state immediately before
// a watchdog decision that stopped a run. It intentionally stores semantic
// inputs rather than the derived majorProgressMark so a newer checkout can
// re-evaluate those inputs with its current definition of meaningful progress.
type WatchdogRepro struct {
	Version       int                 `json:"version"`
	Boundary      string              `json:"boundary"`
	Round         int                 `json:"round"`
	Completed     int                 `json:"completed,omitempty"`
	Strategic     bool                `json:"strategic"`
	State         WatchdogPolicyState `json:"state"`
	Current       WatchdogObservation `json:"current,omitempty"`
	Before        WatchdogObservation `json:"before,omitempty"`
	After         WatchdogObservation `json:"after,omitempty"`
	ExpectedStop  string              `json:"expected_stop"`
	ExpectedCause string              `json:"expected_cause"`
}

type WatchdogPolicyState struct {
	StuckAfter             int                  `json:"stuck_after"`
	StagnationAfter        int                  `json:"stagnation_after"`
	HighWater              WatchdogProgressMark `json:"high_water"`
	LastMajorProgressRound int                  `json:"last_major_progress_round"`
	StagnationEscalated    bool                 `json:"stagnation_escalated"`
	StuckEscalated         bool                 `json:"stuck_escalated"`
	Stuck                  int                  `json:"stuck"`
	Dead                   WatchdogDeadPosition `json:"dead"`
}

type WatchdogProgressMark struct {
	Badges     int   `json:"badges"`
	Events     int   `json:"events"`
	Maps       int   `json:"maps"`
	PartyCount int   `json:"party_count"`
	DexOwned   int   `json:"dex_owned"`
	MaxLevel   uint8 `json:"max_level"`
}

type WatchdogDeadPosition struct {
	Map       uint8 `json:"map"`
	X         uint8 `json:"x"`
	Y         uint8 `json:"y"`
	Completed int   `json:"completed"`
	Streak    int   `json:"streak"`
	Set       bool  `json:"set"`
}

// WatchdogObservation is the subset needed by both watchdog progress tests.
// VisitedMaps belongs to Knowledge at runtime, but rides the observation input
// here because it is one semantic input to the decision being replayed.
type WatchdogObservation struct {
	Map          uint8       `json:"map"`
	X            uint8       `json:"x"`
	Y            uint8       `json:"y"`
	PartyCount   int         `json:"party_count"`
	PartyLevels  []uint8     `json:"party_levels,omitempty"`
	Badges       []string    `json:"badges,omitempty"`
	Events       []string    `json:"events,omitempty"`
	PokedexOwned []SpeciesID `json:"pokedex_owned,omitempty"`
	VisitedMaps  []uint8     `json:"visited_maps,omitempty"`
}

type WatchdogReplayResult struct {
	Stop         Stop   `json:"-"`
	StopName     string `json:"stop"`
	Cause        string `json:"cause,omitempty"`
	ReplanReason string `json:"replan_reason,omitempty"`
}

func (w *runWatchdogPolicy) captureRoundBoundary(round int, obs Observation, known *Knowledge, completed int, strategic bool) WatchdogRepro {
	return WatchdogRepro{
		Version:   WatchdogReproVersion,
		Boundary:  WatchdogBoundaryRound,
		Round:     round,
		Completed: completed,
		Strategic: strategic,
		State:     w.reproState(),
		Current:   watchdogObservationOf(obs, known),
	}
}

func (w *runWatchdogPolicy) captureSuccessfulObjective(round int, before, after Observation, strategic bool) WatchdogRepro {
	return WatchdogRepro{
		Version:   WatchdogReproVersion,
		Boundary:  WatchdogBoundaryObjective,
		Round:     round,
		Strategic: strategic,
		State:     w.reproState(),
		Before:    watchdogObservationOf(before, nil),
		After:     watchdogObservationOf(after, nil),
	}
}

func (w *runWatchdogPolicy) reproState() WatchdogPolicyState {
	return WatchdogPolicyState{
		StuckAfter:             w.stuckAfter,
		StagnationAfter:        w.stagnationAfter,
		HighWater:              watchdogProgressMarkOf(w.majorProgress),
		LastMajorProgressRound: w.lastMajorProgressRound,
		StagnationEscalated:    w.stagnationEscalated,
		StuckEscalated:         w.stuckEscalated,
		Stuck:                  w.stuck,
		Dead: WatchdogDeadPosition{
			Map: w.dead.Map, X: w.dead.X, Y: w.dead.Y, Completed: w.dead.completed, Streak: w.dead.streak, Set: w.dead.set,
		},
	}
}

func (r *WatchdogRepro) recordExpected(d runWatchdogDecision) {
	r.ExpectedStop = stopReplayName(d.Stop)
	r.ExpectedCause = watchdogCauseName(d.Cause)
}

func persistWatchdogRepro(dir string, repro WatchdogRepro, decision runWatchdogDecision) {
	if dir == "" || decision.Stop == StopUnset {
		return
	}
	repro.recordExpected(decision)
	data, err := json.MarshalIndent(repro, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, WatchdogReproArtifactName), append(data, '\n'), 0o644)
}

// ReplayWatchdogRepro re-runs only the captured policy transition. It neither
// opens a ROM nor calls a planner, so it is safe for CI and stable across model
// endpoints. StopUnset means the newer policy no longer terminates here.
func ReplayWatchdogRepro(r WatchdogRepro) (WatchdogReplayResult, error) {
	if r.Version != WatchdogReproVersion {
		return WatchdogReplayResult{}, fmt.Errorf("agent: watchdog repro version %d, want %d", r.Version, WatchdogReproVersion)
	}
	w := watchdogPolicyFromRepro(r.State)
	var d runWatchdogDecision
	switch r.Boundary {
	case WatchdogBoundaryRound:
		obs, known := r.Current.runtime()
		d = w.roundBoundary(r.Round, obs, known, r.Completed, r.Strategic)
	case WatchdogBoundaryObjective:
		before, _ := r.Before.runtime()
		after, _ := r.After.runtime()
		d = w.successfulObjective(before, after, r.Strategic)
	default:
		return WatchdogReplayResult{}, fmt.Errorf("agent: unknown watchdog repro boundary %q", r.Boundary)
	}
	return WatchdogReplayResult{
		Stop:         d.Stop,
		StopName:     stopReplayName(d.Stop),
		Cause:        watchdogCauseName(d.Cause),
		ReplanReason: d.ReplanReason,
	}, nil
}

func watchdogPolicyFromRepro(s WatchdogPolicyState) *runWatchdogPolicy {
	return &runWatchdogPolicy{
		stuckAfter:             s.StuckAfter,
		stagnationAfter:        s.StagnationAfter,
		majorProgress:          majorProgressMarkFromWatchdog(s.HighWater),
		lastMajorProgressRound: s.LastMajorProgressRound,
		stagnationEscalated:    s.StagnationEscalated,
		stuckEscalated:         s.StuckEscalated,
		stuck:                  s.Stuck,
		dead: deadPosition{
			Map: s.Dead.Map, X: s.Dead.X, Y: s.Dead.Y, completed: s.Dead.Completed, streak: s.Dead.Streak, set: s.Dead.Set,
		},
	}
}

func watchdogObservationOf(obs Observation, known *Knowledge) WatchdogObservation {
	out := WatchdogObservation{
		Map:          obs.Map,
		X:            obs.X,
		Y:            obs.Y,
		PartyCount:   obs.PartyCount,
		Badges:       append([]string(nil), obs.Badges...),
		Events:       append([]string(nil), obs.Events...),
		PokedexOwned: append([]SpeciesID(nil), obs.PokedexOwned...),
		PartyLevels:  make([]uint8, 0, len(obs.Party)),
	}
	for _, mon := range obs.Party {
		out.PartyLevels = append(out.PartyLevels, mon.Level)
	}
	if known != nil {
		out.VisitedMaps = make([]uint8, 0, len(known.Visited))
		for id := range known.Visited {
			out.VisitedMaps = append(out.VisitedMaps, id)
		}
		sort.Slice(out.VisitedMaps, func(i, j int) bool { return out.VisitedMaps[i] < out.VisitedMaps[j] })
	}
	return out
}

func (in WatchdogObservation) runtime() (Observation, *Knowledge) {
	obs := Observation{
		Map:          in.Map,
		X:            in.X,
		Y:            in.Y,
		PartyCount:   in.PartyCount,
		Badges:       append([]string(nil), in.Badges...),
		Events:       append([]string(nil), in.Events...),
		PokedexOwned: append([]SpeciesID(nil), in.PokedexOwned...),
		Party:        make([]PartyMon, len(in.PartyLevels)),
	}
	for i, level := range in.PartyLevels {
		obs.Party[i].Level = level
	}
	known := &Knowledge{Visited: make(map[uint8]bool, len(in.VisitedMaps))}
	for _, id := range in.VisitedMaps {
		known.Visited[id] = true
	}
	return obs, known
}

func watchdogProgressMarkOf(m majorProgressMark) WatchdogProgressMark {
	return WatchdogProgressMark{
		Badges: m.Badges, Events: m.Events, Maps: m.Maps, PartyCount: m.PartyCount, DexOwned: m.DexOwned, MaxLevel: m.MaxLevel,
	}
}

func majorProgressMarkFromWatchdog(m WatchdogProgressMark) majorProgressMark {
	return majorProgressMark{
		Badges: m.Badges, Events: m.Events, Maps: m.Maps, PartyCount: m.PartyCount, DexOwned: m.DexOwned, MaxLevel: m.MaxLevel,
	}
}

func watchdogCauseName(c runWatchdogCause) string {
	switch c {
	case runWatchdogStagnation:
		return "stagnation"
	case runWatchdogStagnationRecurred:
		return "stagnation_recurred"
	case runWatchdogDeadPosition:
		return "dead_position"
	case runWatchdogShortStuck:
		return "short_stuck"
	default:
		return ""
	}
}

func stopReplayName(s Stop) string {
	switch s {
	case StopDone:
		return "done"
	case StopStuck:
		return "stuck"
	case StopBudget:
		return "budget"
	case StopFailed:
		return "failed"
	case StopError:
		return "error"
	default:
		return ""
	}
}
