package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPMediaJobs(t *testing.T) {
	var calls []string
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.RequestURI())
		res.Write([]byte(`{"id":"job-1","state":"queued","jobs":[]}`)) //nolint:errcheck
	}))
	defer wall.Close()
	control := &mcpControl{wallBase: wall.URL, http: wall.Client()}
	ctx := context.Background()

	if _, _, err := control.listMediaJobs(ctx, nil, mcpMediaJobsInput{RunID: "run-1", State: "failed"}); err != nil {
		t.Fatal(err)
	}
	for _, call := range []func() error{
		func() error { _, _, err := control.getMediaJob(ctx, nil, mcpMediaJobInput{ID: "job-1"}); return err },
		func() error { _, _, err := control.retryMediaJob(ctx, nil, mcpMediaJobInput{ID: "job-1"}); return err },
		func() error { _, _, err := control.cancelMediaJob(ctx, nil, mcpMediaJobInput{ID: "job-1"}); return err },
	} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"GET /v1/media/render-jobs?limit=20&run_id=run-1&state=failed",
		"GET /v1/media/render-jobs/job-1",
		"POST /v1/media/render-jobs/job-1/retry",
		"POST /v1/media/render-jobs/job-1/cancel",
	}
	for i := range want {
		if i >= len(calls) || calls[i] != want[i] {
			t.Fatalf("calls = %q, want %q", calls, want)
		}
	}
	if _, _, err := control.retryMediaJob(ctx, nil, mcpMediaJobInput{}); err == nil {
		t.Fatal("retry without id should fail")
	}
}
