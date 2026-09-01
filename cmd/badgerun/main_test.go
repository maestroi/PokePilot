package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

// These tests cover argument parsing and table formatting only. The harness
// itself — emulator, model, ROM — is not part of `go test ./...` on purpose:
// a scoreboard that needs a live model server does not belong in the suite.

func TestParseConfigDefaults(t *testing.T) {
	t.Setenv("POKEMON_RED_ROM", "roms/pokemon_red.gb")
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.romPath != "roms/pokemon_red.gb" {
		t.Errorf("romPath = %q, want the POKEMON_RED_ROM value", cfg.romPath)
	}
	if len(cfg.starters) != 3 || cfg.starters[0] != "charmander" || cfg.starters[2] != "bulbasaur" {
		t.Errorf("starters = %v, want all three in canonical order", cfg.starters)
	}
	if cfg.n != 3 {
		t.Errorf("n = %d, want default 3", cfg.n)
	}
	if len(cfg.seeds) != 3 || cfg.seeds[0] != 1 || cfg.seeds[2] != 3 {
		t.Errorf("seeds = %v, want [1 2 3]", cfg.seeds)
	}
	if cfg.maxRounds <= 0 || cfg.maxFrames <= 0 {
		t.Errorf("budget defaults must be positive: rounds=%d frames=%d", cfg.maxRounds, cfg.maxFrames)
	}
	if cfg.injectFact {
		t.Error("injectFact = true by default; the fact-injection flag MUST default off")
	}
	if cfg.fact == "" {
		t.Error("fact is empty; -inject-fact needs a sentence to inject")
	}
}

func TestParseConfigFlags(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-rom", "x.gb",
		"-starter", "charmander",
		"-n", "2",
		"-seeds", "5,7",
		"-max-rounds", "10",
		"-max-frames", "1000",
		"-out", "somewhere",
		"-inject-fact",
		"-fact", "Butterfree is Bug.",
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.romPath != "x.gb" || len(cfg.starters) != 1 || cfg.starters[0] != "charmander" {
		t.Errorf("rom/starter = %q %v", cfg.romPath, cfg.starters)
	}
	if cfg.n != 2 || len(cfg.seeds) != 2 || cfg.seeds[0] != 5 || cfg.seeds[1] != 7 {
		t.Errorf("n/seeds = %d %v", cfg.n, cfg.seeds)
	}
	if cfg.maxRounds != 10 || cfg.maxFrames != 1000 || cfg.outDir != "somewhere" {
		t.Errorf("budget/out = %d %d %q", cfg.maxRounds, cfg.maxFrames, cfg.outDir)
	}
	if !cfg.injectFact || cfg.fact != "Butterfree is Bug." {
		t.Errorf("injectFact/fact = %v %q", cfg.injectFact, cfg.fact)
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no rom", nil},
		{"bad starter", []string{"-rom", "x.gb", "-starter", "pikachu"}},
		{"empty seed field", []string{"-rom", "x.gb", "-seeds", "1,,2"}},
		{"non-integer seed", []string{"-rom", "x.gb", "-seeds", "a"}},
		{"zero runs", []string{"-rom", "x.gb", "-n", "0"}},
		{"zero rounds", []string{"-rom", "x.gb", "-max-rounds", "0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "no rom" {
				t.Setenv("POKEMON_RED_ROM", "")
			}
			if _, err := parseConfig(c.args); err == nil {
				t.Fatalf("parseConfig(%v) succeeded, want an error", c.args)
			}
		})
	}
}

func TestParseSeeds(t *testing.T) {
	seeds, err := parseSeeds("0, 12")
	if err != nil || len(seeds) != 2 || seeds[0] != 0 || seeds[1] != 12 {
		t.Errorf("parseSeeds = %v, %v; want [0 12], nil (seed 0 is legal: replays identically)", seeds, err)
	}
	if _, err := parseSeeds(""); err == nil {
		t.Error("parseSeeds(\"\") succeeded, want an error")
	}
}

func TestParseStarters(t *testing.T) {
	all, err := parseStarters("all")
	if err != nil || len(all) != 3 {
		t.Errorf("parseStarters(all) = %v, %v", all, err)
	}
	one, err := parseStarters("SQUIRTLE")
	if err != nil || len(one) != 1 || one[0] != "squirtle" {
		t.Errorf("parseStarters(SQUIRTLE) = %v, %v", one, err)
	}
	if _, err := parseStarters("ecranon"); err == nil {
		t.Error("parseStarters(ecranon) succeeded, want an error")
	}
}

func TestFormatTable(t *testing.T) {
	rs := []runResult{
		{starter: "charmander", seed: 1, badge: true, frames: 123456, toBadge: 98765,
			calls: 42, ok: 20, failed: 3, battles: 25, blackouts: 2, stop: "done", where: "Pewter City (10,5)",
			retries: 1, transport: 2, rejected: 3, fallbacks: 4,
			promptTokens: 5000, completionTokens: 600, promptHash: "64e079be"},
		{starter: "squirtle", seed: 2, frames: 200000,
			calls: 60, ok: 10, failed: 5, battles: 8, blackouts: 0, stop: "stuck", where: "Route 22 (3,4)"},
		{starter: "bulbasaur", seed: 3, frames: 50000, resumed: 90,
			calls: 12, ok: 2, failed: 1, battles: 3, blackouts: 1, stop: "budget", where: "Route 2 (15,13)"},
	}
	got := formatTable(rs)
	want := `starter    seed badge     frames  calls ok/fail battles blackouts stop     where
charmander 1    yes       98765*     42  20/3        25         2 done     Pewter City (10,5)
  retries=1 transport=2 rejected=3 fallbacks=4 tokens=5000/600 prompt=64e079be
squirtle   2    no        200000     60  10/5         8         0 stuck    Route 22 (3,4)
  retries=0 transport=0 rejected=0 fallbacks=0 tokens=0/0 prompt=
bulbasaur  3    no         50000     12   2/1         3         1 budget   Route 2 (15,13)
  retries=0 transport=0 rejected=0 fallbacks=0 tokens=0/0 prompt= resumed=90
`
	if got != want {
		t.Errorf("formatTable:\ngot:\n%s\nwant:\n%s", got, want)
	}
	// The badge row reports frames-to-badge (marked *), not total frames.
	if !strings.Contains(got, "98765*") || strings.Contains(got, "123456") {
		t.Errorf("badge row must show toBadge with *, not total frames:\n%s", got)
	}
	// The second line carries the cost of the run: every diagnostic must be
	// present, none silently dropped, and the values must be the row's own.
	if !strings.Contains(got, "retries=1 transport=2 rejected=3 fallbacks=4 tokens=5000/600 prompt=64e079be") {
		t.Errorf("cost line must report retries, health counters, token totals and prompt hash:\n%s", got)
	}
	// A resumed row announces where it picked up (resumed=N: this row's
	// round 1 is the original run's round N+1); a fresh row carries no
	// marker, so today's rows are unchanged.
	if !strings.Contains(got, "prompt= resumed=90") {
		t.Errorf("resumed row must carry resumed=N:\n%s", got)
	}
	if strings.Count(got, "resumed=") != 1 {
		t.Errorf("exactly one row is resumed; fresh rows must not say so:\n%s", got)
	}
}

// TestNewestCheckpoint pins the resume selection against the ACTUAL naming
// checkpointRing.write produces: round-%03d-frame-%010d-<slug>.state. Both
// numbers are zero-padded, so the lexicographic max IS the newest round —
// the files below include the ordering case that is only true BECAUSE of the
// padding (unpadded, "10" would sort before "9"), plus the noise a real
// ring directory carries: knowledge files and final.state.
func TestNewestCheckpoint(t *testing.T) {
	runDir := t.TempDir()
	ck := filepath.Join(runDir, "checkpoints")
	if err := os.MkdirAll(ck, 0o755); err != nil {
		t.Fatal(err)
	}
	make := func(name string) {
		if err := os.WriteFile(filepath.Join(ck, name), []byte("state"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	make("round-003-frame-0000001234-talk-professor-oak.state")
	make("round-009-frame-0000000010-take-item.state") // frame 10 > 9 only because both are padded
	make("round-010-frame-0000009876-go-to-pewter-city.state")
	make("round-010-frame-0000009876-go-to-pewter-city.state.knowledge-v4.json")
	make(".knowledge-abc123.tmp")
	make("final.state")

	ckptDir, path, round, frame, err := newestCheckpoint(runDir)
	if err != nil {
		t.Fatalf("newestCheckpoint: %v", err)
	}
	if filepath.Base(path) != "round-010-frame-0000009876-go-to-pewter-city.state" {
		t.Errorf("newest = %q, want the round-010 state (lexicographic max of zero-padded names)", path)
	}
	if round != 10 || frame != 9876 {
		t.Errorf("round/frame = %d/%d, want 10/9876", round, frame)
	}
	if ckptDir != ck {
		t.Errorf("ckptDir = %q, want %q", ckptDir, ck)
	}

	// The checkpoints directory itself is accepted, not just the run dir.
	if _, p, r, f, err := newestCheckpoint(ck); err != nil || filepath.Base(p) != filepath.Base(path) || r != 10 || f != 9876 {
		t.Errorf("newestCheckpoint(ck) = %q %d %d %v; want the same answer", p, r, f, err)
	}
}

func TestNewestCheckpointErrors(t *testing.T) {
	// Empty directory: nothing to resume from.
	if _, _, _, _, err := newestCheckpoint(t.TempDir()); err == nil {
		t.Error("newestCheckpoint(empty dir) succeeded, want an error")
	}
	// A directory with only non-checkpoint states: final.state is not a
	// ring entry and must not be picked up.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "checkpoints"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checkpoints", "final.state"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := newestCheckpoint(dir); err == nil {
		t.Error("newestCheckpoint(final.state only) succeeded, want an error")
	}
	// Malformed checkpoint-shaped names are not ring entries.
	if err := os.WriteFile(filepath.Join(dir, "checkpoints", "round-x-frame-1-y.state"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := newestCheckpoint(dir); err == nil {
		t.Error("newestCheckpoint(malformed names only) succeeded, want an error")
	}
}

// TestCheckpointRoundFrame pins the parse of the writer's exact format.
func TestCheckpointRoundFrame(t *testing.T) {
	if r, f, ok := checkpointRoundFrame("round-042-frame-0003000000-take-item.state"); !ok || r != 42 || f != 3000000 {
		t.Errorf("= %d %d %v, want 42 3000000 true", r, f, ok)
	}
	for _, bad := range []string{
		"final.state",
		"round-010-frame-0000009876-go.state.knowledge-v4.json",
		"round-abc-frame-0000000001-x.state",
		"round-010-frame-x-y.state",
		"round-010-frame-.state",
	} {
		if _, _, ok := checkpointRoundFrame(bad); ok {
			t.Errorf("checkpointRoundFrame(%q) = ok, want false", bad)
		}
	}
}

// TestParseConfigResume pins the flag: absent = today's behaviour (all
// resume fields zero), present = the newest checkpoint resolved at parse
// time so a bad directory fails before any run starts.
func TestParseConfigResume(t *testing.T) {
	t.Setenv("POKEMON_RED_ROM", "x.gb")

	cfg, err := parseConfig([]string{"-rom", "x.gb"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.resumeDir != "" || cfg.resumePath != "" || cfg.resumeRound != 0 || cfg.resumeFrame != 0 {
		t.Errorf("without -resume the resume fields must be zero: %+v", cfg)
	}

	runDir := t.TempDir()
	ck := filepath.Join(runDir, "checkpoints")
	if err := os.MkdirAll(ck, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ck, "round-005-frame-0000000100-a.state"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ck, "round-009-frame-0000000900-b.state"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err = parseConfig([]string{"-rom", "x.gb", "-resume", runDir})
	if err != nil {
		t.Fatalf("parseConfig -resume: %v", err)
	}
	if cfg.resumeDir != runDir || filepath.Base(cfg.resumePath) != "round-009-frame-0000000900-b.state" {
		t.Errorf("resumeDir/resumePath = %q %q, want the run dir and the round-009 state", cfg.resumeDir, cfg.resumePath)
	}
	if cfg.resumeRound != 9 || cfg.resumeFrame != 900 {
		t.Errorf("resumeRound/resumeFrame = %d %d, want 9 900", cfg.resumeRound, cfg.resumeFrame)
	}
	if cfg.resumeCkpt != ck {
		t.Errorf("resumeCkpt = %q, want %q", cfg.resumeCkpt, ck)
	}

	if _, err := parseConfig([]string{"-rom", "x.gb", "-resume", filepath.Join(runDir, "nope")}); err == nil {
		t.Error("parseConfig -resume (missing dir) succeeded, want an error")
	}
}

// TestPromptHash pins the comparability marker: a pure function over the
// four values the request is built from, stable for equal input, and
// sensitive to every one of them. The before/after pair is the S10-1 case:
// the argument annotations changed the reply schema, so the hash must.
func TestPromptHash(t *testing.T) {
	const (
		base   = "You are choosing the next objective for a Pokemon Red player."
		goal   = "Earn the Boulder Badge."
		extra  = "\n\nOne fact about this game: Rock-type Pokemon resist Fire."
		schema = `{"type":"object","properties":{"choice":{"type":"integer"},"intent":{"type":"string"}},"required":["choice"]}`
	)
	before := agent.PromptHash(base, goal, extra, schema)
	if len(before) != 8 {
		t.Fatalf("hash = %q, want 8 hex chars", before)
	}
	for _, c := range before {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("hash = %q, not lowercase hex", before)
		}
	}
	// Stable: same four values, same hash.
	if got := agent.PromptHash(base, goal, extra, schema); got != before {
		t.Errorf("hash is not stable: %s != %s", got, before)
	}
	// S10-1's change: the schema gained argument annotations. Different
	// prompt, different hash — that is the whole point of the marker.
	schemaAnnotated := schema[:len(schema)-1] + `,"level":{"type":"integer"}}` + `}`
	if after := agent.PromptHash(base, goal, extra, schemaAnnotated); after == before {
		t.Errorf("hash must change when the reply schema changes: %s", before)
	}
	// Each of the other three values, alone, also changes the hash.
	if got := agent.PromptHash(base, goal+" (harder)", extra, schema); got == before {
		t.Errorf("hash must change when the goal changes")
	}
	if got := agent.PromptHash(base, goal, "", schema); got == before {
		t.Errorf("hash must change when the extra system text changes")
	}
	if got := agent.PromptHash(base+" More.", goal, extra, schema); got == before {
		t.Errorf("hash must change when the base prompt changes")
	}
	// Field boundaries: moving bytes between fields must not collide.
	if got := agent.PromptHash(base+goal, "", extra, schema); got == before {
		t.Errorf("hash must not conflate field boundaries")
	}
}

func TestHasBadge(t *testing.T) {
	if !hasBadge(agent.Observation{Badges: []string{"Boulder", "Cascade"}}, "Boulder") {
		t.Error("hasBadge(Boulder, Cascade; Boulder) = false")
	}
	if !hasBadge(agent.Observation{Badges: []string{"Cascade", "Thunder"}}, "Cascade") {
		t.Error("hasBadge(Cascade, Thunder; Cascade) = false")
	}
	if hasBadge(agent.Observation{Badges: []string{"Cascade", "Thunder"}}, "Boulder") {
		t.Error("hasBadge(Cascade, Thunder; Boulder) = true")
	}
	if hasBadge(agent.Observation{}, "Boulder") {
		t.Error("hasBadge(no badges) = true")
	}
}
