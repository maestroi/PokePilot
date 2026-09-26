package agent

import (
	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func yellowProgressionKnown(id ProgressID) bool {
	if id == yellowprofile.ProgressYellowLabRivalResolved {
		return true
	}
	_, ok := gen1EarlyProgressionExecutor(id)
	return ok
}

// ProgressionObjectives keeps Yellow ownership only where its story genuinely
// differs. Once the Pikachu/lab-rival opening is complete, Parcel/Pokedex and
// Brock use the same early-Kanto progression order as Red/Blue.
func (a *yellowObjectiveAdapter) ProgressionObjectives(obs Observation) []Objective {
	if !obs.Story.Has(yellowprofile.ProgressYellowLabRivalResolved) {
		if obs.PartyCount == 0 && !obs.Story.Has(yellowprofile.ProgressYellowStarterReceived) {
			return nil
		}
		return []Objective{{
			Kind:     KindProgress,
			Progress: yellowprofile.ProgressYellowLabRivalResolved,
			Note:     "(resume Yellow's scripted Pikachu opening and finish the lab rival sequence at a stable boundary)",
		}}
	}
	return gen1EarlyProgressionObjectives(obs, true)
}

// Keep the import anchored to the shared semantic vocabulary at compile time;
// Yellow must never invent cartridge-specific aliases for these common facts.
var _ = []ProgressID{gen1.ProgressPokedexAcquired, gen1.ProgressBoulderBadge}
