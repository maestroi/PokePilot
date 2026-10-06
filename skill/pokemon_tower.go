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

	// pokemonTower6FRareCandy is a ground item ball, not story loot, but it
	// sits in the one-tile-wide gap connecting 6F's upper and lower corridor
	// halves (pokered/data/maps/objects/PokemonTower6F.asm: object_event 6,
	// 8, ..., RARE_CANDY) and Pickup's own contract never walks a ball's
	// tile, only approaches it (see Pickup's doc comment). MEASURED on
	// rom.ParseMap map 0x93: with that tile treated as occupied, no route
	// exists at all from 6F's entrance to its exit warp — the ball must be
	// collected to open the only path through, not merely to loot it.
	pokemonTower6FRareCandyX uint8 = 6
	pokemonTower6FRareCandyY uint8 = 8
	rareCandyItem            uint8 = 0x28

	pokemonTowerTravelEngagements = 60
	mrFujiRescueBudget            = 15000
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
		fujiObjectID, ok, err := mapObjectIDAt(romData, mrFujisHouseMap, 3, 1)
		if err != nil {
			return fmt.Errorf("skill: PokemonTower: resolve Mr. Fuji house object: %w", err)
		}
		if ok {
			if _, _, live := liveObjectPosition(m, fujiObjectID); live {
				return receivePokeFlute(m, romData, policy)
			}
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

	if m.Peek8(sym.CurMap) != pokemonTower7FMap {
		// Pickup approaches within the CURRENT map only; it does not cross
		// maps on its own. Reach 6F's own 5F-side warp landing first (always
		// walkable and always reachable, whatever floor the climb resumes
		// from), then Pickup's local approach can find a tile beside the ball.
		if m.Peek8(sym.CurMap) != pokemonTower6FMap {
			sixFLanding := Destination{Map: pokemonTower6FMap, X: 18, Y: 9}
			if _, err := travelPokemonTower(m, romData, sixFLanding, policy, pokemonTowerTravelEngagements); err != nil {
				return fmt.Errorf("skill: PokemonTower: reach 6F for the Rare Candy: %w", err)
			}
		}
		// The ball's toggleable-object flag, not the bag, says whether the
		// corridor is still sealed: a Rare Candy from anywhere else must not
		// skip the pickup (run-d6dokr184ky81 walked out of the Tower instead).
		live, err := pokemonTower6FRareCandyLive(m, romData)
		if err != nil {
			return err
		}
		if live {
			if err := Pickup(m, romData, pokemonTower6FRareCandyX, pokemonTower6FRareCandyY, rareCandyItem, policy); err != nil {
				return fmt.Errorf("skill: PokemonTower: collect 6F's Rare Candy (blocks the only route through): %w", err)
			}
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
			// Use the same deterministic escape bound as TravelFlee. Five tries
			// is only probabilistic (issue #395); a failed fifth roll would turn
			// an ordinary Tower encounter into an untyped unknown_failure and
			// terminate progression even though nothing is actually blocked.
			if err := Flee(m, guaranteedWildFleeAttempts); err != nil {
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

// pokemonTower6FRareCandyLive reports whether 6F's corridor-sealing Rare
// Candy ball is still on the map. It must be called on 6F: the toggleable
// object list only describes the current map.
func pokemonTower6FRareCandyLive(m *emu.Emu, romData []byte) (bool, error) {
	id, ok, err := mapObjectIDAt(romData, pokemonTower6FMap, pokemonTower6FRareCandyX, pokemonTower6FRareCandyY)
	if err != nil || !ok {
		return false, fmt.Errorf("skill: PokemonTower: resolve 6F Rare Candy object: ok=%v err=%v", ok, err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	return !state.HiddenObjectIDs(&mem)[uint8(id)], nil
}

func travelPokemonTower(m *emu.Emu, romData []byte, dest Destination, policy MovePolicy, maxEngagements int) (TravelResult, error) {
	var egresses []EmergencyEgress
	res, err := travel(m, policy, maxEngagements,
		recoveringGoTo(m, romData, dest, nil, &egresses),
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return blackoutInProgressFor(m) },
		towerBattleResolver(m, policy),
	)
	res.EmergencyEgresses = append(res.EmergencyEgresses, egresses...)
	return res, err
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

	fujiObjectID, ok, err := mapObjectIDAt(romData, pokemonTower7FMap, 10, 3)
	if err != nil {
		return fmt.Errorf("skill: PokemonTower: resolve Mr. Fuji object: %w", err)
	}
	if !ok {
		return fmt.Errorf("skill: PokemonTower: no Mr. Fuji object at (10,3) on 7F")
	}
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
// ordinary map graph. B2F/B3F use ordinary Traverse; the shared local planner
// consumes Red's forced-movement edges. B4F uses the live-open boss door because the immutable ROM collision still
// contains the closed block after the guards have opened it in RAM.
func leaveRocketHideoutForTower(m *emu.Emu, romData []byte, policy MovePolicy) error {
	for {
		switch m.Peek8(sym.CurMap) {
		case rocketHideoutB4FMap:
			// B4F is two halves: the stair/Lift Key side and the elevator side
			// (boss room + lobby). Nothing walks between them, so the boss room
			// leaves by elevator (run-12vowvyawgx0b3jl0srufdx8tq round 61).
			stairSide, err := rocketB4FStairSideReachable(m, romData)
			if err != nil {
				return fmt.Errorf("B4F: inspect stair side: %w", err)
			}
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB4FMap, To: rocketHideoutB3FMap, WarpX: 19, WarpY: 10}
			if !stairSide {
				edge = world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB4FMap, To: rocketHideoutElevatorMap, WarpX: 24, WarpY: 15}
			}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("B4F -> %#04x: %w", edge.To, err)
			}
		case rocketHideoutElevatorMap:
			// B2F, not B1F: B1F's elevator landing is cut off from the Game
			// Corner stair; B2F rejoins the ordinary stair chain below.
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutElevatorMap, To: rocketHideoutB2FMap, WarpX: 2, WarpY: 1}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("elevator -> B2F: %w", err)
			}
		case rocketHideoutB3FMap:
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB3FMap, To: rocketHideoutB2FMap, WarpX: 25, WarpY: 6}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("B3F forced-movement floor -> B2F: %w", err)
			}
		case rocketHideoutB2FMap:
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB2FMap, To: rocketHideoutB1FMap, WarpX: 27, WarpY: 8}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("B2F forced-movement floor -> B1F: %w", err)
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

// rocketB4FStairSideReachable reports whether the player can walk to the
// B3F stair's side of B4F on the live map.
func rocketB4FStairSideReachable(m *emu.Emu, romData []byte) (bool, error) {
	h, err := rom.ParseMap(romData, rocketHideoutB4FMap)
	if err != nil {
		return false, err
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return false, err
	}
	x, y := playerXY(m)
	_, err = world.FindPath(grid, int(x), int(y), int(rocketB4FEntry.X), int(rocketB4FEntry.Y), nil)
	if errors.Is(err, world.ErrNoPath) {
		return false, nil
	}
	return err == nil, err
}
