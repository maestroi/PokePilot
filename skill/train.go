package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// The tileset table (pokered.sym: Tilesets = 03:47BE) is the same table
// world/grid.go reads for its collision side: 12 bytes per entry, the
// grass tile at +10, the block-data pointer at +1. Train keeps its own
// copy of the offsets because it needs the grass tile and block offset,
// which world does not expose.
const (
	trainTilesetsBank    uint8  = 0x03
	trainTilesetsAddr    uint16 = 0x47BE
	trainTilesetEntryLen      = 12
)

// TrainResult reports what a grind session did.
type TrainResult struct {
	StartLevel int  // the lead's level when the session began
	EndLevel   int  // the lead's level when it ended
	Battles    int  // wild battles fought
	BlackedOut bool // a battle ended in ResultLost
	Reached    bool // EndLevel >= targetLevel
}

// Train levels the lead party member by fighting the wild encounters that
// tall grass on the current map throws at it.
//
// The session ping-pongs the player between two nearby grass cells: each
// leg is a Travel, so an encounter that interrupts a leg is fought with
// policy and Travel re-plans the rest of the leg from the encounter tile.
// After every leg the lead's level is re-read from RAM, and the session
// ends when the level reaches targetLevel, when maxBattles battles have
// been fought, or when a blackout ends it.
//
// Both axes bound the session: at most maxBattles battles are fought and
// reaching the target ends it early. Not reaching the target within
// budget is a result (Reached == false, nil error), not a failure. A
// blackout ends the session and is reported: with no healthy mon left,
// grinding is over. A blackout is a legitimate ending, not a failure —
// cumulative damage across several battles can faint even a lead that
// beats the local wilds one at a time — and it does not visibly transport
// the player to a center in the fixture ROM, so the reported position is
// wherever the last leg's post-battle re-plan left the player, and
// resuming a grind means healing first.
//
// Grass cells come from the ROM: the tileset table names the map's grass
// tile and the map's blocks say which cells stand on it, and only
// walkable cells count, so a leg is never planned into a wall. If an
// encounter interrupts a leg exactly as the budget runs out, the pending
// battle is still fought: Train never leaves a battle in progress, so a
// session may fight maxBattles+1.
func Train(m *emu.Emu, romData []byte, targetLevel int, policy MovePolicy, maxBattles int) (TrainResult, error) {
	if policy == nil {
		return TrainResult{}, fmt.Errorf("skill: Train: nil policy")
	}
	if targetLevel < 1 {
		return TrainResult{}, fmt.Errorf("skill: Train: targetLevel must be >= 1, got %d", targetLevel)
	}
	if maxBattles <= 0 {
		return TrainResult{}, fmt.Errorf("skill: Train: maxBattles must be > 0, got %d", maxBattles)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return TrainResult{}, fmt.Errorf("skill: Train: player not controllable on map %#04x", m.Peek8(sym.CurMap))
	}
	now := currentWorld(m)
	res := TrainResult{StartLevel: int(state.DecodeParty(&mem).Mons[0].Level)}

	grass, grid, err := grassCells(romData, now.Map)
	if err != nil {
		return res, err
	}
	if len(grass) == 0 {
		return res, fmt.Errorf("skill: Train: no walkable tall grass on map %#04x", now.Map)
	}
	a, b, ok := grindPair(grass, grid, int(now.X), int(now.Y))
	if !ok {
		return res, fmt.Errorf("skill: Train: map %#04x has no two walkable grass cells close enough to grind between", now.Map)
	}

	// maxLegs bounds the session when encounters come sparser than the
	// budget assumes (Route 1 fights about one per six legs, measured):
	// tripping it means the map's grass has no wild rate at all.
	maxLegs := 20*maxBattles + 60
	next := b
	legs := 0
	for {
		tr, err := Travel(m, romData, Destination{Map: now.Map, X: uint8(next.x), Y: uint8(next.y)}, policy, maxBattles-res.Battles)
		res.Battles += tr.Battles
		if tr.BlackedOut {
			res.BlackedOut = true
		}
		if err != nil && !battleInFlight(m) {
			// A non-battle failure: nothing is left in progress, so the
			// error is reported as-is.
			res.EndLevel = leadLevel(m)
			res.Reached = res.EndLevel >= targetLevel
			return res, err
		}
		if err != nil {
			// Travel's budget ran out with a fresh encounter pending.
			// Finish it: the session ends, but nothing may be left
			// mid-battle.
			outcome, berr := Battle(m, policy)
			if berr != nil {
				return res, fmt.Errorf("skill: Train: battle %d: %w", res.Battles+1, berr)
			}
			res.Battles++
			if outcome == state.ResultLost {
				res.BlackedOut = true
			}
		}
		res.EndLevel = leadLevel(m)
		res.Reached = res.EndLevel >= targetLevel
		if res.BlackedOut || res.Reached || res.Battles >= maxBattles {
			return res, nil
		}
		if legs+1 > maxLegs {
			return res, fmt.Errorf("skill: Train: %d legs without enough encounters (want %d battles on map %#04x; its grass has no wild rate?)",
				legs+1, maxBattles, now.Map)
		}
		next = flip(a, b, next)
		legs++
	}
}

// cell is a game-tile position on a map.
type cell struct{ x, y int }

// grassCells returns the walkable cells of mapID that stand on the
// tileset's grass tile — the cells where the game actually rolls wild
// encounters — along with the map's collision grid. The grass tile and
// block data come from the tileset table, exactly as world/grid.go reads
// it for collisions.
func grassCells(romData []byte, mapID uint8) ([]cell, *world.Grid, error) {
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: Train: map %#04x: %w", mapID, err)
	}
	tsOff, err := bankedOff(trainTilesetsBank, trainTilesetsAddr)
	if err != nil {
		return nil, nil, err
	}
	entry := tsOff + int(h.Tileset)*trainTilesetEntryLen
	if entry+trainTilesetEntryLen > len(romData) {
		return nil, nil, fmt.Errorf("skill: Train: map %#04x: tileset %d entry out of ROM", mapID, h.Tileset)
	}
	grass := romData[entry+10]
	blockOff, err := bankedOff(romData[entry], uint16(romData[entry+1])|uint16(romData[entry+2])<<8)
	if err != nil {
		return nil, nil, err
	}
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: Train: map %#04x: %w", mapID, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: Train: map %#04x: %w", mapID, err)
	}

	var out []cell
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if !grid.Walkable(x, y) {
				continue
			}
			bx, by := x/2, y/2
			sx, sy := x%2, y%2
			id := blocks[by*int(h.WidthBlocks)+bx]
			// The game rolls encounters on the bottom-right tile of the
			// 2x2 step, so that is the tile that must be grass.
			if romData[blockOff+int(id)*16+(2*sy+1)*4+(2*sx+1)] == grass {
				out = append(out, cell{x, y})
			}
		}
	}
	return out, grid, nil
}

// grindPair picks the two cells the session ping-pongs between: a is the
// grass cell nearest the player, and b is a grass cell on the same row or
// column, 1-6 cells away, chosen for the densest grass along the straight
// path between them. Every cell of that path is walkable, so each leg is
// a short straight walk.
func grindPair(grass []cell, grid *world.Grid, px, py int) (cell, cell, bool) {
	at := cell{px, py}
	a := grass[0]
	for _, c := range grass {
		if dist(c, at) < dist(a, at) {
			a = c
		}
	}
	isGrass := make(map[cell]bool, len(grass))
	for _, c := range grass {
		isGrass[c] = true
	}

	var (
		best      cell
		bestScore = -1
		bestDist  = 1 << 30
	)
	for _, c := range grass {
		if c == a || (c.x != a.x && c.y != a.y) {
			continue
		}
		d := dist(a, c)
		if d < 1 || d > 6 {
			continue
		}
		dx, dy := sign(c.x-a.x), sign(c.y-a.y)
		ok := true
		for s := 1; s < d; s++ {
			if !grid.Walkable(a.x+dx*s, a.y+dy*s) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		score := 0
		for s := 0; s <= d; s++ {
			if isGrass[cell{a.x + dx*s, a.y + dy*s}] {
				score++
			}
		}
		if score > bestScore || (score == bestScore && d < bestDist) {
			best, bestScore, bestDist = c, score, d
		}
	}
	if bestScore < 0 {
		return cell{}, cell{}, false
	}
	return a, best, true
}

// battleInFlight reports whether a battle is in progress in RAM: the same
// check Battle performs before it will fight.
func battleInFlight(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.DecodeBattle(&mem) != nil
}

// leadLevel re-reads the lead party member's level from RAM.
func leadLevel(m *emu.Emu) int {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return int(state.DecodeParty(&mem).Mons[0].Level)
}

// flip returns the other end of the ping-pong.
func flip(a, b, cur cell) cell {
	if cur == a {
		return b
	}
	return a
}

// bankedOff converts a banked address (bank:addr) to a ROM file offset,
// the same mapping world.bankedOffset uses. Kept local: Train only reads
// the tileset table, which the world package does not expose.
func bankedOff(bank uint8, addr uint16) (int, error) {
	if addr >= 0x4000 {
		return int(bank)*0x4000 + int(addr-0x4000), nil
	}
	if bank != 0 {
		return 0, fmt.Errorf("address %04X in bank %d is below 0x4000", addr, bank)
	}
	return int(addr), nil
}

func dist(a, b cell) int { return absInt(a.x-b.x) + absInt(a.y-b.y) }

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	if v < 0 {
		return -1
	}
	return 1
}
