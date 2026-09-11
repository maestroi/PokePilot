package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// TestRedProgressionWithholdsSilphScopeAndPokeFluteBeforeThunderBadge:
// run-g9ojxmtgvrff1ezck9g7t1o7x got stuck chasing "Acquire the Poke Flute"
// at 2 badges while Lt. Surge — reachable, Cut already usable — was never
// attempted. The vanilla critical path clears Surge before Celadon/Lavender;
// Erika (Rainbow Badge) already requires the Thunder Badge here. Silph Scope
// and the Poke Flute sit on the same leg of that path and want the same gate.
func TestRedProgressionWithholdsSilphScopeAndPokeFluteBeforeThunderBadge(t *testing.T) {
	rocketHideoutMap := uint8(0xC7) // Rocket Hideout B1F; skill.RocketHideoutAvailable
	pokemonTowerMap := uint8(0x90)  // Pokemon Tower 3F; skill.PokemonTowerAvailable

	base := Observation{PartyCount: 1, Party: []PartyMon{{Level: 30, HP: 80, MaxHP: 80}}}

	without := base
	without.Map = rocketHideoutMap
	if got := countProgress(redProgressionObjectives(without), redProgressSilphScopeAcquired); got != 0 {
		t.Errorf("Rocket Hideout offers Silph Scope progression %d times without the Thunder Badge, want 0", got)
	}

	without.Map = pokemonTowerMap
	without.Story = ProgressState{{ID: redProgressSilphScopeAcquired, Complete: true}}
	if got := countProgress(redProgressionObjectives(without), redProgressPokeFluteAcquired); got != 0 {
		t.Errorf("Pokemon Tower offers Poke Flute progression %d times without the Thunder Badge, want 0", got)
	}

	with := without
	with.Badges = []string{state.BadgeThunder.String()}
	with.Map = rocketHideoutMap
	with.Story = nil
	if got := countProgress(redProgressionObjectives(with), redProgressSilphScopeAcquired); got != 1 {
		t.Errorf("Rocket Hideout offers Silph Scope progression %d times with the Thunder Badge, want 1", got)
	}

	with.Map = pokemonTowerMap
	with.Story = ProgressState{{ID: redProgressSilphScopeAcquired, Complete: true}}
	if got := countProgress(redProgressionObjectives(with), redProgressPokeFluteAcquired); got != 1 {
		t.Errorf("Pokemon Tower offers Poke Flute progression %d times with the Thunder Badge, want 1", got)
	}
}
