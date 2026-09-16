package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Coverage is the generic, persisted breadth summary for a run. It deliberately
// describes gameplay surfaces rather than Pokemon Red implementation details so
// future game adapters can feed the same contract.
type Coverage struct {
	UniqueMapsVisited     int
	TrainersDefeated      int
	NPCInteractions       int
	UniqueItemsAcquired   int
	UniqueItemsUsed       int
	DexOwned              int
	DexSeen               int
	OptionalMilestones    int
	TMsHMsAcquired        int
	TMsHMsUsed            int
	Evolutions            int
	Catches               int
	UniqueSpeciesAcquired int
}

const coverageFileVersion = 1

type coverageTracker struct {
	ItemsAcquired   map[ItemID]bool
	ItemsUsed       map[ItemID]bool
	SpeciesAcquired map[SpeciesID]bool
	Trainers        map[string]bool
	Milestones      map[string]bool
	MachinesAcquired map[ItemID]bool
	MachinesUsed     map[ItemID]bool
	Evolutions       map[string]bool
	Catches          int
}

type coverageFile struct {
	Version          int                `json:"version"`
	ItemsAcquired    map[ItemID]bool    `json:"items_acquired,omitempty"`
	ItemsUsed        map[ItemID]bool    `json:"items_used,omitempty"`
	SpeciesAcquired  map[SpeciesID]bool `json:"species_acquired,omitempty"`
	Trainers         map[string]bool    `json:"trainers,omitempty"`
	Milestones       map[string]bool    `json:"optional_milestones,omitempty"`
	MachinesAcquired map[ItemID]bool    `json:"machines_acquired,omitempty"`
	MachinesUsed     map[ItemID]bool    `json:"machines_used,omitempty"`
	Evolutions       map[string]bool    `json:"evolutions,omitempty"`
	Catches          int                `json:"catches,omitempty"`
}

func newCoverageTracker() *coverageTracker {
	c := &coverageTracker{}
	c.ensure()
	return c
}

func (c *coverageTracker) ensure() {
	if c.ItemsAcquired == nil {
		c.ItemsAcquired = map[ItemID]bool{}
	}
	if c.ItemsUsed == nil {
		c.ItemsUsed = map[ItemID]bool{}
	}
	if c.SpeciesAcquired == nil {
		c.SpeciesAcquired = map[SpeciesID]bool{}
	}
	if c.Trainers == nil {
		c.Trainers = map[string]bool{}
	}
	if c.Milestones == nil {
		c.Milestones = map[string]bool{}
	}
	if c.MachinesAcquired == nil {
		c.MachinesAcquired = map[ItemID]bool{}
	}
	if c.MachinesUsed == nil {
		c.MachinesUsed = map[ItemID]bool{}
	}
	if c.Evolutions == nil {
		c.Evolutions = map[string]bool{}
	}
}

// seed records durable state already present in an observation. This makes a
// resumed run start from its real inventory/Dex state even when resuming an old
// checkpoint that predates the coverage sidecar. ProgressEarly remains the
// baseline, so run-level deltas still describe only newly covered surfaces.
func (c *coverageTracker) seed(obs Observation) {
	if c == nil {
		return
	}
	c.ensure()
	for _, item := range obs.Bag {
		id := ItemID(strings.ToLower(strings.TrimSpace(item.Name)))
		if id == "" || item.Quantity <= 0 {
			continue
		}
		c.ItemsAcquired[id] = true
		if coverageMachineItem(id) {
			c.MachinesAcquired[id] = true
		}
	}
	for species := range pokedexOwnedSet(obs) {
		if species != "" {
			c.SpeciesAcquired[species] = true
		}
	}
}

// noteSuccess records semantic coverage that cannot be recovered from a later
// RAM snapshot after the fact (for example a defeated trainer or a consumed
// item). The objective is already verified successful when this is called.
func (c *coverageTracker) noteSuccess(o Objective, before, after Observation) {
	if c == nil {
		return
	}
	c.ensure()
	c.seed(after)

	milestone := func(prefix string) {
		c.Milestones[prefix+":"+objectiveStorageKey(o)] = true
	}

	switch o.Kind {
	case KindTalk:
		milestone("npc")
	case KindTrainer:
		c.Trainers[objectiveStorageKey(o)] = true
		milestone("trainer")
	case KindPickup:
		milestone("pickup")
	case KindBuy:
		milestone("purchase")
	case KindCatch:
		c.Catches++
		milestone("catch")
	case KindUseItem:
		if o.Item != "" {
			c.ItemsUsed[o.Item] = true
			if coverageMachineItem(o.Item) {
				c.MachinesUsed[o.Item] = true
			}
		}
		milestone("item-use")
	case KindTrain:
		if keys := coverageEvolutionKeys(o, before, after); len(keys) > 0 {
			for _, key := range keys {
				c.Evolutions[key] = true
			}
			milestone("evolution")
		}
	}
}

func (c *coverageTracker) snapshot(obs Observation, known *Knowledge) Coverage {
	if c == nil {
		c = newCoverageTracker()
	}
	c.ensure()
	owned := pokedexOwnedSet(obs)
	seen := speciesSet(obs.PokedexSeen)
	for species := range owned {
		seen[species] = true
	}

	return Coverage{
		UniqueMapsVisited:     coverageVisitedMaps(known),
		TrainersDefeated:      len(c.Trainers),
		NPCInteractions:       coverageNPCInteractions(known),
		UniqueItemsAcquired:   len(c.ItemsAcquired),
		UniqueItemsUsed:       len(c.ItemsUsed),
		DexOwned:              len(owned),
		DexSeen:               len(seen),
		OptionalMilestones:    len(c.Milestones),
		TMsHMsAcquired:        len(c.MachinesAcquired),
		TMsHMsUsed:            len(c.MachinesUsed),
		Evolutions:            len(c.Evolutions),
		Catches:               c.Catches,
		UniqueSpeciesAcquired: len(c.SpeciesAcquired),
	}
}

func coverageVisitedMaps(known *Knowledge) int {
	if known == nil {
		return 0
	}
	return len(known.Visited)
}

func coverageNPCInteractions(known *Knowledge) int {
	if known == nil {
		return 0
	}
	total := 0
	for _, tiles := range known.Talked {
		total += len(tiles)
	}
	return total
}

func coverageMachineItem(id ItemID) bool {
	name := strings.ToLower(strings.TrimSpace(string(id)))
	return strings.HasPrefix(name, "tm") || strings.HasPrefix(name, "hm")
}

func coverageEvolutionKeys(o Objective, before, after Observation) []string {
	keys := []string{}
	limit := len(before.Party)
	if len(after.Party) < limit {
		limit = len(after.Party)
	}
	for i := 0; i < limit; i++ {
		from, to := before.Party[i].Species, after.Party[i].Species
		if from == "" || to == "" || from == to {
			continue
		}
		keys = append(keys, fmt.Sprintf("%s>%s", from, to))
	}
	if len(keys) == 0 && o.Intent == "dex-evolution" {
		beforeOwned := pokedexOwnedSet(before)
		for species := range pokedexOwnedSet(after) {
			if !beforeOwned[species] {
				keys = append(keys, "dex:"+string(species))
			}
		}
	}
	return keys
}

func coveragePathForState(statePath string) string {
	base := strings.TrimSuffix(filepath.Base(statePath), ".state")
	return filepath.Join(filepath.Dir(statePath), fmt.Sprintf("%s.coverage-v%d.json", base, coverageFileVersion))
}

func isCoverageName(name string) bool {
	return strings.HasSuffix(name, fmt.Sprintf(".coverage-v%d.json", coverageFileVersion))
}

func writeCoverageFile(statePath string, c *coverageTracker) error {
	if c == nil {
		c = newCoverageTracker()
	}
	c.ensure()
	data, err := json.Marshal(coverageFile{
		Version:          coverageFileVersion,
		ItemsAcquired:    c.ItemsAcquired,
		ItemsUsed:        c.ItemsUsed,
		SpeciesAcquired:  c.SpeciesAcquired,
		Trainers:         c.Trainers,
		Milestones:       c.Milestones,
		MachinesAcquired: c.MachinesAcquired,
		MachinesUsed:     c.MachinesUsed,
		Evolutions:       c.Evolutions,
		Catches:          c.Catches,
	})
	if err != nil {
		return fmt.Errorf("encode coverage: %w", err)
	}
	target := coveragePathForState(statePath)
	tmp, err := os.CreateTemp(filepath.Dir(statePath), ".coverage-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp coverage file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write coverage: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close coverage: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("rename coverage into place: %w", err)
	}
	return nil
}

func loadCoverageFile(statePath string, log io.Writer) *coverageTracker {
	c := newCoverageTracker()
	if !strings.HasSuffix(filepath.Base(statePath), ".state") {
		return c
	}
	path := coveragePathForState(statePath)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			logCoverage(log, "cannot read %s (%v); rebuilding observable coverage from resumed state", path, err)
		}
		return c
	}
	var stored coverageFile
	if err := json.Unmarshal(data, &stored); err != nil || stored.Version != coverageFileVersion {
		logCoverage(log, "invalid coverage sidecar beside %s; rebuilding observable coverage from resumed state", statePath)
		return c
	}
	c.ItemsAcquired = stored.ItemsAcquired
	c.ItemsUsed = stored.ItemsUsed
	c.SpeciesAcquired = stored.SpeciesAcquired
	c.Trainers = stored.Trainers
	c.Milestones = stored.Milestones
	c.MachinesAcquired = stored.MachinesAcquired
	c.MachinesUsed = stored.MachinesUsed
	c.Evolutions = stored.Evolutions
	c.Catches = stored.Catches
	c.ensure()
	return c
}

func logCoverage(log io.Writer, format string, args ...any) {
	if log != nil {
		fmt.Fprintf(log, "agent: coverage: "+format+"\n", args...)
	}
}
