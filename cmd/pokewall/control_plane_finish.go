package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

const maxPostgresInlineEvidence = 256 << 10

func sanitizeFinishReport(report farm.FinishReport) farm.FinishReport {
	safe := report
	safe.SaveState = nil
	safe.FramePNG = nil
	safe.Artifacts = append([]farm.Artifact(nil), report.Artifacts...)
	for i := range safe.Artifacts {
		a := &safe.Artifacts[i]
		// Small structured evidence is useful for durable issue retries and is
		// appropriate database data. Binary/large payloads belong in S3.
		if len(a.Data) > maxPostgresInlineEvidence || !strings.Contains(strings.ToLower(a.MediaType), "json") {
			a.Data = nil
		}
	}
	return safe
}

func (cp *controlPlane) persistFinish(w *Wall, report farm.FinishReport) error {
	// Normal HTTP finishes have already had a frame attached before the inner
	// catalog handler can evict the completed tile. Keep this RAM fallback for
	// direct/internal callers that persist a freshly settled run themselves.
	if len(report.FramePNG) == 0 {
		w.mu.Lock()
		if t := w.tiles[report.RunID]; t != nil && t.Finished && len(t.lastFrame) > 0 {
			report.FramePNG = append([]byte(nil), t.lastFrame...)
		}
		w.mu.Unlock()
	}

	attempt := report.Attempt
	if attempt <= 0 {
		w.mu.Lock()
		if t := w.tiles[report.RunID]; t != nil {
			attempt = t.Attempts
		}
		w.mu.Unlock()
		if attempt <= 0 {
			attempt = 1
		}
	}

	durable, err := durabilizeFinishReport(report, attempt)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			durable.cleanupUploads()
		}
	}()

	raw, err := json.Marshal(durable.report)
	if err != nil {
		return err
	}
	tx, err := cp.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`INSERT INTO run_attempts(run_id,attempt,reason,detail,runner_version,seed_burn,report_json,finished_at) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,NOW()) ON CONFLICT(run_id,attempt) DO UPDATE SET reason=EXCLUDED.reason,detail=EXCLUDED.detail,runner_version=EXCLUDED.runner_version,seed_burn=EXCLUDED.seed_burn,report_json=EXCLUDED.report_json,finished_at=NOW()`, durable.report.RunID, attempt, durable.report.Reason, durable.report.Detail, durable.report.RunnerVersion, durable.report.SeedBurn, string(raw)); err != nil {
		return fmt.Errorf("persist run attempt: %w", err)
	}
	for _, p := range durable.artifacts {
		metaRaw, _ := json.Marshal(p.meta)
		var inline any
		if p.inline != nil {
			inline = p.inline
		}
		if _, err := tx.Exec(`INSERT INTO artifacts(run_id,attempt,kind,name,media_type,sha256,store,bucket,object_key,size,metadata_json,inline_data) VALUES($1,$2,'finish',$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11) ON CONFLICT(run_id,attempt,kind,name) DO UPDATE SET media_type=EXCLUDED.media_type,sha256=EXCLUDED.sha256,store=EXCLUDED.store,bucket=EXCLUDED.bucket,object_key=EXCLUDED.object_key,size=EXCLUDED.size,metadata_json=EXCLUDED.metadata_json,inline_data=EXCLUDED.inline_data`, durable.report.RunID, attempt, p.meta.Name, p.meta.MediaType, p.meta.SHA256, p.meta.Store, p.meta.Bucket, p.meta.ObjectKey, checkpointArtifactSize(p.meta, p.inline), string(metaRaw), inline); err != nil {
			return fmt.Errorf("persist artifact %s: %w", p.meta.Name, err)
		}
	}
	if err := cp.persistObjectiveFailuresTx(tx, durable.report, attempt); err != nil {
		return err
	}
	if err := cp.persistStrategicRecordsTx(tx, w, durable.report.RunID, attempt); err != nil {
		return err
	}
	// Typed decisions are not written per call: the run's row keeps their
	// fixed-size LLMStats.DecisionSummary. decision_exchanges only holds
	// rows from older runs.
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (cp *controlPlane) persistStrategicRecordsTx(tx *sql.Tx, w *Wall, runID string, attempt int) error {
	row, ok := w.ramRow(runID)
	if !ok {
		if catalog := catalogFor(w); catalog != nil {
			var err error
			row, ok, err = catalog.get(runID)
			if err != nil {
				return err
			}
		}
	}
	if !ok || row.Stats == nil {
		return nil
	}
	for i, rec := range row.Stats.StrategicRecords {
		offered, _ := json.Marshal(rec.Offered)
		steps, _ := json.Marshal(rec.PlanSteps)
		obs := ""
		if len(rec.Observation) > 0 && json.Valid(rec.Observation) {
			obs = string(rec.Observation)
		}
		q := `INSERT INTO llm_exchanges(run_id,attempt,exchange_index,observation,offered,replan_reason,plan_goal,plan_steps,rejected,error,duration_seconds,backend,model,prompt_tokens,completion_tokens,prefill_tps,decode_tps) VALUES($1,$2,$3,CASE WHEN $4::text='' THEN NULL ELSE $4::jsonb END,$5::jsonb,$6,$7,$8::jsonb,$9,$10,$11,$12,$13,$14,$15,$16,$17) ON CONFLICT(run_id,attempt,exchange_index) DO UPDATE SET observation=EXCLUDED.observation,offered=EXCLUDED.offered,replan_reason=EXCLUDED.replan_reason,plan_goal=EXCLUDED.plan_goal,plan_steps=EXCLUDED.plan_steps,rejected=EXCLUDED.rejected,error=EXCLUDED.error,duration_seconds=EXCLUDED.duration_seconds,backend=EXCLUDED.backend,model=EXCLUDED.model,prompt_tokens=EXCLUDED.prompt_tokens,completion_tokens=EXCLUDED.completion_tokens,prefill_tps=EXCLUDED.prefill_tps,decode_tps=EXCLUDED.decode_tps`
		if _, err := tx.Exec(q, runID, attempt, i, obs, string(offered), rec.ReplanReason, rec.PlanGoal, string(steps), rec.Rejected, rec.Error, rec.DurationSeconds, rec.Backend, rec.Model, rec.PromptTokens, rec.CompletionTokens, rec.PrefillTPS, rec.DecodeTPS); err != nil {
			return fmt.Errorf("persist LLM exchange %d: %w", i, err)
		}
	}
	return nil
}

// controlPlaneHTTPHandler holds successful mutations until their PostgreSQL
// writes complete. Heartbeats persist only the compact recovery snapshot; once
// a run is already marked running, heartbeat-only telemetry hashes identically
// and persistWall returns without issuing a database transaction.
func (w *Wall) controlPlaneHTTPHandler(next http.Handler) http.Handler {
	cp := controlPlaneFor(w)
	if cp == nil {
		return next
	}
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet || req.URL.Path == "/v1/workers" {
			next.ServeHTTP(res, req)
			return
		}
		if strings.HasSuffix(req.URL.Path, "/heartbeat") {
			buffered := newCatalogBufferedWriter()
			next.ServeHTTP(buffered, req)
			if buffered.status >= 200 && buffered.status < 300 {
				if err := cp.persistWall(w); err != nil {
					writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "persist control plane: " + err.Error()})
					return
				}
			}
			buffered.flush(res)
			return
		}
		var finish *farm.FinishReport
		if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/finish") {
			data, err := io.ReadAll(io.LimitReader(req.Body, maxFinishBody+1))
			if err != nil {
				writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if len(data) > maxFinishBody {
				writeJSON(res, http.StatusRequestEntityTooLarge, map[string]string{"error": "finish report too large"})
				return
			}
			var parsed farm.FinishReport
			if json.Unmarshal(data, &parsed) == nil {
				if len(parsed.FramePNG) == 0 {
					parsed.FramePNG = w.captureFinishFrame(parsed.RunID)
				}
				finish = &parsed
				req = withFinishReport(req, parsed)
			}
			req.Body = io.NopCloser(bytes.NewReader(data))
			req.ContentLength = int64(len(data))
		}
		buffered := newCatalogBufferedWriter()
		next.ServeHTTP(buffered, req)
		if buffered.status >= 200 && buffered.status < 300 {
			if finish != nil {
				if err := cp.persistFinish(w, *finish); err != nil {
					writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "persist finish metadata: " + err.Error()})
					return
				}
			}
			if err := cp.persistWall(w); err != nil {
				writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "persist control plane: " + err.Error()})
				return
			}
			if err := cp.persistExperimentController(w); err != nil {
				writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "persist experiments: " + err.Error()})
				return
			}
		}
		buffered.flush(res)
	})
}
