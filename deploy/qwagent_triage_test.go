package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPickSkipsResolvedAndClaimed(t *testing.T) {
	raw := `[
	  {"key":"deadbeef","count":9,"example":"old bug","run_ids":["run-old"],
	   "issue":{"status":"resolved","resolution":"fixed"}},
	  {"key":"cafef00d","count":4,"example":"claimed bug","run_ids":["run-claimed"],
	   "issue":{"status":"open"}},
	  {"key":"0badf00d","count":3,"example":"free bug","run_ids":["run-free"],
	   "issue":{"status":"open"}}
	]`
	var groups []TriageGroup
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		t.Fatal(err)
	}
	got, ok := Pick(groups, []string{
		"fix(farm): claimed bug [triage:cafef00d]",
	})
	if !ok {
		t.Fatal("expected a pick")
	}
	if got.Key != "0badf00d" {
		t.Fatalf("key = %q, want 0badf00d", got.Key)
	}
	if got.RunID() != "run-free" {
		t.Fatalf("run = %q, want run-free", got.RunID())
	}
}

func TestPickEmptyWhenNothingFree(t *testing.T) {
	groups := []TriageGroup{
		{Key: "deadbeef", Count: 9, Issue: &TriageIssue{Status: "resolved", Resolution: "fixed"}},
		{Key: "cafef00d", Count: 4, Issue: &TriageIssue{Status: "open"}},
	}
	if _, ok := Pick(groups, []string{"fix(farm): claimed [triage:cafef00d]"}); ok {
		t.Fatal("expected no pick when every group is resolved or claimed")
	}
}

func TestPickKeepsReopenedOverStaleResolution(t *testing.T) {
	groups := []TriageGroup{
		{Key: "reopen1", Count: 2, RunIDs: []string{"run-r"}, Issue: &TriageIssue{Status: "reopened", Resolution: "fixed"}},
	}
	got, ok := Pick(groups, nil)
	if !ok {
		t.Fatal("reopened issue must stay actionable")
	}
	if got.Key != "reopen1" {
		t.Fatalf("key = %q, want reopen1", got.Key)
	}
}

func TestPromptForbidsSkillSuite(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("qwagent-triage.prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{"[triage:<key>]", "make test-short", "Do not call pokepilot_get_triage"} {
		if !strings.Contains(s, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestScriptHasDryRunAndLock(t *testing.T) {
	body, err := os.ReadFile("qwagent-triage.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"--dry-run",
		"flock -n",
		"opencode run --auto",
		"continuing locally",
		"Follow the attached farm triage packet",
		"--file \"$POKEPILOT_TRIAGE_STATE/packet.md\"",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script missing %q", want)
		}
	}
	if !strings.Contains(s, "--\n\t\"Follow the attached farm triage packet") &&
		!strings.Contains(s, "--\n\"Follow the attached farm triage packet") {
		t.Error("opencode --file is an array flag; the prompt message must come after --")
	}
	if strings.Contains(s, "investigate failed; skip") {
		t.Error("investigate must be best-effort; an already-investigating 409/502 must not skip the local agent")
	}
}
