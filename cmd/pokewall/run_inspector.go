package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

const maxRunDumpBytes = 64 << 20

// wallHTTPHandler layers operator/debug reads over the existing runner wall
// without changing the runner protocol. The fallback is the original Handler,
// so lease/heartbeat/finish remain exactly where they were.
func wallHTTPHandler(w *Wall) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/runs/{id}", w.handleRunInspect)
	mux.HandleFunc("GET /v1/runs/{id}/debug", w.handleRunDebug)
	mux.HandleFunc("GET /v1/runs/{id}/artifacts", w.handleRunArtifacts)
	mux.HandleFunc("GET /v1/runs/{id}/artifacts/{name}/content", w.handleInlineArtifactContent)
	mux.Handle("/", w.Handler())
	return mux
}

type runArtifactView struct {
	Name       string `json:"name"`
	MediaType  string `json:"media_type,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Store      string `json:"store,omitempty"`
	Bucket     string `json:"bucket,omitempty"`
	ObjectKey  string `json:"object_key,omitempty"`
	Size       int64  `json:"size,omitempty"`
	Inline     bool   `json:"inline"`
	Replayable bool   `json:"replayable,omitempty"`
}

type runFinishView struct {
	Attempt       int            `json:"attempt,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	Detail        string         `json:"detail,omitempty"`
	TraceTail     []string       `json:"trace_tail,omitempty"`
	RunnerVersion string         `json:"runner_version,omitempty"`
	SeedBurn      int            `json:"seed_burn"`
	ProgressEarly *farm.Progress `json:"progress_early,omitempty"`
	ProgressFinal *farm.Progress `json:"progress_final,omitempty"`
}

type runDebugSummary struct {
	ProgressKnown   bool           `json:"progress_known"`
	Progressed      bool           `json:"progressed"`
	BadgeDelta      int            `json:"badge_delta,omitempty"`
	EventDelta      int            `json:"event_delta,omitempty"`
	MapDelta        int            `json:"map_delta,omitempty"`
	CoverageDelta   *farm.Coverage `json:"coverage_delta,omitempty"`
	ReplayAvailable bool           `json:"replay_available"`
}

type runTimelineEvent struct {
	Type            string         `json:"type"`
	Source          string         `json:"source,omitempty"`
	Kind            string         `json:"kind,omitempty"`
	At              int64          `json:"at,omitempty"`
	Frame           *uint64        `json:"frame,omitempty"`
	Round           int            `json:"round,omitempty"`
	Attempt         int            `json:"attempt,omitempty"`
	RecoveryAttempt int            `json:"recovery_attempt,omitempty"`
	Message         string         `json:"message,omitempty"`
	Detail          string         `json:"detail,omitempty"`
	Progress        *farm.Progress `json:"progress,omitempty"`
	Question        string         `json:"question,omitempty"`
	Decision        string         `json:"decision,omitempty"`
}

type runDebugView struct {
	Run       tileRow            `json:"run"`
	Finish    *runFinishView     `json:"finish,omitempty"`
	Summary   runDebugSummary    `json:"summary"`
	Timeline  []runTimelineEvent `json:"timeline"`
	Artifacts []runArtifactView  `json:"artifacts"`
	FrameURL  string             `json:"frame_url,omitempty"`
}

func (w *Wall) handleRunInspect(res http.ResponseWriter, req *http.Request) {
	run, report, err := w.loadRunInspection(req.PathValue("id"))
	if err != nil {
		writeRunInspectError(res, err)
		return
	}
	if w.runArtifactsProtected(run.RunID) {
		run.ResumeProtected = true
	}
	out := map[string]any{"run": run}
	if report != nil {
		out["finish"] = finishView(report)
	}
	if run.ResumeProtected {
		out["delete_blocked"] = "run is still required by an active resume lineage: " + run.RunID
	}
	writeJSON(res, http.StatusOK, out)
}

func (w *Wall) handleRunArtifacts(res http.ResponseWriter, req *http.Request) {
	run, report, err := w.loadRunInspection(req.PathValue("id"))
	if err != nil {
		writeRunInspectError(res, err)
		return
	}
	if requested, ok, parseErr := requestedArtifactAttempt(req); parseErr != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": parseErr.Error()})
		return
	} else if ok {
		report, err = w.loadFinishReport(run.RunID, requested)
		if err != nil {
			writeRunInspectError(res, err)
			return
		}
	}
	artifacts := []runArtifactView{}
	attempt := run.Attempts
	if report != nil {
		artifacts = artifactViews(report.Artifacts)
		attempt = report.Attempt
		if attempt == 0 {
			attempt = 1
		}
	}
	writeJSON(res, http.StatusOK, map[string]any{
		"run_id":    run.RunID,
		"attempt":   attempt,
		"artifacts": artifacts,
	})
}

func (w *Wall) handleRunDebug(res http.ResponseWriter, req *http.Request) {
	run, report, err := w.loadRunInspection(req.PathValue("id"))
	if err != nil {
		writeRunInspectError(res, err)
		return
	}
	view := runDebugView{
		Run:       run,
		Artifacts: []runArtifactView{},
		Timeline:  buildRunTimeline(run, report),
		FrameURL:  "/frame?run=" + url.QueryEscape(run.RunID),
	}
	if report != nil {
		view.Finish = finishView(report)
		view.Artifacts = artifactViews(report.Artifacts)
		view.Summary = summarizeRun(report, view.Artifacts)
	} else {
		view.Summary.ReplayAvailable = false
	}
	writeJSON(res, http.StatusOK, view)
}

func (w *Wall) handleInlineArtifactContent(res http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	name := req.PathValue("name")
	run, report, err := w.loadRunInspection(id)
	if err != nil {
		writeRunInspectError(res, err)
		return
	}
	if requested, ok, parseErr := requestedArtifactAttempt(req); parseErr != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": parseErr.Error()})
		return
	} else if ok {
		report, err = w.loadFinishReport(run.RunID, requested)
		if err != nil {
			writeRunInspectError(res, err)
			return
		}
	}
	if report == nil {
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "run has no finish artifacts"})
		return
	}
	for _, artifact := range report.Artifacts {
		if artifact.Name != name {
			continue
		}
		if artifact.Store != "" {
			writeJSON(res, http.StatusConflict, map[string]any{
				"error":      "artifact is stored remotely",
				"store":      artifact.Store,
				"bucket":     artifact.Bucket,
				"object_key": artifact.ObjectKey,
			})
			return
		}
		mediaType := artifact.MediaType
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}
		res.Header().Set("Content-Type", mediaType)
		res.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeBase(artifact.Name)))
		res.Header().Set("Cache-Control", "private, no-store")
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write(artifact.Data)
		return
	}
	writeJSON(res, http.StatusNotFound, map[string]string{"error": "artifact not found"})
}

func requestedArtifactAttempt(req *http.Request) (int, bool, error) {
	raw := strings.TrimSpace(req.URL.Query().Get("attempt"))
	if raw == "" {
		return 0, false, nil
	}
	attempt, err := strconv.Atoi(raw)
	if err != nil || attempt < 1 {
		return 0, false, fmt.Errorf("invalid attempt %q: want a positive integer", raw)
	}
	return attempt, true, nil
}

func (w *Wall) loadFinishReport(runID string, attempt int) (*farm.FinishReport, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" || attempt < 1 {
		return nil, fs.ErrNotExist
	}
	if cp := controlPlaneFor(w); cp != nil {
		return cp.finishReport(runID, attempt)
	}
	if w.dumpsDir == "" {
		return nil, fs.ErrNotExist
	}
	paths := []string{filepath.Join(w.dumpsDir, fmt.Sprintf("%s-attempt-%d.json", safeBase(runID), attempt))}
	if attempt == 1 {
		paths = append(paths, filepath.Join(w.dumpsDir, safeDumpName(runID)))
	}
	for _, path := range uniqueStrings(paths) {
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > maxRunDumpBytes {
			return nil, fmt.Errorf("finish dump %s exceeds %d bytes", filepath.Base(path), maxRunDumpBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var report farm.FinishReport
		if err := json.Unmarshal(data, &report); err != nil {
			return nil, fmt.Errorf("decode finish dump %s: %w", filepath.Base(path), err)
		}
		reportAttempt := report.Attempt
		if reportAttempt < 1 {
			reportAttempt = 1
		}
		if report.RunID != runID || reportAttempt != attempt {
			continue
		}
		return &report, nil
	}
	return nil, fs.ErrNotExist
}

func (w *Wall) loadRunInspection(runID string) (tileRow, *farm.FinishReport, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return tileRow{}, nil, fs.ErrNotExist
	}
	run, found := w.snapshotRun(runID)
	report, err := w.loadLatestFinishReport(runID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return tileRow{}, nil, err
	}
	if !found && report == nil {
		return tileRow{}, nil, fs.ErrNotExist
	}
	if !found && report != nil {
		attempt := report.Attempt
		if attempt < 1 {
			attempt = 1
		}
		run = tileRow{
			RunID:    report.RunID,
			Status:   statusDone,
			Attempts: attempt,
			Reason:   report.Reason,
			Detail:   report.Detail,
		}
	}
	return run, report, nil
}

func (w *Wall) loadLatestFinishReport(runID string) (*farm.FinishReport, error) {
	if cp := controlPlaneFor(w); cp != nil {
		return cp.latestFinishReport(runID)
	}
	if w.dumpsDir == "" {
		return nil, fs.ErrNotExist
	}
	paths, err := filepath.Glob(filepath.Join(w.dumpsDir, safeBase(runID)+"-attempt-*.json"))
	if err != nil {
		return nil, err
	}
	// Attempt 1 keeps the historical exact filename.
	first := filepath.Join(w.dumpsDir, safeDumpName(runID))
	if _, err := os.Stat(first); err == nil {
		paths = append(paths, first)
	}
	paths = uniqueStrings(paths)
	if len(paths) == 0 {
		return nil, fs.ErrNotExist
	}
	sort.Strings(paths)
	var best *farm.FinishReport
	bestAttempt := -1
	var decodeErr error
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > maxRunDumpBytes {
			decodeErr = fmt.Errorf("finish dump %s exceeds %d bytes", filepath.Base(path), maxRunDumpBytes)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			decodeErr = err
			continue
		}
		var report farm.FinishReport
		if err := json.Unmarshal(data, &report); err != nil {
			decodeErr = fmt.Errorf("decode finish dump %s: %w", filepath.Base(path), err)
			continue
		}
		if report.RunID != runID {
			continue
		}
		attempt := report.Attempt
		if attempt < 1 {
			attempt = 1
		}
		if best == nil || attempt > bestAttempt {
			copy := report
			best = &copy
			bestAttempt = attempt
		}
	}
	if best != nil {
		return best, nil
	}
	if decodeErr != nil {
		return nil, decodeErr
	}
	return nil, fs.ErrNotExist
}

func finishView(report *farm.FinishReport) *runFinishView {
	if report == nil {
		return nil
	}
	return &runFinishView{
		Attempt:       report.Attempt,
		Reason:        report.Reason,
		Detail:        report.Detail,
		TraceTail:     append([]string(nil), report.TraceTail...),
		RunnerVersion: report.RunnerVersion,
		SeedBurn:      report.SeedBurn,
		ProgressEarly: report.ProgressEarly,
		ProgressFinal: report.ProgressFinal,
	}
}

func artifactViews(artifacts []farm.Artifact) []runArtifactView {
	out := make([]runArtifactView, 0, len(artifacts))
	for _, artifact := range artifacts {
		size := artifact.Size
		if size == 0 && len(artifact.Data) > 0 {
			size = int64(len(artifact.Data))
		}
		out = append(out, runArtifactView{
			Name:       artifact.Name,
			MediaType:  artifact.MediaType,
			SHA256:     artifact.SHA256,
			Store:      artifact.Store,
			Bucket:     artifact.Bucket,
			ObjectKey:  artifact.ObjectKey,
			Size:       size,
			Inline:     artifact.Store == "",
			Replayable: artifact.Name == "run.gbrun",
		})
	}
	return out
}

func summarizeRun(report *farm.FinishReport, artifacts []runArtifactView) runDebugSummary {
	var summary runDebugSummary
	for _, artifact := range artifacts {
		if artifact.Replayable {
			summary.ReplayAvailable = true
			break
		}
	}
	if report == nil || report.ProgressEarly == nil || report.ProgressFinal == nil {
		return summary
	}
	summary.ProgressKnown = true
	summary.BadgeDelta = report.ProgressFinal.Badges - report.ProgressEarly.Badges
	summary.EventDelta = report.ProgressFinal.Events - report.ProgressEarly.Events
	summary.MapDelta = report.ProgressFinal.Maps - report.ProgressEarly.Maps
	summary.CoverageDelta = coverageDelta(report.ProgressEarly.Coverage, report.ProgressFinal.Coverage)
	summary.Progressed = summary.BadgeDelta != 0 || summary.EventDelta != 0 || summary.MapDelta != 0 || report.ProgressFinal.Map != report.ProgressEarly.Map || coverageProgressed(summary.CoverageDelta)
	return summary
}

func coverageDelta(early, final *farm.Coverage) *farm.Coverage {
	if early == nil || final == nil {
		return nil
	}
	return &farm.Coverage{
		UniqueMapsVisited:     final.UniqueMapsVisited - early.UniqueMapsVisited,
		TrainersDefeated:      final.TrainersDefeated - early.TrainersDefeated,
		NPCInteractions:       final.NPCInteractions - early.NPCInteractions,
		UniqueItemsAcquired:   final.UniqueItemsAcquired - early.UniqueItemsAcquired,
		UniqueItemsUsed:       final.UniqueItemsUsed - early.UniqueItemsUsed,
		DexOwned:              final.DexOwned - early.DexOwned,
		DexSeen:               final.DexSeen - early.DexSeen,
		OptionalMilestones:    final.OptionalMilestones - early.OptionalMilestones,
		TMsHMsAcquired:        final.TMsHMsAcquired - early.TMsHMsAcquired,
		TMsHMsUsed:            final.TMsHMsUsed - early.TMsHMsUsed,
		Evolutions:            final.Evolutions - early.Evolutions,
		Catches:               final.Catches - early.Catches,
		UniqueSpeciesAcquired: final.UniqueSpeciesAcquired - early.UniqueSpeciesAcquired,
	}
}

func coverageProgressed(delta *farm.Coverage) bool {
	if delta == nil {
		return false
	}
	return delta.UniqueMapsVisited != 0 || delta.TrainersDefeated != 0 || delta.NPCInteractions != 0 ||
		delta.UniqueItemsAcquired != 0 || delta.UniqueItemsUsed != 0 || delta.DexOwned != 0 || delta.DexSeen != 0 ||
		delta.OptionalMilestones != 0 || delta.TMsHMsAcquired != 0 || delta.TMsHMsUsed != 0 || delta.Evolutions != 0 ||
		delta.Catches != 0 || delta.UniqueSpeciesAcquired != 0
}

func buildRunTimeline(run tileRow, report *farm.FinishReport) []runTimelineEvent {
	events := make([]runTimelineEvent, 0, len(run.Activity)+5)
	hasQueued := false
	hasDecision := false
	hasTerminal := false
	for _, activity := range run.Activity {
		event := runTimelineEvent{
			Type:            "activity",
			Source:          activity.Source,
			Kind:            activity.Kind,
			At:              activity.At,
			Round:           activity.Round,
			Attempt:         activity.Attempt,
			RecoveryAttempt: activity.RecoveryAttempt,
			Message:         activity.Summary,
			Detail:          activity.Detail,
		}
		if activity.Frame != 0 {
			frame := activity.Frame
			event.Frame = &frame
		}
		switch activity.Kind {
		case "queued":
			hasQueued = true
		case "decision":
			hasDecision = true
			event.Decision = activity.Summary
		case "goal", "terminal", "cancelled":
			hasTerminal = true
		}
		events = append(events, event)
	}
	if run.QueuedAt != 0 && !hasQueued {
		events = append(events, runTimelineEvent{Type: "queued", Source: "system", Kind: "queued", At: run.QueuedAt, Message: "Run queued"})
	}
	if report != nil && report.ProgressEarly != nil {
		events = append(events, runTimelineEvent{
			Type: "progress_early", Source: "milestone", Kind: "progress", Round: report.ProgressEarly.Round,
			Message: "Progress snapshot before the first objective", Progress: report.ProgressEarly,
		})
	}
	if (run.Question != "" || run.Decision != "") && !hasDecision {
		frame := run.Frame
		events = append(events, runTimelineEvent{
			Type: "latest_decision", Source: "llm", Kind: "decision", Frame: &frame, Message: "Last persisted planner decision",
			Question: run.Question, Decision: run.Decision,
		})
	}
	if report != nil && report.ProgressFinal != nil {
		frame := run.Frame
		events = append(events, runTimelineEvent{
			Type: "progress_final", Source: "milestone", Kind: "progress", Frame: &frame, Round: report.ProgressFinal.Round,
			Message: "Final progress snapshot", Progress: report.ProgressFinal,
		})
	}
	if run.Status == statusDone && (run.Reason != "" || (report != nil && report.Reason != "")) && !hasTerminal {
		frame := run.Frame
		reason, detail := run.Reason, run.Detail
		if report != nil {
			reason, detail = report.Reason, report.Detail
		}
		message := reason
		if detail != "" {
			message += ": " + detail
		}
		events = append(events, runTimelineEvent{Type: "finished", Source: "system", Kind: "finish", At: run.EndedAt, Frame: &frame, Message: message, Detail: detail})
	}
	return events
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func writeRunInspectError(res http.ResponseWriter, err error) {
	if errors.Is(err, fs.ErrNotExist) {
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}
