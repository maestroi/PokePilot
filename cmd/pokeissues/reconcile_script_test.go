package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconcileFarmIssuesScriptClosesOnlyPreRepairObservation(t *testing.T) {
	for _, bin := range []string{"bash", "git", "jq"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is required: %v", bin, err)
		}
	}

	script, err := filepath.Abs(filepath.Join("..", "..", ".github", "scripts", "reconcile-farm-issues.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bash", "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}

	repo := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "test")

	writeCommit := func(value, message string) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, "state.txt"), []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("git", "add", "state.txt")
		run("git", "commit", "-m", message)
		return run("git", "rev-parse", "HEAD")
	}

	observed := writeCommit("observed", "observed failure")
	repair := writeCommit("repair", "merged repair")
	postFix := writeCommit("post-fix", "post-fix recurrence")

	generated := "<!-- pokepilot-generated:github-issues-v1 -->"
	body := func(key, latest string) string {
		return "<!-- pokepilot-latest-observed-revision:" + latest + " -->\n" +
			generated + "\n\n- **Triage key:** \u0060" + key + "\u0060\n- **Revision:** \u0060" + latest + "\u0060\n"
	}
	issuesRaw, err := json.Marshal([]map[string]any{
		{"number": 1, "pull_request": nil, "body": body("fixedkey", observed)},
		{"number": 2, "pull_request": nil, "body": body("fixedkey", postFix)},
		{"number": 3, "pull_request": nil, "body": body("nofix", observed)},
	})
	if err != nil {
		t.Fatal(err)
	}
	issuesPath := filepath.Join(repo, "issues.json")
	if err := os.WriteFile(issuesPath, issuesRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	pullsRaw, err := json.Marshal([]map[string]any{{
		"number": 44, "merged_at": "2026-09-19T20:00:00Z", "merge_commit_sha": repair,
		"title": "fix [triage:fixedkey]", "body": "",
	}})
	if err != nil {
		t.Fatal(err)
	}
	pullsPath := filepath.Join(repo, "pulls.json")
	if err := os.WriteFile(pullsPath, pullsRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(repo, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeGH := "#!/usr/bin/env bash\nset -euo pipefail\n" +
		"case \"$*\" in\n" +
		"  *\"/issues?state=open&per_page=100\"*) cat \"$GH_FAKE_ISSUES\" ;;\n" +
		"  *\"/pulls?state=closed&base=main&per_page=100\"*) cat \"$GH_FAKE_PULLS\" ;;\n" +
		"  *) echo \"unexpected gh call: $*\" >&2; exit 2 ;;\n" +
		"esac\n"
	ghPath := filepath.Join(binDir, "gh")
	if err := os.WriteFile(ghPath, []byte(fakeGH), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", script)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"REPO=o/r",
		"DRY_RUN=1",
		"GH_FAKE_ISSUES="+issuesPath,
		"GH_FAKE_PULLS="+pullsPath,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("reconciler: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{
		"Would close #1:",
		"Keep #2: no merged [triage:fixedkey] repair contains latest observation",
		"Keep #3: no merged [triage:nofix] repair contains latest observation",
		"closed=1 kept=2 skipped=0 dry_run=1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}
