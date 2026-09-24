package agent

import gameruntime "github.com/maestroi/pokepilot/game"

// redObjectiveProgressionPrerequisites is the adapter-owned ordering contract
// for story objectives whose native skills assume earlier semantic stages.
// The generic runtime never learns these Red-specific relationships.
func redObjectiveProgressionPrerequisites(o Objective) []ProgressID {
	if o.Kind == KindGym && o.Place == "viridian gym" {
		return []ProgressID{ProgressViridianGymOpen}
	}
	if o.Kind != KindProgress {
		return nil
	}

	switch o.Progress {
	case ProgressSecretKeyOwned:
		return []ProgressID{
			redProgressFuchsiaProgressionComplete,
			redProgressSilphRescueComplete,
			redProgressMarshBadge,
		}
	case redProgressVolcanoBadge:
		return []ProgressID{ProgressSecretKeyOwned}
	case redProgressEarthBadge:
		return []ProgressID{redProgressVolcanoBadge}
	case ProgressRoute22RivalResolved:
		return []ProgressID{redProgressEarthBadge}
	case ProgressRoute23BadgeChecks:
		return []ProgressID{ProgressRoute22RivalResolved}
	case redProgressVictoryRoadCleared:
		return []ProgressID{ProgressRoute23BadgeChecks}
	case redProgressIndigoPlateauReady:
		return []ProgressID{redProgressVictoryRoadCleared}
	case ProgressLeagueChallengeStarted:
		return []ProgressID{redProgressIndigoPlateauReady}
	case redProgressLeagueLoreleiDefeated:
		return []ProgressID{ProgressLeagueChallengeStarted}
	case redProgressLeagueBrunoDefeated:
		return []ProgressID{ProgressLeagueChallengeStarted, redProgressLeagueLoreleiDefeated}
	case redProgressLeagueAgathaDefeated:
		return []ProgressID{
			ProgressLeagueChallengeStarted,
			redProgressLeagueLoreleiDefeated,
			redProgressLeagueBrunoDefeated,
		}
	case redProgressLeagueLanceDefeated:
		return []ProgressID{
			ProgressLeagueChallengeStarted,
			redProgressLeagueLoreleiDefeated,
			redProgressLeagueBrunoDefeated,
			redProgressLeagueAgathaDefeated,
		}
	case ProgressLeagueChampionDefeated:
		return []ProgressID{
			ProgressLeagueChallengeStarted,
			redProgressLeagueLoreleiDefeated,
			redProgressLeagueBrunoDefeated,
			redProgressLeagueAgathaDefeated,
			redProgressLeagueLanceDefeated,
		}
	case ProgressMainStoryComplete:
		return []ProgressID{ProgressLeagueChampionDefeated}
	default:
		return nil
	}
}

func redMissingProgressionPrerequisites(o Objective, obs Observation) []ProgressID {
	required := redObjectiveProgressionPrerequisites(o)
	missing := make([]ProgressID, 0, len(required))
	for _, id := range required {
		if id != "" && !obs.Story.Has(id) {
			missing = append(missing, id)
		}
	}
	return missing
}

func redProgressionRecoverySafe(id ProgressID, obs Observation) bool {
	o := Objective{Kind: KindProgress, Progress: id}
	if len(redMissingProgressionPrerequisites(o, obs)) != 0 {
		return false
	}
	switch id {
	case ProgressSecretKeyOwned,
		redProgressEarthBadge,
		ProgressRoute22RivalResolved,
		ProgressRoute23BadgeChecks,
		redProgressVictoryRoadCleared,
		redProgressIndigoPlateauReady,
		ProgressLeagueChallengeStarted,
		redProgressLeagueLoreleiDefeated,
		redProgressLeagueBrunoDefeated,
		redProgressLeagueAgathaDefeated,
		redProgressLeagueLanceDefeated,
		ProgressLeagueChampionDefeated,
		ProgressMainStoryComplete:
		return true
	default:
		return false
	}
}

// RecoveryForProgressionPrerequisite maps portable facts back to their Red
// objective owners. A direct KindProgress owner is generic; this adapter hook
// additionally handles facts produced by another objective shape (Marsh Badge,
// Viridian Gym open) and explicitly opts a small set of resumable story stages
// into recovery-only synthesis.
func (a *redObjectiveAdapter) RecoveryForProgressionPrerequisite(
	id ProgressID,
	obs Observation,
) (PrerequisiteRecoveryLink, bool) {
	switch id {
	case redProgressMarshBadge:
		if objectives := redMarshBadgeObjectives(obs); len(objectives) != 0 {
			return PrerequisiteRecoveryLink{Objective: objectives[0]}, true
		}
		return PrerequisiteRecoveryLink{}, false
	case ProgressViridianGymOpen:
		return PrerequisiteRecoveryLink{
			Objective: Objective{Kind: KindProgress, Progress: redProgressEarthBadge},
		}, true
	default:
		if !redProgressionKnown(id) {
			return PrerequisiteRecoveryLink{}, false
		}
		return PrerequisiteRecoveryLink{
			Objective:    Objective{Kind: KindProgress, Progress: id},
			RecoverySafe: redProgressionRecoverySafe(id, obs),
		}, true
	}
}

func progressionPrerequisiteError(ids []ProgressID) error {
	if len(ids) == 0 {
		return nil
	}
	missing := make([]gameruntime.Prerequisite, 0, len(ids))
	for _, id := range ids {
		missing = append(missing, gameruntime.ProgressionPrerequisite(id))
	}
	return &gameruntime.PrerequisiteMissingError{Missing: missing}
}
