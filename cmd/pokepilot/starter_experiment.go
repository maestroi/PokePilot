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
	cartridge, _, cartridgeErr := profiles.DetectCartridge(base)
	if cartridgeErr == nil && (string(cartridge.ID()) == "tetris" || string(cartridge.ID()) == "boxxle") {
		// Non-Pokémon cartridges have no starter experiment: reload the base
		// ROM byte-identically and skip the Red starter-patch path.
		if strings.TrimSpace(spec.Starter) != "" {
			return fmt.Errorf("%s does not use a starter, got %q", cartridge.ID(), spec.Starter)
		}
		starterExperimentRuns.Delete(spec.RunID)
		if err := m.LoadROMBytes(base, string(cartridge.ID())); err != nil {
			return fmt.Errorf("reload %s ROM: %w", cartridge.ID(), err)
		}
		return nil
	}

	if cartridgeErr == nil && isGen2GameID(string(cartridge.ID())) {
		starter := strings.ToLower(strings.TrimSpace(spec.Starter))
		if !isGen2StarterRequest(starter) {
			return fmt.Errorf("%s starter must be chikorita, cyndaquil, or totodile; got %q", cartridge.ID(), spec.Starter)
		}
		// Gold/Silver starter selection is owned by the cartridge opening script
		// and the GS objective adapter. Never run the Gen-I ROM-patch experiment
		// path against a Gen-II cartridge.
		starterExperimentRuns.Delete(spec.RunID)
		if err := m.LoadROMBytes(base, string(cartridge.ID())); err != nil {
			return fmt.Errorf("reload %s ROM: %w", cartridge.ID(), err)
		}
		return nil
	}

	profile, _, detectErr := profiles.Detect(base)

	var selection redstarter.Selection
	var err error
	if detectErr == nil && string(profile.ID()) == "pokemon-yellow" {
		// Yellow's starter is a cartridge script, not a Red ROM experiment.
		// Ignore stale Red-style starter metadata from older persisted leases:
		// the cartridge always provides Pikachu and must remain byte-identical.
		selection, err = redstarter.Resolve("", spec.Seed)
		if err == nil {
			selection.Request = "pikachu"
			selection.Species = "pikachu"
			selection.Slot = "scripted"
		}
	} else {
		selection, err = redstarter.Resolve(spec.Starter, spec.Seed)
	}
	if err != nil {
		return err
	}
	derived, patch, err := redstarter.Patch(base, selection)
	if err != nil {
		return err
	}
	name := "pokemon-red"
	if detectErr == nil {
		name = string(profile.ID())
	}
	if selection.Experiment() {
		name = fmt.Sprintf("pokemon-red-starter-%s-%02x", selection.Mode, selection.Raw)
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
