// Command pokeissues adapts PokéWall's durable issue-handoff protocol to
// GitHub Issues. PokéWall keeps its existing outbox, fingerprinting, quarantine,
// and status-sync behavior; this service makes GitHub the issue system of
// record instead of requiring a private Agent Orchestrator deployment.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultHTTPAddr       = ":8080"
	defaultGitHubAPI      = "https://api.github.com"
	defaultGitHubWeb      = "https://github.com"
	defaultRequestTimeout = 30 * time.Second
	maxRequestBytes       = 64 << 20
	maxReportBytes        = 1 << 20
	maxArtifactBytes      = 16 << 20
	maxGitHubResponse     = 2 << 20
	maxSummaryBytes       = 12 << 10
	maxEvidenceValueBytes = 2 << 10
)

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

type artifactMeta struct {
	Name      string
	MediaType string
	Size      int64
	SHA256    string
	Data      []byte
}

type reproCheckpoint struct {
	State     artifactMeta
	Knowledge artifactMeta
}

type githubIssue struct {
	Number      int64           `json:"number"`
	State       string          `json:"state"`
	StateReason string          `json:"state_reason"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	HTMLURL     string          `json:"html_url"`
	PullRequest json.RawMessage `json:"pull_request"`
}

type githubComment struct {
	Body string `json:"body"`
}

type githubClient struct {
	apiBase string
	webBase string
	repo    string
	token   string
	runBase string
	http    *http.Client
}

type issueServer struct {
	github *githubClient
}

func main() {
	httpAddr := flag.String("http", defaultHTTPAddr, "listen address")
	repo := flag.String("repo", envOr("POKEPILOT_GITHUB_REPO", "maestroi/PokePilot"), "GitHub repository in owner/name form")
	apiBase := flag.String("github-api", envOr("POKEPILOT_GITHUB_API", defaultGitHubAPI), "GitHub API base")
	webBase := flag.String("github-web", envOr("POKEPILOT_GITHUB_WEB", defaultGitHubWeb), "GitHub web base")
	runBase := flag.String("run-base", os.Getenv("POKEPILOT_RUN_BASE_URL"), "optional operator URL used for run/debug links in issues")
	flag.Parse()

	token := strings.TrimSpace(os.Getenv("POKEPILOT_GITHUB_TOKEN"))
	if token == "" {
		log.Fatal("pokeissues: POKEPILOT_GITHUB_TOKEN is required")
	}
	client, err := newGitHubClient(*apiBase, *webBase, *repo, token, *runBase, defaultRequestTimeout)
	if err != nil {
		log.Fatalf("pokeissues: %v", err)
	}

	srv := &http.Server{
		Addr:              *httpAddr,
		Handler:           (&issueServer{github: client}).handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("pokeissues listening on http://%s (GitHub repo %s)", *httpAddr, client.repo)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func newGitHubClient(apiBase, webBase, repo, token, runBase string, timeout time.Duration) (*githubClient, error) {
	repo = strings.TrimSpace(repo)
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil, fmt.Errorf("GitHub repo must be owner/name, got %q", repo)
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("GitHub token is required")
	}
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	return &githubClient{
		apiBase: strings.TrimRight(strings.TrimSpace(apiBase), "/"),
		webBase: strings.TrimRight(strings.TrimSpace(webBase), "/"),
		repo:    repo,
		token:   strings.TrimSpace(token),
		runBase: strings.TrimRight(strings.TrimSpace(runBase), "/"),
		http:    &http.Client{Timeout: timeout},
	}, nil
}

func (s *issueServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "backend": "github"})
	})
	mux.HandleFunc("POST /api/projects/{project}/issue-reports", s.handleReport)
	mux.HandleFunc("GET /api/issues/{id}", s.handleGetIssue)
	mux.HandleFunc("POST /api/issues/{id}/investigate", s.handleInvestigate)
	return mux
}

func (s *issueServer) handleReport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	manifest, artifacts, err := decodeIssueReport(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("pokeissues: report received source=%s fingerprint=%s external_id=%s artifacts=%d", manifest.Source, manifest.Fingerprint, manifest.ExternalID, len(artifacts))
	result, created, err := s.github.report(r.Context(), manifest, artifacts)
	if err != nil {
		log.Printf("pokeissues: report failed fingerprint=%s external_id=%s: %v", manifest.Fingerprint, manifest.ExternalID, err)
		writeGitHubError(w, err)
		return
	}
	action := "deduplicated"
	if created {
		action = "created"
	}
	log.Printf("pokeissues: report %s issue=%d fingerprint=%s external_id=%s", action, result.Issue.IssueNumber, manifest.Fingerprint, manifest.ExternalID)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, result)
}

func (s *issueServer) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	result, err := s.github.getIssueStatus(r.Context(), r.PathValue("id"))
	if err != nil {
		writeGitHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *issueServer) handleInvestigate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	log.Printf("pokeissues: investigation requested issue=%s", id)
	if err := s.github.investigate(r.Context(), id); err != nil {
		log.Printf("pokeissues: investigation request failed issue=%s: %v", id, err)
		writeGitHubError(w, err)
		return
	}
	log.Printf("pokeissues: investigation recorded issue=%s", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "investigating"})
}

func decodeIssueReport(r *http.Request) (issueReportManifest, []artifactMeta, error) {
	var manifest issueReportManifest
	reader, err := r.MultipartReader()
	if err != nil {
		return manifest, nil, fmt.Errorf("multipart report: %w", err)
	}
	var artifacts []artifactMeta
	seenReport := false
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return manifest, nil, fmt.Errorf("read multipart: %w", err)
		}
		switch part.FormName() {
		case "report":
			if seenReport {
				return manifest, nil, errors.New("duplicate report part")
			}
			data, err := io.ReadAll(io.LimitReader(part, maxReportBytes+1))
			if err != nil {
				return manifest, nil, err
			}
			if len(data) > maxReportBytes {
				return manifest, nil, errors.New("report part is too large")
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				return manifest, nil, fmt.Errorf("decode report: %w", err)
			}
			seenReport = true
		case "artifact":
			meta, err := readArtifactMeta(part)
			if err != nil {
				return manifest, nil, err
			}
			artifacts = append(artifacts, meta)
		}
	}
	if !seenReport {
		return manifest, nil, errors.New("missing report part")
	}
	if err := validateManifest(manifest); err != nil {
		return manifest, nil, err
	}
	return manifest, artifacts, nil
}

func readArtifactMeta(part *multipart.Part) (artifactMeta, error) {
	name := strings.TrimSpace(part.FileName())
	if name == "" || strings.ContainsAny(name, `/\\`) || name == "." || name == ".." {
		return artifactMeta{}, fmt.Errorf("unsafe artifact name %q", name)
	}
	h := sha256.New()
	var retained bytes.Buffer
	writer := io.Writer(h)
	if isPortableReproCandidate(name) {
		writer = io.MultiWriter(h, &retained)
	}
	n, err := io.Copy(writer, io.LimitReader(part, maxArtifactBytes+1))
	if err != nil {
		return artifactMeta{}, err
	}
	if n > maxArtifactBytes {
		return artifactMeta{}, fmt.Errorf("artifact %q exceeds %d bytes", name, maxArtifactBytes)
	}
	return artifactMeta{
		Name:      name,
		MediaType: part.Header.Get("Content-Type"),
		Size:      n,
		SHA256:    hex.EncodeToString(h.Sum(nil)),
		Data:      retained.Bytes(),
	}, nil
}

func validateManifest(m issueReportManifest) error {
	if strings.TrimSpace(m.Source) == "" {
		return errors.New("missing source")
	}
	if strings.TrimSpace(m.Fingerprint) == "" {
		return errors.New("missing fingerprint")
	}
	if strings.TrimSpace(m.ExternalID) == "" {
		return errors.New("missing external_id")
	}
	if strings.TrimSpace(m.Title) == "" {
		return errors.New("missing title")
	}
	if len(m.Title) > 256 {
		return errors.New("title exceeds GitHub's 256-byte limit")
	}
	return nil
}

func (c *githubClient) report(ctx context.Context, manifest issueReportManifest, artifacts []artifactMeta) (issueReportResponse, bool, error) {
	var out issueReportResponse
	existing, found, err := c.findIssue(ctx, manifest.Fingerprint, manifest.ExternalID)
	if err != nil {
		return out, false, err
	}
	if found {
		if strings.EqualFold(existing.State, "closed") && !strings.EqualFold(existing.StateReason, "not_planned") {
			fixedRevision, stale, gateErr := c.staleRecurrence(ctx, existing.Number, manifest.ObservedRevision)
			if gateErr == nil && stale {
				if err := c.ensureStaleOccurrenceComment(ctx, existing.Number, manifest, artifacts, fixedRevision); err != nil {
					return out, false, err
				}
				result := reportResponse(existing, manifest.ExternalID, true)
				result.Automation.Warning = fmt.Sprintf("stale occurrence on revision %s predates resolution baseline %s; issue left closed", manifest.ObservedRevision, fixedRevision)
				return result, false, nil
			}
			// If the closure baseline or ancestry lookup fails, preserve the old
			// fail-open behavior: a possible real regression is safer to reopen.
			if err := c.reopenIssue(ctx, existing.Number); err != nil {
				return out, false, err
			}
			if err := c.ensureOccurrenceComment(ctx, existing.Number, manifest, artifacts); err != nil {
				return out, false, err
			}
			existing.State = "open"
			existing.StateReason = "reopened"
		}
		if err := c.attachPortableRepro(ctx, &existing, manifest, artifacts); err != nil {
			return out, false, err
		}
		return reportResponse(existing, manifest.ExternalID, true), false, nil
	}

	body := renderIssueBody(c.runBase, manifest, artifacts)
	payload := map[string]any{"title": manifest.Title, "body": body}
	var created githubIssue
	if err := c.doJSON(ctx, http.MethodPost, c.repoPath("issues"), payload, &created); err != nil {
		return out, false, err
	}
	if err := c.attachPortableRepro(ctx, &created, manifest, artifacts); err != nil {
		return out, false, err
	}
	return reportResponse(created, manifest.ExternalID, false), true, nil
}

func reportResponse(issue githubIssue, externalID string, deduplicated bool) issueReportResponse {
	var out issueReportResponse
	out.Issue.ID = strconv.FormatInt(issue.Number, 10)
	out.Issue.IssueNumber = issue.Number
	out.Issue.Status = githubStatus(issue)
	out.Occurrence.ID = externalID
	out.Occurrence.ExternalID = externalID
	out.Deduplicated = deduplicated
	out.Automation.Status = "captured"
	return out
}

func (c *githubClient) findIssue(ctx context.Context, fingerprint, externalID string) (githubIssue, bool, error) {
	fpMarker := fingerprintMarker(fingerprint)
	extMarker := externalIDMarker(externalID)
	for page := 1; ; page++ {
		path := c.repoPath("issues") + "?state=all&sort=created&direction=desc&per_page=100&page=" + strconv.Itoa(page)
		var issues []githubIssue
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &issues); err != nil {
			return githubIssue{}, false, err
		}
		for _, issue := range issues {
			if len(issue.PullRequest) != 0 && string(issue.PullRequest) != "null" {
				continue
			}
			if strings.Contains(issue.Body, extMarker) || strings.Contains(issue.Body, fpMarker) {
				return issue, true, nil
			}
		}
		if len(issues) < 100 {
			return githubIssue{}, false, nil
		}
		if page >= 50 {
			return githubIssue{}, false, errors.New("GitHub issue scan exceeded 5000 entries")
		}
	}
}

func (c *githubClient) reopenIssue(ctx context.Context, number int64) error {
	payload := map[string]string{"state": "open"}
	return c.doJSON(ctx, http.MethodPatch, c.repoPath("issues", strconv.FormatInt(number, 10)), payload, nil)
}

func (c *githubClient) ensureOccurrenceComment(ctx context.Context, number int64, manifest issueReportManifest, artifacts []artifactMeta) error {
	marker := externalIDMarker(manifest.ExternalID)
	path := c.repoPath("issues", strconv.FormatInt(number, 10), "comments")
	for page := 1; ; page++ {
		var comments []githubComment
		if err := c.doJSON(ctx, http.MethodGet, path+"?per_page=100&page="+strconv.Itoa(page), nil, &comments); err != nil {
			return err
		}
		for _, comment := range comments {
			if strings.Contains(comment.Body, marker) {
				return nil
			}
		}
		if len(comments) < 100 {
			break
		}
	}
	payload := map[string]string{"body": renderOccurrenceComment(c.runBase, manifest, artifacts)}
	return c.doJSON(ctx, http.MethodPost, path, payload, nil)
}

func (c *githubClient) getIssueStatus(ctx context.Context, id string) (issueStatusResponse, error) {
	var out issueStatusResponse
	n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if err != nil || n <= 0 {
		return out, &githubHTTPError{Status: http.StatusNotFound, Message: "unknown GitHub issue id"}
	}
	var issue githubIssue
	if err := c.doJSON(ctx, http.MethodGet, c.repoPath("issues", strconv.FormatInt(n, 10)), nil, &issue); err != nil {
		return out, err
	}
	out.ID = strconv.FormatInt(issue.Number, 10)
	out.IssueNumber = issue.Number
	out.Status = githubStatus(issue)
	out.OccurrenceCount = 1
	if strings.EqualFold(issue.State, "closed") {
		if strings.EqualFold(issue.StateReason, "not_planned") {
			out.Resolution = "not_planned"
		} else {
			out.Resolution = "fixed"
			// Publish the same frozen baseline PokéWall uses for regression
			// diagnostics. Failure to resolve it must not break lifecycle sync.
			if fixedRevision, err := c.fixedRevisionAtClose(ctx, issue.Number); err == nil {
				out.FixedRevision = fixedRevision
			}
		}
	}
	return out, nil
}

func (c *githubClient) investigate(ctx context.Context, id string) error {
	n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if err != nil || n <= 0 {
		return &githubHTTPError{Status: http.StatusNotFound, Message: "unknown GitHub issue id"}
	}
	path := c.repoPath("issues", strconv.FormatInt(n, 10), "comments")
	const marker = "<!-- pokepilot-investigation-request -->"
	var comments []githubComment
	if err := c.doJSON(ctx, http.MethodGet, path+"?per_page=100", nil, &comments); err != nil {
		return err
	}
	for _, comment := range comments {
		if strings.Contains(comment.Body, marker) {
			return nil
		}
	}
	return c.doJSON(ctx, http.MethodPost, path, map[string]string{
		"body": marker + "\nInvestigation requested from the PokePilot operator console. The local qwagent triage loop may claim this failure group independently.",
	}, nil)
}

func githubStatus(issue githubIssue) string {
	if strings.EqualFold(issue.State, "closed") {
		return "resolved"
	}
	return "open"
}

func (c *githubClient) repoPath(parts ...string) string {
	repo := strings.Split(c.repo, "/")
	path := "/repos/" + url.PathEscape(repo[0]) + "/" + url.PathEscape(repo[1])
	for _, part := range parts {
		path += "/" + url.PathEscape(part)
	}
	return path
}

type githubHTTPError struct {
	Status  int
	Message string
}

func (e *githubHTTPError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("github: status %d", e.Status)
	}
	return fmt.Sprintf("github: status %d: %s", e.Status, e.Message)
}

func (c *githubClient) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiBase+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGitHubResponse))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var msg struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &msg)
		return &githubHTTPError{Status: resp.StatusCode, Message: msg.Message}
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

func renderIssueBody(runBase string, manifest issueReportManifest, artifacts []artifactMeta) string {
	var b strings.Builder
	b.WriteString(fingerprintMarker(manifest.Fingerprint))
	b.WriteByte('\n')
	b.WriteString(externalIDMarker(manifest.ExternalID))
	b.WriteString("\n<!-- pokepilot-generated:github-issues-v1 -->\n\n")
	b.WriteString("Automated PokePilot farm failure. GitHub is the issue system of record; repeated active occurrences remain grouped by fingerprint.\n\n")
	renderMetadata(&b, runBase, manifest)
	b.WriteString("\n## Summary\n\n")
	b.WriteString(markdownSafeText(truncateUTF8(manifest.Summary, maxSummaryBytes)))
	b.WriteString("\n")
	renderEvidence(&b, manifest.Evidence)
	renderReproduction(&b, runBase, manifest, artifacts)
	renderArtifacts(&b, artifacts)
	return truncateUTF8(b.String(), 62<<10)
}

func renderOccurrenceComment(runBase string, manifest issueReportManifest, artifacts []artifactMeta) string {
	var b strings.Builder
	b.WriteString(externalIDMarker(manifest.ExternalID))
	b.WriteString("\n### Farm regression recurrence\n\n")
	renderMetadata(&b, runBase, manifest)
	b.WriteString("\n")
	b.WriteString(markdownSafeText(truncateUTF8(manifest.Summary, 8<<10)))
	b.WriteString("\n")
	renderReproduction(&b, runBase, manifest, artifacts)
	renderArtifacts(&b, artifacts)
	return truncateUTF8(b.String(), 60<<10)
}

func renderMetadata(b *strings.Builder, runBase string, manifest issueReportManifest) {
	key := triageKey(manifest.Fingerprint)
	if key != "" {
		fmt.Fprintf(b, "- **Triage key:** `%s`\n", key)
	}
	fmt.Fprintf(b, "- **Fingerprint:** `%s`\n", markdownCode(manifest.Fingerprint))
	if manifest.ObservedRevision != "" {
		fmt.Fprintf(b, "- **Revision:** `%s`\n", markdownCode(manifest.ObservedRevision))
	}
	if !manifest.ObservedAt.IsZero() {
		fmt.Fprintf(b, "- **Observed:** %s\n", manifest.ObservedAt.UTC().Format(time.RFC3339))
	}
	if manifest.Severity != "" {
		fmt.Fprintf(b, "- **Severity:** `%s`\n", markdownCode(manifest.Severity))
	}
	if runID := evidenceString(manifest.Evidence, "run_id"); runID != "" {
		if runBase != "" {
			fmt.Fprintf(b, "- **Run:** [`%s`](%s/v1/runs/%s/debug)\n", markdownCode(runID), runBase, url.PathEscape(runID))
		} else {
			fmt.Fprintf(b, "- **Run:** `%s`\n", markdownCode(runID))
		}
	}
}

func renderEvidence(b *strings.Builder, raw json.RawMessage) {
	var values map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return
	}
	keys := []string{
		"classification", "disposition", "objective", "error", "cause", "outcome",
		"attempt", "seed", "seed_burn", "map", "x", "y", "occurrences_in_run",
		"first_round", "last_round", "recovered", "recovered_count", "terminal_count",
		"blocking", "run_reason", "runner_version", "observed_revision",
	}
	var rows [][2]string
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		var text string
		switch v := value.(type) {
		case string:
			text = v
		case float64, bool:
			text = fmt.Sprint(v)
		default:
			continue
		}
		text = strings.TrimSpace(truncateUTF8(text, maxEvidenceValueBytes))
		if text != "" {
			rows = append(rows, [2]string{key, text})
		}
	}
	if len(rows) == 0 {
		return
	}
	b.WriteString("\n## Evidence\n\n| Field | Value |\n| --- | --- |\n")
	for _, row := range rows {
		fmt.Fprintf(b, "| `%s` | %s |\n", row[0], markdownTable(row[1]))
	}
}

func renderReproduction(b *strings.Builder, runBase string, manifest issueReportManifest, artifacts []artifactMeta) {
	checkpoint, ok := findReproCheckpoint(artifacts)
	if !ok {
		return
	}
	runID := evidenceString(manifest.Evidence, "run_id")
	if runID == "" {
		return
	}
	attempt := evidenceInt(manifest.Evidence, "attempt")

	b.WriteString("\n## Reproduce\n\n")
	fmt.Fprintf(b, "- **Checkpoint:** `%s`\n", markdownCode(checkpoint.State.Name))
	fmt.Fprintf(b, "- **Knowledge:** `%s`\n", markdownCode(checkpoint.Knowledge.Name))
	fmt.Fprintf(b, "- **State SHA-256:** `%s`\n", checkpoint.State.SHA256)
	fmt.Fprintf(b, "- **Knowledge SHA-256:** `%s`\n", checkpoint.Knowledge.SHA256)
	if runBase == "" {
		b.WriteString("\nThe exact checkpoint pair is retained in the private PokePilot run store. Set `POKEPILOT_RUN_BASE_URL` on `pokeissues` to emit a ready-to-run fetch command.\n")
		return
	}

	b.WriteString("\nThe state bytes stay private. `pokerepro` fetches this exact checkpoint and its paired knowledge through the authenticated Run Inspector API.\n\n")
	b.WriteString("```bash\n")
	fmt.Fprintf(b, "go run ./cmd/pokerepro -wall %s -run %s", shellArg(runBase), shellArg(runID))
	if attempt > 0 {
		fmt.Fprintf(b, " -attempt %d", attempt)
	}
	fmt.Fprintf(b, " -checkpoint %s -play\n", shellArg(checkpoint.State.Name))
	b.WriteString("```\n")
}

func findReproCheckpoint(artifacts []artifactMeta) (reproCheckpoint, bool) {
	byName := make(map[string]artifactMeta, len(artifacts))
	for _, artifact := range artifacts {
		byName[artifact.Name] = artifact
	}

	var best reproCheckpoint
	for _, state := range artifacts {
		if !strings.HasPrefix(state.Name, "round-") || !strings.HasSuffix(state.Name, ".state") {
			continue
		}
		stem := strings.TrimSuffix(state.Name, ".state")
		var knowledge artifactMeta
		found := false
		for name, candidate := range byName {
			if !strings.HasPrefix(name, stem+".knowledge-v") || !strings.HasSuffix(name, ".json") {
				continue
			}
			if !found || name > knowledge.Name {
				knowledge = candidate
				found = true
			}
		}
		if !found {
			continue
		}
		if best.State.Name == "" || state.Name > best.State.Name {
			best = reproCheckpoint{State: state, Knowledge: knowledge}
		}
	}
	return best, best.State.Name != ""
}

func renderArtifacts(b *strings.Builder, artifacts []artifactMeta) {
	if len(artifacts) == 0 {
		return
	}
	b.WriteString("\n## Captured artifacts\n\nBinary evidence stays in the PokePilot run store; GitHub receives metadata only.\n\n")
	b.WriteString("| Name | Media type | Bytes | SHA-256 |\n| --- | --- | ---: | --- |\n")
	for _, artifact := range artifacts {
		fmt.Fprintf(b, "| `%s` | `%s` | %d | `%s` |\n",
			markdownCode(artifact.Name), markdownCode(artifact.MediaType), artifact.Size, artifact.SHA256)
	}
}

func evidenceString(raw json.RawMessage, key string) string {
	var values map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func evidenceInt(raw json.RawMessage, key string) int {
	var values map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return 0
	}
	switch value := values[key].(type) {
	case float64:
		return int(value)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(value))
		return n
	default:
		return 0
	}
}

func shellArg(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func fingerprintMarker(fingerprint string) string {
	return "<!-- pokepilot-fingerprint:" + htmlCommentSafe(fingerprint) + " -->"
}

func externalIDMarker(externalID string) string {
	return "<!-- pokepilot-external-id:" + htmlCommentSafe(externalID) + " -->"
}

func triageKey(fingerprint string) string {
	fp := strings.TrimPrefix(strings.TrimSpace(fingerprint), "sha256:")
	if len(fp) < 16 {
		return ""
	}
	return fp[:16]
}

func htmlCommentSafe(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "--", "-")
}

func markdownSafeText(s string) string {
	if strings.TrimSpace(s) == "" {
		return "_(no summary)_"
	}
	return s
}

func markdownCode(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}

func markdownTable(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", "<br>")
	return s
}

func truncateUTF8(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	for n > 0 && !utf8.ValidString(s[:n]) {
		n--
	}
	return s[:n]
}

func writeGitHubError(w http.ResponseWriter, err error) {
	var ghErr *githubHTTPError
	if errors.As(err, &ghErr) {
		status := http.StatusBadGateway
		switch ghErr.Status {
		case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity:
			status = ghErr.Status
		case http.StatusTooManyRequests:
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
