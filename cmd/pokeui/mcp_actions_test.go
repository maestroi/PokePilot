package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPRunActions(t *testing.T) {
	var posts []string
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/runs/run-endless":
			res.Write([]byte(`{"run":{"run_id":"run-endless","endless":true}}`)) //nolint:errcheck
		case req.Method == http.MethodGet:
			res.Write([]byte(`{"run":{"run_id":"run-1"}}`)) //nolint:errcheck
		default:
			body, _ := io.ReadAll(req.Body)
			posts = append(posts, req.URL.Path+" "+string(body))
			json.NewEncoder(res).Encode(map[string]string{"run_id": "new", "status": "queued"}) //nolint:errcheck
		}
	}))
	defer wall.Close()
	control := &mcpControl{wallBase: wall.URL, http: wall.Client()}
	ctx := context.Background()

	if _, out, err := control.cloneRun(ctx, nil, mcpRunInput{RunID: "run-1"}); err != nil || out["run_id"] != "new" {
		t.Fatalf("clone = %v, %v", out, err)
	}
	if _, _, err := control.cloneRun(ctx, nil, mcpRunInput{RunID: "run-endless"}); err == nil {
		t.Fatal("cloning an endless run must be refused")
	}
	if _, _, err := control.queueRepro(ctx, nil, mcpReproInput{RunID: "run-1", Attempt: 2, Checkpoint: "r40"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := control.flagStuck(ctx, nil, mcpFlagStuckInput{RunID: "run-1", Note: "loops in Mt Moon"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/v1/runs/run-1/clone ",
		`/v1/runs/run-1/repro {"checkpoint":"r40","attempt":2}`,
		`/v1/runs/run-1/flag-stuck {"note":"loops in Mt Moon"}`,
	}
	if len(posts) != len(want) {
		t.Fatalf("posts = %q, want %q", posts, want)
	}
	for i := range want {
		if posts[i] != want[i] {
			t.Fatalf("post %d = %q, want %q", i, posts[i], want[i])
		}
	}
}
