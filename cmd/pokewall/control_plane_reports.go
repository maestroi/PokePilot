package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"

	"github.com/maestroi/pokepilot/farm"
)

// latestFinishReport reads the authoritative finish record for one run directly
// from PostgreSQL. Local finish JSON is only a compatibility cache in control-
// plane mode and must never be able to resurrect history after the DB is reset.
func (cp *controlPlane) latestFinishReport(runID string) (*farm.FinishReport, error) {
	var attempt int
	if err := cp.db.QueryRow(`SELECT attempt FROM run_attempts WHERE run_id=? ORDER BY attempt DESC LIMIT 1`, runID).Scan(&attempt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}
	return cp.finishReport(runID, attempt)
}

// finishReport reconstructs one sanitized FinishReport from run_attempts plus
// the artifact index. Small inline artifacts regain their bytes; S3 artifacts
// remain references. Large payloads are intentionally never pulled through
// PostgreSQL.
func (cp *controlPlane) finishReport(runID string, attempt int) (*farm.FinishReport, error) {
	var raw []byte
	err := cp.db.QueryRow(
		`SELECT report_json FROM run_attempts WHERE run_id=? AND attempt=?`,
		runID, attempt,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, err
	}

	var report farm.FinishReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, err
	}
	if report.RunID == "" {
		report.RunID = runID
	}
	if report.Attempt <= 0 {
		report.Attempt = attempt
	}

	rows, err := cp.db.Query(
		`SELECT metadata_json,inline_data FROM artifacts WHERE run_id=? AND attempt=? AND kind='finish' ORDER BY name`,
		runID, attempt,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []farm.Artifact
	for rows.Next() {
		var metaRaw, inline []byte
		if err := rows.Scan(&metaRaw, &inline); err != nil {
			return nil, err
		}
		var artifact farm.Artifact
		if err := json.Unmarshal(metaRaw, &artifact); err != nil {
			return nil, err
		}
		if inline != nil {
			artifact.Data = append([]byte(nil), inline...)
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(artifacts) > 0 {
		report.Artifacts = artifacts
	}
	return &report, nil
}
