package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/profiles"
	redstarter "github.com/maestroi/pokepilot/red/starter"
)

type starterExperimentRunMeta struct {
	selection redstarter.Selection
	patch     redstarter.PatchInfo
}

var starterExperimentRuns sync.Map // run id -> starterExperimentRunMeta

// prepareStarterExperiment derives the exact cartridge image this lease must
// execute and reloads GomeBoy with it before any boot/checkpoint state is
// restored. It always reloads, including vanilla, so a worker that just ran a
// Mewtwo experiment cannot leak that cartridge into its next lease.
func prepareStarterExperiment(m *emu.Emu, spec farm.Spec) error {
	base := m.ROM() // semantic/base ROM, even when the previous lease was patched
	selection, err := redstarter.Resolve(spec.Starter, spec.Seed)
	if err != nil {
		return err
	}
	if !selection.Experiment() {
		// A vanilla lease only restores the unpatched cartridge. It must not
		// go through the Oak-ball patcher, which is pinned to Red's image:
		// Blue and Yellow have no experiment to apply and would otherwise be
		// refused before they boot.
		profile, _, err := profiles.Detect(base)
		if err != nil {
			return fmt.Errorf("detect base ROM: %w", err)
		}
		if err := m.LoadROMBytes(base, string(profile.ID())); err != nil {
			return fmt.Errorf("reload base ROM: %w", err)
		}
		sum := sha1.Sum(base)
		digest := hex.EncodeToString(sum[:])
		starterExperimentRuns.Store(spec.RunID, starterExperimentRunMeta{
			selection: selection,
			patch:     redstarter.PatchInfo{BaseSHA1: digest, EffectiveSHA1: digest, Description: "vanilla ROM"},
		})
		return nil
	}
	derived, patch, err := redstarter.Patch(base, selection)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("pokemon-red-starter-%s-%02x", selection.Mode, selection.Raw)
	if err := m.LoadDerivedROM(base, derived, name); err != nil {
		return fmt.Errorf("load derived ROM: %w", err)
	}
	starterExperimentRuns.Store(spec.RunID, starterExperimentRunMeta{selection: selection, patch: patch})
	m.TraceNote("starter", selection.Summary()+" · "+patch.Description)
	return nil
}

func starterExperimentMetadata(runID string) map[string]string {
	value, ok := starterExperimentRuns.Load(runID)
	if !ok {
		return nil
	}
	meta := value.(starterExperimentRunMeta)
	out := map[string]string{
		"starter_mode":       string(meta.selection.Mode),
		"rom_base_sha1":      meta.patch.BaseSHA1,
		"rom_effective_sha1": meta.patch.EffectiveSHA1,
		"rom_patch":          meta.patch.Description,
	}
	if meta.selection.Species != "" {
		out["starter_species"] = string(meta.selection.Species)
	}
	if meta.selection.Slot != "" {
		out["starter_slot"] = meta.selection.Slot
	}
	if meta.selection.Pool != "" {
		out["starter_pool"] = string(meta.selection.Pool)
	}
	if len(meta.patch.Changes) > 0 {
		changes := make([]string, 0, len(meta.patch.Changes))
		for _, change := range meta.patch.Changes {
			changes = append(changes, fmt.Sprintf("0x%x:%02x>%02x", change.Offset, change.From, change.To))
		}
		out["rom_patch_bytes"] = strings.Join(changes, ",")
	}
	return out
}
