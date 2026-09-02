package farm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientLeaseReturnsSpec(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/lease" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(Spec{RunID: "r1", Planner: "scripted"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	spec, err := c.Lease(context.Background())
	if err != nil {
		t.Fatalf("Lease: %v", err)
	}
	if spec == nil || spec.RunID != "r1" {
		t.Fatalf("Lease = %+v, want RunID r1", spec)
	}
}

func TestClientLeaseNoneReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	spec, err := c.Lease(context.Background())
	if err != nil {
		t.Fatalf("Lease: %v", err)
	}
	if spec != nil {
		t.Fatalf("Lease = %+v, want nil (none ready)", spec)
	}
}

func TestClientHeartbeatReturnsCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/r1/heartbeat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(HeartbeatReply{Cancel: true})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	reply, err := c.Heartbeat(context.Background(), Heartbeat{RunID: "r1"})
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if !reply.Cancel {
		t.Fatalf("Heartbeat reply = %+v, want Cancel true", reply)
	}
}

func TestClientFinishPostsReport(t *testing.T) {
	var got FinishReport
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/r1/finish" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.Finish(context.Background(), FinishReport{RunID: "r1", Reason: "done"}); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got.RunID != "r1" || got.Reason != "done" {
		t.Fatalf("server received %+v", got)
	}
}

func TestClientFinishRejectsInvalidArtifacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("Finish must not send an invalid report")
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.Finish(context.Background(), FinishReport{
		RunID:    "r1",
		SeedBurn: -1,
	})
	if err == nil {
		t.Fatal("Finish accepted negative seed burn")
	}
}

func TestClientCheckpointPostsArtifacts(t *testing.T) {
	data := []byte("periodic-state")
	sum := sha256.Sum256(data)
	want := CheckpointReport{
		RunID:   "r1",
		Attempt: 1,
		Artifacts: []Artifact{{
			Name:      "periodic-00000018000.state",
			MediaType: "application/octet-stream",
			SHA256:    hex.EncodeToString(sum[:]),
			Data:      data,
		}},
	}
	var got CheckpointReport
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs/r1/checkpoint" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.Checkpoint(context.Background(), want); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if got.RunID != "r1" || got.Attempt != 1 || len(got.Artifacts) != 1 || got.Artifacts[0].Name != want.Artifacts[0].Name {
		t.Fatalf("server received %+v", got)
	}
}

func TestClientCheckpointRejectsInvalidArtifacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("Checkpoint must not send an invalid report")
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.Checkpoint(context.Background(), CheckpointReport{
		RunID: "r1",
		Artifacts: []Artifact{{
			Name:      "../evil.state",
			MediaType: "application/octet-stream",
			SHA256:    strings.Repeat("0", 64),
			Data:      []byte("x"),
		}},
	})
	if err == nil {
		t.Fatal("Checkpoint accepted an unsafe name")
	}
}

func TestClientPingSendsVersion(t *testing.T) {
	var got WorkerPing
	srv := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		json.NewDecoder(req.Body).Decode(&got) //nolint:errcheck
		res.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.Version = "abc123"
	if err := c.Ping(context.Background(), []string{"10.0.1.5:8099"}); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if got.Version != "abc123" {
		t.Fatalf("ping version = %q, want abc123", got.Version)
	}
}

func TestClientHeartbeatSendsVersion(t *testing.T) {
	var got Heartbeat
	srv := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		json.NewDecoder(req.Body).Decode(&got)        //nolint:errcheck
		json.NewEncoder(res).Encode(HeartbeatReply{}) //nolint:errcheck
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.Version = "abc123"
	if _, err := c.Heartbeat(context.Background(), Heartbeat{RunID: "r1"}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if got.Version != "abc123" {
		t.Fatalf("heartbeat version = %q, want abc123", got.Version)
	}
}

func TestNewClientNormalizesBaseURLAndSetsTimeout(t *testing.T) {
	c := NewClient("http://example.test///")
	if c.BaseURL != "http://example.test" {
		t.Fatalf("BaseURL = %q, want normalized URL", c.BaseURL)
	}
	if c.HTTP == nil || c.HTTP.Timeout != defaultHTTPTimeout {
		t.Fatalf("HTTP timeout = %v, want %v", c.HTTP.Timeout, defaultHTTPTimeout)
	}
}

func TestClientEscapesRunIDPathSegment(t *testing.T) {
	const runID = "run/with space?%"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/v1/runs/run%2Fwith%20space%3F%25/heartbeat"
		if got := r.URL.EscapedPath(); got != want {
			t.Fatalf("escaped path = %q, want %q", got, want)
		}
		json.NewEncoder(w).Encode(HeartbeatReply{}) //nolint:errcheck
	}))
	defer srv.Close()

	c := NewClient(srv.URL + "/")
	if _, err := c.Heartbeat(context.Background(), Heartbeat{RunID: runID}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
}

func TestClientStatusErrorIncludesResponseDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "worker rejected", http.StatusConflict)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.Ping(context.Background(), nil)
	if err == nil {
		t.Fatal("Ping succeeded, want status error")
	}
	if got := err.Error(); !strings.Contains(got, "status 409") || !strings.Contains(got, "worker rejected") {
		t.Fatalf("Ping error = %q, want status and response detail", got)
	}
}
