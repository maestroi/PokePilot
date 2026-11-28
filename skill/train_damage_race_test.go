package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

// run-1lr7kwvnvn52m1msa6de9nskdu: a L7 Zubat's Leech Life takes 1 HP per turn
// off a Route 2 Pidgey that hits back for about 5. Direct training must flee
// that race instead of holding the lead to a blackout, and keep fighting a
// race it is winning.
func TestTrainingDamageRaceFleesALosingWildFight(t *testing.T) {
	turn := func(lead, foe uint16) game.BattleState { return game.BattleState{ActiveHP: lead, EnemyHP: foe} }

	var pidgey trainingDamageRace
	if pidgey.lost(turn(24, 20)) {
		t.Fatal("first main menu has no exchange to judge")
	}
	if !pidgey.lost(turn(20, 19)) {
		t.Fatal("Zubat 24->20 vs Pidgey 20->19: lead faints first, want flee")
	}

	var rattata trainingDamageRace
	rattata.lost(turn(24, 18))
	if rattata.lost(turn(20, 13)) {
		t.Fatal("Zubat 24->20 vs Rattata 18->13: lead wins the race, want fight")
	}

	var drain trainingDamageRace
	drain.lost(turn(20, 20))
	if drain.lost(turn(21, 19)) {
		t.Fatal("drain healed the lead above its start: nothing lost, want fight")
	}
}
