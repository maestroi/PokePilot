package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
)

const farmResumeMarker = ".farm-resume"

// prepareFarmAttempt restores either a durable checkpoint from the immediately
// previous lost worker or the ordinary boot state. Resume lookup is deliberately
// best-effort: a missing/corrupt/unreachable checkpoint falls back to the exact
// fresh-attempt path that farm mode used before recovery support.
func prepareFarmAttempt(m *emu.Emu, client *farm.Client, spec farm.Spec, planner string, bootState []byte, checkpointDir string) (dir string, burn int, err error) {
	dir = checkpointDir
	if dir == "" && planner == "llm" {
		dir, err = os.MkdirTemp("", "pokefarm-checkpoints-")
		if err != nil {
			return "", 0, fmt.Errorf("checkpoint dir: %w", err)
		}
	}
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return dir, 0, fmt.Errorf("checkpoint dir: %w", err)
		}
		_ = os.Remove(filepath.Join(dir, farmResumeMarker))
	}

	// LLM objective checkpoints are a paired emulator state + knowledge
	// snapshot, so they are safe to continue across process/build boundaries.
	// Scripted runs keep their historic fresh retry behavior for now.
	if planner == "llm" && spec.Attempt > 1 && dir != "" {
		ctx, cancel := context.WithTimeout(context.Background(), farmHTTPTimeout)
		cp, lookupErr := client.ResumeCheckpoint(ctx, spec.RunID, spec.Attempt)
		cancel()
		if lookupErr != nil {
			log.Printf("farm: %s: resume lookup failed; starting attempt %d fresh: %v", spec.RunID, spec.Attempt, lookupErr)
		} else if cp != nil {
			if materializeErr := materializeFarmResume(dir, *cp); materializeErr != nil {
				log.Printf("farm: %s: resume checkpoint unusable; starting attempt %d fresh: %v", spec.RunID, spec.Attempt, materializeErr)
			} else if loadErr := m.LoadState(cp.State.Data); loadErr != nil {
				_ = os.Remove(filepath.Join(dir, farmResumeMarker))
				log.Printf("farm: %s: resume state rejected; starting attempt %d fresh: %v", spec.RunID, spec.Attempt, loadErr)
			} else {
				log.Printf("farm: %s: resumed attempt %d from attempt %d checkpoint %s", spec.RunID, spec.Attempt, cp.Attempt, cp.State.Name)
				m.TraceNote(fmt.Sprintf("resumed from attempt %d checkpoint %s", cp.Attempt, cp.State.Name))
				return dir, 0, nil
			}
		}
	}

	if err := m.LoadState(bootState); err != nil {
		return dir, 0, fmt.Errorf("load state: %w", err)
	}
	burn = seedBurn(spec.Seed)
	if burn > 0 {
		m.StepFrames(burn)
		fmt.Printf("seed %d: burned %d idle frames, so this run's luck differs\n", spec.Seed, burn)
	}
	return dir, burn, nil
}

func materializeFarmResume(dir string, cp farm.ResumeCheckpoint) error {
	if dir == "" {
		return fmt.Errorf("empty checkpoint dir")
	}
	if cp.State.Name == "" || filepath.Base(cp.State.Name) != cp.State.Name || !strings.HasSuffix(cp.State.Name, ".state") {
		return fmt.Errorf("unsafe resume state name %q", cp.State.Name)
	}
	if cp.Knowledge == nil {
		return fmt.Errorf("resume checkpoint %s has no paired knowledge", cp.State.Name)
	}
	if cp.Knowledge.Name == "" || filepath.Base(cp.Knowledge.Name) != cp.Knowledge.Name || !strings.HasSuffix(cp.Knowledge.Name, ".json") {
		return fmt.Errorf("unsafe resume knowledge name %q", cp.Knowledge.Name)
	}
	if err := os.WriteFile(filepath.Join(dir, cp.State.Name), cp.State.Data, 0o644); err != nil {
		return fmt.Errorf("write resume state: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, cp.Knowledge.Name), cp.Knowledge.Data, 0o644); err != nil {
		return fmt.Errorf("write resume knowledge: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, farmResumeMarker), []byte(cp.State.Name+"\n"), 0o644); err != nil {
		return fmt.Errorf("write resume marker: %w", err)
	}
	return nil
}

// farmResumePath is intentionally marker-based rather than "latest state in
// the directory": an operator-supplied checkpoint directory may contain old
// diagnostics that must never make an unrelated fresh lease resume itself.
func farmResumePath(dir string) string {
	if dir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, farmResumeMarker))
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(data))
	if name == "" || filepath.Base(name) != name || !strings.HasSuffix(name, ".state") {
		return ""
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}
