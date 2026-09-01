package main

import (
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
			calls: 42, prompt: "a1b2c3d4", ok: 20, failed: 3, battles: 25, blackouts: 2, stop: "done", where: "Pewter City (10,5)"},
		{starter: "squirtle", seed: 2, frames: 200000,
			calls: 60, prompt: "a1b2c3d4", ok: 10, failed: 5, battles: 8, blackouts: 0, stop: "stuck", where: "Route 22 (3,4)"},
	}
	got := formatTable(rs)
	want := `starter    seed badge     frames  calls ok/fail battles blackouts prompt   stop     where
charmander 1    yes       98765*     42  20/3        25         2 a1b2c3d4 done     Pewter City (10,5)
squirtle   2    no        200000     60  10/5         8         0 a1b2c3d4 stuck    Route 22 (3,4)
`
	if got != want {
		t.Errorf("formatTable:\ngot:\n%s\nwant:\n%s", got, want)
	}
	// The badge row reports frames-to-badge (marked *), not total frames.
	if !strings.Contains(got, "98765*") || strings.Contains(got, "123456") {
		t.Errorf("badge row must show toBadge with *, not total frames:\n%s", got)
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
