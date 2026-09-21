package controller

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/world"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowCatchEncounterCap = 40
	yellowCatchGrassLegs    = 600
	yellowCatchBallBudget   = 8
	yellowStaticRetryCount  = 6
	yellowStaticBallBudget  = 12
	yellowCatchMenuBudget   = 3000
	yellowCatchSettleBudget = 5000
)

var (
	ErrYellowCatchHuntExhausted = errors.New("yellow capture: hunt exhausted")
	ErrYellowCatchOutOfBalls    = errors.New("yellow capture: out of balls")
	ErrYellowStaticUnavailable  = errors.New("yellow capture: one-time static source unavailable")
	ErrYellowStaticExhausted    = errors.New("yellow capture: static capture attempts exhausted")
)

type CaptureResult struct {
	Caught      bool
	Species     uint8
	BallsThrown int
	Encounters  int
}

type yellowStaticSite struct {
	species uint8
	name    string
	mapID   uint8
	x, y    uint8
	event   uint16
}

var yellowStaticSites = []yellowStaticSite{
	{species: 0x4a, name: "Articuno", mapID: 0xa2, x: 6, y: 1, event: 0x9da},
	{species: 0x4b, name: "Zapdos", mapID: 0x53, x: 4, y: 9, event: 0x46a},
	{species: 0x49, name: "Moltres", mapID: 0xc2, x: 11, y: 5, event: 0x53e},
	{species: 0x83, name: "Mewtwo", mapID: 0xe3, x: 27, y: 13, event: 0x8c1},
}

type yellowSnorlaxSite struct {
	name       string
	mapID      uint8
	objectX    uint8
	objectY    uint8
	fightEvent uint16
	beatEvent  uint16
}

var yellowSnorlaxSites = []yellowSnorlaxSite{
	{name: "Route 12 Snorlax", mapID: yellowRoute12, objectX: 10, objectY: 62, fightEvent: yellowEventFightRoute12Snorlax, beatEvent: yellowEventBeatRoute12Snorlax},
	{name: "Route 16 Snorlax", mapID: yellowRoute16, objectX: 26, objectY: 10, fightEvent: yellowEventFightRoute16Snorlax, beatEvent: yellowEventBeatRoute16Snorlax},
}

func yellowStaticSiteForSpecies(species uint8) (yellowStaticSite, bool) {
	for _, site := range yellowStaticSites {
		if site.species == species {
			return site, true
		}
	}
	return yellowStaticSite{}, false
}

func yellowBattleMainMenuUp(m *emu.Emu) bool {
	return battlePhaseFor(screenText(m), m.Peek8(sym.MaxMenuItem), m.Peek8(sym.ForcePlayerToChooseMon) != 0) == battlePhaseMainMenu
}

func waitYellowBattleMainMenu(m *emu.Emu) error {
	for frame := 0; frame < yellowCatchMenuBudget; frame++ {
		if m.Peek8(sym.IsInBattle) == 0 {
			return fmt.Errorf("yellow capture: battle ended before the main menu opened")
		}
		if yellowBattleMainMenuUp(m) {
			return nil
		}
		if nicknamePrompt(m) {
			if err := declineNickname(m); err != nil {
				return err
			}
			continue
		}
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return fmt.Errorf("yellow capture: unexpected battle choice: screen=%q",
				strings.Join(strings.Fields(screenText(m)), " "))
		}
		m.Tap(emu.A, 3, 7)
	}
	return fmt.Errorf("yellow capture: battle main menu did not appear within %d frames", yellowCatchMenuBudget)
}

func yellowCaptureBall(m *emu.Emu, finite bool) (uint8, bool) {
	order := []uint8{0x04, 0x03, 0x02, 0x01} // Poké, Great, Ultra, Master
	if finite {
		order = []uint8{0x01, 0x02, 0x03, 0x04} // preserve one-time sources first
	}
	for _, item := range order {
		if _, qty := yellowBagEntry(m, item); qty > 0 {
			return item, true
		}
	}
	return 0, false
}

func useYellowBattleItem(m *emu.Emu, item uint8) error {
	if m.Peek8(sym.IsInBattle) == 0 {
		return fmt.Errorf("yellow capture: no battle in progress")
	}
	if err := waitYellowBattleMainMenu(m); err != nil {
		return err
	}
	idx, before := yellowBagEntry(m, item)
	if idx < 0 || before <= 0 {
		return fmt.Errorf("%w: item %#02x", ErrYellowCatchOutOfBalls, item)
	}

	// ITEM is the bottom-left battle command.
	if err := selectYellowBattleMainMenu(m, yellowBattleMenuLeftX, 1); err != nil {
		return fmt.Errorf("yellow capture: select ITEM: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(900, func(m *emu.Emu) bool {
		return m.Peek8(sym.ListMenuID) == yellowItemListMenuID && m.Peek8(sym.ListCount) != 0
	}); err != nil {
		return fmt.Errorf("yellow capture: battle bag did not open: %w", err)
	}
	if err := selectYellowBagEntry(m, idx); err != nil {
		return fmt.Errorf("yellow capture: select ball %#02x: %w", item, err)
	}

	for frame := 0; frame < yellowCatchMenuBudget; frame++ {
		_, after := yellowBagEntry(m, item)
		if after == before-1 {
			return nil
		}
		if nicknamePrompt(m) {
			if err := declineNickname(m); err != nil {
				return fmt.Errorf("yellow capture: decline nickname: %w", err)
			}
			continue
		}
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return fmt.Errorf("yellow capture: unexpected choice while using ball: screen=%q",
				strings.Join(strings.Fields(screenText(m)), " "))
		}
		if m.Peek8(sym.IsInBattle) == 0 {
			return fmt.Errorf("yellow capture: battle ended before ball %#02x consumption was verified", item)
		}
		m.Tap(emu.A, 3, 7)
	}
	return fmt.Errorf("yellow capture: ball %#02x count did not decrease from %d", item, before)
}

func throwYellowBall(m *emu.Emu, romData []byte, species uint8, finite bool) (caught, ended bool, err error) {
	ball, ok := yellowCaptureBall(m, finite)
	if !ok {
		return false, false, ErrYellowCatchOutOfBalls
	}
	if err := useYellowBattleItem(m, ball); err != nil {
		return false, false, err
	}

	for frame := 0; frame < yellowCatchSettleBudget; frame++ {
		owned, ownedErr := yellowPokedexOwnsInternal(m, romData, species)
		if ownedErr == nil && owned {
			if m.Peek8(sym.IsInBattle) == 0 {
				if err := waitYellowControllable(m, romData, yellowCatchSettleBudget); err != nil {
					return false, true, err
				}
				return true, true, nil
			}
		}
		if m.Peek8(sym.IsInBattle) == 0 {
			if err := waitYellowControllable(m, romData, yellowCatchSettleBudget); err != nil {
				return false, true, err
			}
			owned, ownedErr = yellowPokedexOwnsInternal(m, romData, species)
			return ownedErr == nil && owned, true, ownedErr
		}
		if yellowBattleMainMenuUp(m) {
			return false, false, nil
		}
		if nicknamePrompt(m) {
			if err := declineNickname(m); err != nil {
				return false, false, err
			}
			continue
		}
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return false, false, fmt.Errorf("yellow capture: unexpected choice after ball throw: screen=%q",
				strings.Join(strings.Fields(screenText(m)), " "))
		}
		m.Tap(emu.A, 3, 7)
	}
	return false, false, fmt.Errorf("yellow capture: ball result did not settle within %d frames", yellowCatchSettleBudget)
}

type yellowHuntCell struct {
	x, y int
}

func chooseYellowGrassPair(romData []byte, mapID uint8, sx, sy int) (yellowHuntCell, yellowHuntCell, error) {
	cells, err := yellowrom.GrassEncounterCells(romData, mapID)
	if err != nil {
		return yellowHuntCell{}, yellowHuntCell{}, err
	}
	if len(cells) < 2 {
		return yellowHuntCell{}, yellowHuntCell{}, fmt.Errorf("yellow capture: map %#02x has fewer than two encounter cells", mapID)
	}
	h, err := yellowrom.ParseMap(romData, mapID)
	if err != nil {
		return yellowHuntCell{}, yellowHuntCell{}, err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return yellowHuntCell{}, yellowHuntCell{}, err
	}
	comps := world.Components(grid)
	if !grid.InBounds(sx, sy) || comps[sy][sx] == 0 {
		return yellowHuntCell{}, yellowHuntCell{}, fmt.Errorf("yellow capture: player (%d,%d) is outside a walkable component", sx, sy)
	}
	wantComp := comps[sy][sx]
	blocked := staticObjectBlockers(h, nil)
	candidates := make([]yellowHuntCell, 0, len(cells))
	for _, c := range cells {
		x, y := int(c.X), int(c.Y)
		if !grid.InBounds(x, y) || comps[y][x] != wantComp || blocked[[2]int{x, y}] {
			continue
		}
		candidates = append(candidates, yellowHuntCell{x: x, y: y})
	}
	if len(candidates) < 2 {
		return yellowHuntCell{}, yellowHuntCell{}, fmt.Errorf("yellow capture: map %#02x has fewer than two reachable free encounter cells", mapID)
	}
	sort.Slice(candidates, func(i, j int) bool {
		di := yellowAbsInt(candidates[i].x-sx) + yellowAbsInt(candidates[i].y-sy)
		dj := yellowAbsInt(candidates[j].x-sx) + yellowAbsInt(candidates[j].y-sy)
		if di != dj {
			return di < dj
		}
		if candidates[i].y != candidates[j].y {
			return candidates[i].y < candidates[j].y
		}
		return candidates[i].x < candidates[j].x
	})

	a := candidates[0]
	for _, b := range candidates[1:] {
		if yellowAbsInt(a.x-b.x)+yellowAbsInt(a.y-b.y) == 1 && grid.Passable(a.x, a.y, b.x, b.y) {
			return a, b, nil
		}
	}
	for _, b := range candidates[1:] {
		if _, err := world.FindPath(grid, a.x, a.y, b.x, b.y, blocked); err == nil {
			return a, b, nil
		}
	}
	return yellowHuntCell{}, yellowHuntCell{}, fmt.Errorf("yellow capture: no connected encounter pair on map %#02x", mapID)
}

func yellowWaitEnemySpecies(m *emu.Emu) (uint8, error) {
	for frame := 0; frame < 1200; frame++ {
		if m.Peek8(sym.IsInBattle) == 0 {
			return 0, fmt.Errorf("yellow capture: encounter ended before enemy species appeared")
		}
		if species := m.Peek8(sym.EnemyMonSpecies); species != 0 {
			return species, nil
		}
		m.StepFrame()
	}
	return 0, fmt.Errorf("yellow capture: enemy species did not appear")
}

// CaptureWildGrass hunts one Yellow grass/cave habitat. The target is never
// attacked: unwanted encounters use the normal battle controller, while a
// wanted encounter receives only bounded ball throws and is accepted solely
// after its Yellow Pokédex owned bit is set.
func CaptureWildGrass(m *emu.Emu, romData []byte, species uint8) (CaptureResult, error) {
	var result CaptureResult
	if m == nil {
		return result, fmt.Errorf("yellow capture: nil emulator")
	}
	owned, err := yellowPokedexOwnsInternal(m, romData, species)
	if err != nil {
		return result, err
	}
	if owned {
		return CaptureResult{Caught: true, Species: species}, nil
	}
	mapID := m.Peek8(sym.CurMap)
	has, err := yellowrom.HasWildSpecies(romData, mapID, gen1rom.HabitatGrass, species)
	if err != nil {
		return result, err
	}
	if !has {
		return result, fmt.Errorf("yellow capture: species %#02x is not in map %#02x grass table", species, mapID)
	}

	a, b, err := chooseYellowGrassPair(romData, mapID, int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord)))
	if err != nil {
		return result, err
	}
	next := a

	for legs := 0; legs < yellowCatchGrassLegs && result.Encounters < yellowCatchEncounterCap; legs++ {
		if err := walkTo(m, romData, next.x, next.y, nil); err != nil && m.Peek8(sym.IsInBattle) == 0 {
			// Recompute around the live position in case a moving sprite took
			// one of the deterministic hunt cells.
			a, b, err = chooseYellowGrassPair(romData, mapID, int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord)))
			if err != nil {
				return result, err
			}
			next = a
			continue
		}
		if next == a {
			next = b
		} else {
			next = a
		}
		if m.Peek8(sym.IsInBattle) == 0 {
			continue
		}
		if m.Peek8(sym.IsInBattle) != 1 {
			if _, err := Battle(m, romData); err != nil {
				return result, fmt.Errorf("yellow capture: non-wild interruption: %w", err)
			}
			continue
		}

		enemy, err := yellowWaitEnemySpecies(m)
		if err != nil {
			return result, err
		}
		result.Encounters++
		if enemy != species {
			outcome, err := Battle(m, romData)
			if err != nil {
				return result, fmt.Errorf("yellow capture: unwanted species %#02x: %w", enemy, err)
			}
			if outcome.Outcome == BattleOutcomeLost {
				return result, fmt.Errorf("yellow capture: blacked out while hunting species %#02x", species)
			}
			continue
		}

		for thrown := 0; thrown < yellowCatchBallBudget; thrown++ {
			caught, ended, err := throwYellowBall(m, romData, species, false)
			if errors.Is(err, ErrYellowCatchOutOfBalls) {
				break
			}
			if err != nil {
				return result, err
			}
			result.BallsThrown++
			if caught {
				result.Caught = true
				result.Species = species
				return result, nil
			}
			if ended {
				break
			}
		}
		if m.Peek8(sym.IsInBattle) != 0 {
			if _, err := Battle(m, romData); err != nil {
				return result, fmt.Errorf("yellow capture: end uncaught target battle: %w", err)
			}
		}
		if _, ok := yellowCaptureBall(m, false); !ok {
			return result, ErrYellowCatchOutOfBalls
		}
	}

	return result, fmt.Errorf("%w: map=%#02x encounters=%d", ErrYellowCatchHuntExhausted, mapID, result.Encounters)
}

func initiateYellowStaticBattle(m *emu.Emu, romData []byte, site yellowStaticSite) error {
	if yellowEventSet(m, site.event) {
		return fmt.Errorf("%w: %s event %#03x already set", ErrYellowStaticUnavailable, site.name, site.event)
	}

	h, err := yellowrom.ParseMap(romData, site.mapID)
	if err != nil {
		return err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return err
	}
	target := [2]int{int(site.x), int(site.y)}
	blocked := staticObjectBlockers(h, &target)
	sx, sy := int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord))
	steps, face, err := world.FindPathAdjacent(grid, sx, sy, int(site.x), int(site.y), blocked)
	if err != nil {
		return fmt.Errorf("yellow capture: %s approach: %w", site.name, err)
	}
	if err := walkPath(m, site.mapID, steps); err != nil {
		return fmt.Errorf("yellow capture: %s approach walk: %w", site.name, err)
	}
	btn, ok := buttonFor(face)
	if !ok {
		return fmt.Errorf("yellow capture: %s invalid facing step", site.name)
	}
	m.Tap(btn, 3, 7)
	m.Tap(emu.A, 3, 7)

	for frame := 0; frame < 1800; frame++ {
		if m.Peek8(sym.IsInBattle) != 0 {
			enemy, err := yellowWaitEnemySpecies(m)
			if err != nil {
				return err
			}
			if enemy != site.species {
				return fmt.Errorf("yellow capture: %s started species %#02x, want %#02x", site.name, enemy, site.species)
			}
			return nil
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("%w: %s did not start battle", ErrYellowStaticUnavailable, site.name)
}

func routeToYellowSnorlaxActivation(m *emu.Emu, romData []byte, site yellowSnorlaxSite) error {
	h, err := yellowrom.ParseMap(romData, site.mapID)
	if err != nil {
		return err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return err
	}
	target := [2]int{int(site.objectX), int(site.objectY)}
	blocked := staticObjectBlockers(h, &target)
	var candidates []yellowHuntCell
	for _, step := range []world.Step{world.StepDown, world.StepUp, world.StepRight, world.StepLeft} {
		x, y := int(site.objectX)+step.DX, int(site.objectY)+step.DY
		if grid.InBounds(x, y) && grid.Walkable(x, y) && !blocked[[2]int{x, y}] {
			candidates = append(candidates, yellowHuntCell{x: x, y: y})
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("yellow capture: %s has no walkable activation tile", site.name)
	}
	var last error
	for _, candidate := range candidates {
		if err := GoTo(m, romData, site.mapID, uint8(candidate.x), uint8(candidate.y)); err == nil {
			if _, _, ok := yellowSnorlaxFluteTarget(m); ok {
				return nil
			}
			last = fmt.Errorf("arrival (%d,%d) is not a legal flute activation tile", candidate.x, candidate.y)
		} else {
			last = err
		}
	}
	return fmt.Errorf("yellow capture: reach %s: %w", site.name, last)
}

func captureYellowSnorlax(m *emu.Emu, romData []byte) (CaptureResult, error) {
	const species = uint8(0x84)
	var result CaptureResult
	owned, err := yellowPokedexOwnsInternal(m, romData, species)
	if err != nil {
		return result, err
	}
	if owned {
		return CaptureResult{Caught: true, Species: species}, nil
	}

	for _, site := range yellowSnorlaxSites {
		if yellowEventSet(m, site.beatEvent) {
			continue
		}
		if err := routeToYellowSnorlaxActivation(m, romData, site); err != nil {
			continue
		}
		checkpoint, err := m.SaveState()
		if err != nil {
			return result, fmt.Errorf("yellow capture: checkpoint %s: %w", site.name, err)
		}
		phases := [...]int{0, 17, 37, 61, 89, 127}
		for attempt := 0; attempt < yellowStaticRetryCount; attempt++ {
			if attempt > 0 {
				if err := m.LoadState(checkpoint); err != nil {
					return result, fmt.Errorf("yellow capture: restore %s attempt %d: %w", site.name, attempt+1, err)
				}
				m.StepFrames(phases[attempt])
			}
			_, beatEvent, err := startPokeFluteSnorlaxBattle(m, romData)
			if err != nil {
				_ = m.LoadState(checkpoint)
				break
			}
			enemy, err := yellowWaitEnemySpecies(m)
			if err != nil || enemy != species {
				_ = m.LoadState(checkpoint)
				if err != nil {
					return result, err
				}
				return result, fmt.Errorf("yellow capture: %s started species %#02x, want Snorlax", site.name, enemy)
			}
			result.Encounters++
			for thrown := 0; thrown < yellowStaticBallBudget; thrown++ {
				caught, ended, err := throwYellowBall(m, romData, species, true)
				if errors.Is(err, ErrYellowCatchOutOfBalls) {
					break
				}
				if err != nil {
					break
				}
				result.BallsThrown++
				if caught {
					if !yellowEventSet(m, beatEvent) {
						for frame := 0; frame < 1800 && !yellowEventSet(m, beatEvent); frame++ {
							if m.Peek8(sym.FontLoaded) != 0 {
								m.Tap(emu.A, 3, 7)
							} else {
								m.StepFrame()
							}
						}
					}
					result.Caught = true
					result.Species = species
					return result, nil
				}
				if ended {
					break
				}
			}
			if err := m.LoadState(checkpoint); err != nil {
				return result, fmt.Errorf("yellow capture: rollback %s attempt %d: %w", site.name, attempt+1, err)
			}
		}
	}

	return result, fmt.Errorf("%w: both Yellow Snorlax sources are unavailable or exhausted", ErrYellowStaticExhausted)
}

// CaptureStatic captures Yellow's finite legendary overworld encounters with
// rollback-safe RNG phases. A failed ball phase restores the exact state from
// before interaction, so Articuno/Zapdos/Moltres/Mewtwo are never consumed by
// a bounded failed attempt.
func CaptureStatic(m *emu.Emu, romData []byte, species uint8) (CaptureResult, error) {
	var result CaptureResult
	if m == nil {
		return result, fmt.Errorf("yellow capture: nil emulator")
	}
	if species == 0x84 {
		return captureYellowSnorlax(m, romData)
	}
	owned, err := yellowPokedexOwnsInternal(m, romData, species)
	if err != nil {
		return result, err
	}
	if owned {
		return CaptureResult{Caught: true, Species: species}, nil
	}
	site, ok := yellowStaticSiteForSpecies(species)
	if !ok {
		return result, fmt.Errorf("yellow capture: species %#02x has no implemented Yellow static site", species)
	}

	if m.Peek8(sym.CurMap) != site.mapID {
		h, err := yellowrom.ParseMap(romData, site.mapID)
		if err != nil {
			return result, err
		}
		grid, err := world.Build(romData, h)
		if err != nil {
			return result, err
		}
		target := [2]int{int(site.x), int(site.y)}
		blocked := staticObjectBlockers(h, &target)
		var destinations []yellowHuntCell
		for _, step := range []world.Step{world.StepDown, world.StepUp, world.StepRight, world.StepLeft} {
			x, y := int(site.x)+step.DX, int(site.y)+step.DY
			if grid.InBounds(x, y) && grid.Walkable(x, y) && !blocked[[2]int{x, y}] {
				destinations = append(destinations, yellowHuntCell{x: x, y: y})
			}
		}
		if len(destinations) == 0 {
			return result, fmt.Errorf("yellow capture: %s has no walkable approach", site.name)
		}
		var travelErr error
		for _, dest := range destinations {
			if err := GoTo(m, romData, site.mapID, uint8(dest.x), uint8(dest.y)); err == nil {
				travelErr = nil
				break
			} else {
				travelErr = err
			}
		}
		if travelErr != nil {
			return result, fmt.Errorf("yellow capture: reach %s: %w", site.name, travelErr)
		}
	}

	checkpoint, err := m.SaveState()
	if err != nil {
		return result, fmt.Errorf("yellow capture: checkpoint %s: %w", site.name, err)
	}
	phases := [...]int{0, 17, 37, 61, 89, 127}
	for attempt := 0; attempt < yellowStaticRetryCount; attempt++ {
		if attempt > 0 {
			if err := m.LoadState(checkpoint); err != nil {
				return result, fmt.Errorf("yellow capture: restore %s attempt %d: %w", site.name, attempt+1, err)
			}
			m.StepFrames(phases[attempt])
		}
		if err := initiateYellowStaticBattle(m, romData, site); err != nil {
			_ = m.LoadState(checkpoint)
			return result, err
		}
		result.Encounters++

		for thrown := 0; thrown < yellowStaticBallBudget; thrown++ {
			caught, ended, err := throwYellowBall(m, romData, species, true)
			if errors.Is(err, ErrYellowCatchOutOfBalls) {
				break
			}
			if err != nil {
				break
			}
			result.BallsThrown++
			if caught {
				result.Caught = true
				result.Species = species
				return result, nil
			}
			if ended {
				break
			}
		}
		if err := m.LoadState(checkpoint); err != nil {
			return result, fmt.Errorf("yellow capture: rollback %s attempt %d: %w", site.name, attempt+1, err)
		}
	}
	return result, fmt.Errorf("%w: %s after %d rollback-safe phases", ErrYellowStaticExhausted, site.name, yellowStaticRetryCount)
}

func chooseYellowWaterPair(romData []byte, mapID uint8, sx, sy int) (yellowHuntCell, yellowHuntCell, error) {
	cells, err := yellowrom.WaterEncounterCells(romData, mapID)
	if err != nil {
		return yellowHuntCell{}, yellowHuntCell{}, err
	}
	if len(cells) < 2 {
		return yellowHuntCell{}, yellowHuntCell{}, fmt.Errorf("yellow capture: map %#02x has fewer than two Surf encounter cells", mapID)
	}
	h, err := yellowrom.ParseMap(romData, mapID)
	if err != nil {
		return yellowHuntCell{}, yellowHuntCell{}, err
	}
	grid, err := world.BuildFromBlocksForTraversal(romData, h, nil, world.TraversalWater)
	if err != nil {
		return yellowHuntCell{}, yellowHuntCell{}, err
	}
	comps := world.Components(grid)
	if !grid.InBounds(sx, sy) || comps[sy][sx] == 0 {
		return yellowHuntCell{}, yellowHuntCell{}, fmt.Errorf("yellow capture: Surf position (%d,%d) is outside a water component", sx, sy)
	}
	wantComp := comps[sy][sx]
	blocked := staticObjectBlockers(h, nil)
	candidates := make([]yellowHuntCell, 0, len(cells))
	for _, c := range cells {
		x, y := int(c.X), int(c.Y)
		if !grid.InBounds(x, y) || comps[y][x] != wantComp || blocked[[2]int{x, y}] {
			continue
		}
		candidates = append(candidates, yellowHuntCell{x: x, y: y})
	}
	if len(candidates) < 2 {
		return yellowHuntCell{}, yellowHuntCell{}, fmt.Errorf("yellow capture: map %#02x has fewer than two reachable Surf encounter cells", mapID)
	}
	sort.Slice(candidates, func(i, j int) bool {
		di := yellowAbsInt(candidates[i].x-sx) + yellowAbsInt(candidates[i].y-sy)
		dj := yellowAbsInt(candidates[j].x-sx) + yellowAbsInt(candidates[j].y-sy)
		if di != dj {
			return di < dj
		}
		if candidates[i].y != candidates[j].y {
			return candidates[i].y < candidates[j].y
		}
		return candidates[i].x < candidates[j].x
	})
	a := candidates[0]
	for _, b := range candidates[1:] {
		if yellowAbsInt(a.x-b.x)+yellowAbsInt(a.y-b.y) == 1 && grid.Passable(a.x, a.y, b.x, b.y) {
			return a, b, nil
		}
	}
	for _, b := range candidates[1:] {
		if _, err := world.FindPath(grid, a.x, a.y, b.x, b.y, blocked); err == nil {
			return a, b, nil
		}
	}
	return yellowHuntCell{}, yellowHuntCell{}, fmt.Errorf("yellow capture: no connected Surf encounter pair on map %#02x", mapID)
}

func walkYellowTraversalTo(m *emu.Emu, romData []byte, tx, ty int, mode world.TraversalMode) error {
	mapID := m.Peek8(sym.CurMap)
	h, err := yellowrom.ParseMap(romData, mapID)
	if err != nil {
		return err
	}
	grid, err := world.BuildFromBlocksForTraversal(romData, h, nil, mode)
	if err != nil {
		return err
	}
	sx, sy := int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord))
	steps, err := world.FindPath(grid, sx, sy, tx, ty, staticObjectBlockers(h, nil))
	if err != nil {
		return err
	}
	return walkPath(m, mapID, steps)
}

type yellowShore struct {
	standX, standY int
	waterX, waterY int
	distance       int
}

func nearestYellowShore(m *emu.Emu, romData []byte) (yellowShore, error) {
	mapID := m.Peek8(sym.CurMap)
	h, err := yellowrom.ParseMap(romData, mapID)
	if err != nil {
		return yellowShore{}, err
	}
	land, err := world.BuildFromBlocksForTraversal(romData, h, nil, world.TraversalLand)
	if err != nil {
		return yellowShore{}, err
	}
	water, err := world.BuildFromBlocksForTraversal(romData, h, nil, world.TraversalWater)
	if err != nil {
		return yellowShore{}, err
	}
	sx, sy := int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord))
	blocked := staticObjectBlockers(h, nil)
	dirs := []world.Step{world.StepUp, world.StepLeft, world.StepRight, world.StepDown}
	best := yellowShore{distance: int(^uint(0) >> 1)}
	found := false
	for y := 0; y < land.Height; y++ {
		for x := 0; x < land.Width; x++ {
			if !land.Walkable(x, y) || blocked[[2]int{x, y}] {
				continue
			}
			steps, pathErr := world.FindPath(land, sx, sy, x, y, blocked)
			if pathErr != nil {
				continue
			}
			for _, d := range dirs {
				nx, ny := x+d.DX, y+d.DY
				if !water.InBounds(nx, ny) || land.Passable(x, y, nx, ny) || !water.Passable(x, y, nx, ny) {
					continue
				}
				field, fieldOK := water.FieldTile(nx, ny)
				collision, collisionOK := water.Tile(nx, ny)
				if (!fieldOK || field != 0x14) && (!collisionOK || collision != 0x14) {
					continue
				}
				candidate := yellowShore{
					standX: x, standY: y, waterX: nx, waterY: ny, distance: len(steps),
				}
				if !found || candidate.distance < best.distance ||
					(candidate.distance == best.distance &&
						(candidate.standY < best.standY ||
							(candidate.standY == best.standY && candidate.standX < best.standX))) {
					best, found = candidate, true
				}
			}
		}
	}
	if !found {
		return yellowShore{}, fmt.Errorf("yellow capture: no reachable shoreline on map %#02x", mapID)
	}
	return best, nil
}

func faceYellowCoordinate(m *emu.Emu, x, y int) error {
	dx := x - int(m.Peek8(sym.XCoord))
	dy := y - int(m.Peek8(sym.YCoord))
	var step world.Step
	switch {
	case dx == 1 && dy == 0:
		step = world.StepRight
	case dx == -1 && dy == 0:
		step = world.StepLeft
	case dx == 0 && dy == 1:
		step = world.StepDown
	case dx == 0 && dy == -1:
		step = world.StepUp
	default:
		return fmt.Errorf("yellow capture: target (%d,%d) is not adjacent for facing", x, y)
	}
	btn, ok := buttonFor(step)
	if !ok {
		return fmt.Errorf("yellow capture: invalid facing direction")
	}
	m.Tap(btn, 3, 7)
	return nil
}

// CaptureWildWater enters Surf when needed and hunts Yellow's water encounter
// table on the current map. Wanted targets receive only verified ball throws;
// unwanted encounters are resolved by the Yellow battle controller.
func CaptureWildWater(m *emu.Emu, romData []byte, species uint8) (CaptureResult, error) {
	var result CaptureResult
	if m == nil {
		return result, fmt.Errorf("yellow capture: nil emulator")
	}
	owned, err := yellowPokedexOwnsInternal(m, romData, species)
	if err != nil {
		return result, err
	}
	if owned {
		return CaptureResult{Caught: true, Species: species}, nil
	}
	mapID := m.Peek8(sym.CurMap)
	has, err := yellowrom.HasWildSpecies(romData, mapID, gen1rom.HabitatWater, species)
	if err != nil {
		return result, err
	}
	if !has {
		return result, fmt.Errorf("yellow capture: species %#02x is not in map %#02x water table", species, mapID)
	}

	if m.Peek8(sym.WalkBikeSurfState) != 2 {
		shore, err := nearestYellowShore(m, romData)
		if err != nil {
			return result, err
		}
		if err := walkTo(m, romData, shore.standX, shore.standY, nil); err != nil {
			return result, fmt.Errorf("yellow capture: reach Surf shoreline: %w", err)
		}
		if err := faceYellowCoordinate(m, shore.waterX, shore.waterY); err != nil {
			return result, err
		}
		if err := UseFieldMove(m, romData, FieldSurf); err != nil {
			return result, fmt.Errorf("yellow capture: enter Surf: %w", err)
		}
		if m.Peek8(sym.WalkBikeSurfState) != 2 {
			return result, fmt.Errorf("yellow capture: Surf did not enter surfing state")
		}
	}

	a, b, err := chooseYellowWaterPair(romData, mapID, int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord)))
	if err != nil {
		return result, err
	}
	next := a
	for legs := 0; legs < yellowCatchGrassLegs && result.Encounters < yellowCatchEncounterCap; legs++ {
		if err := walkYellowTraversalTo(m, romData, next.x, next.y, world.TraversalWater); err != nil && m.Peek8(sym.IsInBattle) == 0 {
			a, b, err = chooseYellowWaterPair(romData, mapID, int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord)))
			if err != nil {
				return result, err
			}
			next = a
			continue
		}
		if next == a {
			next = b
		} else {
			next = a
		}
		if m.Peek8(sym.IsInBattle) == 0 {
			continue
		}
		if m.Peek8(sym.IsInBattle) != 1 {
			if _, err := Battle(m, romData); err != nil {
				return result, err
			}
			continue
		}
		enemy, err := yellowWaitEnemySpecies(m)
		if err != nil {
			return result, err
		}
		result.Encounters++
		if enemy != species {
			outcome, err := Battle(m, romData)
			if err != nil {
				return result, fmt.Errorf("yellow capture: unwanted Surf species %#02x: %w", enemy, err)
			}
			if outcome.Outcome == BattleOutcomeLost {
				return result, fmt.Errorf("yellow capture: blacked out during Surf hunt")
			}
			continue
		}
		for thrown := 0; thrown < yellowCatchBallBudget; thrown++ {
			caught, ended, err := throwYellowBall(m, romData, species, false)
			if errors.Is(err, ErrYellowCatchOutOfBalls) {
				break
			}
			if err != nil {
				return result, err
			}
			result.BallsThrown++
			if caught {
				result.Caught = true
				result.Species = species
				return result, nil
			}
			if ended {
				break
			}
		}
		if m.Peek8(sym.IsInBattle) != 0 {
			if _, err := Battle(m, romData); err != nil {
				return result, err
			}
		}
		if _, ok := yellowCaptureBall(m, false); !ok {
			return result, ErrYellowCatchOutOfBalls
		}
	}
	return result, fmt.Errorf("%w: Surf map=%#02x encounters=%d", ErrYellowCatchHuntExhausted, mapID, result.Encounters)
}


const (
	yellowSafariGateMap   = 0x9c
	yellowSafariEastMap   = 0xd9
	yellowSafariNorthMap  = 0xda
	yellowSafariWestMap   = 0xdb
	yellowSafariCenterMap = 0xdc

	yellowSafariSessions = 3
	yellowSafariLegs     = 500
)

func yellowSafariHabitat(mapID uint8) bool {
	return mapID >= yellowSafariEastMap && mapID <= yellowSafariCenterMap
}

func yellowSafariActive(m *emu.Emu) bool {
	return m.Peek8(sym.NumSafariBalls) > 0 &&
		(yellowSafariHabitat(m.Peek8(sym.CurMap)) || m.Peek8(sym.CurMap) == yellowSafariGateMap)
}

func enterYellowSafari(m *emu.Emu, romData []byte) error {
	if yellowSafariActive(m) && yellowSafariHabitat(m.Peek8(sym.CurMap)) {
		return nil
	}
	if err := GoTo(m, romData, yellowSafariGateMap, 4, 3); err != nil {
		if m.Peek8(sym.CurMap) != yellowSafariGateMap {
			return fmt.Errorf("yellow Safari: reach gate: %w", err)
		}
	}

	for frame := 0; frame < 9000; frame++ {
		if yellowSafariHabitat(m.Peek8(sym.CurMap)) && m.Peek8(sym.NumSafariBalls) > 0 {
			return waitYellowControllable(m, romData, 1200)
		}
		text := strings.ToUpper(screenText(m))
		if m.Peek8(sym.MaxMenuItem) == 1 && strings.Contains(text, "YES") && strings.Contains(text, "NO") {
			if err := selectYellowTwoOption(m, false); err != nil {
				return fmt.Errorf("yellow Safari: accept admission: %w", err)
			}
			continue
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
			continue
		}
		if m.Peek8(sym.CurMap) == yellowSafariGateMap &&
			m.Peek8(sym.JoyIgnore) == 0 &&
			m.Peek8(sym.YCoord) >= 3 {
			m.Tap(emu.Up, 3, 7)
			continue
		}
		m.StepFrame()
	}
	return fmt.Errorf("yellow Safari: admission did not reach an active session")
}

func leaveYellowSafari(m *emu.Emu, romData []byte) error {
	if !yellowSafariActive(m) {
		return nil
	}
	if m.Peek8(sym.CurMap) != yellowSafariGateMap {
		if err := GoTo(m, romData, yellowSafariGateMap, 4, 1); err != nil &&
			m.Peek8(sym.CurMap) != yellowSafariGateMap {
			return fmt.Errorf("yellow Safari: return to gate: %w", err)
		}
	}
	for frame := 0; frame < 6000; frame++ {
		if m.Peek8(sym.NumSafariBalls) == 0 && m.Peek8(sym.CurMap) != yellowSafariGateMap {
			return waitYellowControllable(m, romData, 1200)
		}
		text := strings.ToUpper(screenText(m))
		if m.Peek8(sym.MaxMenuItem) == 1 && strings.Contains(text, "YES") && strings.Contains(text, "NO") {
			if err := selectYellowTwoOption(m, false); err != nil {
				return fmt.Errorf("yellow Safari: confirm early exit: %w", err)
			}
			continue
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
			continue
		}
		if m.Peek8(sym.CurMap) == yellowSafariGateMap &&
			m.Peek8(sym.JoyIgnore) == 0 &&
			m.Peek8(sym.YCoord) <= 1 {
			m.Tap(emu.Down, 3, 7)
			continue
		}
		m.StepFrame()
	}
	return fmt.Errorf("yellow Safari: early exit did not settle")
}

func yellowSafariBattleMenuUp(m *emu.Emu) bool {
	return battlePhaseFor(screenText(m), m.Peek8(sym.MaxMenuItem), m.Peek8(sym.ForcePlayerToChooseMon) != 0) == battlePhaseSafariMenu
}

func throwYellowSafariBall(m *emu.Emu, romData []byte, species uint8) (caught, ended bool, err error) {
	for frame := 0; frame < yellowCatchMenuBudget && !yellowSafariBattleMenuUp(m); frame++ {
		if m.Peek8(sym.IsInBattle) == 0 {
			return false, true, nil
		}
		m.Tap(emu.A, 3, 7)
	}
	if !yellowSafariBattleMenuUp(m) {
		return false, false, fmt.Errorf("yellow Safari: BALL menu did not appear")
	}
	before := m.Peek8(sym.NumSafariBalls)
	if before == 0 {
		return false, false, ErrYellowCatchOutOfBalls
	}
	if err := selectYellowBattleMainMenu(m, yellowBattleMenuLeftX, 0); err != nil {
		return false, false, fmt.Errorf("yellow Safari: select BALL: %w", err)
	}
	m.Tap(emu.A, 3, 7)

	for frame := 0; frame < yellowCatchSettleBudget; frame++ {
		owned, ownedErr := yellowPokedexOwnsInternal(m, romData, species)
		if ownedErr == nil && owned && m.Peek8(sym.IsInBattle) == 0 {
			if err := waitYellowControllable(m, romData, yellowCatchSettleBudget); err != nil {
				return false, true, err
			}
			return true, true, nil
		}
		if m.Peek8(sym.IsInBattle) == 0 {
			owned, ownedErr = yellowPokedexOwnsInternal(m, romData, species)
			return ownedErr == nil && owned, true, ownedErr
		}
		if yellowSafariBattleMenuUp(m) {
			after := m.Peek8(sym.NumSafariBalls)
			if after+1 != before {
				return false, false, fmt.Errorf("yellow Safari: BALL count changed %d -> %d, want exactly one consumed", before, after)
			}
			return false, false, nil
		}
		if nicknamePrompt(m) {
			if err := declineNickname(m); err != nil {
				return false, false, err
			}
			continue
		}
		m.Tap(emu.A, 3, 7)
	}
	return false, false, fmt.Errorf("yellow Safari: BALL result did not settle")
}

func travelYellowSafariHabitat(m *emu.Emu, romData []byte, mapID uint8) error {
	cells, err := yellowrom.GrassEncounterCells(romData, mapID)
	if err != nil {
		return err
	}
	if len(cells) == 0 {
		return fmt.Errorf("yellow Safari: target map %#02x has no encounter grass", mapID)
	}
	limit := len(cells)
	if limit > 12 {
		limit = 12
	}
	var last error
	for i := 0; i < limit; i++ {
		if !yellowSafariActive(m) {
			return fmt.Errorf("yellow Safari: session expired while routing to map %#02x", mapID)
		}
		c := cells[i]
		if err := GoTo(m, romData, mapID, c.X, c.Y); err == nil {
			return nil
		} else {
			last = err
		}
	}
	return fmt.Errorf("yellow Safari: no reachable grass entry on map %#02x: %w", mapID, last)
}

// CaptureSafari owns Yellow's paid Safari session lifecycle. It enters through
// the real gate, hunts only on the requested Safari map, runs from unwanted
// encounters, throws only Safari Balls at the wanted species, and starts a
// fresh bounded session when the 502-step/ball budget expires.
func CaptureSafari(m *emu.Emu, romData []byte, targetMap, species uint8) (CaptureResult, error) {
	var result CaptureResult
	if m == nil {
		return result, fmt.Errorf("yellow Safari: nil emulator")
	}
	if !yellowSafariHabitat(targetMap) {
		return result, fmt.Errorf("yellow Safari: map %#02x is not a Safari habitat", targetMap)
	}
	owned, err := yellowPokedexOwnsInternal(m, romData, species)
	if err != nil {
		return result, err
	}
	if owned {
		return CaptureResult{Caught: true, Species: species}, nil
	}
	has, err := yellowrom.HasWildSpecies(romData, targetMap, gen1rom.HabitatGrass, species)
	if err != nil {
		return result, err
	}
	if !has {
		return result, fmt.Errorf("yellow Safari: species %#02x is not in map %#02x encounter table", species, targetMap)
	}

	for session := 0; session < yellowSafariSessions; session++ {
		if err := enterYellowSafari(m, romData); err != nil {
			return result, fmt.Errorf("yellow Safari: enter session %d: %w", session+1, err)
		}
		if err := travelYellowSafariHabitat(m, romData, targetMap); err != nil {
			if !yellowSafariActive(m) {
				continue
			}
			return result, err
		}

		a, b, err := chooseYellowGrassPair(romData, targetMap, int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord)))
		if err != nil {
			return result, err
		}
		next := a
		for legs := 0; legs < yellowSafariLegs && yellowSafariActive(m); legs++ {
			if err := walkTo(m, romData, next.x, next.y, nil); err != nil && m.Peek8(sym.IsInBattle) == 0 {
				a, b, err = chooseYellowGrassPair(romData, targetMap, int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord)))
				if err != nil {
					return result, err
				}
				next = a
				continue
			}
			if next == a {
				next = b
			} else {
				next = a
			}
			if m.Peek8(sym.IsInBattle) == 0 {
				continue
			}
			enemy, err := yellowWaitEnemySpecies(m)
			if err != nil {
				return result, err
			}
			result.Encounters++
			if enemy != species {
				if _, err := Battle(m, romData); err != nil {
					return result, fmt.Errorf("yellow Safari: flee unwanted species %#02x: %w", enemy, err)
				}
				continue
			}

			for thrown := 0; thrown < yellowCatchBallBudget && m.Peek8(sym.NumSafariBalls) > 0; thrown++ {
				caught, ended, err := throwYellowSafariBall(m, romData, species)
				if errors.Is(err, ErrYellowCatchOutOfBalls) {
					break
				}
				if err != nil {
					return result, err
				}
				result.BallsThrown++
				if caught {
					result.Caught = true
					result.Species = species
					return result, nil
				}
				if ended {
					break
				}
			}
			if m.Peek8(sym.IsInBattle) != 0 {
				if _, err := Battle(m, romData); err != nil {
					return result, fmt.Errorf("yellow Safari: leave uncaught wanted battle: %w", err)
				}
			}
		}

		if yellowSafariActive(m) {
			if err := leaveYellowSafari(m, romData); err != nil {
				return result, fmt.Errorf("yellow Safari: close session %d: %w", session+1, err)
			}
		}
	}
	return result, fmt.Errorf("%w: Safari sessions=%d encounters=%d balls=%d",
		ErrYellowCatchHuntExhausted, yellowSafariSessions, result.Encounters, result.BallsThrown)
}
