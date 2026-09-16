package main

import (
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
	derived, patch, err := redstarter.Patch(base, selection)
	if err != nil {
		return err
	}
	// The derived cartridge is labelled with the image's own game id, so a
	// Blue starter experiment is not filed under Red in replays and traces.
	name := "pokemon-red"
	if profile, _, err := profiles.Detect(base); err == nil {
		name = string(profile.ID())
	}
	if selection.Experiment() {
		name = fmt.Sprintf("%s-starter-%s-%02x", name, selection.Mode, selection.Raw)
	}
	if err := m.LoadDerivedROM(base, derived, name); err != nil {
		return fmt.Errorf("load derived ROM: %w", err)
	}
	starterExperimentRuns.Store(spec.RunID, starterExperimentRunMeta{selection: selection, patch: patch})
	if selection.Experiment() {
		m.TraceNote("starter", selection.Summary()+" · "+patch.Description)
	}
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
