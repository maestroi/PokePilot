package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
)

// Server-owned Full E2E production (#2794). Every finished run tagged
// purpose=full_e2e is advanced through broadcast render -> producer ->
// episode/Short edits -> packages -> private review candidates without any
// browser staying open. Each pass drives this server's own HTTP handlers
// in-process, so validation and quality gates are exactly the operator path.
// Renders stay wall-leased jobs; the only extra state is one small S3 object
// per run that pins the (non-deterministic, LLM-backed) producer assessment
// and records progress. It never approves or uploads: publishing stays a
// Studio action.
//
// ponytail: one pokereplay replica owns this loop (same as the publisher);
// add a wall lease per run if replay is ever scaled out.

const (
	productionInterval    = time.Minute
	productionMaxFailures = 5
	productionDashboard   = "/v1/dashboard?status=done&purpose=full_e2e&limit=25&offset=0"
)

type productionState struct {
	Version        int               `json:"version"`
	RunID          string            `json:"run_id"`
	Stage          string            `json:"stage"`
	Done           bool              `json:"done"`
	AssessmentHash string            `json:"assessment_hash,omitempty"`
	Verdict        string            `json:"verdict,omitempty"`
	Plans          map[string]string `json:"plans,omitempty"`
	Candidates     map[string]string `json:"candidates,omitempty"`
	Error          string            `json:"error,omitempty"`
	Failures       int               `json:"failures,omitempty"`
	UpdatedAt      int64             `json:"updated_at"`
}

func productionKey(runID string) string {
	sum := sha256.Sum256([]byte(runID))
	return path.Join("studio", hex.EncodeToString(sum[:]), "production", "v1.json")
}

func (s *replayServer) loadProduction(ctx context.Context, runID string) (productionState, error) {
	st := productionState{Version: 1, RunID: runID, Stage: "pending"}
	obj, err := s.store.GetObject(ctx, productionKey(runID), "")
	if artifactstore.IsNotFound(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	defer obj.Body.Close()
	err = json.NewDecoder(io.LimitReader(obj.Body, 64<<10)).Decode(&st)
	return st, err
}

func (s *replayServer) saveProduction(ctx context.Context, st productionState) error {
	st.UpdatedAt = time.Now().Unix()
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.store.PutObject(ctx, productionKey(st.RunID), "application/json", data)
	return err
}

func (s *replayServer) handleProductionStatus(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "studio storage disabled"})
		return
	}
	st, err := s.loadProduction(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleProductionRetry re-arms a run whose production exhausted its retries
// (a failed_* stage), keeping the pinned assessment and plans so the loop
// resumes where it stopped instead of re-asking the producer.
func (s *replayServer) handleProductionRetry(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "studio storage disabled"})
		return
	}
	st, err := s.loadProduction(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	stage, failed := strings.CutPrefix(st.Stage, "failed_")
	if !st.Done || !failed {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "production is not in a failed stage"})
		return
	}
	st.Stage, st.Done, st.Failures, st.Error = stage, false, 0, ""
	if err := s.saveProduction(r.Context(), st); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *replayServer) runFullE2EProduction(ctx context.Context, interval time.Duration) {
	if s.store == nil {
		return
	}
	self := s.handler()
	tick := func() {
		var dash struct {
			Runs []struct {
				RunID string `json:"run_id"`
			} `json:"runs"`
		}
		if _, err := s.mediaJobRequest(ctx, http.MethodGet, productionDashboard, nil, &dash); err != nil {
			log.Printf("pokereplay: production: list full_e2e runs: %v", err)
			return
		}
		for _, run := range dash.Runs {
			if ctx.Err() != nil {
				return
			}
			s.advanceProduction(ctx, self, run.RunID)
		}
	}
	tick()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}

func (s *replayServer) advanceProduction(ctx context.Context, self http.Handler, runID string) {
	if runID == "" {
		return
	}
	st, err := s.loadProduction(ctx, runID)
	if err != nil {
		log.Printf("pokereplay: production %s: load state: %v", runID, err)
		return
	}
	if st.Done {
		return
	}
	before := st
	stepErr := s.productionStep(ctx, self, &st)
	if stepErr != nil {
		st.Error = stepErr.Error()
		st.Failures++
		if st.Failures >= productionMaxFailures {
			st.Done = true
			st.Stage = "failed_" + st.Stage
		}
		log.Printf("pokereplay: production %s: %s: %v (failure %d/%d)", runID, st.Stage, stepErr, st.Failures, productionMaxFailures)
	} else {
		st.Error, st.Failures = "", 0
	}
	if st.Stage != before.Stage || st.Done != before.Done || st.Error != before.Error || st.Failures != before.Failures ||
		st.AssessmentHash != before.AssessmentHash || len(st.Plans) != len(before.Plans) || len(st.Candidates) != len(before.Candidates) {
		if st.Stage != before.Stage {
			log.Printf("pokereplay: production %s: %s", runID, st.Stage)
		}
		if err := s.saveProduction(ctx, st); err != nil {
			log.Printf("pokereplay: production %s: save state: %v", runID, err)
		}
	}
}

// productionStep advances as far as it can and returns at the first stage
// still waiting on a render. Every call it makes is idempotent except the
// producer assessment, which is pinned in st once it succeeds.
func (s *replayServer) productionStep(ctx context.Context, self http.Handler, st *productionState) error {
	root := "/v1/runs/" + url.PathEscape(st.RunID)
	call := func(method, route string, body, out any) (int, error) {
		return selfJSON(ctx, self, method, root+route, body, out)
	}

	if st.Stage == "pending" {
		var rec struct {
			Run struct {
				Status string `json:"status"`
				Reason string `json:"reason"`
				Stats  struct {
					GoalComplete bool `json:"goal_complete"`
				} `json:"stats"`
			} `json:"run"`
		}
		if _, err := s.mediaJobRequest(ctx, http.MethodGet, root, nil, &rec); err != nil {
			return err
		}
		if rec.Run.Status != "done" {
			return nil
		}
		// The game adapter's goal flag is the authority; badges or goal text are not.
		if rec.Run.Reason != "done" || !rec.Run.Stats.GoalComplete {
			st.Stage, st.Done = "goal_not_complete", true
			return nil
		}
		st.Stage = "broadcast_render"
	}

	if st.Stage == "broadcast_render" {
		var rs replayStatus
		if _, err := call(http.MethodPost, "/replay/render?mode=broadcast", struct{}{}, &rs); err != nil {
			return err
		}
		if rs.State != "ready" {
			return nil
		}
		st.Stage = "producer"
	}

	if st.AssessmentHash == "" {
		var reply struct {
			Assessment struct {
				Hash             string `json:"hash"`
				PublicationClass string `json:"publication_class"`
			} `json:"assessment"`
		}
		if _, err := call(http.MethodPost, "/producer/assess", struct{}{}, &reply); err != nil {
			return err
		}
		if !validEditHash(reply.Assessment.Hash) {
			return errors.New("producer returned no assessment hash")
		}
		st.AssessmentHash, st.Verdict = reply.Assessment.Hash, reply.Assessment.PublicationClass
	}

	var formats []string
	switch st.Verdict {
	case "full_episode":
		formats = []string{"episode", "short"}
	case "short_only":
		formats = []string{"short"}
	case "archive_only", "do_not_publish":
		if _, err := call(http.MethodPost, "/studio/publications", map[string]string{"assessment_hash": st.AssessmentHash, "format": "archive"}, nil); err != nil {
			return err
		}
		st.Stage, st.Done = "archived_"+st.Verdict, true
		return nil
	default:
		st.Stage, st.Done = "unknown_verdict", true
		return fmt.Errorf("unknown producer verdict %q", st.Verdict)
	}
	if st.Plans == nil {
		st.Plans = map[string]string{}
	}
	if st.Candidates == nil {
		st.Candidates = map[string]string{}
	}

	for _, format := range formats {
		if st.Candidates[format] != "" {
			continue
		}
		st.Stage = format + "_render"
		if st.Plans[format] == "" {
			var reply struct {
				Plan struct {
					Hash string `json:"hash"`
				} `json:"plan"`
			}
			if _, err := call(http.MethodPost, "/studio/edits", map[string]string{"assessment_hash": st.AssessmentHash, "profile": format}, &reply); err != nil {
				return err
			}
			if !validEditHash(reply.Plan.Hash) {
				return errors.New(format + " edit plan has no hash")
			}
			st.Plans[format] = reply.Plan.Hash
		}
		var es editStatus
		if _, err := call(http.MethodPost, "/studio/edits/"+st.Plans[format]+"/render", struct{}{}, &es); err != nil {
			return err
		}
		if es.State != "ready" {
			return nil
		}

		st.Stage = format + "_package"
		var pkg struct {
			Package struct {
				Hash   string `json:"hash"`
				Titles []struct {
					ID string `json:"id"`
				} `json:"titles"`
				Shorts []struct {
					ID string `json:"id"`
				} `json:"shorts"`
			} `json:"package"`
		}
		if _, err := call(http.MethodPost, "/studio/packages", map[string]string{
			"assessment_hash": st.AssessmentHash, "edit_plan_hash": st.Plans[format], "style": "documentary",
		}, &pkg); err != nil {
			return err
		}
		titles := pkg.Package.Titles
		if format == "short" {
			titles = pkg.Package.Shorts
		}
		if !validEditHash(pkg.Package.Hash) || len(titles) == 0 || titles[0].ID == "" {
			return errors.New(format + " package has no hash or event-backed title")
		}
		proposal := map[string]any{
			"assessment_hash": st.AssessmentHash, "package_hash": pkg.Package.Hash,
			"format": format, "title_id": titles[0].ID, "privacy": "private",
		}
		if format == "episode" {
			thumbs := "/studio/packages/" + pkg.Package.Hash + "/thumbnails"
			var list struct {
				Manifest struct {
					Candidates []struct {
						ID string `json:"id"`
					} `json:"candidates"`
				} `json:"manifest"`
			}
			if _, err := call(http.MethodPost, thumbs, map[string]any{"variants": 1}, &list); err != nil {
				return err
			}
			if len(list.Manifest.Candidates) == 0 || !validEditHash(list.Manifest.Candidates[0].ID) {
				return errors.New("episode has no thumbnail candidate")
			}
			id := list.Manifest.Candidates[0].ID
			if _, err := call(http.MethodPost, thumbs+"/"+id+"/select", struct{}{}, nil); err != nil {
				return err
			}
			proposal["thumbnail_id"] = id
		}

		st.Stage = format + "_candidate"
		var candidate struct {
			ID     string `json:"id"`
			State  string `json:"state"`
			Reason string `json:"reason"`
		}
		if _, err := call(http.MethodPost, "/studio/publications", proposal, &candidate); err != nil {
			return err
		}
		if !validEditHash(candidate.ID) {
			return errors.New(format + " candidate has no id")
		}
		// A quality-gate rejection is still a durable, reviewable outcome.
		st.Candidates[format] = candidate.ID
		if candidate.State != "queued" && candidate.State != "approved" && candidate.State != "published" {
			log.Printf("pokereplay: production %s: %s candidate %s: %s", st.RunID, format, candidate.State, candidate.Reason)
		}
	}
	st.Stage, st.Done = "ready_for_review", true
	return nil
}

// selfJSON calls this server's own handler in-process.
func selfJSON(ctx context.Context, h http.Handler, method, route string, body, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, route, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code < 200 || rec.Code >= 300 {
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &payload)
		if payload.Error == "" {
			payload.Error = http.StatusText(rec.Code)
		}
		return rec.Code, fmt.Errorf("%s %s: %d %s", method, route, rec.Code, payload.Error)
	}
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			return rec.Code, fmt.Errorf("decode %s: %w", route, err)
		}
	}
	return rec.Code, nil
}
