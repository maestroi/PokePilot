package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func putSwitchMon(mem *state.Mem, slot int, mon state.Mon) {
	if slot+1 > int(mem.U8(sym.PartyCount)) {
		mem[sym.PartyCount] = uint8(slot + 1)
	}
	base := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize
	mem[base+sym.MonSpecies] = mon.Species
	mem[base+sym.MonLevel] = mon.Level
	mem[base+sym.MonStatus] = mon.Status
	mem[base+sym.MonType1] = mon.Type1
	mem[base+sym.MonType2] = mon.Type2
	put := func(off uint16, value uint16) {
		mem[base+off] = uint8(value >> 8)
		mem[base+off+1] = uint8(value)
	}
	put(sym.MonHP, mon.HP)
	put(sym.MonMaxHP, mon.MaxHP)
	put(sym.MonAttack, mon.Attack)
	put(sym.MonDefense, mon.Defense)
	put(sym.MonSpeed, mon.Speed)
	put(sym.MonSpecial, mon.Special)
	copy(mem.Slice(base+sym.MonMoves, 4), mon.Moves[:])
	copy(mem.Slice(base+sym.MonPP, 4), mon.PP[:])
}

func switchBattle(active state.Mon, enemyTypes [2]uint8, moves ...uint8) state.BattleState {
	b := state.BattleState{
		Kind:          state.BattleTrainer,
		ActiveSpecies: active.Species,
		ActiveLevel:   active.Level,
		ActiveHP:      active.HP,
		ActiveMaxHP:   active.MaxHP,
		ActiveAttack:  active.Attack,
		ActiveDefense: active.Defense,
		ActiveSpecial: active.Special,
		ActiveType1:   active.Type1,
		ActiveType2:   active.Type2,
		EnemySpecies:  95,
		EnemyLevel:    20,
		EnemyHP:       60,
		EnemyMaxHP:    60,
		EnemyAttack:   60,
		EnemyDefense:  60,
		EnemySpecial:  60,
		EnemyType1:    enemyTypes[0],
		EnemyType2:    enemyTypes[1],
	}
	for i, id := range moves {
		b.Moves[i] = state.Move{ID: id, PP: 20}
	}
	return b
}

func healthySwitchMon(species, level uint8, type1, type2 uint8, moves ...uint8) state.Mon {
	mon := state.Mon{
		Species: species, Level: level, HP: 60, MaxHP: 60,
		Type1: type1, Type2: type2,
		Attack: 60, Defense: 60, Speed: 60, Special: 60,
	}
	for i, id := range moves {
		mon.Moves[i], mon.PP[i] = id, 20
	}
	return mon
}

func TestChooseTacticalSwitchPrefersMaterialMatchup(t *testing.T) {
	tackle := rom.Move{ID: 33, Power: 35, Type: typeNormal, Accuracy: 255, PP: 35}
	bubble := rom.Move{ID: 145, Power: 20, Type: typeWater, Accuracy: 255, PP: 30}
	chart := []typePair{
		{typeNormal, typeRock, 5},
		{typeWater, typeRock, 20},
		{typeWater, typeGround, 20},
	}
	romData := fakeROMChart(t, chart, tackle, bubble)
	active := healthySwitchMon(1, 20, typeNormal, typeNormal, tackle.ID)
	bench := healthySwitchMon(7, 20, typeWater, typeWater, bubble.ID)
	var mem state.Mem
	putSwitchMon(&mem, 0, active)
	putSwitchMon(&mem, 1, bench)
	mem[sym.PlayerMonNumber] = 0
	b := switchBattle(active, [2]uint8{typeRock, typeGround}, tackle.ID)

	decision := chooseTacticalSwitch(romData, &mem, b)
	if !decision.Legal || !decision.Switch || decision.Slot != 1 {
		t.Fatalf("decision = %+v, want tactical switch to WATER slot 1", decision)
	}
	if decision.Reason != "material-matchup-improvement" {
		t.Fatalf("reason = %q, want material-matchup-improvement", decision.Reason)
	}
}

func TestChooseTacticalSwitchStaysForEquivalentCandidate(t *testing.T) {
	move := rom.Move{ID: 33, Power: 50, Type: typeNormal, Accuracy: 255, PP: 20}
	romData := fakeROM(t, move)
	active := healthySwitchMon(1, 20, typeNormal, typeNormal, move.ID)
	bench := healthySwitchMon(2, 20, typeNormal, typeNormal, move.ID)
	var mem state.Mem
	putSwitchMon(&mem, 0, active)
	putSwitchMon(&mem, 1, bench)
	mem[sym.PlayerMonNumber] = 0
	b := switchBattle(active, [2]uint8{typeNormal, typeNormal}, move.ID)

	decision := chooseTacticalSwitch(romData, &mem, b)
	if !decision.Legal || decision.Switch {
		t.Fatalf("decision = %+v, want deliberate stay for equivalent bench", decision)
	}
	if decision.Reason != "candidate-not-materially-better" {
		t.Fatalf("reason = %q, want candidate-not-materially-better", decision.Reason)
	}
}

func TestChooseTacticalSwitchIgnoresDisabledActiveMove(t *testing.T) {
	strong := rom.Move{ID: 33, Power: 120, Type: typeNormal, Accuracy: 255, PP: 5}
	weak := rom.Move{ID: 10, Power: 20, Type: typeNormal, Accuracy: 255, PP: 35}
	benchMove := rom.Move{ID: 1, Power: 60, Type: typeNormal, Accuracy: 255, PP: 20}
	romData := fakeROM(t, strong, weak, benchMove)
	active := healthySwitchMon(1, 20, typeNormal, typeNormal, strong.ID, weak.ID)
	bench := healthySwitchMon(2, 20, typeNormal, typeNormal, benchMove.ID)
	var mem state.Mem
	putSwitchMon(&mem, 0, active)
	putSwitchMon(&mem, 1, bench)
	mem[sym.PlayerMonNumber] = 0
	b := switchBattle(active, [2]uint8{typeNormal, typeNormal}, strong.ID, weak.ID)
	b.DisabledMove = 1

	decision := chooseTacticalSwitch(romData, &mem, b)
	if !decision.Switch || decision.Slot != 1 {
		t.Fatalf("decision = %+v, want bench switch when active's strong move is disabled", decision)
	}
	if decision.Active.BestMove.MoveID != weak.ID {
		t.Fatalf("active best move = %d, want usable weak move %d instead of disabled %d",
			decision.Active.BestMove.MoveID, weak.ID, strong.ID)
	}
}

func TestChooseTacticalSwitchRejectsCriticallyWeakBench(t *testing.T) {
	tackle := rom.Move{ID: 33, Power: 35, Type: typeNormal, Accuracy: 255, PP: 35}
	bubble := rom.Move{ID: 145, Power: 20, Type: typeWater, Accuracy: 255, PP: 30}
	chart := []typePair{{typeWater, typeRock, 20}, {typeWater, typeGround, 20}}
	romData := fakeROMChart(t, chart, tackle, bubble)
	active := healthySwitchMon(1, 20, typeNormal, typeNormal, tackle.ID)
	bench := healthySwitchMon(7, 20, typeWater, typeWater, bubble.ID)
	bench.HP, bench.MaxHP = 20, 100 // exactly 20%: voluntary filter
	var mem state.Mem
	putSwitchMon(&mem, 0, active)
	putSwitchMon(&mem, 1, bench)
	mem[sym.PlayerMonNumber] = 0
	b := switchBattle(active, [2]uint8{typeRock, typeGround}, tackle.ID)

	decision := chooseTacticalSwitch(romData, &mem, b)
	if decision.Switch {
		t.Fatalf("decision = %+v, critically weak bench must not be a voluntary switch-in", decision)
	}
}

func TestChooseTacticalSwitchRejectsFrozenBench(t *testing.T) {
	tackle := rom.Move{ID: 33, Power: 35, Type: typeNormal, Accuracy: 255, PP: 35}
	bubble := rom.Move{ID: 145, Power: 20, Type: typeWater, Accuracy: 255, PP: 30}
	chart := []typePair{{typeWater, typeRock, 20}, {typeWater, typeGround, 20}}
	romData := fakeROMChart(t, chart, tackle, bubble)
	active := healthySwitchMon(1, 20, typeNormal, typeNormal, tackle.ID)
	bench := healthySwitchMon(7, 20, typeWater, typeWater, bubble.ID)
	bench.Status = 1 << 5 // FRZ
	var mem state.Mem
	putSwitchMon(&mem, 0, active)
	putSwitchMon(&mem, 1, bench)
	mem[sym.PlayerMonNumber] = 0
	b := switchBattle(active, [2]uint8{typeRock, typeGround}, tackle.ID)

	decision := chooseTacticalSwitch(romData, &mem, b)
	if decision.Switch {
		t.Fatalf("decision = %+v, frozen bench must not be a voluntary switch-in", decision)
	}
}

func TestBestReplacementSlotUsesMatchupInsteadOfPartyOrder(t *testing.T) {
	tackle := rom.Move{ID: 33, Power: 35, Type: typeNormal, Accuracy: 255, PP: 35}
	bubble := rom.Move{ID: 145, Power: 20, Type: typeWater, Accuracy: 255, PP: 30}
	chart := []typePair{
		{typeNormal, typeRock, 5},
		{typeWater, typeRock, 20},
		{typeWater, typeGround, 20},
	}
	romData := fakeROMChart(t, chart, tackle, bubble)
	fainted := healthySwitchMon(1, 20, typeNormal, typeNormal, tackle.ID)
	fainted.HP = 0
	firstLive := healthySwitchMon(2, 20, typeNormal, typeNormal, tackle.ID)
	best := healthySwitchMon(7, 20, typeWater, typeWater, bubble.ID)
	var mem state.Mem
	putSwitchMon(&mem, 0, fainted)
	putSwitchMon(&mem, 1, firstLive)
	putSwitchMon(&mem, 2, best)
	mem[sym.PlayerMonNumber] = 0
	b := switchBattle(fainted, [2]uint8{typeRock, typeGround}, tackle.ID)

	slot, eval := bestReplacementSlot(romData, &mem, b)
	if slot != 2 {
		t.Fatalf("replacement slot = %d (%+v), want WATER slot 2 instead of first live slot", slot, eval)
	}
}

func TestBestReplacementPreservesMultiFieldMoveCarrierOnTie(t *testing.T) {
	surf := rom.Move{ID: 0x39, Power: 95, Type: typeWater, Accuracy: 255, PP: 15}
	cut := rom.Move{ID: 0x0f, Power: 50, Type: typeNormal, Accuracy: 242, PP: 30}
	romData := fakeROM(t, surf, cut)
	fainted := healthySwitchMon(1, 20, typeNormal, typeNormal, cut.ID)
	fainted.HP = 0
	utility := healthySwitchMon(7, 20, typeWater, typeWater, surf.ID, cut.ID)
	fighter := healthySwitchMon(8, 20, typeWater, typeWater, surf.ID)
	var mem state.Mem
	putSwitchMon(&mem, 0, fainted)
	putSwitchMon(&mem, 1, utility)
	putSwitchMon(&mem, 2, fighter)
	mem[sym.PlayerMonNumber] = 0
	b := switchBattle(fainted, [2]uint8{typeNormal, typeNormal}, cut.ID)

	slot, eval := bestReplacementSlot(romData, &mem, b)
	if slot != 2 {
		t.Fatalf("replacement slot = %d (%+v), want equal fighter slot 2 to preserve multi-field carrier", slot, eval)
	}
}
