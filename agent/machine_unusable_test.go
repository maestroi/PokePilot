package agent

import (
	"path/filepath"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

func TestMachineUnusableKnowledgeRewritesWithoutSaveState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-001-frame-0002134510-trade.state")
	ring := &checkpointRing{dir: dir, lastState: statePath}
	known := NewKnowledge(nil)
	known.noteMachineUnusable(Objective{
		Kind: KindCatch, Species: "vulpix", Intent: dexVirtualVersionIntent, Slot: 1,
	}, gameruntime.ErrMachineUnusable)

	if err := ring.rewriteKnowledge(known, dexVirtualVersionIntent, 0); err != nil {
		t.Fatalf("rewriteKnowledge: %v", err)
	}

	resumed := LoadCheckpointMemory(statePath, nil, nil)
	if !virtualTradeMachineUnusable(resumed.Knowledge) {
		t.Fatal("resumed knowledge does not record a poisoned virtual trade")
	}
}
