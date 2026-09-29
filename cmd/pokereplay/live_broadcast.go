package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/media/compositor"
)

const (
	liveBroadcastFPS             = 20
	liveBroadcastSubscriberQueue = 2
	liveBroadcastBoundary        = "pokepilot-live"
	maxLiveFrameBytes            = 4 << 20
)

type liveBroadcastStatus struct {
	RunID         string `json:"run_id"`
	State         string `json:"state"`
	Frame         uint64 `json:"frame,omitempty"`
	TargetFPS     int    `json:"target_fps"`
	Subscribers   int    `json:"subscribers,omitempty"`
	DroppedFrames uint64 `json:"dropped_frames,omitempty"`
	Reconnects    uint64 `json:"reconnects,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	UpdatedAtMS   int64  `json:"updated_at_unix_ms,omitempty"`
}

type liveRunEnvelope struct {
	Run liveRunView `json:"run"`
}

type liveRunView struct {
	RunID    string       `json:"run_id"`
	Status   string       `json:"status"`
	Goal     string       `json:"goal,omitempty"`
	Planner  string       `json:"planner,omitempty"`
	Decision string       `json:"decision,omitempty"`
	Attempts int          `json:"attempts,omitempty"`
	Frame    uint64       `json:"frame"`
	Map      uint8        `json:"map"`
	X        uint8        `json:"x"`
	Y        uint8        `json:"y"`
	Player   *farm.Player `json:"player,omitempty"`
	Stats    *struct {
		Round  int    `json:"round,omitempty"`
		Intent string `json:"intent,omitempty"`
	} `json:"stats,omitempty"`
	Activity []liveRunActivity `json:"activity,omitempty"`
}

type liveRunActivity struct {
	Source  string `json:"source"`
	Kind    string `json:"kind"`
	Frame   uint64 `json:"frame,omitempty"`
	Round   int    `json:"round,omitempty"`
	Attempt int    `json:"attempt,omitempty"`
	Summary string `json:"summary"`
	Detail  string `json:"detail,omitempty"`
}

type liveEncodedFrame struct {
	frame uint64
	jpeg  []byte
}

type liveBroadcastSession struct {
	server *replayServer
	runID  string

	mu          sync.Mutex
	status      liveBroadcastStatus
	latest      liveEncodedFrame
	subscribers map[uint64]chan liveEncodedFrame
	nextSub     uint64
	done        chan struct{}
	cancel      context.CancelFunc
}

func (s *replayServer) liveSession(runID string) *liveBroadcastSession {
	runID = strings.TrimSpace(runID)
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	if session := s.liveSessions[runID]; session != nil {
		if session.snapshot().State != "ended" {
			return session
		}
		delete(s.liveSessions, runID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	session := &liveBroadcastSession{
		server: s,
		runID:  runID,
		status: liveBroadcastStatus{
			RunID:     runID,
			State:     "starting",
			TargetFPS: liveBroadcastFPS,
		},
		subscribers: make(map[uint64]chan liveEncodedFrame),
		done:        make(chan struct{}),
		cancel:      cancel,
	}
	s.liveSessions[runID] = session
	go session.run(ctx)
	return session
}

func (s *replayServer) liveSessionIfPresent(runID string) *liveBroadcastSession {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	return s.liveSessions[strings.TrimSpace(runID)]
}

func (s *replayServer) stopLiveSessions() {
	s.liveMu.Lock()
	sessions := make([]*liveBroadcastSession, 0, len(s.liveSessions))
	for _, session := range s.liveSessions {
		sessions = append(sessions, session)
	}
	s.liveMu.Unlock()
	for _, session := range sessions {
		session.cancel()
	}
}

func (s *liveBroadcastSession) snapshot() liveBroadcastStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.status
	out.Subscribers = len(s.subscribers)
	return out
}

func (s *liveBroadcastSession) subscribe() (<-chan liveEncodedFrame, func()) {
	s.mu.Lock()
	id := s.nextSub
	s.nextSub++
	ch := make(chan liveEncodedFrame, liveBroadcastSubscriberQueue)
	s.subscribers[id] = ch
	if len(s.latest.jpeg) > 0 {
		ch <- s.latest
	}
	s.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.subscribers, id)
			s.mu.Unlock()
		})
	}
}

func (s *liveBroadcastSession) publish(frame liveEncodedFrame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest = frame
	dropped := uint64(0)
	for _, ch := range s.subscribers {
		select {
		case ch <- frame:
			continue
		default:
		}
		// Slow readers lose stale presentation frames rather than backpressuring
		// the shared producer, pokewall, or the gameplay process.
		select {
		case <-ch:
			dropped++
		default:
		}
		select {
		case ch <- frame:
		default:
			dropped++
		}
	}
	s.status.Frame = frame.frame
	s.status.DroppedFrames += dropped
	if dropped > 0 {
		s.status.State = "lagging"
	} else {
		s.status.State = "live"
	}
	s.status.LastError = ""
	s.status.UpdatedAtMS = time.Now().UnixMilli()
}

func (s *liveBroadcastSession) setUnavailable(state string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state == "" {
		state = "disconnected"
	}
	s.status.State = state
	if err != nil {
		s.status.LastError = clipError(err)
	}
	s.status.UpdatedAtMS = time.Now().UnixMilli()
}

func (s *liveBroadcastSession) markReconnected() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.State == "disconnected" || s.status.State == "encoder_error" {
		s.status.Reconnects++
	}
}

func (s *liveBroadcastSession) finish(state string) {
	s.mu.Lock()
	if state == "" {
		state = "ended"
	}
	s.status.State = state
	s.status.UpdatedAtMS = time.Now().UnixMilli()
	for id, ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, id)
	}
	s.mu.Unlock()
	close(s.done)
}

func (s *liveBroadcastSession) run(ctx context.Context) {
	defer s.finish("ended")
	interval := time.Second / liveBroadcastFPS
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	first := true
	for {
		if !first {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		first = false

		run, err := s.server.fetchLiveRun(ctx, s.runID)
		if err != nil {
			s.setUnavailable("disconnected", err)
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(run.Status), "running") {
			return
		}

		raw, err := s.server.fetchLiveRawFrame(ctx, s.runID)
		if err != nil {
			s.setUnavailable("disconnected", err)
			continue
		}
		timeline := liveTimelineFromRun(run)
		rendered, err := renderLiveBroadcastFrame(raw, s.runID, liveRunAttempt(run), timeline)
		if err != nil {
			s.setUnavailable("encoder_error", err)
			continue
		}
		s.markReconnected()
		s.publish(liveEncodedFrame{frame: run.Frame, jpeg: rendered})
	}
}

func liveRunAttempt(run liveRunView) int {
	attempt := run.Attempts
	if strings.EqualFold(strings.TrimSpace(run.Status), "running") {
		attempt++
	}
	if attempt < 1 {
		attempt = 1
	}
	return attempt
}

func liveTimelineFromRun(run liveRunView) farm.MediaTimeline {
	round := 0
	intent := ""
	if run.Stats != nil {
		round = run.Stats.Round
		intent = run.Stats.Intent
	}
	attempt := liveRunAttempt(run)
	timeline := farm.MediaTimeline{
		Version: farm.MediaTimelineVersion,
		Run: farm.MediaRunSummary{
			RunID:   run.RunID,
			Status:  run.Status,
			Goal:    run.Goal,
			Planner: run.Planner,
		},
		Attempt:         attempt,
		EndFrame:        run.Frame,
		FramesPerSecond: farm.GameBoyFramesPerSecond,
		Snapshots: []farm.MediaSnapshot{{
			Frame:     run.Frame,
			Round:     round,
			Objective: firstNonEmpty(run.Decision, run.Goal),
			Location:  farm.MediaLocation{Map: run.Map, X: run.X, Y: run.Y},
			Player:    run.Player,
			Planner:   farm.MediaPlannerState{Intent: intent},
		}},
	}
	for _, activity := range run.Activity {
		if activity.Attempt > 0 && activity.Attempt != attempt {
			continue
		}
		if !liveActivityIsPresentationEvent(activity) {
			continue
		}
		frame := activity.Frame
		if frame > run.Frame {
			frame = run.Frame
		}
		timeline.Events = append(timeline.Events, farm.MediaEvent{
			Type:      liveActivityEventType(activity),
			Frame:     frame,
			Round:     activity.Round,
			Summary:   firstNonEmpty(activity.Summary, activity.Detail),
			Evidence:  fmt.Sprintf("live:%s:%s:%d:%d", activity.Source, activity.Kind, activity.Attempt, activity.Frame),
			Objective: run.Decision,
		})
	}
	return timeline.Normalized()
}

func liveActivityIsPresentationEvent(activity liveRunActivity) bool {
	switch strings.ToLower(strings.TrimSpace(activity.Source)) {
	case "milestone", "recovery":
		return true
	case "system":
		switch strings.ToLower(strings.TrimSpace(activity.Kind)) {
		case "attempt_start", "checkpoint", "finished", "failed", "blackout":
			return true
		}
	}
	return false
}

func liveActivityEventType(activity liveRunActivity) string {
	source := strings.ToLower(strings.TrimSpace(activity.Source))
	kind := strings.ToLower(strings.TrimSpace(activity.Kind))
	switch {
	case source == "milestone" && kind == "badge":
		return "badge_acquired"
	case source == "recovery":
		return "recovery_" + firstNonEmpty(kind, "event")
	case kind != "":
		return kind
	default:
		return firstNonEmpty(source, "event")
	}
}

func renderLiveBroadcastFrame(rawPNG []byte, runID string, attempt int, timeline farm.MediaTimeline) ([]byte, error) {
	return compositor.RenderFrame(rawPNG, runID, attempt, timeline)
}

func (s *replayServer) fetchLiveRun(ctx context.Context, runID string) (liveRunView, error) {
	var envelope liveRunEnvelope
	endpoint := s.wallBase + "/v1/runs/" + url.PathEscape(strings.TrimSpace(runID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return envelope.Run, err
	}
	res, err := s.wallHTTP.Do(req)
	if err != nil {
		return envelope.Run, fmt.Errorf("pokewall live run unavailable: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxWallResponseBytes+1))
	if err != nil {
		return envelope.Run, err
	}
	if len(data) > maxWallResponseBytes {
		return envelope.Run, fmt.Errorf("pokewall live run response exceeds %d bytes", maxWallResponseBytes)
	}
	if res.StatusCode == http.StatusNotFound {
		return envelope.Run, errRunNotFound
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return envelope.Run, fmt.Errorf("pokewall live run returned %s", res.Status)
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return envelope.Run, fmt.Errorf("decode pokewall live run: %w", err)
	}
	if strings.TrimSpace(envelope.Run.RunID) == "" {
		envelope.Run.RunID = strings.TrimSpace(runID)
	}
	return envelope.Run, nil
}

func (s *replayServer) fetchLiveRawFrame(ctx context.Context, runID string) ([]byte, error) {
	endpoint := s.wallBase + "/frame?run=" + url.QueryEscape(strings.TrimSpace(runID)) + "&latest=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := s.wallHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pokewall live frame unavailable: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("pokewall live frame returned %s", res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxLiveFrameBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > maxLiveFrameBytes {
		return nil, fmt.Errorf("pokewall live frame has invalid size %d", len(data))
	}
	return data, nil
}

func (s *replayServer) handleLiveStatus(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run id is required"})
		return
	}
	if session := s.liveSessionIfPresent(runID); session != nil {
		writeJSON(w, http.StatusOK, session.snapshot())
		return
	}
	run, err := s.fetchLiveRun(r.Context(), runID)
	if errors.Is(err, errRunNotFound) {
		writeJSON(w, http.StatusNotFound, liveBroadcastStatus{RunID: runID, State: "missing", TargetFPS: liveBroadcastFPS})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, liveBroadcastStatus{RunID: runID, State: "disconnected", TargetFPS: liveBroadcastFPS, LastError: clipError(err)})
		return
	}
	state := "ended"
	if strings.EqualFold(strings.TrimSpace(run.Status), "running") {
		state = "idle"
	}
	writeJSON(w, http.StatusOK, liveBroadcastStatus{RunID: runID, State: state, Frame: run.Frame, TargetFPS: liveBroadcastFPS})
}

func (s *replayServer) handleLiveStream(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run id is required"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming is unavailable"})
		return
	}
	session := s.liveSession(runID)
	frames, unsubscribe := session.subscribe()
	defer unsubscribe()

	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+liveBroadcastBoundary)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-PokePilot-Live-FPS", strconv.Itoa(liveBroadcastFPS))
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case frame, open := <-frames:
			if !open {
				fmt.Fprintf(w, "--%s--\r\n", liveBroadcastBoundary) //nolint:errcheck
				flusher.Flush()
				return
			}
			if len(frame.jpeg) == 0 {
				continue
			}
			if _, err := fmt.Fprintf(
				w,
				"--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\nX-PokePilot-Frame: %d\r\n\r\n",
				liveBroadcastBoundary,
				len(frame.jpeg),
				frame.frame,
			); err != nil {
				return
			}
			if _, err := w.Write(frame.jpeg); err != nil {
				return
			}
			if _, err := io.WriteString(w, "\r\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
