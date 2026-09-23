package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/qualification"
)

func TestParseConfig(t *testing.T) {
	t.Setenv("POKEMON_RED_ROM", "/private/red.gb")
	t.Setenv("POKEPILOT_QUALIFICATION_CORPUS", "/private/corpus")
	cfg, err := parseConfig([]string{"-profile", "skills", "-case", "rom-short", "-out", "evidence"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.romPath != "/private/red.gb" || cfg.corpus != "/private/corpus" || cfg.profile != "skills" || cfg.only != "rom-short" || cfg.out != "evidence" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestParseConfigRejectsEmptyOutput(t *testing.T) {
	if _, err := parseConfig([]string{"-out", ""}); err == nil {
		t.Fatal("empty -out accepted")
	}
}

func TestListDoesNotNeedROM(t *testing.T) {
	t.Setenv("POKEMON_RED_ROM", "")
	var out bytes.Buffer
	if err := run(config{list: true, out: "unused"}, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, id := range []string{"rom-short", "opening-brock", "rocket-hideout", "fuchsia-koga-surf-strength", "silph-sabrina", "cinnabar-blaine", "viridian-giovanni", "victory-road-indigo", "fresh-hall-of-fame"} {
		if !strings.Contains(text, id) {
			t.Errorf("list missing %q: %s", id, text)
		}
	}
	if strings.Contains(text, "blocked #33") || strings.Contains(text, "blocked #34") || strings.Contains(text, "blocked #35") || strings.Contains(text, "blocked #36") || strings.Contains(text, "blocked #37") {
		t.Fatalf("landed late-game milestones are still listed blocked: %s", text)
	}
	if strings.Contains(text, "product blocked #39") {
		t.Fatalf("fresh full qualification still reported as product-blocked: %s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "fresh-hall-of-fame") && !strings.Contains(line, "ready") {
			t.Fatalf("fresh-hall-of-fame is not listed ready: %s", line)
		}
	}
}

func TestRunRejectsUnsupportedROMBeforeCreatingOutput(t *testing.T) {
	root := t.TempDir()
	romPath := filepath.Join(root, "wrong.gb")
	if err := os.WriteFile(romPath, []byte("not pokemon red"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	err := run(config{romPath: romPath, corpus: filepath.Join(root, "corpus"), profile: "skills", out: out}, &bytes.Buffer{})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsupported rom") {
		t.Fatalf("err = %v, want unsupported ROM", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("qualification output exists despite ROM rejection: %v", statErr)
	}
}

func TestVerifyExpectationUsesSemanticBag(t *testing.T) {
	obs := agent.Observation{Bag: []agent.Item{{Name: "Silph Scope", Quantity: 1}}}
	if err := verifyExpectation(qualification.Expectation{Kind: "item", Value: "silph scope"}, obs); err != nil {
		t.Fatal(err)
	}
	if err := verifyExpectation(qualification.Expectation{Kind: "item", Value: "poke flute"}, obs); err == nil {
		t.Fatal("missing item accepted")
	}
}

func TestVerifyExpectationUsesSemanticBadgeAndProgress(t *testing.T) {
	obs := agent.Observation{
		Badges: []string{"Marsh"},
		Story: agent.ProgressState{
			{ID: agent.ProgressID("indigo_plateau_ready"), Complete: true},
		},
	}
	if err := verifyExpectation(qualification.Expectation{Kind: "badge", Value: "marsh"}, obs); err != nil {
		t.Fatal(err)
	}
	if err := verifyExpectation(qualification.Expectation{Kind: "badge", Value: "Earth"}, obs); err == nil {
		t.Fatal("missing badge accepted")
	}
	if err := verifyExpectation(qualification.Expectation{Kind: "progress", Value: "indigo_plateau_ready"}, obs); err != nil {
		t.Fatal(err)
	}
	if err := verifyExpectation(qualification.Expectation{Kind: "progress", Value: "main_story_complete"}, obs); err == nil {
		t.Fatal("missing progress accepted")
	}
}

func TestCorpusCheckpointCannotEscapeRoot(t *testing.T) {
	root := t.TempDir()
	inside, err := corpusCheckpoint(root, "rocket-hideout/start.state")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(inside, root+string(filepath.Separator)) {
		t.Fatalf("inside path = %q, root = %q", inside, root)
	}
	if _, err := corpusCheckpoint(root, "../outside.state"); err == nil {
		t.Fatal("escaping checkpoint accepted")
	}
}

func TestPreparePrivateCheckpointEvidenceCopiesAndHashes(t *testing.T) {
	root := t.TempDir()
	corpus := filepath.Join(root, "corpus")
	out := filepath.Join(root, "out")
	caseDir := filepath.Join(out, "cases", "elite-four-loss-recovery")
	checkpoint := filepath.Join(corpus, "elite-four-loss-recovery", "start.state")
	if err := os.MkdirAll(filepath.Dir(checkpoint), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("private-checkpoint-bytes")
	if err := os.WriteFile(checkpoint, data, 0o600); err != nil {
		t.Fatal(err)
	}

	evidence, hash, err := preparePrivateCheckpointEvidence(
		config{corpus: corpus, out: out},
		qualification.Case{ID: "elite-four-loss-recovery", Checkpoint: "elite-four-loss-recovery/start.state"},
		caseDir,
	)
	if err != nil {
		t.Fatal(err)
	}
	if hash != sha256Hex(data) {
		t.Fatalf("checkpoint hash = %q, want %q", hash, sha256Hex(data))
	}
	if len(evidence) != 1 || evidence[0] != filepath.Join("cases", "elite-four-loss-recovery", "start.state") {
		t.Fatalf("evidence = %v", evidence)
	}
	copied, err := os.ReadFile(filepath.Join(caseDir, "start.state"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(copied, data) {
		t.Fatalf("copied checkpoint = %q, want %q", copied, data)
	}
}

func TestPreparePrivateCheckpointEvidenceFailsBeforeChildRunWhenMissing(t *testing.T) {
	root := t.TempDir()
	caseDir := filepath.Join(root, "out", "cases", "missing")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := preparePrivateCheckpointEvidence(
		config{corpus: filepath.Join(root, "corpus"), out: filepath.Join(root, "out")},
		qualification.Case{ID: "missing", Checkpoint: "missing/start.state"},
		caseDir,
	)
	if err == nil || !strings.Contains(err.Error(), "missing/start.state") {
		t.Fatalf("err = %v, want direct missing checkpoint diagnostic", err)
	}
}

func TestPreparePrivateCheckpointEvidenceLeavesFixtureCasesAlone(t *testing.T) {
	evidence, hash, err := preparePrivateCheckpointEvidence(
		config{corpus: "/not/used", out: t.TempDir()},
		qualification.Case{ID: "opening-brock", Checkpoint: "fixture:forest_north_gate"},
		t.TempDir(),
	)
	if err != nil || hash != "" || len(evidence) != 0 {
		t.Fatalf("fixture checkpoint = evidence:%v hash:%q err:%v", evidence, hash, err)
	}
}

func TestQualificationEnvOverridesROMAndDisablesFarm(t *testing.T) {
	t.Setenv("POKEMON_RED_ROM", "/wrong.gb")
	t.Setenv("POKEPILOT_QUALIFICATION_CORPUS", "/wrong/corpus")
	t.Setenv("POKEPILOT_ORCH_URL", "http://wall")
	env := qualificationEnv(config{romPath: "/verified.gb", corpus: "/verified/corpus"}, "/tmp/fixtures")
	got := envMap(env)
	if got["POKEMON_RED_ROM"] != "/verified.gb" {
		t.Fatalf("POKEMON_RED_ROM = %q", got["POKEMON_RED_ROM"])
	}
	if got["POKEPILOT_QUALIFICATION_CORPUS"] != "/verified/corpus" {
		t.Fatalf("POKEPILOT_QUALIFICATION_CORPUS = %q", got["POKEPILOT_QUALIFICATION_CORPUS"])
	}
	if got["POKEPILOT_FIXTURE_DIR"] != "/tmp/fixtures" {
		t.Fatalf("POKEPILOT_FIXTURE_DIR = %q", got["POKEPILOT_FIXTURE_DIR"])
	}
	if got["POKEPILOT_ORCH_URL"] != "" {
		t.Fatalf("POKEPILOT_ORCH_URL = %q, want disabled", got["POKEPILOT_ORCH_URL"])
	}
}

func TestManifestModelDoesNotIncludeTokenField(t *testing.T) {
	data, err := jsonForTest(t, manifest{Model: modelInfo{URL: "http://model", Model: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "token\"") || strings.Contains(string(data), "secret") {
		t.Fatalf("manifest unexpectedly exposes a credential-shaped field: %s", data)
	}
}

func TestCopyJourneyFailureState(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.MkdirAll(filepath.Join("skill", "failure"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("skill", "failure", "TestThing.state"), []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	caseDir := filepath.Join(root, "case")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := copyJourneyFailureState("^TestThing$", caseDir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(got)
	if err != nil || string(data) != "state" {
		t.Fatalf("copied state = %q, err=%v", data, err)
	}
}

func envMap(env []string) map[string]string {
	out := map[string]string{}
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

func jsonForTest(t *testing.T, v any) ([]byte, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := writeJSON(path, v); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
