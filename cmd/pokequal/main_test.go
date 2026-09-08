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
	for _, id := range []string{"rom-short", "opening-brock", "rocket-hideout", "fresh-hall-of-fame"} {
		if !strings.Contains(out.String(), id) {
			t.Errorf("list missing %q: %s", id, out.String())
		}
	}
	if !strings.Contains(out.String(), "blocked #33") || !strings.Contains(out.String(), "runner ready; product blocked #39") {
		t.Fatalf("list does not distinguish future/product blockers: %s", out.String())
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

func TestQualificationEnvOverridesROMAndDisablesFarm(t *testing.T) {
	t.Setenv("POKEMON_RED_ROM", "/wrong.gb")
	t.Setenv("POKEPILOT_ORCH_URL", "http://wall")
	env := qualificationEnv(config{romPath: "/verified.gb"}, "/tmp/fixtures")
	got := envMap(env)
	if got["POKEMON_RED_ROM"] != "/verified.gb" {
		t.Fatalf("POKEMON_RED_ROM = %q", got["POKEMON_RED_ROM"])
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
