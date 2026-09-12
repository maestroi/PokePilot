package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/maestroi/pokepilot/farm"
)

const (
	issueSource            = "pokefarm"
	issueResponseLimit     = 1 << 20
	defaultIssueTimeout    = 30 * time.Second
	defaultStatusSyncEvery = 30 * time.Second
	maxIssueTitleBytes     = 200
)

type issueClient struct {
	apiBase   string
	projectID string
	uiBase    string
	http      *http.Client
}

func newIssueClient(apiBase, projectID, uiBase string, timeout time.Duration) *issueClient {
	if timeout <= 0 {
		timeout = defaultIssueTimeout
	}
	return &issueClient{
		apiBase:   strings.TrimRight(apiBase, "/"),
		projectID: projectID,
		uiBase:    strings.TrimRight(uiBase, "/"),
		http:      &http.Client{Timeout: timeout},
	}
}

func (c *issueClient) issueURL(id string) string {
	return c.uiBase + "/issues/" + id
}

type issueReportManifest struct {
	Source           string          `json:"source"`
	Fingerprint      string          `json:"fingerprint"`
	ExternalID       string          `json:"external_id"`
	Title            string          `json:"title"`
	Summary          string          `json:"summary"`
	ObservedAt       time.Time       `json:"observed_at,omitempty"`
	ObservedRevision string          `json:"observed_revision,omitempty"`
	Severity         string          `json:"severity,omitempty"`
	Evidence         json.RawMessage `json:"evidence"`
}

type issueReportResponse struct {
	Issue struct {
		ID          string `json:"id"`
		IssueNumber int64  `json:"issue_number"`
		Status      string `json:"status"`
	} `json:"issue"`
	Occurrence struct {
		ID         string `json:"id"`
		ExternalID string `json:"external_id"`
	} `json:"occurrence"`
	Deduplicated bool `json:"deduplicated"`
	Automation   struct {
		Status  string `json:"status"`
		Warning string `json:"warning,omitempty"`
	} `json:"automation"`
}

type issueStatusResponse struct {
	ID              string `json:"id"`
	IssueNumber     int64  `json:"issue_number"`
	Status          string `json:"status"`
	Resolution      string `json:"resolution,omitempty"`
	OccurrenceCount int64  `json:"occurrence_count"`
	FixedRevision   string `json:"fixed_revision,omitempty"`
}

func (c *issueClient) Report(ctx context.Context, manifest issueReportManifest, artifacts []farm.Artifact) (issueReportResponse, error) {
	var out issueReportResponse
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: artifacts}); err != nil {
		return out, err
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		err := writeIssueMultipart(mw, manifest, artifacts)
		closeErr := mw.Close()
		if err == nil {
			err = closeErr
		}
		pw.CloseWithError(err)
	}()
	url := c.apiBase + "/api/projects/" + c.projectID + "/issue-reports"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, issueResponseLimit))
	if err != nil {
		return out, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return out, issueHTTPError(resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("decode issue report: %w", err)
	}
	return out, nil
}

func (c *issueClient) GetIssue(ctx context.Context, id string) (issueStatusResponse, error) {
	var out issueStatusResponse
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+"/api/issues/"+id, nil)
	if err != nil {
		return out, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, issueResponseLimit))
	if err != nil {
		return out, err
	}
	if resp.StatusCode != http.StatusOK {
		return out, issueHTTPError(resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *issueClient) Investigate(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/api/issues/"+id+"/investigate", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, issueResponseLimit))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
		return nil
	}
	if issueAlreadyInvestigating(resp.StatusCode, body) {
		return nil
	}
	return issueHTTPError(resp.StatusCode, body)
}

func issueAlreadyInvestigating(statusCode int, body []byte) bool {
	if statusCode != http.StatusConflict {
		return false
	}
	return strings.Contains(strings.ToLower(string(body)), "investigating")
}

func writeIssueMultipart(mw *multipart.Writer, manifest issueReportManifest, artifacts []farm.Artifact) error {
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="report"`)
	hdr.Set("Content-Type", "application/json")
	part, err := mw.CreatePart(hdr)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(part).Encode(manifest); err != nil {
		return err
	}
	for _, a := range artifacts {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="artifact"; filename="%s"`, a.Name))
		media := a.MediaType
		if media == "" {
			media = "application/octet-stream"
		}
		h.Set("Content-Type", media)
		p, err := mw.CreatePart(h)
		if err != nil {
			return err
		}
		if _, err := p.Write(a.Data); err != nil {
			return err
		}
	}
	return nil
}

func issueHTTPError(code int, body []byte) error {
	var wrapped struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &wrapped) == nil && wrapped.Error != "" {
		return fmt.Errorf("agent orchestrator: status %d: %s", code, wrapped.Error)
	}
	return fmt.Errorf("agent orchestrator: status %d", code)
}

func (w *Wall) SetIssueClient(c *issueClient) {
	w.mu.Lock()
	w.issues = c
	w.mu.Unlock()
	go w.runOutboxDispatcher()
	go w.runStatusSync(defaultStatusSyncEvery)
}

func (w *Wall) runOutboxDispatcher() {
	backoff := time.Second
	for {
		e, ok := w.nextOutbox()
		if !ok {
			time.Sleep(time.Second)
			backoff = time.Second
			continue
		}
		if err := w.dispatchOccurrence(e); err != nil {
			retryable := isRetryableIssueError(err)
			w.noteOutboxResult(e.ExternalID, err, retryable)
			if retryable {
				time.Sleep(backoff)
				if backoff < 30*time.Second {
					backoff *= 2
				}
			} else {
				time.Sleep(time.Second)
			}
			continue
		}
		backoff = time.Second
	}
}

func (w *Wall) nextOutbox() (outboxEntry, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now().Unix()
	for _, e := range w.outbox {
		if e.Status != outboxPending {
			continue
		}
		if e.NextAttempt > now {
			continue
		}
		return e, true
	}
	return outboxEntry{}, false
}

func (w *Wall) dispatchOccurrence(e outboxEntry) error {
	c := w.issueClient()
	if c == nil {
		return fmt.Errorf("issue integration is not configured")
	}
	dump, arts, err := w.loadOccurrenceEvidence(e)
	if err != nil {
		return err
	}
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: arts, SeedBurn: dump.SeedBurn}); err != nil {
		return err
	}
	pattern := normalizeDetail(dump.Detail)
	key, fp := failureIdentity(pattern)
	markerKey, markerFP, structured := farm.ParseFailureDetailMarker(dump.Detail)
	if structured {
		key, fp = markerKey, markerFP
		if e.Key != "" && e.Key != key {
			return fmt.Errorf("structured failure outbox key %q does not match marker key %q", e.Key, key)
		}
	} else if e.Key != "" {
		key = e.Key
	}

	// A v2 structured run also carries objective-failures.json. That reporter
	// owns the richer occurrence: it has typed cause/state plus the nearest
	// objective checkpoint. Settling the generic run outbox here prevents one
	// terminal failure from becoming two Agent Orchestrator occurrences.
	if structured && hasObjectiveFailureArtifact(dump) {
		w.quarantineOccurrence(e, fp, dump.RunnerVersion, "structured objective failure reporter owns canonical occurrence")
		return nil
	}

	prior := IssueLink{}
	disposition := occurrenceReport
	if structured {
		prior, disposition = w.issueDispositionForKey(key)
		if disposition == occurrenceQuarantine {
			w.quarantineOccurrence(e, fp, dump.RunnerVersion, "equivalent structured fingerprint already has active issue")
			return nil
		}
	}

	title := truncateBytes(dump.Detail, maxIssueTitleBytes)
	if title == "" {
		title = "pokefarm failure"
	}
	severity := "normal"
	classification := "observed"
	if disposition == occurrenceRegression {
		severity = "critical"
		classification = "regression"
		title = truncateBytes("[farm][regression] "+title, maxIssueTitleBytes)
	}

	w.mu.Lock()
	var seed int64
	var question, decision string
	if t := w.tiles[e.RunID]; t != nil {
		seed = t.Seed
		question = t.Question
		decision = t.Decision
	}
	w.mu.Unlock()
	evidence, _ := json.Marshal(map[string]any{
		"classification":       classification,
		"disposition":          string(disposition),
		"run_id":               e.RunID,
		"attempt":              e.Attempt,
		"seed":                 seed,
		"seed_burn":            dump.SeedBurn,
		"reason":               dump.Reason,
		"detail":               dump.Detail,
		"trace_tail":           dump.TraceTail,
		"question":             question,
		"decision":             decision,
		"runner_version":       dump.RunnerVersion,
		"prior_issue_status":   prior.Status,
		"prior_resolution":     prior.Resolution,
		"prior_fixed_revision": prior.FixedRevision,
	})
	summary := dump.Detail
	if disposition == occurrenceRegression {
		summary = fmt.Sprintf("Regression candidate: fingerprint %s reproduced on revision %q after issue %d was resolved at revision %q. %s",
			fp, dump.RunnerVersion, prior.IssueNumber, prior.FixedRevision, dump.Detail)
	}
	manifest := issueReportManifest{
		Source:           issueSource,
		Fingerprint:      fp,
		ExternalID:       e.ExternalID,
		Title:            title,
		Summary:          summary,
		ObservedAt:       time.Now().UTC(),
		ObservedRevision: dump.RunnerVersion,
		Severity:         severity,
		Evidence:         evidence,
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultIssueTimeout)
	defer cancel()
	result, err := c.Report(ctx, manifest, arts)
	if err != nil {
		return err
	}
	if result.Issue.ID == "" {
		return fmt.Errorf("agent orchestrator returned an empty issue id")
	}
	now := time.Now().Unix()
	w.mu.Lock()
	link := w.issueLinks[key]
	link.IssueID = result.Issue.ID
	link.IssueNumber = result.Issue.IssueNumber
	link.IssueURL = c.issueURL(result.Issue.ID)
	link.Status = result.Issue.Status
	link.LastReportedRun = e.RunID
	link.LastObservedRun = e.RunID
	link.LastObservedRevision = strings.TrimSpace(dump.RunnerVersion)
	link.LastDisposition = string(disposition)
	link.UpdatedAt = now
	link.Fingerprint = fp
	w.issueLinks[key] = link
	ent := w.outbox[e.ExternalID]
	ent.Status = outboxComplete
	ent.Error = ""
	ent.Note = ""
	ent.UpdatedAt = now
	w.outbox[e.ExternalID] = ent
	w.mu.Unlock()
	w.saveState()
	return nil
}

func (w *Wall) loadOccurrenceEvidence(e outboxEntry) (farm.FinishReport, []farm.Artifact, error) {
	if w.dumpsDir == "" {
		return farm.FinishReport{}, nil, fmt.Errorf("no dump directory")
	}
	name := safeDumpName(e.RunID)
	if e.Attempt > 1 {
		name = fmt.Sprintf("%s-attempt-%d.json", safeBase(e.RunID), e.Attempt)
	}
	path := filepath.Join(w.dumpsDir, name)
	data, err := os.ReadFile(path)
	if err != nil && e.Attempt > 1 && errors.Is(err, os.ErrNotExist) {
		// Legacy/hand-built runners can omit FinishReport.Attempt on retries.
		// handleFinish historically wrote those reports to run.json while the
		// issue outbox inferred the real attempt from Tile.Attempts, so pending
		// occurrences pointed at run-attempt-N.json that never existed. Fall
		// back to the legacy filename so those already-persisted outbox entries
		// self-heal after an upgrade.
		path = filepath.Join(w.dumpsDir, safeDumpName(e.RunID))
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return farm.FinishReport{}, nil, err
	}
	var dump farm.FinishReport
	if err := json.Unmarshal(data, &dump); err != nil {
		return farm.FinishReport{}, nil, err
	}
	if dump.RunID != "" && dump.RunID != e.RunID {
		return farm.FinishReport{}, nil, fmt.Errorf("finish dump %s belongs to run %q, wanted %q", path, dump.RunID, e.RunID)
	}
	if dump.Attempt != 0 && e.Attempt > 0 && dump.Attempt != e.Attempt {
		return farm.FinishReport{}, nil, fmt.Errorf("finish dump %s belongs to attempt %d, wanted %d", path, dump.Attempt, e.Attempt)
	}
	if dump.Attempt == 0 && e.Attempt > 0 {
		// Normalize legacy reports before constructing evidence so the uploaded
		// manifest records the actual attempt represented by this outbox entry.
		dump.Attempt = e.Attempt
	}
	var arts []farm.Artifact
	if len(dump.SaveState) > 0 {
		arts = append(arts, hashedNamed("final.state", "application/octet-stream", dump.SaveState))
	}
	if len(dump.FramePNG) > 0 {
		arts = append(arts, hashedNamed("final.png", "image/png", dump.FramePNG))
	}
	arts = append(arts, dump.Artifacts...)
	manifest, _ := json.Marshal(struct {
		RunID         string   `json:"run_id"`
		Attempt       int      `json:"attempt"`
		Reason        string   `json:"reason"`
		Detail        string   `json:"detail"`
		RunnerVersion string   `json:"runner_version,omitempty"`
		SeedBurn      int      `json:"seed_burn"`
		TraceTail     []string `json:"trace_tail,omitempty"`
	}{dump.RunID, e.Attempt, dump.Reason, dump.Detail, dump.RunnerVersion, dump.SeedBurn, dump.TraceTail})
	arts = append(arts, hashedNamed("finish-manifest.json", "application/json", manifest))
	return dump, arts, nil
}

func hashedNamed(name, media string, data []byte) farm.Artifact {
	sum := sha256.Sum256(data)
	return farm.Artifact{
		Name:      name,
		MediaType: media,
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      append([]byte(nil), data...),
	}
}

func (w *Wall) noteOutboxResult(externalID string, err error, retryable bool) {
	w.mu.Lock()
	e, ok := w.outbox[externalID]
	if !ok {
		w.mu.Unlock()
		return
	}
	e.Error = err.Error()
	e.Note = ""
	e.UpdatedAt = time.Now().Unix()
	if retryable {
		e.Status = outboxPending
		delay := 2 * time.Second
		e.NextAttempt = time.Now().Add(delay).Unix()
	} else {
		e.Status = outboxError
	}
	w.outbox[externalID] = e
	w.mu.Unlock()
	w.saveState()
	log.Printf("pokewall: issue report %s: %v", externalID, err)
}

func (w *Wall) issueClient() *issueClient {
	w.mu.Lock()
	c := w.issues
	w.mu.Unlock()
	return c
}

func (w *Wall) runStatusSync(every time.Duration) {
	if every <= 0 {
		every = defaultStatusSyncEvery
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for range ticker.C {
		w.syncIssueStatuses()
	}
}

func (w *Wall) syncIssueStatuses() {
	c := w.issueClient()
	if c == nil {
		return
	}
	w.mu.Lock()
	ids := make([]IssueLink, 0, len(w.issueLinks))
	keys := make([]string, 0, len(w.issueLinks))
	for k, v := range w.issueLinks {
		if v.IssueID == "" {
			continue
		}
		keys = append(keys, k)
		ids = append(ids, v)
	}
	w.mu.Unlock()
	for i, link := range ids {
		ctx, cancel := context.WithTimeout(context.Background(), defaultIssueTimeout)
		got, err := c.GetIssue(ctx, link.IssueID)
		cancel()
		w.mu.Lock()
		cur, ok := w.issueLinks[keys[i]]
		if !ok {
			w.mu.Unlock()
			continue
		}
		if err != nil {
			cur.Stale = true
			w.issueLinks[keys[i]] = cur
			w.mu.Unlock()
			continue
		}
		cur.Status = got.Status
		cur.Resolution = got.Resolution
		cur.OccurrenceCount = got.OccurrenceCount
		cur.FixedRevision = got.FixedRevision
		cur.IssueNumber = got.IssueNumber
		cur.Stale = false
		cur.UpdatedAt = time.Now().Unix()
		w.issueLinks[keys[i]] = cur
		w.mu.Unlock()
	}
	w.saveState()
}

func (w *Wall) handleInvestigate(res http.ResponseWriter, req *http.Request) {
	key := req.PathValue("key")
	c := w.issueClient()
	if c == nil {
		writeJSON(res, http.StatusServiceUnavailable, map[string]string{"error": "issue integration is not configured"})
		return
	}
	w.mu.Lock()
	link, ok := w.issueLinks[key]
	w.mu.Unlock()
	if !ok || link.IssueID == "" {
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown failure group " + key})
		return
	}
	switch strings.ToLower(strings.TrimSpace(link.Status)) {
	case "investigating", "in_progress", "in-progress":
		writeJSON(res, http.StatusOK, link)
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), defaultIssueTimeout)
	defer cancel()
	if err := c.Investigate(ctx, link.IssueID); err != nil {
		writeJSON(res, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	link.Status = "investigating"
	link.UpdatedAt = time.Now().Unix()
	w.mu.Lock()
	w.issueLinks[key] = link
	cp := link
	w.mu.Unlock()
	w.saveState()
	writeJSON(res, http.StatusOK, cp)
}

func isRetryableIssueError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "status 4") && !strings.Contains(msg, "status 409") {
		return false
	}
	if strings.Contains(msg, "sha256") || strings.Contains(msg, "unsafe") || strings.Contains(msg, "negative seed") {
		return false
	}
	return true
}

func truncateBytes(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	for n > 0 && !utf8.ValidString(s[:n]) {
		n--
	}
	return s[:n]
}

func parseIssueFlags(api, project, ui string) (*issueClient, error) {
	api, project, ui = strings.TrimSpace(api), strings.TrimSpace(project), strings.TrimSpace(ui)
	n := 0
	if api != "" {
		n++
	}
	if project != "" {
		n++
	}
	if ui != "" {
		n++
	}
	if n == 0 {
		return nil, nil
	}
	if n != 3 {
		return nil, fmt.Errorf("issues integration requires -issues-api, -issues-project, and -issues-ui together")
	}
	return newIssueClient(api, project, ui, defaultIssueTimeout), nil
}
