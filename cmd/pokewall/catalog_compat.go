package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// catalogOperatorCompatibility keeps the operator/debug behavior that used to
// be derived from finished tiles in RAM. The catalog remains the history
// source; issue links and outbox state stay live in the wall state store
// (PostgreSQL in production, state.json only in legacy/local mode) and are
// overlaid at read time so a later GitHub status change never leaves stale JSON.
func (w *Wall) catalogOperatorCompatibility(next http.Handler) http.Handler {
	if catalogFor(w) == nil {
		return next
	}
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == "/v1/triage" {
			w.handleCatalogTriage(res)
			return
		}

		rewrite := req.Method == http.MethodGet && (req.URL.Path == "/v1/dashboard" || isCatalogRunRead(req.URL.Path))
		deleteRun := req.Method == http.MethodDelete && isCatalogRunRoot(req.URL.Path)
		if !rewrite && !deleteRun {
			next.ServeHTTP(res, req)
			return
		}

		buffered := newCatalogBufferedWriter()
		next.ServeHTTP(buffered, req)
		if buffered.status < 200 || buffered.status >= 300 {
			buffered.flush(res)
			return
		}

		if deleteRun {
			if catalog := catalogFor(w); catalog != nil {
				if err := catalog.delete(catalogRunIDFromPath(req.URL.Path)); err != nil {
					writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "delete catalog run: " + err.Error()})
					return
				}
			}
			buffered.flush(res)
			return
		}

		var (
			body []byte
			err  error
		)
		if req.URL.Path == "/v1/dashboard" {
			body, err = w.overlayDashboardIssues(buffered.body.Bytes())
		} else {
			body, err = w.overlayRunEnvelopeIssue(buffered.body.Bytes())
		}
		if err != nil {
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "catalog response overlay: " + err.Error()})
			return
		}
		for key, values := range buffered.header {
			res.Header()[key] = append([]string(nil), values...)
		}
		res.WriteHeader(buffered.status)
		_, _ = res.Write(body)
	})
}

func isCatalogRunRoot(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return len(parts) == 3 && parts[0] == "v1" && parts[1] == "runs" && parts[2] != ""
}

func isCatalogRunRead(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 3 && parts[0] == "v1" && parts[1] == "runs" && parts[2] != "" {
		return true
	}
	return len(parts) == 4 && parts[0] == "v1" && parts[1] == "runs" && parts[2] != "" && parts[3] == "debug"
}

func catalogRunIDFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "v1" && parts[1] == "runs" {
		return parts[2]
	}
	return ""
}

func (w *Wall) currentIssueForRow(row tileRow) *IssueLink {
	if row.Status != statusDone || (row.Reason != "error" && row.Reason != "lost") || strings.TrimSpace(row.Detail) == "" {
		return nil
	}
	key, _ := failureIdentity(normalizeDetail(row.Detail))
	w.mu.Lock()
	link, ok := w.issueLinks[key]
	w.mu.Unlock()
	if !ok || link.IssueID == "" {
		return nil
	}
	copy := link
	return &copy
}

func (w *Wall) overlayDashboardIssues(data []byte) ([]byte, error) {
	var dashboard runtimeDashboardView
	if err := json.Unmarshal(data, &dashboard); err != nil {
		return nil, err
	}
	for i := range dashboard.Runs {
		dashboard.Runs[i].Issue = w.currentIssueForRow(dashboard.Runs[i])
	}
	return json.Marshal(dashboard)
}

func (w *Wall) overlayRunEnvelopeIssue(data []byte) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	raw, ok := envelope["run"]
	if !ok {
		return data, nil
	}
	var row tileRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, err
	}
	row.Issue = w.currentIssueForRow(row)
	updated, err := json.Marshal(row)
	if err != nil {
		return nil, err
	}
	envelope["run"] = updated
	return json.Marshal(envelope)
}

// catalogTriage streams historical failures from the configured run catalog
// (PostgreSQL in production, SQLite only in legacy/local mode) and keeps one
// accumulator per normalized failure pattern plus a small newest-run sample.
// This preserves the old triage semantics without retaining every finished
// Tile in the wall process.
func (w *Wall) catalogTriage() ([]triageGroup, error) {
	if cp := controlPlaneFor(w); cp != nil {
		return cp.objectiveFailureTriage(w)
	}
	catalog := catalogFor(w)
	if catalog == nil {
		return w.triage(), nil
	}
	rows, err := catalog.db.Query(`SELECT row_json FROM runs WHERE status=? AND outcome IN ('error','lost') ORDER BY ended_at DESC, queued_at DESC, run_id DESC`, statusDone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type accumulator struct {
		example string
		count   int
		ids     []string
	}
	groups := make(map[string]*accumulator)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var row tileRow
		if err := json.Unmarshal(data, &row); err != nil {
			return nil, err
		}
		if strings.TrimSpace(row.Detail) == "" {
			continue
		}
		pattern := normalizeDetail(row.Detail)
		group := groups[pattern]
		if group == nil {
			group = &accumulator{example: row.Detail}
			groups[pattern] = group
		}
		group.count++
		if len(group.ids) < triageRunIDCap {
			group.ids = append(group.ids, row.RunID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]triageGroup, 0, len(groups))
	w.mu.Lock()
	defer w.mu.Unlock()
	for pattern, group := range groups {
		key, fingerprint := failureIdentity(pattern)
		item := triageGroup{
			Pattern: pattern, Key: key, Fingerprint: fingerprint,
			Count: group.count, Example: group.example, RunIDs: group.ids,
			Outbox: outboxStatusForKey(w.outbox, key),
		}
		if link, ok := w.issueLinks[key]; ok && link.IssueID != "" {
			copy := link
			item.Issue = &copy
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Pattern < out[j].Pattern
	})
	return out, nil
}

func (w *Wall) handleCatalogTriage(res http.ResponseWriter) {
	groups, err := w.catalogTriage()
	if err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "query catalog triage: " + err.Error()})
		return
	}
	writeJSON(res, http.StatusOK, groups)
}
