package main

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestStatsPlannerInjectsExplicitPlayStyleIntoSystemContext(t *testing.T) {
	p := newStatsPlannerWithRunPolicy("", "", agent.PlayStyleCompletionist, agent.RiskToleranceBalanced, agent.WildEncountersPlanner, "Beat the Elite Four and Champion.", nil, nil, nil)
	if !strings.Contains(p.baseExtraSystem, "PLAY STYLE: COMPLETIONIST") {
		t.Fatalf("base system context missing completionist policy: %q", p.baseExtraSystem)
	}

	p.prepareRunContext(agent.Observation{})
	if !strings.Contains(p.inner.ExtraSystem, "PLAY STYLE: COMPLETIONIST") {
		t.Fatalf("active system context missing completionist policy: %q", p.inner.ExtraSystem)
	}
}

func TestStatsPlannerLegacyEmptyPlayStyleKeepsSystemContextUnchanged(t *testing.T) {
	p := newStatsPlannerWithRunPolicy("", "", "", "", "", "Beat the Elite Four and Champion.", nil, nil, nil)
	if p.baseExtraSystem != "" {
		t.Fatalf("legacy empty play style changed base system prompt: %q", p.baseExtraSystem)
	}
}

func TestPolicyRawWriterShowsPolicyAndPreservesPromptTail(t *testing.T) {
	snap := &heartbeatSnap{}
	w := policyRawWriter{
		snap:           snap,
		playStyle:      agent.PlayStyleCompletionist,
		riskTolerance:  agent.RiskToleranceBalanced,
		wildEncounters: agent.WildEncountersPlanner,
	}

	prompt := "=== prompt (model qwen3.8-27b) ===\n[system]\nHEAD SYSTEM INSTRUCTIONS\n" +
		strings.Repeat("route-blockage-data-", maxRawPrompt) +
		"\nOffered objectives (copy the sentence after the number, not the number):\n" +
		"1: talk at (1,3)  [completionist 1.23: interaction+exploration; natural +0.31 new-npc]\n"

	if _, err := w.Write([]byte(prompt)); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	got := snap.load().Raw
	for _, want := range []string{
		"=== run policy ===",
		"play_style=completionist",
		"risk_tolerance=balanced",
		"wild_encounters=planner",
		"HEAD SYSTEM INSTRUCTIONS",
		"… clipped middle …",
		"Offered objectives",
		"[completionist 1.23:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("raw exchange missing %q:\n%s", want, got)
		}
	}
	if len(got) > maxRawPrompt {
		t.Fatalf("raw prompt preview = %d bytes, want <= %d", len(got), maxRawPrompt)
	}
}
