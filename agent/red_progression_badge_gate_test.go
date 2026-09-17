package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// The deterministic story chain now treats Erika as a positive prerequisite
// for the Rocket Hideout/Pokemon Tower leg. Thunder still opens the road to
// Celadon, but Rainbow must be committed before the chain can continue east.
func TestRedProgressionWithholdsSilphScopeAndPokeFluteBeforeRainbowBadge(t *testing.T) {
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

	thunderOnly := base
	thunderOnly.Badges = []string{state.BadgeThunder.String()}
	thunderOnly.Map = rocketHideoutMap
	if got := countProgress(redProgressionObjectives(thunderOnly), redProgressSilphScopeAcquired); got != 0 {
		t.Errorf("Rocket Hideout offers Silph Scope progression %d times before Rainbow Badge, want 0", got)
	}

	thunderOnly.Map = pokemonTowerMap
	thunderOnly.Story = ProgressState{{ID: redProgressSilphScopeAcquired, Complete: true}}
	if got := countProgress(redProgressionObjectives(thunderOnly), redProgressPokeFluteAcquired); got != 0 {
		t.Errorf("Pokemon Tower offers Poke Flute progression %d times before Rainbow Badge, want 0", got)
	}

	with := base
	with.Badges = []string{state.BadgeThunder.String(), state.BadgeRainbow.String()}
	with.Map = rocketHideoutMap
	if got := countProgress(redProgressionObjectives(with), redProgressSilphScopeAcquired); got != 1 {
		t.Errorf("Rocket Hideout offers Silph Scope progression %d times with Rainbow Badge, want 1", got)
	}

	with.Map = pokemonTowerMap
	with.Story = ProgressState{{ID: redProgressSilphScopeAcquired, Complete: true}}
	if got := countProgress(redProgressionObjectives(with), redProgressPokeFluteAcquired); got != 1 {
		t.Errorf("Pokemon Tower offers Poke Flute progression %d times with Rainbow Badge, want 1", got)
	}
}

// Cascade is not merely a negative Cut gate anymore: once HM01 exists, the
// chain positively tells the agent to earn the badge that makes Cut legal.
func TestRedProgressionProducesCascadeBadgeBeforeThunderBadge(t *testing.T) {
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
	if got := countProgress(redProgressionObjectives(without), redProgressCascadeBadge); got != 1 {
		t.Errorf("offers cascade_badge progression %d times after HM01 without Cascade, want 1", got)
	}
	if got := countProgress(redProgressionObjectives(without), redProgressThunderBadge); got != 0 {
		t.Errorf("offers thunder_badge progression %d times without Cut unlocked, want 0", got)
	}

	with := base
	with.Badges = []string{state.BadgeCascade.String()}
	with.FieldCapabilities = []FieldCapability{{Name: "cut", BadgeOwned: true, HMOwned: true}}
	if got := countProgress(redProgressionObjectives(with), redProgressCascadeBadge); got != 0 {
		t.Errorf("offers cascade_badge progression %d times after Cascade is owned, want 0", got)
	}
	if got := countProgress(redProgressionObjectives(with), redProgressThunderBadge); got != 1 {
		t.Errorf("offers thunder_badge progression %d times with Cut unlocked, want 1", got)
	}
}

func TestRedProgressionProducesMarshBadgeAfterSilphRescue(t *testing.T) {
	without := Observation{
		Story: ProgressState{{ID: redProgressSilphRescueComplete, Complete: true}},
	}
	if got := countProgress(redProgressionObjectives(without), redProgressMarshBadge); got != 1 {
		t.Fatalf("offers marsh_badge progression %d times after Silph rescue without Marsh, want 1", got)
	}
	if got := countProgress(redProgressionObjectives(without), ProgressSecretKeyOwned); got != 0 {
		t.Fatalf("offers secret_key_owned %d times before Marsh Badge, want 0", got)
	}

	with := without
	with.Badges = []string{state.BadgeMarsh.String()}
	if got := countProgress(redProgressionObjectives(with), redProgressMarshBadge); got != 0 {
		t.Fatalf("offers marsh_badge progression %d times after Marsh is owned, want 0", got)
	}
	if got := countProgress(redProgressionObjectives(with), ProgressSecretKeyOwned); got != 1 {
		t.Fatalf("offers secret_key_owned %d times after Marsh Badge, want 1", got)
	}
}

func TestRedProgressStateProjectsCascadeAndMarshBadges(t *testing.T) {
	var mem state.Mem
	mem[sym.ObtainedBadges] = (1 << uint8(state.BadgeCascade)) | (1 << uint8(state.BadgeMarsh))
	progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{})
	if !progress.Has(redProgressCascadeBadge) {
		t.Fatal("Cascade Badge bit did not project into cascade_badge semantic progress")
	}
	if !progress.Has(redProgressMarshBadge) {
		t.Fatal("Marsh Badge bit did not project into marsh_badge semantic progress")
	}
}
