package deploy

import (
	"encoding/json"
	"os"
	"os/exec"
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

func TestPickPrioritizesOpenCircuit(t *testing.T) {
	groups := []TriageGroup{
		{Key: "frequent", Count: 20, RunIDs: []string{"run-old"}, Issue: &TriageIssue{Status: "open"}},
		{Key: "circuit", Count: 2, RunIDs: []string{"run-blocked"}, Issue: &TriageIssue{Status: "open", CircuitOpen: true}},
	}
	got, ok := Pick(groups, nil)
	if !ok {
		t.Fatal("expected a pick")
	}
	if got.Key != "circuit" {
		t.Fatalf("key = %q, want circuit", got.Key)
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

func TestPickLocalRepairSkipsHistoricalFailureWithoutIssue(t *testing.T) {
	groups := []TriageGroup{
		{Key: "fixed1", Count: 8, RunIDs: []string{"run-old"}},
		{Key: "free1", Count: 2, RunIDs: []string{"run-new"}},
	}
	got, ok := PickWithLocalState(groups, nil, []string{"fixed1"}, nil)
	if !ok {
		t.Fatal("expected unrepaired group")
	}
	if got.Key != "free1" {
		t.Fatalf("key = %q, want free1", got.Key)
	}
}

func TestPickLocalRegressionOverridesStaleResolvedIssue(t *testing.T) {
	groups := []TriageGroup{{
		Key: "regressed1", Count: 9, RunIDs: []string{"run-post-fix"},
		Issue: &TriageIssue{Status: "resolved", Resolution: "fixed"},
	}}
	got, ok := PickWithLocalState(groups, nil, nil, []string{"regressed1"})
	if !ok {
		t.Fatal("post-fix recurrence must be actionable even when issue sync is stale")
	}
	if got.Key != "regressed1" {
		t.Fatalf("key = %q, want regressed1", got.Key)
	}
}

func TestPickOpenPRStillClaimsLocalRegression(t *testing.T) {
	groups := []TriageGroup{{Key: "regressed1", Count: 9}}
	if _, ok := PickWithLocalState(groups,
		[]string{"fix(farm): retry [triage:regressed1]"}, nil, []string{"regressed1"}); ok {
		t.Fatal("open PR must claim a regression")
	}
}

func TestPromptLoadsTriageInstructions(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("qwagent-triage.prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"[triage:<key>]",
		"[farm-issue:<issue_number>]",
		"make test-short",
		"Do not call pokepilot_get_triage",
		".claude/skills/pokefarm-triage/SKILL.md",
		"native skill tool",
		"do not second-guess queue eligibility",
		"repair_pr",
		"failing_checks",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestTriageSkillDocumentsLocalLifecycle(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", ".claude", "skills", "pokefarm-triage", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"name: pokefarm-triage",
		"Unattended qwagent triage",
		"open PR containing `[triage:<key>]`",
		"merged PR containing `[triage:<key>]`",
		"last_observed_revision",
		"fail closed",
		"whose checks have failed",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("triage skill missing %q", want)
		}
	}
}

func TestTriageScriptParsesAsBash(t *testing.T) {
	cmd := exec.Command("bash", "-n", "qwagent-triage.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
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
		"POKEPILOT_TRIAGE_AGENT",
		"cursor_authenticated",
		"agent login",
		"--approve-mcps",
		"--workspace",
		"continuing locally",
		"Follow the attached farm triage packet",
		"--file \"$POKEPILOT_TRIAGE_STATE/packet.md\"",
		"--repaired",
		"--regressed",
		"last_observed_revision",
		"classify-repairs",
		"pick-own-pr",
		"repair_pr",
		"fetch-triage",
		"POKEPILOT_MCP_URL",
		"POKEMON_RED_ROM",
		"roms/pokemon_red.gb",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script missing %q", want)
		}
	}
	if !strings.Contains(s, "-- \\\n\t\t\"Follow the attached farm triage packet") &&
		!strings.Contains(s, "--\n\t\"Follow the attached farm triage packet") &&
		!strings.Contains(s, "--\n\"Follow the attached farm triage packet") {
		t.Error("opencode --file is an array flag; the prompt message must come after --")
	}
	if strings.Contains(s, "investigate failed; skip") {
		t.Error("investigate must be best-effort; an already-investigating 409/502 must not skip the local agent")
	}
	if strings.Contains(s, "${POKEPILOT_WALL}/v1/triage") || strings.Contains(s, "$POKEPILOT_WALL/v1/triage") {
		t.Error("do not curl Access-gated /v1/triage; fetch through /mcp")
	}
}

func TestDecodeTriageGroupsRejectsAccessHTML(t *testing.T) {
	_, err := DecodeTriageGroups([]byte("<html>\r\n<head><title>302 Found</title></head>\r\n"))
	if err == nil {
		t.Fatal("Access login HTML must not decode as triage groups")
	}
}

func TestDecodeTriageGroupsAcceptsWallArray(t *testing.T) {
	groups, err := DecodeTriageGroups([]byte(`[{"key":"abc","count":2,"run_ids":["r1"]}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Key != "abc" {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestPrepareTriageTreeDiscardsDirtyCheckout(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(root, "origin")
	tree := filepath.Join(root, "triage")
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	git(origin, "init", "-b", "main")
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(origin, "tracked.go"), "from-main\n")
	write(filepath.Join(origin, "conflict.txt"), "from-main\n")
	git(origin, "add", "tracked.go", "conflict.txt")
	git(origin, "commit", "-m", "base")
	git(root, "clone", origin, tree)

	git(tree, "checkout", "-b", "fix/wip")
	git(tree, "rm", "conflict.txt")
	write(filepath.Join(tree, "tracked.go"), "from-branch\n")
	git(tree, "add", "tracked.go")
	git(tree, "commit", "-m", "wip")
	// Same shape as a timed-out attempt: a tracked edit checkout would
	// overwrite, plus an untracked path that main already tracks.
	write(filepath.Join(tree, "tracked.go"), "dirty\n")
	write(filepath.Join(tree, "conflict.txt"), "untracked\n")
	write(filepath.Join(tree, "scratch.go"), "scratch\n")

	envFile := filepath.Join(root, "empty-env")
	write(envFile, "")
	cmd := exec.Command("bash", "qwagent-triage.sh", "--prepare-tree")
	cmd.Env = append(os.Environ(),
		"POKEPILOT_ENV="+envFile,
		"POKEPILOT_ROOT="+origin,
		"POKEPILOT_TRIAGE_TREE="+tree,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare: %v\n%s", err, out)
	}

	branch := strings.TrimSpace(git(tree, "rev-parse", "--abbrev-ref", "HEAD"))
	if branch != "main" {
		t.Fatalf("branch = %q, want main", branch)
	}
	for _, name := range []string{"tracked.go", "conflict.txt"} {
		got, err := os.ReadFile(filepath.Join(tree, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "from-main\n" {
			t.Fatalf("%s = %q, want from-main", name, got)
		}
	}
	if _, err := os.Stat(filepath.Join(tree, "scratch.go")); !os.IsNotExist(err) {
		t.Fatalf("scratch.go still present: %v", err)
	}
}

func TestPrepareTriageTreeTracksUpstreamMain(t *testing.T) {
	root := t.TempDir()
	github := filepath.Join(root, "github")
	local := filepath.Join(root, "local")
	tree := filepath.Join(root, "triage")
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(github, 0o755); err != nil {
		t.Fatal(err)
	}
	git(github, "init", "-b", "main")
	write(filepath.Join(github, "tracked.go"), "stale\n")
	git(github, "add", "tracked.go")
	git(github, "commit", "-m", "stale")
	git(root, "clone", github, local)
	git(root, "clone", local, tree)

	write(filepath.Join(github, "tracked.go"), "upstream\n")
	git(github, "add", "tracked.go")
	git(github, "commit", "-m", "upstream")
	write(filepath.Join(tree, "tracked.go"), "dirty\n")

	envFile := filepath.Join(root, "empty-env")
	write(envFile, "")
	cmd := exec.Command("bash", "qwagent-triage.sh", "--prepare-tree")
	cmd.Env = append(os.Environ(),
		"POKEPILOT_ENV="+envFile,
		"POKEPILOT_ROOT="+local,
		"POKEPILOT_TRIAGE_TREE="+tree,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare: %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(tree, "tracked.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "upstream\n" {
		t.Fatalf("tracked.go = %q, want upstream", got)
	}
	origin := strings.TrimSpace(git(tree, "remote", "get-url", "origin"))
	if origin != github {
		t.Fatalf("origin = %q, want %s", origin, github)
	}
	upstream := strings.TrimSpace(git(tree, "rev-parse", "--abbrev-ref", "main@{upstream}"))
	if upstream != "origin/main" {
		t.Fatalf("main upstream = %q, want origin/main", upstream)
	}
}

func TestDecodeTriageKeepsObservedRevision(t *testing.T) {
	groups, err := DecodeTriageGroups([]byte(`[{"key":"k","issue":{"status":"resolved","last_observed_revision":"abc123"}}]`))
	if err != nil {
		t.Fatal(err)
	}
	if groups[0].Issue == nil || groups[0].Issue.LastObservedRevision != "abc123" {
		t.Fatalf("observed revision = %+v", groups[0].Issue)
	}
}

func TestClassifyRepairsUsesFingerprintRevision(t *testing.T) {
	repo := t.TempDir()
	git := testGit(t, repo)
	git("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "note"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "note")
	git("commit", "-m", "failure")
	before := strings.TrimSpace(git("rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "note"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "note")
	git("commit", "-m", "repair")
	repair := strings.TrimSpace(git("rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "note"), []byte("c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "note")
	git("commit", "-m", "later attempt")
	after := strings.TrimSpace(git("rev-parse", "HEAD"))

	repaired, regressed := ClassifyRepairs(repo, []RepairObservation{
		{Key: "stuck", MergeSHA: repair, ObservedRevision: before},
		{Key: "real", MergeSHA: repair, ObservedRevision: after},
		{Key: "missing", MergeSHA: repair, ObservedRevision: ""},
		{Key: "unknown", MergeSHA: repair, ObservedRevision: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"},
	})
	if !sameKeys(repaired, []string{"missing", "stuck", "unknown"}) {
		t.Fatalf("repaired = %v", repaired)
	}
	if !sameKeys(regressed, []string{"real"}) {
		t.Fatalf("regressed = %v", regressed)
	}
}

func TestFalseRegressionDoesNotBeatOpenIssue(t *testing.T) {
	repo := t.TempDir()
	git := testGit(t, repo)
	git("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "note"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "note")
	git("commit", "-m", "failure")
	before := strings.TrimSpace(git("rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "note"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "note")
	git("commit", "-m", "repair")
	repair := strings.TrimSpace(git("rev-parse", "HEAD"))

	groups := []TriageGroup{
		{
			Key: "stuck", Count: 4, Example: "already fixed",
			Issue: &TriageIssue{Status: "resolved", Resolution: "fixed", CircuitOpen: true, IssueNumber: 1450},
		},
		{
			Key: "open1", Count: 2, Example: "still broken", RunIDs: []string{"run-open"},
			Issue: &TriageIssue{Status: "open", CircuitOpen: true, IssueNumber: 1469},
		},
	}
	repaired, regressed := ClassifyRepairs(repo, []RepairObservation{{
		Key: "stuck", MergeSHA: repair, ObservedRevision: before,
	}})
	got, ok := PickWithLocalState(groups, nil, repaired, regressed)
	if !ok {
		t.Fatal("expected the open issue")
	}
	if got.Key != "open1" {
		t.Fatalf("key = %q, want open1 (repaired=%v regressed=%v)", got.Key, repaired, regressed)
	}
}

func TestPickOwnPRFailurePrefersOldestRedTriagePR(t *testing.T) {
	failed := []PullCheck{{Name: "ci / test", Status: "COMPLETED", Conclusion: "FAILURE"}}
	green := []PullCheck{{Name: "ci / test", Status: "COMPLETED", Conclusion: "SUCCESS"}}
	running := []PullCheck{
		{Name: "ci / test", Status: "COMPLETED", Conclusion: "FAILURE"},
		{Name: "ci / lint", Status: "IN_PROGRESS", Conclusion: ""},
	}
	prs := []OpenPullRequest{
		{Number: 10, Title: "human bug", Checks: failed},
		{Number: 20, Title: "fix(farm): done [triage:aaa]", Checks: green},
		{Number: 30, Title: "fix(farm): racing [triage:bbb]", Checks: running},
		{Number: 50, Title: "fix(farm): newer [triage:ddd]", Checks: failed},
		{Number: 40, Title: "fix(farm): older [triage:ccc]", HeadRef: "fix/older", URL: "https://example/40", Checks: failed},
	}
	got, ok := PickOwnPRFailure(prs)
	if !ok {
		t.Fatal("expected a red triage PR")
	}
	if got.Number != 40 || TriageKeyFromTitle(got.Title) != "ccc" {
		t.Fatalf("picked %+v", got)
	}
	if got.FailingChecks() != "ci / test" {
		t.Fatalf("failing checks = %q", got.FailingChecks())
	}
}

func TestPickOwnPRFailureIgnoresPendingAndUntagged(t *testing.T) {
	raw := []byte(`[
	  {"number":1,"title":"fix(farm): wait [triage:aaa]","headRefName":"fix/wait","statusCheckRollup":[
	    {"__typename":"CheckRun","name":"ci / test","status":"QUEUED","conclusion":null}
	  ]},
	  {"number":2,"title":"unrelated","headRefName":"fix/human","statusCheckRollup":[
	    {"__typename":"CheckRun","name":"ci / test","status":"COMPLETED","conclusion":"FAILURE"}
	  ]},
	  {"number":3,"title":"fix(farm): status [triage:bbb]","headRefName":"fix/status","statusCheckRollup":[
	    {"__typename":"StatusContext","context":"ci","state":"PENDING"}
	  ]}
	]`)
	prs, err := DecodePullRequests(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := PickOwnPRFailure(prs); ok {
		t.Fatal("pending or untagged PRs are not own failures")
	}
}

func TestDecodePullRequestFailureConclusion(t *testing.T) {
	raw := []byte(`[{
	  "number":7,"title":"fix(farm): red [triage:abc]","headRefName":"fix/red","url":"https://example/7",
	  "statusCheckRollup":[
	    {"__typename":"CheckRun","name":"ci / test","status":"COMPLETED","conclusion":"FAILURE"},
	    {"__typename":"CheckRun","name":"ci / lint","status":"COMPLETED","conclusion":"SKIPPED"},
	    {"__typename":"StatusContext","context":"build","state":"ERROR"}
	  ]
	}]`)
	prs, err := DecodePullRequests(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].HeadRef != "fix/red" {
		t.Fatalf("prs = %+v", prs)
	}
	if prs[0].FailingChecks() != "build, ci / test" {
		t.Fatalf("failing = %q", prs[0].FailingChecks())
	}
}

func testGit(t *testing.T, dir string) func(args ...string) string {
	t.Helper()
	return func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
}

func sameKeys(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestDecodeTriageGroupsUnwrapsMCPEnvelope(t *testing.T) {
	groups, err := DecodeTriageGroups([]byte(`{"groups":[{"key":"abc","count":2}],"resolved_hidden":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Key != "abc" {
		t.Fatalf("groups = %+v", groups)
	}
}
