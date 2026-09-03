package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	lavenderTownMap          uint8 = 0x04
	route7Map                uint8 = 0x12
	route8Map                uint8 = 0x13
	undergroundRoute7Map     uint8 = 0x4D
	undergroundRoute8Map     uint8 = 0x50
	undergroundWestEastMap   uint8 = 0x79
	lavenderPokemonCenterMap uint8 = 0x8D
	pokemonTower1FMap        uint8 = 0x8E
	pokemonTower2FMap        uint8 = 0x8F
	pokemonTower3FMap        uint8 = 0x90
	pokemonTower4FMap        uint8 = 0x91
	pokemonTower5FMap        uint8 = 0x92
	pokemonTower6FMap        uint8 = 0x93
	pokemonTower7FMap        uint8 = 0x94
	mrFujisHouseMap          uint8 = 0x95

	pokeFluteItem  uint8 = 0x49
	marowakSpecies uint8 = 0x91

	pokemonTowerTravelEngagements = 60
	mrFujiRescueBudget             = 15000
)

var pokemonTowerFujiStand = Destination{Map: pokemonTower7FMap, X: 10, Y: 4}

// PokemonTowerAvailable reports whether the Pokémon Tower story objective can
// sensibly begin or resume on mapID. The Rocket Hideout maps are included on
// purpose: RocketHideout's positive postcondition is the Silph Scope in the
// bag, and that skill currently finishes beside Giovanni's drop on B4F. The
// next story verb must therefore own the deterministic escape from the
// post-#31 boss room instead of requiring a manual input between slices.
func PokemonTowerAvailable(mapID uint8) bool {
	if RocketHideoutAvailable(mapID) {
		return true
	}
	switch mapID {
	case route7Map, undergroundRoute7Map, undergroundWestEastMap, undergroundRoute8Map, route8Map,
		lavenderTownMap, lavenderPokemonCenterMap,
		pokemonTower1FMap, pokemonTower2FMap, pokemonTower3FMap, pokemonTower4FMap,
		pokemonTower5FMap, pokemonTower6FMap, pokemonTower7FMap, mrFujisHouseMap:
		return true
	default:
		return false
	}
}

// PokemonTower clears the Lavender Pokémon Tower story and obtains the Poké
// Flute from Mr. Fuji. Ordinary walking, trainer battles, and dialogue
// interruptions stay delegated to Travel/Battle; the only bespoke phases are
// the Rocket Hideout escape needed by the #31 handoff, the scripted Marowak
// fight (which must be fought rather than fled), and Mr. Fuji's scripted warp
// from 7F to his house.
//
// The positive postcondition is the Poké Flute in the bag. Dialogue closing,
// reaching 7F, beating Marowak, or rescuing Fuji are not sufficient on their
// own. That makes the skill idempotent and keeps a full-bag handoff from being
// recorded as success.
func PokemonTower(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: PokemonTower: nil policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, pokeFluteItem); count > 0 {
		return nil
	}
	if _, count := bagEntry(&mem, silphScopeItem); count < 1 {
		return fmt.Errorf("skill: PokemonTower: SILPH SCOPE is required before entering the Tower story")
	}
	if cur := mem.U8(sym.CurMap); !PokemonTowerAvailable(cur) {
		return fmt.Errorf("skill: PokemonTower: map %#04x is outside the Celadon/Lavender/Tower progression slice", cur)
	}

	// #31 finishes in the Hideout boss room after collecting the Scope. The
	// immutable ROM grid still contains the closed B4F boss-door block and the
	// B2F/B3F arrow tiles are forced movement, so a plain Travel cannot safely
	// escape from that checkpoint. Reuse the measured spinner transitions and
	// the live-open boss-door shape, then hand ordinary routing back to Travel.
	if cur := m.Peek8(sym.CurMap); cur >= rocketHideoutB1FMap && cur <= rocketHideoutB4FMap {
		if err := leaveRocketHideoutForTower(m, romData, policy); err != nil {
			return fmt.Errorf("skill: PokemonTower: leave Rocket Hideout: %w", err)
		}
	}

	// If a resumed checkpoint is already in Fuji's house after the rescue,
	// finish the item handoff directly. Before the rescue Fuji's sprite is
	// hidden; in that case continue through Lavender/Tower instead.
	if m.Peek8(sym.CurMap) == mrFujisHouseMap {
		if _, _, live := liveObjectPosition(m, 5); live { // MRFUJISHOUSE_MR_FUJI
			return receivePokeFlute(m, romData, policy)
		}
	}

	cur := m.Peek8(sym.CurMap)
	if cur < pokemonTower1FMap || cur > pokemonTower7FMap {
		// Establish Lavender as the blackout checkpoint before committing to
		// the long Tower climb. TravelFlee keeps transit encounters from
		// consuming the resources the mandatory rival/Channeler/Rocket fights
		// need, while trainer battles still fall back to Battle.
		center, ok := Place("lavender pokemon center")
		if !ok {
			return fmt.Errorf("skill: PokemonTower: lavender pokemon center place is missing")
		}
		if _, err := TravelFlee(m, romData, center, policy, 40); err != nil {
			return fmt.Errorf("skill: PokemonTower: reach Lavender Pokemon Center: %w", err)
		}
		if err := Heal(m); err != nil {
			return fmt.Errorf("skill: PokemonTower: heal at Lavender Pokemon Center: %w", err)
		}
	}

	if _, err := travelPokemonTower(m, romData, pokemonTowerFujiStand, policy, pokemonTowerTravelEngagements); err != nil {
		return fmt.Errorf("skill: PokemonTower: climb to Mr. Fuji: %w", err)
	}
	if err := rescueMrFuji(m, romData, policy); err != nil {
		return err
	}
	return receivePokeFlute(m, romData, policy)
}

// pokemonTowerMarowakBattle identifies the one Tower encounter that looks
// wild to the battle engine but is mandatory story progress. RESTLESS_SOUL is
// MAROWAK in the decomp; map+kind+species keeps an ordinary Marowak elsewhere
// from becoming special.
func pokemonTowerMarowakBattle(mapID uint8, b *state.BattleState) bool {
	return b != nil && mapID == pokemonTower6FMap && b.Kind == state.BattleWild && b.EnemySpecies == marowakSpecies
}

// towerBattleResolver flees ordinary Tower wild encounters but fights every
// trainer and the scripted Marowak. That preserves PP/HP for mandatory fights
// without teaching Travel itself anything about Pokémon Tower story state.
func towerBattleResolver(m *emu.Emu, policy MovePolicy) resolveBattle {
	return func() (battleResolution, error) {
		var mem state.Mem
		state.Snapshot(m, &mem)
		b := state.DecodeBattle(&mem)
		if b == nil {
			return battleResolution{}, fmt.Errorf("skill: PokemonTower: battle resolver called outside battle")
		}
		if b.Kind == state.BattleWild && !pokemonTowerMarowakBattle(mem.U8(sym.CurMap), b) {
			if err := Flee(m, 5); err != nil {
				if !errors.Is(err, ErrTrainerBattle) {
					return battleResolution{}, fmt.Errorf("skill: PokemonTower: flee wild encounter: %w", err)
				}
				// Defensive fallback if the battle kind changed between the RAM
				// snapshot and Flee's own check.
			} else {
				return battleResolution{fled: true}, nil
			}
		}
		outcome, err := Battle(m, policy)
		return battleResolution{outcome: outcome}, err
	}
}

func travelPokemonTower(m *emu.Emu, romData []byte, dest Destination, policy MovePolicy, maxEngagements int) (TravelResult, error) {
	return travel(m, policy, maxEngagements,
		cutAwareGoTo(m, romData, dest),
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		towerBattleResolver(m, policy),
	)
}

// rescueMrFuji owns the 7F interaction because ordinary Talk expects control
// to return on the same map shortly after the text box closes. Fuji's script
// instead hides his Tower sprite and warps the player to MR_FUJIS_HOUSE. The
// completion predicate is therefore the destination map plus controllability,
// not a fixed number of A presses.
func rescueMrFuji(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != pokemonTower7FMap {
		return fmt.Errorf("skill: PokemonTower: rescue Mr. Fuji on map %#04x, want 7F %#04x", m.Peek8(sym.CurMap), pokemonTower7FMap)
	}

	const fujiObjectID = 4 // POKEMONTOWER7F_MR_FUJI
	tx, ty := uint8(10), uint8(3)
	if x, y, live := liveObjectPosition(m, fujiObjectID); live {
		tx, ty = x, y
	} else {
		return fmt.Errorf("skill: PokemonTower: Mr. Fuji is not live on 7F before the rescue interaction")
	}
	if err := talkBeside(m, romData, tx, ty, policy); err != nil {
		return fmt.Errorf("skill: PokemonTower: approach Mr. Fuji: %w", err)
	}
	if err := Face(m, tx, ty); err != nil {
		return fmt.Errorf("skill: PokemonTower: face Mr. Fuji: %w", err)
	}

	m.Tap(emu.A, 3, 7)
	mem := advanceUntil(m, mrFujiRescueBudget, func(mm *state.Mem) bool {
		return mm.U8(sym.CurMap) == mrFujisHouseMap && state.Controllable(mm)
	})
	if mem.U8(sym.CurMap) != mrFujisHouseMap || !state.Controllable(&mem) {
		return fmt.Errorf("skill: PokemonTower: Mr. Fuji rescue did not warp to house within %d frames: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mrFujiRescueBudget, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	return nil
}

func receivePokeFlute(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != mrFujisHouseMap {
		return fmt.Errorf("skill: PokemonTower: Poké Flute handoff on map %#04x, want Mr. Fuji's house %#04x", m.Peek8(sym.CurMap), mrFujisHouseMap)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, pokeFluteItem); count > 0 {
		return nil
	}
	if _, err := TalkAt(m, romData, 3, 1, policy); err != nil { // MRFUJISHOUSE_MR_FUJI
		return fmt.Errorf("skill: PokemonTower: receive Poké Flute from Mr. Fuji: %w", err)
	}
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, pokeFluteItem); count < 1 {
		return fmt.Errorf("skill: PokemonTower: Poké Flute missing from bag after Mr. Fuji handoff (bag may be full)")
	}
	return nil
}

// leaveRocketHideoutForTower bridges the exact post-#31 checkpoint to the
// ordinary map graph. B2F/B3F use their measured spinner transition tables;
// B4F uses the live-open boss door because the immutable ROM collision still
// contains the closed block after the guards have opened it in RAM.
func leaveRocketHideoutForTower(m *emu.Emu, romData []byte, policy MovePolicy) error {
	for {
		switch m.Peek8(sym.CurMap) {
		case rocketHideoutB4FMap:
			_, y := playerXY(m)
			if y < 10 {
				if err := walkRocketB4FLiveTo(m, romData, rocketB4FEntry); err != nil {
					return fmt.Errorf("B4F boss room -> stair: %w", err)
				}
			}
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB4FMap, To: rocketHideoutB3FMap, WarpX: 19, WarpY: 10}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("B4F -> B3F: %w", err)
			}
		case rocketHideoutB3FMap:
			if err := travelRocketWarp(m, policy, func() error {
				return walkRocketSpinnerWarp(m, romData, rocketHideoutB3FMap, rocketHideoutB2FMap, 25, 6)
			}); err != nil {
				return fmt.Errorf("B3F spinner -> B2F: %w", err)
			}
		case rocketHideoutB2FMap:
			if err := travelRocketWarp(m, policy, func() error {
				return walkRocketSpinnerWarp(m, romData, rocketHideoutB2FMap, rocketHideoutB1FMap, 27, 8)
			}); err != nil {
				return fmt.Errorf("B2F spinner -> B1F: %w", err)
			}
		case rocketHideoutB1FMap:
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB1FMap, To: gameCornerMap, WarpX: 21, WarpY: 2}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("B1F -> Game Corner: %w", err)
			}
		case gameCornerMap, celadonCityMap, celadonPokemonCenterMap:
			return nil
		default:
			return fmt.Errorf("unexpected map %#04x while leaving Rocket Hideout", m.Peek8(sym.CurMap))
		}
	}
}

func walkRocketB4FLiveTo(m *emu.Emu, romData []byte, dest Destination) error {
	if m.Peek8(sym.CurMap) != rocketHideoutB4FMap || dest.Map != rocketHideoutB4FMap {
		return fmt.Errorf("skill: PokemonTower: live B4F walk requested from %#04x to %#04x", m.Peek8(sym.CurMap), dest.Map)
	}
	h, err := rom.ParseMap(romData, rocketHideoutB4FMap)
	if err != nil {
		return err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return err
	}
	applyLiveOpenBlock(grid, 5, 12)

	return walkAround(func() map[[2]int]bool { return spriteBlockers(m) },
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			x, y := playerXY(m)
			return world.FindPath(grid, int(x), int(y), int(dest.X), int(dest.Y), blocked)
		}, func(steps []world.Step) error { return WalkPath(m, steps) },
		func() { m.StepFrames(npcWaitFrames) })
}
