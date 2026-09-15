package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// rebuildFinishCache materializes the small sanitized finish-report cache used
// by the existing inspector and generic issue adapter. PostgreSQL remains the
// authority: the cache is cleared first and can be regenerated after any wall
// move/restart. Large payload bytes are not present in report_json.
func (cp *controlPlane) rebuildFinishCache(w *Wall) error {
	if cp == nil || w == nil || w.dumpsDir == "" {
		return nil
	}
	if err := os.MkdirAll(w.dumpsDir, 0o755); err != nil {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(w.dumpsDir, "*.json"))
	if err != nil {
		return err
	}
	for _, path := range matches {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clear finish cache %s: %w", filepath.Base(path), err)
		}
	}

	rows, err := cp.db.Query(`SELECT run_id,attempt,report_json FROM run_attempts ORDER BY run_id,attempt`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var runID string
		var attempt int
		var raw []byte
		if err := rows.Scan(&runID, &attempt, &raw); err != nil {
			return err
		}
		name := safeDumpName(runID)
		if attempt > 1 {
			name = fmt.Sprintf("%s-attempt-%d.json", safeBase(runID), attempt)
		}
		path := filepath.Join(w.dumpsDir, name)
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, raw, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	return rows.Err()
}
