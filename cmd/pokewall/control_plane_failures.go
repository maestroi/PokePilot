package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func (cp *controlPlane) persistObjectiveFailuresTx(tx *sql.Tx, report farm.FinishReport, attempt int) error {
	failures, err := farm.DecodeObjectiveFailures(report)
	if err != nil {
		return err
	}
	if synthetic, ok := terminalRunFailure(report, failures); ok {
		failures = append(failures, synthetic)
	}
	safeReport := sanitizeFinishReport(report)
	reportRaw, _ := json.Marshal(safeReport)
	for _, failure := range failures {
		key, fp, _, err := objectiveFailureFingerprint(failure)
		if err != nil {
			return err
		}
		failureRaw, _ := json.Marshal(failure)
		ext := objectiveFailureExternalID(report.RunID, attempt, key)
		if _, err := tx.Exec(`INSERT INTO objective_failures(run_id,attempt,failure_key,fingerprint,blocking,terminal_count,failure_json,report_json,delivery_status,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,'pending',NOW()) ON CONFLICT(run_id,attempt,failure_key) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,blocking=EXCLUDED.blocking,terminal_count=EXCLUDED.terminal_count,failure_json=EXCLUDED.failure_json,report_json=EXCLUDED.report_json,updated_at=NOW()`, report.RunID, attempt, key, fp, failure.Blocking, failure.TerminalCount, string(failureRaw), string(reportRaw)); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO issue_fingerprints(failure_key,fingerprint,payload_json,updated_at) VALUES($1,$2,$3::jsonb,NOW()) ON CONFLICT(failure_key) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,payload_json=EXCLUDED.payload_json,updated_at=NOW()`, key, fp, string(failureRaw)); err != nil {
			return err
		}
		occ := map[string]any{"failure": failure, "fingerprint": fp}
		occRaw, _ := json.Marshal(occ)
		if _, err := tx.Exec(`INSERT INTO issue_occurrences(external_id,failure_key,run_id,attempt,payload_json) VALUES($1,$2,$3,$4,$5::jsonb) ON CONFLICT(external_id) DO UPDATE SET payload_json=EXCLUDED.payload_json`, ext, key, report.RunID, attempt, string(occRaw)); err != nil {
			return err
		}
	}
	return nil
}

// RunControlPlaneObjectiveFailures retries structured failure reports from the
// database. Unlike the legacy path, startup never scans dump directories, so
// an old cache cannot create live work after the control-plane DB is reset.
func (w *Wall) RunControlPlaneObjectiveFailures(every time.Duration) {
	cp := controlPlaneFor(w)
	if cp == nil || w.issueClient() == nil {
		return
	}
	if every <= 0 {
		every = defaultObjectiveFailureReportEvery
	}
	for {
		rows, err := cp.pendingObjectiveFailures(32)
		if err != nil {
			log.Printf("pokewall: load objective failure outbox: %v", err)
		} else {
			for _, item := range rows {
				cp.deliverObjectiveFailure(w, item)
			}
		}
		time.Sleep(every)
	}
}

type pendingObjectiveFailure struct {
	runID   string
	key     string
	attempt int
	failure farm.ObjectiveFailure
	report  farm.FinishReport
}

func (cp *controlPlane) pendingObjectiveFailures(limit int) ([]pendingObjectiveFailure, error) {
	rows, err := cp.db.Query(`SELECT run_id,attempt,failure_key,failure_json,report_json FROM objective_failures WHERE delivery_status='pending' ORDER BY updated_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pendingObjectiveFailure
	for rows.Next() {
		var item pendingObjectiveFailure
		var failureRaw, reportRaw []byte
		if err := rows.Scan(&item.runID, &item.attempt, &item.key, &failureRaw, &reportRaw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(failureRaw, &item.failure); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(reportRaw, &item.report); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (cp *controlPlane) deliverObjectiveFailure(w *Wall, item pendingObjectiveFailure) {
	err := w.reportObjectiveFailure(item.report, item.failure)
	status := "complete"
	errText := ""
	if err != nil {
		errText = err.Error()
		if isRetryableIssueError(err) {
			status = "pending"
		} else {
			status = "error"
		}
	}
	if _, dbErr := cp.db.Exec(`UPDATE objective_failures SET delivery_status=$1,delivery_error=$2,updated_at=NOW() WHERE run_id=$3 AND attempt=$4 AND failure_key=$5`, status, errText, item.runID, item.attempt, item.key); dbErr != nil {
		log.Printf("pokewall: update objective failure delivery: %v", dbErr)
	}
	// reportObjectiveFailure updates the in-memory canonical issue link. Persist
	// that immediately so a wall crash after the remote call cannot lose the
	// durable mapping and create a duplicate occurrence later.
	if persistErr := cp.persistWall(w); persistErr != nil {
		log.Printf("pokewall: persist issue link after objective delivery: %v", persistErr)
	}
	if err != nil {
		log.Printf("pokewall: objective failure %s/%d/%s: %v", item.runID, item.attempt, item.key, err)
	}
}
