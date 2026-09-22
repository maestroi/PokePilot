package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mcpToolServer(t *testing.T, wantName string, structured map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if err := json.Unmarshal(raw, &req); err != nil || req.Params.Name != wantName {
			http.Error(w, "unexpected tool", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]any{"structuredContent": structured},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPickCLI(t *testing.T) {
	in := strings.NewReader(`[
	  {"key":"cafef00d","count":4,"example":"claimed","run_ids":["r1"],"issue":{"status":"open"}},
	  {"key":"0badf00d","count":3,"example":"free","run_ids":["r2"],"issue":{"issue_number":432,"status":"open"}}
	]`)
	var out bytes.Buffer
	err := run([]string{"pick", "--claimed", "fix(farm): x [triage:cafef00d]"}, in, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"key": "0badf00d"`) && !strings.Contains(out.String(), `"key":"0badf00d"`) {
		t.Fatalf("output = %s", out.String())
	}
	if !strings.Contains(out.String(), `"issue_number": 432`) && !strings.Contains(out.String(), `"issue_number":432`) {
		t.Fatalf("output did not preserve linked issue number: %s", out.String())
	}
}

func TestPickCLILocalRepairAndRegression(t *testing.T) {
	in := strings.NewReader(`[
	  {"key":"fixed","count":9,"example":"old","run_ids":["old"]},
	  {"key":"regressed","count":4,"example":"again","run_ids":["new"],"issue":{"status":"resolved","resolution":"fixed"}}
	]`)
	var out bytes.Buffer
	err := run([]string{"pick", "--repaired", "fixed", "--regressed", "regressed"}, in, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"key": "regressed"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestPickCLINothingFree(t *testing.T) {
	in := strings.NewReader(`[
	  {"key":"cafef00d","count":4,"example":"claimed","run_ids":["r1"],"issue":{"status":"resolved","resolution":"fixed"}}
	]`)
	err := run([]string{"pick"}, in, &bytes.Buffer{})
	if !errors.Is(err, errNothing) {
		t.Fatalf("err = %v, want errNothing", err)
	}
}

func TestPickCLIRejectsAccessHTML(t *testing.T) {
	err := run([]string{"pick"}, strings.NewReader("<html><title>302 Found</title></html>"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("Access HTML must not pick")
	}
}

func TestRecordAttemptCLIUsesMCP(t *testing.T) {
	srv := mcpToolServer(t, "pokepilot_record_solver_attempt", map[string]any{"attempt_count": 1})
	var out bytes.Buffer
	err := run([]string{
		"record-attempt",
		"--endpoint", srv.URL + "/mcp",
		"--token", "secret",
		"--key", "deadbeef",
		"--id", "attempt-1",
		"--backend", "opencode",
		"--model", "qwen3.8-27b/qwen3.8-27b",
		"--state", "started",
		"--run-id", "run-1",
	}, strings.NewReader(""), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "attempt_count") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestFetchTriageCLIUsesMCP(t *testing.T) {
	srv := mcpToolServer(t, "pokepilot_get_triage", map[string]any{
		"groups": []map[string]any{{"key": "abc", "count": 2, "run_ids": []string{"r1"}}},
	})
	var out bytes.Buffer
	if err := run([]string{"fetch-triage", "--endpoint", srv.URL + "/mcp", "--token", "secret"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"key":"abc"`) && !strings.Contains(out.String(), `"key": "abc"`) {
		t.Fatalf("output = %s", out.String())
	}
}
