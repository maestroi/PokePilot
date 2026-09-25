package deploy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallMCPToolRejectsAccessRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Header().Set("Location", "https://beardsoft.cloudflareaccess.com/cdn-cgi/access/login")
		w.WriteHeader(http.StatusFound)
		io.WriteString(w, "<html>\r\n<head><title>302 Found</title></head>\r\n")
	}))
	t.Cleanup(srv.Close)

	_, err := CallMCPTool(srv.Client(), srv.URL+"/mcp", "token", "pokepilot_get_triage", map[string]any{
		"include_resolved": true,
	})
	if err == nil {
		t.Fatal("Access 302 HTML must not be treated as an MCP result")
	}
	if !strings.Contains(err.Error(), "302") && !strings.Contains(err.Error(), "HTML") && !strings.Contains(err.Error(), "not JSON") {
		t.Fatalf("error = %v, want a status/HTML/JSON signal", err)
	}
}

func TestCallMCPToolReturnsStructuredContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Method != "tools/call" || req.Params.Name != "pokepilot_get_triage" {
			http.Error(w, "unexpected call", http.StatusBadRequest)
			return
		}
		if req.Params.Arguments["include_resolved"] != true {
			http.Error(w, "want include_resolved", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"structuredContent": map[string]any{
					"groups":          []map[string]any{{"key": "abc", "count": 2, "run_ids": []string{"r1"}}},
					"resolved_hidden": 4,
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	raw, err := CallMCPTool(srv.Client(), srv.URL+"/mcp", "secret", "pokepilot_get_triage", map[string]any{
		"include_resolved": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	groups, err := DecodeTriageGroups(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Key != "abc" || groups[0].RunID() != "r1" {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestCallMCPToolSurfacesRPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"error":   map[string]any{"code": -32601, "message": "unknown tool"},
		})
	}))
	t.Cleanup(srv.Close)

	_, err := CallMCPTool(srv.Client(), srv.URL+"/mcp", "secret", "pokepilot_get_triage", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("err = %v, want unknown tool", err)
	}
}
