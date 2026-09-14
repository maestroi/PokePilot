package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
)

const verdictObjectiveFailed = "objective_failed"

// verifySyntheticFailureBudget handles terminal run-policy failures whose
// portable issue fingerprint describes the failure-budget wrapper rather than
// the objective that actually failed. The checkpoint is still the exact
// pre-objective state. Rebuild the deterministic offered menu from that state
// and paired knowledge, then resolve the nested error against exact
// Objective.String sentences. This avoids both an LLM call and lossy parsing of
// human objective syntax.
func verifySyntheticFailureBudget(mat portableMaterialized, resultPath string, verdict portableReproVerdict) (portableReproVerdict, error) {
	if strings.TrimSpace(mat.Manifest.Objective) != "recover from repeated objective failures" {
		verdict.Classification = verdictContractUnavailable
		verdict.Diagnostic = "portable bundle has no structured failure-repro contract; checkpoint-only replay cannot prove a run-level/planner failure"
		return verdict, writePortableReproVerdict(resultPath, mat.Dir, verdict)
	}

	romPath := strings.TrimSpace(os.Getenv("POKEMON_RED_ROM"))
	if romPath == "" {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = "POKEMON_RED_ROM is not set"
		_ = writePortableReproVerdict(resultPath, mat.Dir, verdict)
		return verdict, fmt.Errorf("POKEMON_RED_ROM is not set")
	}
	stateBytes, err := os.ReadFile(mat.StatePath)
	if err != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = err.Error()
		_ = writePortableReproVerdict(resultPath, mat.Dir, verdict)
		return verdict, fmt.Errorf("read checkpoint: %w", err)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = err.Error()
		_ = writePortableReproVerdict(resultPath, mat.Dir, verdict)
		return verdict, fmt.Errorf("open ROM: %w", err)
	}
	defer m.Close()
	if err := m.LoadState(stateBytes); err != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = err.Error()
		_ = writePortableReproVerdict(resultPath, mat.Dir, verdict)
		return verdict, fmt.Errorf("load checkpoint: %w", err)
	}

	graph, err := world.BuildGraph(m.ROM())
	if err != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = err.Error()
		_ = writePortableReproVerdict(resultPath, mat.Dir, verdict)
		return verdict, fmt.Errorf("build map graph: %w", err)
	}
	adjacency := make(map[uint8][]uint8, len(graph.Edges))
	for from, edges := range graph.Edges {
		for _, edge := range edges {
			adjacency[from] = append(adjacency[from], edge.To)
		}
	}
	memory := agent.LoadCheckpointMemory(mat.StatePath, adjacency, nil)
	obs := agent.Observe(m, m.ROM())
	offered := agent.Offer(obs, memory.Knowledge)
	obj, ok, matchErr := underlyingObjectiveFromDiagnostic(mat.Manifest.Diagnostic, offered)
	if matchErr != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = matchErr.Error()
		_ = writePortableReproVerdict(resultPath, mat.Dir, verdict)
		return verdict, matchErr
	}
	if !ok {
		verdict.Classification = verdictContractUnavailable
		verdict.Diagnostic = "failure-budget checkpoint could not resolve its nested objective from the current deterministic offered menu"
		return verdict, writePortableReproVerdict(resultPath, mat.Dir, verdict)
	}

	verdict.Objective = obj.String()
	result, execErr := agent.Execute(m, m.ROM(), obj)
	verdict.Outcome = string(result.Outcome)
	verdict.Cause = string(result.Cause)
	verdict.Diagnostic = result.Summary
	if execErr == nil {
		verdict.Classification = verdictObjectiveSucceeded
	} else {
		// The issue fingerprint belongs to the run-level failure-budget wrapper,
		// so comparing it with a freshly derived objective fingerprint would be
		// misleading. Record the typed objective result directly instead.
		verdict.Classification = verdictObjectiveFailed
	}
	return verdict, writePortableReproVerdict(resultPath, mat.Dir, verdict)
}

func underlyingObjectiveFromDiagnostic(diagnostic string, offered []agent.Objective) (agent.Objective, bool, error) {
	diagnostic = strings.TrimSpace(diagnostic)
	if diagnostic == "" {
		return agent.Objective{}, false, nil
	}
	seen := map[string]bool{}
	matches := make([]agent.Objective, 0, 1)
	for _, obj := range offered {
		name := obj.String()
		if seen[name] {
			continue
		}
		seen[name] = true
		if strings.Contains(diagnostic, "agent: "+name+":") {
			matches = append(matches, obj)
		}
	}
	switch len(matches) {
	case 0:
		return agent.Objective{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		names := make([]string, 0, len(matches))
		for _, obj := range matches {
			names = append(names, obj.String())
		}
		return agent.Objective{}, false, fmt.Errorf("synthetic failure diagnostic matches multiple offered objectives: %s", strings.Join(names, ", "))
	}
}
