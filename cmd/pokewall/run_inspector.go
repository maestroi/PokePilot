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
	ProgressKnown   bool `json:"progress_known"`
	Progressed      bool `json:"progressed"`
	BadgeDelta      int  `json:"badge_delta,omitempty"`
	EventDelta      int  `json:"event_delta,omitempty"`
	MapDelta        int  `json:"map_delta,omitempty"`
	ReplayAvailable bool `json:"replay_available"`
}

type runTimelineEvent struct {
	Type     string         `json:"type"`
	At       int64          `json:"at,omitempty"`
	Frame    *uint64        `json:"frame,omitempty"`
	Round    int            `json:"round,omitempty"`
	Message  string         `json:"message,omitempty"`
	Progress *farm.Progress `json:"progress,omitempty"`
	Question string         `json:"question,omitempty"`
	Decision string         `json:"decision,omitempty"`
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
	out := map[string]any{"run": run}
	if report != nil {
		out["finish"] = finishView(report)
	}
	writeJSON(res, http.StatusOK, out)
}

func (w *Wall) handleRunArtifacts(res http.ResponseWriter, req *http.Request) {
	run, report, err := w.loadRunInspection(req.PathValue("id"))
	if err != nil {
		writeRunInspectError(res, err)
		return
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
		"run_id": run.RunID, "attempt": attempt, "artifacts": artifacts,
	})
}

func (w *Wall) handleRunDebug(res http.ResponseWriter, req *http.Request) {
	run, report, err := w.loadRunInspection(req.PathValue("id"))
	if err != nil {
		writeRunInspectError(res, err)
		return
	}
	view := runDebugView{
		Run: run, Artifacts: []runArtifactView{}, Timeline: buildRunTimeline(run, report),
		FrameURL: "/frame?run=" + url.QueryEscape(run.RunID),
	}
	if report != nil {
		view.Finish = finishView(report)
		view.Artifacts = artifactViews(report.Artifacts)
		view.Summary = summarizeRun(report, view.Artifacts)
	}
	writeJSON(res, http.StatusOK, view)
}

func (w *Wall) handleInlineArtifactContent(res http.ResponseWriter, req *http.Request) {
	id, name := req.PathValue("id"), req.PathValue("name")
	_, report, err := w.loadRunInspection(id)
	if err != nil {
		writeRunInspectError(res, err)
		return
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
				"error": "artifact is stored remotely", "store": artifact.Store,
				"bucket": artifact.Bucket, "object_key": artifact.ObjectKey,
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

func (w *Wall) loadRunInspection(runID string) (tileRow, *farm.FinishReport, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return tileRow{}, nil, fs.ErrNotExist
	}
	var run tileRow
	found := false
	for _, candidate := range w.snapshot().Runs {
		if candidate.RunID == runID {
			run, found = candidate, true
			break
		}
	}
	report, err := w.loadLatestFinishReport(runID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return tileRow{}, nil, err
	}
	if !found && report == nil {
		return tileRow{}, nil, fs.ErrNotExist
	}
	if !found && report != nil {
		attempt := report.Attempt
		if attempt < 1 { attempt = 1 }
		run = tileRow{RunID: report.RunID, Status: statusDone, Attempts: attempt, Reason: report.Reason, Detail: report.Detail}
	}
	return run, report, nil
}

func (w *Wall) loadLatestFinishReport(runID string) (*farm.FinishReport, error) {
	if w.dumpsDir == "" { return nil, fs.ErrNotExist }
	paths, err := filepath.Glob(filepath.Join(w.dumpsDir, safeBase(runID)+"*.json"))
	if err != nil { return nil, err }
	first := filepath.Join(w.dumpsDir, safeDumpName(runID))
	if _, err := os.Stat(first); err == nil { paths = append(paths, first) }
	paths = uniqueStrings(paths)
	if len(paths) == 0 { return nil, fs.ErrNotExist }
	sort.Strings(paths)
	var best *farm.FinishReport
	bestAttempt := -1
	var decodeErr error
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() { continue }
		if info.Size() > maxRunDumpBytes {
			decodeErr = fmt.Errorf("finish dump %s exceeds %d bytes", filepath.Base(p), maxRunDumpBytes)
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil { decodeErr = err; continue }
		var report farm.FinishReport
		if err := json.Unmarshal(data, &report); err != nil {
			decodeErr = fmt.Errorf("decode finish dump %s: %w", filepath.Base(p), err)
			continue
		}
		if report.RunID != runID { continue }
		attempt := report.Attempt
		if attempt < 1 { attempt = 1 }
		if best == nil || attempt > bestAttempt {
			copy := report; best = &copy; bestAttempt = attempt
		}
	}
	if best != nil { return best, nil }
	if decodeErr != nil { return nil, decodeErr }
	return nil, fs.ErrNotExist
}

func finishView(report *farm.FinishReport) *runFinishView {
	if report == nil { return nil }
	return &runFinishView{
		Attempt: report.Attempt, Reason: report.Reason, Detail: report.Detail,
		TraceTail: append([]string(nil), report.TraceTail...), RunnerVersion: report.RunnerVersion,
		SeedBurn: report.SeedBurn, ProgressEarly: report.ProgressEarly, ProgressFinal: report.ProgressFinal,
	}
}

func artifactViews(artifacts []farm.Artifact) []runArtifactView {
	out := make([]runArtifactView, 0, len(artifacts))
	for _, artifact := range artifacts {
		size := artifact.Size
		if size == 0 && len(artifact.Data) > 0 { size = int64(len(artifact.Data)) }
		out = append(out, runArtifactView{
			Name: artifact.Name, MediaType: artifact.MediaType, SHA256: artifact.SHA256,
			Store: artifact.Store, Bucket: artifact.Bucket, ObjectKey: artifact.ObjectKey,
			Size: size, Inline: artifact.Store == "", Replayable: artifact.Name == "run.gbrun",
		})
	}
	return out
}

func summarizeRun(report *farm.FinishReport, artifacts []runArtifactView) runDebugSummary {
	var summary runDebugSummary
	for _, artifact := range artifacts { if artifact.Replayable { summary.ReplayAvailable = true; break } }
	if report == nil || report.ProgressEarly == nil || report.ProgressFinal == nil { return summary }
	summary.ProgressKnown = true
	summary.BadgeDelta = report.ProgressFinal.Badges - report.ProgressEarly.Badges
	summary.EventDelta = report.ProgressFinal.Events - report.ProgressEarly.Events
	summary.MapDelta = report.ProgressFinal.Maps - report.ProgressEarly.Maps
	summary.Progressed = summary.BadgeDelta != 0 || summary.EventDelta != 0 || summary.MapDelta != 0 || report.ProgressFinal.Map != report.ProgressEarly.Map
	return summary
}

func buildRunTimeline(run tileRow, report *farm.FinishReport) []runTimelineEvent {
	events := make([]runTimelineEvent, 0, 5)
	if run.QueuedAt != 0 { events = append(events, runTimelineEvent{Type: "queued", At: run.QueuedAt, Message: "run queued"}) }
	if report != nil && report.ProgressEarly != nil {
		events = append(events, runTimelineEvent{Type: "progress_early", Round: report.ProgressEarly.Round, Message: "progress snapshot before the first objective", Progress: report.ProgressEarly})
	}
	if run.Question != "" || run.Decision != "" {
		frame := run.Frame
		events = append(events, runTimelineEvent{Type: "latest_decision", Frame: &frame, Message: "last persisted planner decision", Question: run.Question, Decision: run.Decision})
	}
	if report != nil && report.ProgressFinal != nil {
		frame := run.Frame
		events = append(events, runTimelineEvent{Type: "progress_final", Frame: &frame, Round: report.ProgressFinal.Round, Message: "final progress snapshot", Progress: report.ProgressFinal})
	}
	if run.Reason != "" || (report != nil && report.Reason != "") {
		frame := run.Frame
		reason, detail := run.Reason, run.Detail
		if report != nil { reason, detail = report.Reason, report.Detail }
		message := reason
		if detail != "" { message += ": " + detail }
		events = append(events, runTimelineEvent{Type: "finished", At: run.EndedAt, Frame: &frame, Message: message})
	}
	return events
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in)); out := make([]string, 0, len(in))
	for _, s := range in { if _, ok := seen[s]; ok { continue }; seen[s] = struct{}{}; out = append(out, s) }
	return out
}

func writeRunInspectError(res http.ResponseWriter, err error) {
	if errors.Is(err, fs.ErrNotExist) { writeJSON(res, http.StatusNotFound, map[string]string{"error": "run not found"}); return }
	writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}
