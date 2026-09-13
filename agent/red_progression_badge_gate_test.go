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

// TestRedProgressionWithholdsThunderBadgeBeforeCascadeBadge: farm runs with
// only the Boulder Badge repeatedly picked "progress thunder_badge" (offered
// as soon as HM01 was in hand) and died preparing Cut. Gen I cannot use Cut
// until its badge and HM are both owned, so the offer follows that
// capability rather than a named gym order.
func TestRedProgressionWithholdsThunderBadgeBeforeCascadeBadge(t *testing.T) {
	base := Observation{
		PartyCount: 1,
		Party:      []PartyMon{{Level: 20, HP: 60, MaxHP: 60}},
		Story:      ProgressState{{ID: redProgressHM01Acquired, Complete: true}},
		FieldCapabilities: []FieldCapability{{
			Name:    "cut",
			HMOwned: true,
		}},
	}

	without := base
	without.FieldCapabilities[0].BadgeOwned = false
	if got := countProgress(redProgressionObjectives(without), redProgressThunderBadge); got != 0 {
		t.Errorf("offers thunder_badge progression %d times without Cut unlocked, want 0", got)
	}

	with := base
	with.FieldCapabilities = []FieldCapability{{Name: "cut", BadgeOwned: true, HMOwned: true}}
	if got := countProgress(redProgressionObjectives(with), redProgressThunderBadge); got != 1 {
		t.Errorf("offers thunder_badge progression %d times with Cut unlocked, want 1", got)
	}
}
