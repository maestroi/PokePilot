package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTriageGroupActionable(t *testing.T) {
	tests := []struct {
		name      string
		issue     map[string]any
		want      bool
		wantState string
		wantFixed string
	}{
		{name: "unlinked", want: true},
		{name: "open", issue: map[string]any{"status": "open"}, want: true},
		{name: "investigating", issue: map[string]any{"status": "investigating"}, want: true},
		{name: "fixed resolution", issue: map[string]any{"status": "resolved", "resolution": "fixed"}, want: false, wantState: "fixed"},
		{name: "fixed revision", issue: map[string]any{"status": "resolved", "resolution": "fixed", "fixed_revision": "abc123"}, want: false, wantState: "fixed", wantFixed: "abc123"},
		{name: "closed", issue: map[string]any{"status": "closed"}, want: false, wantState: "closed"},
		// A new occurrence may reopen an issue before a stale resolution field
		// is cleared. Active status must win or a regression would stay hidden.
		{name: "reopened wins over old resolution", issue: map[string]any{"status": "reopened", "resolution": "fixed"}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			group := map[string]any{"key": "deadbeef"}
			if tc.issue != nil {
				group["issue"] = tc.issue
			}
			if got := triageGroupActionable(group); got != tc.want {
				t.Fatalf("actionable = %v, want %v; group=%v", got, tc.want, group)
			}
			if got, _ := group["actionable"].(bool); got != tc.want {
				t.Errorf("annotation actionable = %v, want %v", got, tc.want)
			}
			if got, _ := group["resolution_state"].(string); got != tc.wantState {
				t.Errorf("resolution_state = %q, want %q", got, tc.wantState)
			}
			if got, _ := group["fixed_revision"].(string); got != tc.wantFixed {
				t.Errorf("fixed_revision = %q, want %q", got, tc.wantFixed)
			}
		})
	}
}

func TestMCPGetTriageHidesResolvedByDefault(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/triage" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"key": "open", "count": 3},
			{"key": "fixed", "count": 9, "issue": map[string]any{"status": "resolved", "resolution": "fixed", "fixed_revision": "abc123"}},
			{"key": "regression", "count": 1, "issue": map[string]any{"status": "reopened", "resolution": "fixed"}},
		})
	}))
	t.Cleanup(wall.Close)

	control := &mcpControl{wallBase: wall.URL, http: wall.Client()}
	_, out, err := control.getTriage(context.Background(), nil, mcpTriageInput{})
	if err != nil {
		t.Fatal(err)
	}
	groups, ok := out["groups"].([]map[string]any)
	if !ok {
		t.Fatalf("groups type = %T, want []map[string]any", out["groups"])
	}
	if len(groups) != 2 || groups[0]["key"] != "open" || groups[1]["key"] != "regression" {
		t.Fatalf("groups = %v, want open + regression", groups)
	}
	if got := out["resolved_hidden"]; got != 1 {
		t.Fatalf("resolved_hidden = %v, want 1", got)
	}

	_, all, err := control.getTriage(context.Background(), nil, mcpTriageInput{IncludeResolved: true})
	if err != nil {
		t.Fatal(err)
	}
	allGroups := all["groups"].([]map[string]any)
	if len(allGroups) != 3 {
		t.Fatalf("include_resolved groups = %d, want 3", len(allGroups))
	}
	if got, _ := allGroups[1]["actionable"].(bool); got {
		t.Fatalf("resolved group should remain annotated actionable=false: %v", allGroups[1])
	}
}
