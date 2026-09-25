package agent

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

// yellowSemanticObservationAdapter observes Yellow through the shared Gen-I
// enrichment. The cartridge binds the canonical memory view and its ROM
// tables, so live RAM and static data decode with the engine Red and Blue
// use. Yellow supplies what is its own: the profile observation (identity,
// story, Pikachu), its Dex catalog and its objective catalog.
type yellowSemanticObservationAdapter struct{}

func (yellowSemanticObservationAdapter) GameID() game.GameID { return yellowprofile.GameID }

func init() {
	registerSemanticObservationAdapter(yellowSemanticObservationAdapter{})
}

func (yellowSemanticObservationAdapter) Observe(m *emu.Emu, romData []byte, profile game.GameProfile) (Observation, error) {
	return observeGen1(m, romData, profile, gen1ObservationFacts{
		Dex:     buildYellowDexCatalog,
		Catalog: yellowObjectiveCatalog,
	})
}
