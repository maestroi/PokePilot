package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSFullCampaignGoalTracksJohtoBadgesThenHallOfFame(t *testing.T) {
	for _, gameID := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		t.Run(string(gameID), func(t *testing.T) {
			for _, name := range []string{"elite-four", "hall-of-fame", "Reach the Hall of Fame."} {
				goal, structured, err := PlannerGoal(name)
				if err != nil || !structured || goal.Kind != GoalEliteFour {
					t.Fatalf("%s goal %q not a deterministic full campaign: %+v %v", gameID, name, goal, err)
				}
			}
			goal, _, _ := PlannerGoal("elite-four")
			obs := Observation{
				GameID: gameID,
				Badges: []string{"zephyr", "hive", "plain", "fog", "storm", "mineral", "glacier", "boulder", "cascade"},
				Story:  ProgressState{{ID: ProgressCampaignBadges, Value: 7}},
			}
			if got := EvaluateGoal(goal, obs); got.Complete || got.Current != 7 || got.Target != 9 {
				t.Fatalf("Kanto badges must not raise Johto campaign progress: %+v", got)
			}
			obs.Badges[7] = "rising"
			obs.Story = ProgressState{{ID: ProgressCampaignBadges, Complete: true, Value: 8}}
			if got := EvaluateGoal(goal, obs); got.Complete || got.Current != 8 || got.Target != 9 {
				t.Fatalf("eight Johto badges must leave Hall of Fame as ninth milestone: %+v", got)
			}
			obs.Story = append(obs.Story,
				ProgressFact{ID: gsprofile.ProgressLeagueLanceDefeated, Complete: true},
				ProgressFact{ID: gsprofile.ProgressLeagueHallOfFameEntered, Complete: true},
			)
			if got := EvaluateGoal(goal, obs); got.Complete {
				t.Fatalf("raw Gen2 ending fact cannot bypass the portable goal verifier: %+v", got)
			}
			obs.Story = append(obs.Story, ProgressFact{ID: ProgressMainStoryComplete, Complete: true})
			if got := EvaluateGoal(goal, obs); !got.Complete || got.Current != 9 || got.Target != 9 || got.Kind != GoalEliteFour {
				t.Fatalf("Gen2 verified Hall of Fame did not complete 9/9: %+v", got)
			}
			obs.Story[0].Value = 7
			obs.Story[0].Complete = false
			if got := EvaluateGoal(goal, obs); got.Complete || got.Current != 7 {
				t.Fatalf("Hall of Fame without eight Johto badges must not complete full campaign: %+v", got)
			}
		})
	}
}
