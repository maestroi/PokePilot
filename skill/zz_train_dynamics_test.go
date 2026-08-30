package skill_test

// Scratch measurement: how does a lone L5 starter do in Route 1 grass?
// Run: ZBAT=1 POKEMON_RED_ROM=... POKEPILOT_FIXTURE_DIR=/tmp/pokepilot-fixtures \
//   go test ./skill -run '^TestZZTrainDynamics$' -v > /tmp/zz_train.log 2>&1

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

func TestZZTrainDynamics(t *testing.T) {
	if testing.Short() {
		t.Skip("needs ROM and emulator")
	}
	m := fixture.Load(t, "post_starter")
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	for i, mon := range party.Mons {
		t.Logf("party[%d] species=%#02x level=%d hp=%d/%d", i, mon.Species, mon.Level, mon.HP, mon.MaxHP)
	}

	dest := route1Grass(t, m.ROM())
	tr, err := fixture.Travel(m, dest, skill.StatAwareMove(m.ROM()), 20)
	t.Logf("travel: battles=%d blackedOut=%v err=%v", tr.Battles, tr.BlackedOut, err)
	if err != nil {
		return
	}

	res, terr := skill.Train(m, m.ROM(), 12, skill.StatAwareMove(m.ROM()), 20)
	t.Logf("train: start=%d end=%d battles=%d reached=%v blackedOut=%v err=%v",
		res.StartLevel, res.EndLevel, res.Battles, res.Reached, res.BlackedOut, terr)

	state.Snapshot(m, &mem)
	party = state.DecodeParty(&mem)
	for i, mon := range party.Mons {
		t.Logf("after party[%d] species=%#02x level=%d hp=%d/%d fainted=%v", i, mon.Species, mon.Level, mon.HP, mon.MaxHP, mon.Fainted())
	}
}
