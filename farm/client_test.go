package farm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
