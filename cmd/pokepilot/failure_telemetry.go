package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

var (
	failureRoundRE = regexp.MustCompile(`^round ([0-9]+): (.+) -> failed: (.+), map ([0-9a-fA-F]{2}) at \(([0-9]+),([0-9]+)\)$`)
	progressRoundRE = regexp.MustCompile(`^round ([0-9]+): major progress -> `)
	failureHexRE = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	failureNumRE = regexp.MustCompile(`[0-9]+`)
)

// objectiveFailureTelemetry is process-local run telemetry. A farm worker runs
// one lease at a time, so the controlled agent log is a cheap and reliable
// observation seam: Run already writes every failed objective with round,
// error and final map/coordinates. The summary is drained into the finish
// artifact before the worker leases its next run.
var objectiveFailureTelemetry = struct {
	sync.Mutex
	groups            map[string]*farm.ObjectiveFailure
	lastProgressRound int
}{groups: map[string]*farm.ObjectiveFailure{}}

func observeAgentLogLine(line string) {
	if m := progressRoundRE.FindStringSubmatch(line); m != nil {
		round, _ := strconv.Atoi(m[1])
		objectiveFailureTelemetry.Lock()
		if round > objectiveFailureTelemetry.lastProgressRound {
			objectiveFailureTelemetry.lastProgressRound = round
		}
		objectiveFailureTelemetry.Unlock()
		return
	}

	m := failureRoundRE.FindStringSubmatch(line)
	if m == nil {
		return
	}
	round, err := strconv.Atoi(m[1])
	if err != nil {
		return
	}
	map64, err := strconv.ParseUint(m[4], 16, 8)
	if err != nil {
		return
	}
	x64, err := strconv.ParseUint(m[5], 10, 8)
	if err != nil {
		return
	}
	y64, err := strconv.ParseUint(m[6], 10, 8)
	if err != nil {
		return
	}
	objective, detail := m[2], m[3]
	mapID := uint8(map64)
	key := objectiveFailureTelemetryKey(objective, detail, mapID)

	objectiveFailureTelemetry.Lock()
	defer objectiveFailureTelemetry.Unlock()
	f := objectiveFailureTelemetry.groups[key]
	if f == nil {
		f = &farm.ObjectiveFailure{
			Objective:  objective,
			Error:      detail,
			FirstRound: round,
		}
		objectiveFailureTelemetry.groups[key] = f
	}
	f.Count++
	f.LastRound = round
	f.Map = mapID
	f.X = uint8(x64)
	f.Y = uint8(y64)
}

func objectiveFailureTelemetryKey(objective, detail string, mapID uint8) string {
	// Normalize volatile numbers inside the objective/error while preserving
	// the map as an explicit dimension. That merges "level 10"/"level 11"
	// instances of the same failure shape but does not merge a Mt. Moon route
	// wall with an otherwise identical route wall on Route 1.
	s := objective + " | " + detail
	s = failureHexRE.ReplaceAllString(s, "<hex>")
	s = failureNumRE.ReplaceAllString(s, "<n>")
	return fmt.Sprintf("%s | map=%02x", strings.TrimSpace(s), mapID)
}

// drainObjectiveFailureTelemetry finalizes the current run and clears the
// process-local collector. A failure is recovered when the run made major
// monotonic progress after its last occurrence (or completed its goal). A
// repeated, still-unrecovered failure on a run that ended failed/stuck is a
// progression-blocker candidate; the wall reports those as critical issues.
func drainObjectiveFailureTelemetry(reason string) []farm.ObjectiveFailure {
	objectiveFailureTelemetry.Lock()
	groups := objectiveFailureTelemetry.groups
	lastProgress := objectiveFailureTelemetry.lastProgressRound
	objectiveFailureTelemetry.groups = map[string]*farm.ObjectiveFailure{}
	objectiveFailureTelemetry.lastProgressRound = 0
	objectiveFailureTelemetry.Unlock()

	observedAt := time.Now().UTC()
	out := make([]farm.ObjectiveFailure, 0, len(groups))
	for _, src := range groups {
		f := *src
		f.Recovered = reason == "done" || lastProgress > f.LastRound
		f.Blocking = !f.Recovered && f.Count >= 2 && (reason == "failed" || reason == "stuck")
		f.ObservedAt = observedAt
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Blocking != out[j].Blocking {
			return out[i].Blocking
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].LastRound != out[j].LastRound {
			return out[i].LastRound > out[j].LastRound
		}
		return out[i].Objective < out[j].Objective
	})
	return out
}

func resetObjectiveFailureTelemetry() {
	objectiveFailureTelemetry.Lock()
	objectiveFailureTelemetry.groups = map[string]*farm.ObjectiveFailure{}
	objectiveFailureTelemetry.lastProgressRound = 0
	objectiveFailureTelemetry.Unlock()
}
