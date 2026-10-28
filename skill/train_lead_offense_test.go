package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

// The move set of run-1n4yivkvkl9hk272sbitqvmre2's Marowak: BONE_CLUB is its
// only direct damage, GROWL/LEER/BIDE make no HP progress. Against a Flying
// wild BONE_CLUB scores zero, so the held lead must yield rather than burn
// every PP pool toward STRUGGLE.
func TestTrainingLeadOffenseFromMoveRoles(t *testing.T) {
	const boneClub, growl, leer, bide, leechSeed = 125, 45, 43, 117, 73
	roles := map[uint16]game.BattleMoveRole{
		boneClub:  game.BattleMoveRoleDirectDamage,
		growl:     game.BattleMoveRoleOther,
		leer:      game.BattleMoveRoleSetup,
		bide:      game.BattleMoveRoleOther,
		leechSeed: game.BattleMoveRoleResidualDamage,
	}
	marowak := func(boneClubPP uint8, disabled bool) [4]game.BattleMove {
		return [4]game.BattleMove{
			{ID: boneClub, PP: boneClubPP, Disabled: disabled},
			{ID: growl, PP: 40},
			{ID: leer, PP: 30},
			{ID: bide, PP: 10},
		}
	}
	cases := []struct {
		name          string
		moves         [4]game.BattleMove
		boneClubScore int64
		want          trainingLeadOffense
	}{
		{"immune opponent keeps the lead's damaging PP", marowak(20, false), 0, trainingLeadOffense{effective: false, damaging: true}},
		{"damaging PP spent", marowak(0, false), 500, trainingLeadOffense{effective: false, damaging: false}},
		{"effective attack available", marowak(20, false), 500, trainingLeadOffense{effective: true, damaging: true}},
		{"only attack disabled", marowak(20, true), 500, trainingLeadOffense{effective: false, damaging: true}},
		{"residual damage is progress", [4]game.BattleMove{{ID: leechSeed, PP: 10}, {ID: growl, PP: 40}}, 0, trainingLeadOffense{effective: true, damaging: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			strategy := &fakeGen2CombatStrategy{
				scores: map[uint16]int64{boneClub: tc.boneClubScore},
				roles:  roles,
			}
			got := evaluateTrainingLeadOffense(strategy, nil, game.BattleState{Moves: tc.moves})
			if got != tc.want {
				t.Fatalf("offense = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestTrainEngagementsCountFledEncounters(t *testing.T) {
	if got := (TrainResult{Battles: 7, Fled: 53}).engagements(); got != 60 {
		t.Fatalf("engagements = %d, want 60", got)
	}
}
