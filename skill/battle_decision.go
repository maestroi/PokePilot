package skill

import (
	"errors"
	"fmt"
	"math"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// This file is the Gen I adapter for the portable battle-decision contract
// (game.BattleDecisionState). It only DESCRIBES a turn: which moves,
// switches, items and RUN are legal right now. Executing a chosen action
// stays with Battle, SwitchActive and UseBattleMedicine.

// ErrBattleNotActionable reports a battle whose turn is not a FIGHT/ITEM/
// PKMN/RUN decision: no battle, the Safari Zone's BALL/BAIT/ROCK menu, or the
// Old Man's scripted catch demo.
var ErrBattleNotActionable = errors.New("skill: battle has no actionable turn decision")

// Gen1BattleSnapshot is the RAM-decoded input to BuildBattleDecisionState.
// Keeping it a plain value lets fixture tests describe a turn without an
// emulator.
type Gen1BattleSnapshot struct {
	Battle       state.BattleState
	BattleType   uint8
	Party        state.PartyState
	ActiveSlot   int
	ActiveStatus uint8
	EnemyStatus  uint8
	Bag          []state.BagItem
}

// BattleDecisionOptions says which non-move actions the caller permits.
// Items lists the permitted item ids; only those the adapter can prove would
// have an effect are ever declared legal.
type BattleDecisionOptions struct {
	Switches bool
	Items    []uint8
	Recent   []string
}

const (
	battleTypeNormal uint8 = 0
)

// BattleMedicineItems is the item set whose battle effect the adapter can
// verify from party RAM: ordinary HP and status medicine.
func BattleMedicineItems() []uint8 {
	items := make([]uint8, 0, len(hpMedicines)+6)
	for _, med := range hpMedicines {
		items = append(items, med.item)
	}
	return append(items, itemAntidote, itemBurnHeal, itemIceHeal, itemAwakening, itemParlyzHeal, itemFullHeal)
}

// ReadBattleSnapshot decodes the current turn's inputs from RAM.
func ReadBattleSnapshot(mem *state.Mem) (Gen1BattleSnapshot, error) {
	b := state.DecodeBattle(mem)
	if b == nil {
		return Gen1BattleSnapshot{}, fmt.Errorf("%w: no battle in progress", ErrBattleNotActionable)
	}
	return Gen1BattleSnapshot{
		Battle:       *b,
		BattleType:   mem.U8(sym.BattleType),
		Party:        state.DecodeParty(mem),
		ActiveSlot:   int(mem.U8(sym.PlayerMonNumber)),
		ActiveStatus: mem.U8(sym.BattleMonStatus),
		EnemyStatus:  mem.U8(sym.EnemyMonStatus),
		Bag:          state.DecodeInventory(mem).Items,
	}, nil
}

// BuildBattleDecisionState produces the complete legal turn for a Gen I
// battle at its main menu. romData supplies move metadata and the type chart;
// nil omits that metadata but never changes legality.
func BuildBattleDecisionState(romData []byte, snap Gen1BattleSnapshot, opts BattleDecisionOptions) (game.BattleDecisionState, error) {
	if snap.BattleType != battleTypeNormal {
		return game.BattleDecisionState{}, fmt.Errorf("%w: battle type %d", ErrBattleNotActionable, snap.BattleType)
	}
	b := snap.Battle
	s := game.BattleDecisionState{
		Context:    battleContext(b.Kind),
		ActiveSlot: snap.ActiveSlot,
		Active:     gen1ActiveMon(b, snap.ActiveStatus),
		Opponent: game.BattleMon{
			Species: gen1Species(b.EnemySpecies),
			Level:   int(b.EnemyLevel),
			HP:      int(b.EnemyHP),
			MaxHP:   int(b.EnemyMaxHP),
			Status:  state.Mon{Status: snap.EnemyStatus}.StatusName(),
			Types:   gen1Types(b.EnemyType1, b.EnemyType2),
		},
		Moves: gen1MoveOptions(romData, b),
		// Gen I never lets the player flee a trainer battle.
		CanRun: b.Kind == state.BattleWild,
	}
	if len(opts.Recent) > 0 {
		n := len(opts.Recent)
		if n > game.MaxBattleRecent {
			n = game.MaxBattleRecent
		}
		s.Recent = append([]string(nil), opts.Recent[len(opts.Recent)-n:]...)
	}

	if opts.Switches {
		for i, mon := range snap.Party.Mons {
			if i == snap.ActiveSlot {
				continue
			}
			sw := game.BattleSwitchOption{Slot: i, BattleMon: gen1PartyMon(mon)}
			if mon.Fainted() {
				sw.Unusable = game.BattleUnusableFainted
			}
			s.Switches = append(s.Switches, sw)
		}
	}

	for _, item := range opts.Items {
		qty := bagQuantity(snap.Bag, item)
		id, ok := data.Item(item)
		if qty == 0 || !ok {
			continue
		}
		for i, mon := range snap.Party.Mons {
			status := mon.Status
			if i == snap.ActiveSlot {
				// The battle copy is live; party status is written back later.
				status = snap.ActiveStatus
			}
			if medicineHasEffect(item, mon, status) {
				s.Items = append(s.Items, game.BattleItemOption{Item: id, Target: i, Quantity: int(qty)})
			}
		}
	}

	return s, s.Validate()
}

// MoveOnlyBattleDecisionState serves the MovePolicy seam, which sees only
// BattleState: the same move options with every non-move action withheld.
func MoveOnlyBattleDecisionState(romData []byte, b game.BattleState) (game.BattleDecisionState, error) {
	s := game.BattleDecisionState{
		Context: battleContext(b.Kind),
		Active:  gen1ActiveMon(b, 0),
		Opponent: game.BattleMon{
			Species: gen1Species(b.EnemySpecies),
			Level:   int(b.EnemyLevel),
			HP:      int(b.EnemyHP),
			MaxHP:   int(b.EnemyMaxHP),
			Types:   gen1Types(b.EnemyType1, b.EnemyType2),
		},
		Moves: gen1MoveOptions(romData, b),
	}
	return s, s.Validate()
}

func battleContext(kind game.BattleKind) game.BattleContext {
	if kind == game.BattleTrainer {
		return game.BattleContextTrainer
	}
	return game.BattleContextWild
}

func gen1ActiveMon(b game.BattleState, status uint8) game.BattleMon {
	return game.BattleMon{
		Species: gen1Species(b.ActiveSpecies),
		Level:   int(b.ActiveLevel),
		HP:      int(b.ActiveHP),
		MaxHP:   int(b.ActiveMaxHP),
		Status:  state.Mon{Status: status}.StatusName(),
		Types:   gen1Types(b.ActiveType1, b.ActiveType2),
	}
}

func gen1PartyMon(mon state.Mon) game.BattleMon {
	return game.BattleMon{
		Species: gen1Species(mon.Species),
		Level:   int(mon.Level),
		HP:      int(mon.HP),
		MaxHP:   int(mon.MaxHP),
		Status:  mon.StatusName(),
		Types:   gen1Types(mon.Type1, mon.Type2),
	}
}

// gen1MoveOptions lists every known move. Legality mirrors
// BattleState.Usable exactly so the typed path and MovePolicy agree.
func gen1MoveOptions(romData []byte, b game.BattleState) []game.BattleMoveOption {
	var out []game.BattleMoveOption
	for i, mv := range b.Moves {
		if mv.ID == 0 {
			continue
		}
		opt := game.BattleMoveOption{Slot: i, Move: gen1MoveID(mv.ID), PP: int(mv.PP)}
		switch {
		case mv.Disabled:
			opt.Unusable = game.BattleUnusableDisabled
		case mv.PP == 0:
			opt.Unusable = game.BattleUnusableNoPP
		}
		if romData != nil {
			if meta, err := rom.LookupMove(romData, mv.ID); err == nil {
				opt.Type, _ = data.TypeName(meta.Type)
				opt.Power = int(meta.Power)
				opt.Accuracy = int(math.Round(float64(meta.Accuracy) * 100 / 255))
				if meta.Power > 0 {
					if eff, err := rom.TypeEffectiveness(romData, meta.Type, b.EnemyType1, b.EnemyType2); err == nil {
						opt.Effectiveness = float64(eff) / float64(rom.NeutralEffect)
					}
				}
			}
		}
		out = append(out, opt)
	}
	return out
}

func gen1Species(raw uint8) game.SpeciesID {
	if id, ok := data.Species(raw); ok {
		return id
	}
	return game.SpeciesID(fmt.Sprintf("unknown_species_%#02x", raw))
}

func gen1MoveID(raw uint8) game.MoveID {
	if id, ok := data.Move(raw); ok {
		return id
	}
	return game.MoveID(fmt.Sprintf("unknown_move_%#02x", raw))
}

func gen1Types(t1, t2 uint8) []string {
	a, ok1 := data.TypeName(t1)
	b, ok2 := data.TypeName(t2)
	switch {
	case !ok1 && !ok2:
		return nil
	case !ok2 || a == b:
		return []string{a}
	case !ok1:
		return []string{b}
	}
	return []string{a, b}
}

func bagQuantity(bag []state.BagItem, item uint8) uint8 {
	var qty uint8
	for _, it := range bag {
		if it.ID == item {
			qty += it.Quantity
		}
	}
	return qty
}

// medicineHasEffect is Gen I's "IT WON'T HAVE ANY EFFECT" rule for ordinary
// medicine: fainted targets are never valid, HP items need missing HP, and
// status items need a status they cure. Unknown items are never declared.
func medicineHasEffect(item uint8, mon state.Mon, status uint8) bool {
	if mon.Fainted() || mon.MaxHP == 0 {
		return false
	}
	missingHP := mon.HP < mon.MaxHP
	name := state.Mon{Status: status}.StatusName()
	switch item {
	case itemFullRestore:
		return missingHP || name != ""
	case itemFullHeal:
		return name != ""
	}
	for _, med := range hpMedicines {
		if med.item == item {
			return missingHP
		}
	}
	if name == "" {
		return false
	}
	cure, ok := statusCure(name)
	return ok && cure == item
}

func statusCure(status string) (uint8, bool) {
	switch status {
	case "poisoned":
		return itemAntidote, true
	case "burned":
		return itemBurnHeal, true
	case "frozen":
		return itemIceHeal, true
	case "asleep":
		return itemAwakening, true
	case "paralyzed":
		return itemParlyzHeal, true
	}
	return 0, false
}
