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

// prepareFarmAttempt restores either an explicitly queued repro checkpoint, a
// durable checkpoint from the wall (lost worker, endless error retry, or
// endless successor of a failed campaign), or the ordinary boot state.
// Worker-loss and successor resume remain best-effort. Explicit repro is
// strict: silently falling back to boot would produce a green-looking
// verification run that never exercised the failure checkpoint it was created
// to test.
func prepareFarmAttempt(m *emu.Emu, client *farm.Client, spec farm.Spec, planner string, bootState []byte, checkpointDir string) (dir string, burn int, err error) {
	dir = checkpointDir
	// Reconstruct the cartridge before loading any state. Checked checkpoints
	// include the effective ROM SHA-256, so an exact starter request + seed
	// replays cleanly while an accidental mismatch is rejected by GomeBoy.
	if err := prepareStarterExperiment(m, spec); err != nil {
		return dir, 0, fmt.Errorf("starter experiment: %w", err)
	}

	explicitRepro := spec.Attempt == 1 && strings.HasPrefix(spec.RunID, "replay-")
	var repro *farm.ResumeCheckpoint
	if explicitRepro {
		ctx, cancel := context.WithTimeout(context.Background(), farmHTTPTimeout)
		cp, lookupErr := client.ReplayCheckpoint(ctx, spec.RunID)
		cancel()
		if lookupErr != nil {
			return dir, 0, fmt.Errorf("replay checkpoint lookup: %w", lookupErr)
		}
		if cp == nil {
			return dir, 0, fmt.Errorf("replay run %s has no pinned replay checkpoint", spec.RunID)
		}
		repro = cp
	}

	if dir == "" && (planner == "llm" || repro != nil) {
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

	if repro != nil {
		if planner != "llm" {
			return dir, 0, fmt.Errorf("replay checkpoint requires llm planner; got %q", planner)
		}
		if err := materializeFarmResume(dir, *repro); err != nil {
			return dir, 0, fmt.Errorf("materialize replay checkpoint: %w", err)
		}
		if err := m.LoadState(repro.State.Data); err != nil {
			discardFarmResume(dir, *repro)
			return dir, 0, fmt.Errorf("load replay checkpoint %s: %w", repro.State.Name, err)
		}
		log.Printf("farm: %s: replaying source attempt %d checkpoint %s", spec.RunID, repro.Attempt, repro.State.Name)
		m.TraceNote("replay", fmt.Sprintf("source attempt %d checkpoint %s", repro.Attempt, repro.State.Name))
		return dir, 0, nil
	}

	// LLM objective checkpoints are a paired emulator state + knowledge
	// snapshot, so they are safe to continue across process/build boundaries.
	// Attempt 1 also asks: endless successors of a failed campaign resume
	// from the parent's latest major checkpoint, and ordinary first leases
	// get 204 and boot. Scripted runs keep their historic fresh retry.
	//
	// A missing/unusable checkpoint is deliberately best-effort: falling back
	// to bootState below must never wedge the farm (see farm.Client.ResumeCheckpoint).
	// But for attempt>1 or an endless successor, a resume was actually
	// expected, so silently landing on a fresh cartridge is a real regression
	// worth surfacing loudly rather than looking like a live mid-game hang.
	isRetryAttempt := spec.Attempt >= 2
	expectedResume := planner == "llm" && (isRetryAttempt || spec.Endless)
	fallbackReason := ""
	if planner == "llm" && dir != "" {
		ctx, cancel := context.WithTimeout(context.Background(), farmResumeTimeout)
		cp, lookupErr := client.ResumeCheckpoint(ctx, spec.RunID, spec.Attempt)
		cancel()
		if lookupErr != nil && expectedResume {
			// The wall never said "nothing to resume" (that is a 204); it was
			// slow, restarting or failed. Booting fresh here publishes early
			// checkpoints under this attempt and throws away the campaign, so
			// end the attempt and let the next lease look again.
			return dir, 0, fmt.Errorf("resume lookup failed for attempt %d: %w", spec.Attempt, lookupErr)
		}
		if lookupErr != nil {
			fallbackReason = fmt.Sprintf("resume lookup failed: %v", lookupErr)
			log.Printf("farm: %s: resume lookup failed; starting attempt %d fresh: %v", spec.RunID, spec.Attempt, lookupErr)
		} else if cp != nil {
			if materializeErr := materializeFarmResume(dir, *cp); materializeErr != nil {
				discardFarmResume(dir, *cp)
				fallbackReason = fmt.Sprintf("resume checkpoint unusable: %v", materializeErr)
				log.Printf("farm: %s: resume checkpoint unusable; starting attempt %d fresh: %v", spec.RunID, spec.Attempt, materializeErr)
			} else if loadErr := m.LoadState(cp.State.Data); loadErr != nil {
				// The wall offered a state this build cannot load. Drop the
				// materialized pair so the uploader cannot republish it as this
				// attempt's progress; the wall's older usable checkpoint stays
				// the next resume candidate.
				discardFarmResume(dir, *cp)
				fallbackReason = fmt.Sprintf("resume state rejected: %v", loadErr)
				log.Printf("farm: %s: resume state rejected; starting attempt %d fresh: %v", spec.RunID, spec.Attempt, loadErr)
			} else {
				log.Printf("farm: %s: resumed attempt %d from attempt %d checkpoint %s", spec.RunID, spec.Attempt, cp.Attempt, cp.State.Name)
				m.TraceNote("resume", fmt.Sprintf("attempt %d checkpoint %s", cp.Attempt, cp.State.Name))
				return dir, 0, nil
			}
		} else {
			fallbackReason = "no checkpoint found"
		}
	}
	if warn, message := resumeFallbackWarning(spec.Attempt, expectedResume, fallbackReason); warn {
		log.Printf("farm: %s: WARNING: %s", spec.RunID, message)
		m.TraceNote("resume-fallback", message)
	}

	// bootState is the runner's raw post-boot state captured from the verified
	// base ROM. Raw states intentionally do not bind a ROM hash; the only code
	// bytes this experiment changes are later Oak Lab immediates, so restoring
	// that pre-starter state onto the derived cartridge is deterministic.
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

// resumeFallbackWarning reports whether an attempt that was expected to
// resume from a checkpoint (a retry, or an endless successor) instead fell
// back to a fresh cartridge, and the message to surface for it. Falling back
// itself is fine and by design (see farm.Client.ResumeCheckpoint); silently
// doing so on an attempt that should have carried forward mid-game progress
// is not, and previously it was indistinguishable from a genuine in-game
// hang until someone went looking for it.
func resumeFallbackWarning(attempt int, expectedResume bool, fallbackReason string) (warn bool, message string) {
	if !expectedResume {
		return false, ""
	}
	reason := fallbackReason
	if reason == "" {
		reason = "resume not attempted"
	}
	return true, fmt.Sprintf("attempt %d expected to resume but is starting from a fresh cartridge (%s)", attempt, reason)
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
	// The resume pair becomes this attempt's checkpoint ring, which the uploader
	// publishes back to the wall. Only a complete pair may land there: an empty
	// state is what a reader observes mid-write, and materializing one would
	// both fail LoadState and re-publish the empty file under the new attempt,
	// which is how one bad read used to wedge a run's resume lineage forever.
	if err := farm.ValidateCheckpointState(cp.State); err != nil {
		return err
	}
	if len(cp.State.Data) == 0 {
		return fmt.Errorf("resume checkpoint %s carries no state bytes", cp.State.Name)
	}
	if len(cp.Knowledge.Data) == 0 {
		return fmt.Errorf("resume checkpoint %s carries no knowledge bytes", cp.State.Name)
	}
	statePath := filepath.Join(dir, cp.State.Name)
	knowledgePath := filepath.Join(dir, cp.Knowledge.Name)
	if err := writeFileAtomic(statePath, cp.State.Data); err != nil {
		return fmt.Errorf("write resume state: %w", err)
	}
	if err := writeFileAtomic(knowledgePath, cp.Knowledge.Data); err != nil {
		_ = os.Remove(statePath)
		return fmt.Errorf("write resume knowledge: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(dir, farmResumeMarker), []byte(cp.State.Name+"\n")); err != nil {
		_ = os.Remove(statePath)
		_ = os.Remove(knowledgePath)
		return fmt.Errorf("write resume marker: %w", err)
	}
	return nil
}

// discardFarmResume removes a resume pair that this attempt could not use. A
// state the runner itself rejected must not stay in the checkpoint ring: the
// uploader would publish it as this attempt's own checkpoint and the next lease
// would load it again.
func discardFarmResume(dir string, cp farm.ResumeCheckpoint) {
	if dir == "" {
		return
	}
	names := []string{cp.State.Name}
	if cp.Knowledge != nil {
		names = append(names, cp.Knowledge.Name)
	}
	for _, name := range names {
		if name == "" || filepath.Base(name) != name {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
	_ = os.Remove(filepath.Join(dir, farmResumeMarker))
}

// writeFileAtomic writes data to path through a same-directory temp file and a
// rename. The checkpoint dir is scanned by a timer-driven uploader, so a reader
// must never observe a half-written state; that is what os.WriteFile allows and
// what published a complete checkpoint as a 0-byte one.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".ckpt-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
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
