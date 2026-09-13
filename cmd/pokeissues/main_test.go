package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeGitHub struct {
	mu              sync.Mutex
	issues          []githubIssue
	comments        map[int64][]githubComment
	created         int
	patched         int
	commented       int
	lastCreateTitle string
	lastCreateBody  string
	auth             []string
}

func newFakeGitHub() *fakeGitHub {
	return &fakeGitHub{comments: map[int64][]githubComment{}}
}

func (f *fakeGitHub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(f.issues)
	})
	mux.HandleFunc("POST /repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			testHTTPError(w, http.StatusBadRequest, err.Error())
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		f.created++
		f.lastCreateTitle = payload.Title
		f.lastCreateBody = payload.Body
		issue := githubIssue{Number: int64(40 + f.created), State: "open", Title: payload.Title, Body: payload.Body, HTMLURL: "https://github.test/o/r/issues/41"}
		f.issues = append([]githubIssue{issue}, f.issues...)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(issue)
	})
	mux.HandleFunc("GET /repos/o/r/issues/{number}", func(w http.ResponseWriter, r *http.Request) {
		n := mustNumber(r.PathValue("number"))
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, issue := range f.issues {
			if issue.Number == n {
				_ = json.NewEncoder(w).Encode(issue)
				return
			}
		}
		testHTTPError(w, http.StatusNotFound, "not found")
	})
	mux.HandleFunc("PATCH /repos/o/r/issues/{number}", func(w http.ResponseWriter, r *http.Request) {
		n := mustNumber(r.PathValue("number"))
		f.mu.Lock()
		defer f.mu.Unlock()
		for i := range f.issues {
			if f.issues[i].Number == n {
				f.issues[i].State = "open"
				f.issues[i].StateReason = "reopened"
				f.patched++
				_ = json.NewEncoder(w).Encode(f.issues[i])
				return
			}
		}
		testHTTPError(w, http.StatusNotFound, "not found")
	})
	mux.HandleFunc("GET /repos/o/r/issues/{number}/comments", func(w http.ResponseWriter, r *http.Request) {
		n := mustNumber(r.PathValue("number"))
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.comments[n])
	})
	mux.HandleFunc("POST /repos/o/r/issues/{number}/comments", func(w http.ResponseWriter, r *http.Request) {
		n := mustNumber(r.PathValue("number"))
		var payload githubComment
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			testHTTPError(w, http.StatusBadRequest, err.Error())
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.comments[n] = append(f.comments[n], payload)
		f.commented++
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(payload)
	})
	return mux
}

func mustNumber(raw string) int64 {
	var n int64
	if _, err := fmt.Sscan(raw, &n); err != nil {
		panic(err)
	}
	return n
}

func testHTTPError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
}

func newTestServer(t *testing.T, fake *fakeGitHub) (*httptest.Server, *httptest.Server) {
	t.Helper()
	gh := httptest.NewServer(fake.handler())
	client, err := newGitHubClient(gh.URL, "https://github.test", "o/r", "secret", "https://pokemon.test", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	issues := httptest.NewServer((&issueServer{github: client}).handler())
	t.Cleanup(issues.Close)
	t.Cleanup(gh.Close)
	return issues, gh
}

func reportRequest(t *testing.T, endpoint string, manifest issueReportManifest, artifactName string, artifact []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="report"`)
	h.Set("Content-Type", "application/json")
	part, err := mw.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(part).Encode(manifest); err != nil {
		t.Fatal(err)
	}
	if artifactName != "" {
		h = make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="artifact"; filename="`+artifactName+`"`)
		h.Set("Content-Type", "application/octet-stream")
		part, err = mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(artifact); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint+"/api/projects/pokepilot/issue-reports", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func sampleManifest(externalID string) issueReportManifest {
	return issueReportManifest{
		Source:           "pokefarm",
		Fingerprint:      "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ExternalID:       externalID,
		Title:            "[farm] objective failure: GoTo: no path",
		Summary:          "GoTo failed while advancing story progression.",
		ObservedAt:       time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC),
		ObservedRevision: "abc123",
		Severity:         "critical",
		Evidence: json.RawMessage(`{
			"run_id":"run-42","attempt":3,"classification":"progression-blocker",
			"objective":"GoTo Cerulean","error":"no path","map":"0x03","x":1,"y":2,
			"trace_tail":["private noisy trace that should not be copied"]
		}`),
	}
}

func TestReportCreatesGitHubIssueWithoutArtifactBytes(t *testing.T) {
	fake := newFakeGitHub()
	issues, _ := newTestServer(t, fake)
	resp := reportRequest(t, issues.URL, sampleManifest("run-42-attempt-3-objective-key"), "round-003.state", []byte("secret-state-bytes"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var got issueReportResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Issue.IssueNumber != 41 || got.Issue.ID != "41" || got.Deduplicated {
		t.Fatalf("response = %+v", got)
	}
	fake.mu.Lock()
	body := fake.lastCreateBody
	title := fake.lastCreateTitle
	auth := append([]string(nil), fake.auth...)
	fake.mu.Unlock()
	if title != sampleManifest("x").Title {
		t.Fatalf("title=%q", title)
	}
	for _, want := range []string{
		"pokepilot-fingerprint:sha256:0123456789abcdef",
		"pokepilot-external-id:run-42-attempt-3-objective-key",
		"Triage key:** `0123456789abcdef`",
		"https://pokemon.test/v1/runs/run-42/debug",
		"`round-003.state`",
		"progression-blocker",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("issue body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "secret-state-bytes") || strings.Contains(body, "private noisy trace") {
		t.Fatalf("issue body leaked binary/private trace:\n%s", body)
	}
	if len(auth) == 0 || auth[0] != "Bearer secret" {
		t.Fatalf("authorization headers = %v", auth)
	}
}

func TestReportDeduplicatesOpenFingerprint(t *testing.T) {
	fake := newFakeGitHub()
	m := sampleManifest("old-occurrence")
	fake.issues = []githubIssue{{Number: 7, State: "open", Body: renderIssueBody("", m, nil)}}
	issues, _ := newTestServer(t, fake)
	resp := reportRequest(t, issues.URL, sampleManifest("new-occurrence"), "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got issueReportResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Issue.IssueNumber != 7 || !got.Deduplicated {
		t.Fatalf("response=%+v", got)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.created != 0 || fake.commented != 0 || fake.patched != 0 {
		t.Fatalf("created=%d commented=%d patched=%d", fake.created, fake.commented, fake.patched)
	}
}

func TestReportReopensClosedIssueAndAddsOneIdempotentRecurrence(t *testing.T) {
	fake := newFakeGitHub()
	old := sampleManifest("old-occurrence")
	fake.issues = []githubIssue{{Number: 9, State: "closed", StateReason: "completed", Body: renderIssueBody("", old, nil)}}
	issues, _ := newTestServer(t, fake)
	manifest := sampleManifest("regression-occurrence")
	for i := 0; i < 2; i++ {
		resp := reportRequest(t, issues.URL, manifest, "final.state", []byte("state"))
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("try %d status=%d body=%s", i, resp.StatusCode, body)
		}
		resp.Body.Close()
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.patched != 1 || fake.commented != 1 || fake.created != 0 {
		t.Fatalf("patched=%d commented=%d created=%d", fake.patched, fake.commented, fake.created)
	}
	if len(fake.comments[9]) != 1 || !strings.Contains(fake.comments[9][0].Body, "regression-occurrence") {
		t.Fatalf("comments=%+v", fake.comments[9])
	}
}

func TestStatusMapsGitHubClosedReasons(t *testing.T) {
	for _, tc := range []struct {
		reason     string
		resolution string
	}{
		{reason: "completed", resolution: "fixed"},
		{reason: "not_planned", resolution: "not_planned"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			fake := newFakeGitHub()
			fake.issues = []githubIssue{{Number: 12, State: "closed", StateReason: tc.reason}}
			issues, _ := newTestServer(t, fake)
			resp, err := http.Get(issues.URL + "/api/issues/12")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var got issueStatusResponse
			if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.Status != "resolved" || got.Resolution != tc.resolution || got.IssueNumber != 12 {
				t.Fatalf("status=%+v", got)
			}
		})
	}
}

func TestInvestigateAddsSingleComment(t *testing.T) {
	fake := newFakeGitHub()
	fake.issues = []githubIssue{{Number: 22, State: "open"}}
	issues, _ := newTestServer(t, fake)
	for i := 0; i < 2; i++ {
		resp, err := http.Post(issues.URL+"/api/issues/22/investigate", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status=%d", resp.StatusCode)
		}
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.commented != 1 || len(fake.comments[22]) != 1 {
		t.Fatalf("commented=%d comments=%+v", fake.commented, fake.comments[22])
	}
}
