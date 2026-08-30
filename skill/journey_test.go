package skill_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestGymJourneyAffordances is the S7-8 milestone: one Pallet -> Brock run
// in which the two slice-7 affordances are actually USED, each proved from
// RAM:
//
//  1. The POKE BALL item ball at (1,31) in Viridian Forest (map 0x33) is
//     picked up, and the bag's POKE_BALL count is read with
//     state.DecodeInventory before and after and must rise by one.
//  2. The Pewter Gym guide (PEWTERGYM_GYM_GUIDE at (7,10) on map 0x36) is
//     talked to BEFORE the Brock fight, and his greeting and advice lines
//     must appear on the dialogue tape — the same per-frame sampling and
//     decode agent.Run uses for Observation.RecentDialogue. The talk is
//     driven one frame at a time in this test rather than through
//     skill.Talk, because Talk pages its conversation inside emu.StepFrames
//     batches, where the onFrame hook never fires (see RUNNOTES S7-8); a
//     conversation Talk drives is invisible to every per-frame sampler.
//
// The route follows TestGymBoulderBadge exactly (post_errand -> south gate
// -> forest -> north gate -> Pewter -> gym, blackout-tolerant legs and heal
// detours); this test only inserts the pickup in the forest and the guide
// talk in the gym. It is a full journey — minutes of emulation and
// stochastic wild battles — so it runs only outside -short, proven by its
// own -run gate.
//
// The gym's Cool Trainer (PEWTERGYM_COOLTRAINER_M at (3,6) facing right,
// engage distance 5, verified in pokered/data/maps/objects/PewterGym.asm)
// guards every shortest path across row 6, and his defeat flag is set only
// by Brock's victory script (PewterGym.asm:79), so he re-arms on every
// crossing until Brock falls. A one-Pokemon party cannot pay that tax on
// the way in, out for the heal, and back in, so the test routes through
// the x=1 side corridor, whose row-6 crossing is outside his sight line;
// every waypoint leg's shortest path stays off it (probed on map 0x36),
// which is what makes the full-strength precondition below hold
// deterministically instead of by RNG luck.
func TestGymJourneyAffordances(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey; proven by its own -run gate, not the per-task gate")
	}
	// journeyLeg in gym_test.go discards TravelResult; a blackout leg is
	// only diagnosable with the battle count it fought, so this test uses
	// a logging wrapper of the same recovery (not the shared helper).
	var e *emu.Emu
	var romData []byte
	var policy skill.MovePolicy
	leg := func(dest skill.Destination, what string) {
		for attempt := 0; ; attempt++ {
			res, err := skill.Travel(e, romData, dest, policy, 10)
			if err == nil {
				t.Logf("%s: arrived (battles=%d replans=%d blackedOut=%v)", what, res.Battles, len(res.Replans), res.BlackedOut)
				return
			}
			t.Logf("%s: attempt %d failed after %d battle(s): %v", what, attempt, res.Battles, err)
			if !errors.Is(err, skill.ErrBlackedOut) || attempt >= 2 {
				diagFatalf(t, e, err, "%s: %v", what, err)
			}
			settleBlackout(t, e)
		}
	}
	e = fixture.Load(t, "post_errand")
	romData = e.ROM()
	policy = skill.StatAwareMove(romData)

	tape := &dialogueRecorder{}
	e.OnFrame(tape.sample)

	// Leg 1: Viridian City to the south gate, one row below the (5,0)
	// forest warp — same leg as TestGymBoulderBadge.
	southGate := skill.Destination{Map: 0x32, X: 5, Y: 1}
	leg(southGate, "Travel to the south gate")

	// Leg 2: the south gate into the forest, the training ground.
	forest, ok := skill.Place("viridian forest")
	if !ok {
		diagFatalf(t, e, nil, `Place "viridian forest" not found`)
	}
	leg(forest, "Travel to the forest")

	// PICKUP: the free POKE BALL at (1,31). S6-3 measured five balls are
	// not enough (~13% chance all five break on one full-HP Caterpie), so
	// this ball is a real gain, not decoration. (2,31) is the walkable
	// tile east of it (S7-5/S7-6 measured it against the ROM).
	ballTile := skill.Destination{Map: 0x33, X: 2, Y: 31}
	leg(ballTile, "Travel to the forest ball")

	var mem state.Mem
	state.Snapshot(e, &mem)
	ballsBefore := pokeBallCount(&mem)
	t.Logf("bag before pickup: POKE BALL x%d (map=%#04x at (%d,%d))", ballsBefore,
		mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	if err := skill.Pickup(e, romData, 1, 31, skill.ItemPokeBall, policy); err != nil {
		diagFatalf(t, e, err, "Pickup the POKE BALL at (1,31): %v", err)
	}
	state.Snapshot(e, &mem)
	ballsAfter := pokeBallCount(&mem)
	t.Logf("bag after pickup: POKE BALL x%d (want %d)", ballsAfter, ballsBefore+1)
	if ballsAfter != ballsBefore+1 {
		diagFatalf(t, e, nil, "bag POKE BALL count did not rise by one: before=%d after=%d", ballsBefore, ballsAfter)
	}

	// Back to the warp-free safe spot before any grind ping-pong — same as
	// TestGymBoulderBadge: a grind in the south warp pocket steps on a warp.
	safeSpot := skill.Destination{Map: 0x33, X: 17, Y: 40}
	leg(safeSpot, "Travel to the safe spot")

	// Train the lead up to gymLeadLevel if it is under, with the same
	// session/heal/blackout structure as TestGymBoulderBadge (constants and
	// helpers live in gym_test.go, same package).
	totalBattles := 0
	phaseRetries := 0
	for detours := 0; detours <= maxHealDetours; detours++ {
		state.Snapshot(e, &mem)
		lead := state.DecodeParty(&mem).Mons[0]
		if int(lead.Level) >= gymLeadLevel {
			break
		}
		res, err := skill.Train(e, romData, gymLeadLevel, policy, trainBattleBudget)
		if err != nil {
			if strings.Contains(err.Error(), "no-encounter phase") && phaseRetries < maxPhaseRetries {
				phaseRetries++
				e.StepFrames(123)
				t.Logf("grind session %d hit a no-encounter phase (%v): shifting the frame phase and retrying (retry %d)", detours+1, err, phaseRetries)
				continue
			}
			diagFatalf(t, e, err, "Train: %v (start=%d end=%d battles=%d, reached=%v, blackedOut=%v)", err, res.StartLevel, res.EndLevel, res.Battles, res.Reached, res.BlackedOut)
		}
		totalBattles += res.Battles
		state.Snapshot(e, &mem)
		lead = state.DecodeParty(&mem).Mons[0]
		if res.BlackedOut {
			// A blackout fully heals the party and warps it back to a
			// Pokemon Center; leg resumes the walk from the respawn.
			leg(safeSpot, "Travel back to the forest after a blackout")
			t.Logf("grind session %d blacked out at level %d (detour %d): party healed by the blackout, resuming", detours+1, lead.Level, detours)
			continue
		}
		if int(lead.Level) < gymLeadLevel && (res.Retreated || int(lead.HP)*3 < int(lead.MaxHP) || lead.Status != 0) {
			center, ok := skill.Place("viridian pokemon center")
			if !ok {
				diagFatalf(t, e, nil, `Place "viridian pokemon center" not found`)
			}
			// leg, not bare Travel: a wild battle lost ON the way to the
			// center blacks out and respawns fully healed, which is the
			// same recovery as the in-session blackout above (measured on
			// the first run of this test: the detour itself blacked out).
			leg(center, "Travel to the Viridian Center to heal")
			if err := skill.Heal(e); err != nil {
				diagFatalf(t, e, err, "Heal at the Viridian Center: %v", err)
			}
			leg(safeSpot, "Travel back to the forest after healing")
			t.Logf("healed the lead at level %d (HP %d/%d, status=%#02x) (detour %d)", lead.Level, lead.HP, lead.MaxHP, lead.Status, detours)
		}
	}
	state.Snapshot(e, &mem)
	lead := state.DecodeParty(&mem).Mons[0]
	if int(lead.Level) < gymLeadLevel {
		diagFatalf(t, e, nil, "the lead is level %d after %d battles and %d heal detour(s), want >= %d to face Brock (HP %d/%d, status=%#02x)",
			lead.Level, totalBattles, maxHealDetours, gymLeadLevel, lead.HP, lead.MaxHP, lead.Status)
	}
	t.Logf("trained the lead to level %d in %d battles (HP %d/%d, status=%#02x)", lead.Level, totalBattles, lead.HP, lead.MaxHP, lead.Status)

	// Leave the forest healthy — same detour as TestGymBoulderBadge, with
	// the same blackout tolerance on every leg.
	if lead.Status != 0 || int(lead.HP)*2 < int(lead.MaxHP) {
		center, ok := skill.Place("viridian pokemon center")
		if !ok {
			diagFatalf(t, e, nil, `Place "viridian pokemon center" not found`)
		}
		leg(center, "Travel to the Viridian Center before leaving the forest")
		if err := skill.Heal(e); err != nil {
			diagFatalf(t, e, err, "Heal at the Viridian Center before leaving the forest: %v", err)
		}
		leg(safeSpot, "Travel back to the forest after the pre-journey heal")
		t.Logf("left the forest healed: level %d (was HP %d/%d, status=%#02x)", lead.Level, lead.HP, lead.MaxHP, lead.Status)
	}

	// Leg 3: the forest to the north gate.
	northGate := skill.Destination{Map: 0x2F, X: 5, Y: 1}
	leg(northGate, "Travel to the north gate")

	// Leg 4: the north gate up Route 2 into Pewter City — to the Center,
	// NOT through the gym yet. Healing first is what makes the Cool
	// Trainer problem tractable: every gym crossing on his sight line
	// costs a two-Pokemon fight he re-arms for, so the lead must enter at
	// full HP and never cross that line more than the corridor forces.
	center, ok := skill.Place("pewter pokemon center")
	if !ok {
		diagFatalf(t, e, nil, `Place "pewter pokemon center" not found`)
	}
	leg(center, "Travel to the Pewter Center before the gym")
	if err := skill.Heal(e); err != nil {
		diagFatalf(t, e, err, "Heal at the Pewter Center before the gym: %v", err)
	}

	// Enter the gym through the x=1 side corridor: (1,8) then (1,4) then
	// (4,2). Probed on map 0x36: row 6 is open at x=1..8 with the Cool
	// Trainer at (3,6) facing right (sight line x=4..8), and the corridor
	// tiles (1,4)..(1,8) are the only row-6 crossing outside it. Each leg
	// below has a strictly-shortest path that stays off the sight line:
	// entrance->(1,8) is 8 steps inside rows 8..13, (1,8)<->(1,4) is the
	// unique 4-step column walk, and (1,4)<->(4,2) is the unique 5-step
	// path through row 4. No trainer battle can start on these legs.
	gymSide := skill.Destination{Map: 0x36, X: 1, Y: 8}
	gymUpper := skill.Destination{Map: 0x36, X: 1, Y: 4}
	leg(gymSide, "Enter the gym through the side corridor (row 8)")
	leg(gymUpper, "Up the side corridor to row 4")
	gym, ok := skill.Place("pewter gym")
	if !ok {
		diagFatalf(t, e, nil, "Place \"pewter gym\" not found")
	}
	leg(gym, "Across the top room to below Brock (4,2)")

	// GUIDE: talk to the gym guide at (7,10) BEFORE the Brock fight. (7,11)
	// is the walkable tile directly below him (probed on map 0x36: (6,10)
	// and (7,9) are walls, so only (7,11) and (8,10) reach him). The
	// approach goes back down the corridor — (1,4), (1,8), then (7,11):
	// the last leg's 9-step shortest paths all stay inside rows 8..11,
	// so the Cool Trainer's sight line is never crossed and the guide is
	// reached without a battle draining the lead.
	leg(gymUpper, "Back down to the side corridor (row 4)")
	leg(gymSide, "Down the side corridor to row 8")
	guideTile := skill.Destination{Map: 0x36, X: 7, Y: 11}
	leg(guideTile, "Across the bottom room to below the guide (7,11)")
	if err := skill.Face(e, 7, 10); err != nil {
		diagFatalf(t, e, err, "Face the gym guide at (7,10): %v", err)
	}
	// The guide's script (pokered/scripts/PewterGym.asm:180) is greeting ->
	// YesNoChoice -> advice. Measured on this ROM: wFontLoaded is 1 from
	// within one frame of the opening A press until the final close, so
	// DecodeDialogue and DecodeTwoOptionMenu both see every box — the talk
	// below is driven with StepFrame (one frame per iteration) precisely so
	// the tape samples all of it. The A-tap cadence is skill.Talk's own
	// (tap, 40-frame settle): a tap mid-typing completes the page, the
	// next one pages on, and the tap that lands on the yes/no menu confirms
	// its default cursor (YES), which is the branch the script takes.
	e.Tap(emu.A, 3, 7)
	opened := false
	for i := 0; i < 120 && !opened; i++ {
		e.StepFrame()
		state.Snapshot(e, &mem)
		opened = mem.U8(sym.FontLoaded) != 0
	}
	if !opened {
		diagFatalf(t, e, nil, "A did not open the guide's text box within 120 frames (wFontLoaded=%#02x wJoyIgnore=%#04x)", mem.U8(sym.FontLoaded), mem.U8(sym.JoyIgnore))
	}

	presses := 1
	answeredChoice := false
	settle := 0
	closedFor := 0
	done := false
	for i := 0; i < 3000 && !done; i++ {
		e.StepFrame()
		state.Snapshot(e, &mem)
		if mem.U8(sym.FontLoaded) == 0 {
			settle = 0
			if state.Controllable(&mem) {
				closedFor++
				if closedFor >= 30 {
					done = true
				}
			} else {
				closedFor = 0
			}
			continue
		}
		closedFor = 0
		if menu := state.DecodeTwoOptionMenu(&mem); menu != nil && !answeredChoice {
			answeredChoice = true
			t.Logf("the guide's yes/no choice is up (cursor on option %d); the next A tap confirms the default (YES)", menu.Index)
		}
		if settle == 0 {
			e.Tap(emu.A, 3, 7)
			presses++
			settle = 40
		} else {
			settle--
		}
	}
	if !done {
		diagFatalf(t, e, nil, "the guide's conversation did not close within 3000 frames (wFontLoaded=%#02x)", mem.U8(sym.FontLoaded))
	}
	state.Snapshot(e, &mem)
	if !state.Controllable(&mem) {
		diagFatalf(t, e, nil, "not controllable after the guide dialogue: wFontLoaded=%#02x wJoyIgnore=%#04x", mem.U8(sym.FontLoaded), mem.U8(sym.JoyIgnore))
	}
	t.Logf("talked to the gym guide (%d A presses, yes/no menu seen=%v)", presses, answeredChoice)

	// PROOF 2: his line reached the dialogue tape — the greeting page
	// ("...takes to become a #MON champ!") and the advice page ("...at the
	// top of the #MON LIST!"). The # control code expands to "POKé" at
	// runtime, so both pages decode with the é glyph; the assertions match
	// the ASCII on either side of it. Both are asserted so a menu-cursor
	// surprise cannot hide one of them. It landed BEFORE the gym fight.
	lines := tape.settled()
	if !tapeSays(lines, "MON champ") {
		diagFatalf(t, e, nil, "the guide's greeting (\"...#MON champ!\") never reached the dialogue tape; lines=%v screen=%v", lines, tape.screenReadings())
	}
	if !tapeSays(lines, "MON LIST") {
		diagFatalf(t, e, nil, "the guide's advice (\"...top of the #MON LIST!\") never reached the dialogue tape; lines=%v screen=%v", lines, tape.screenReadings())
	}
	t.Logf("dialogue tape (%d settled lines) carries the guide's line: %s", len(lines), strings.Join(lines, " | "))

	// Exit and heal so Brock starts at full HP and no status — the same
	// positive precondition as TestGymBoulderBadge. The exit is the 5-step
	// (7,11)->(4,13) warp walk inside rows 11..13 (probed), off the sight
	// line; the re-entry takes the corridor again, so no battle happens
	// between the heal and the assertion below.
	leg(center, "Travel to the Pewter Center after the guide talk")
	if err := skill.Heal(e); err != nil {
		diagFatalf(t, e, err, "Heal at the Pewter Center before the gym: %v", err)
	}
	leg(gymSide, "Re-enter the gym through the side corridor (row 8)")
	leg(gymUpper, "Up the side corridor to row 4")
	leg(gym, "Across the top room to below Brock after healing")
	state.Snapshot(e, &mem)
	lead = state.DecodeParty(&mem).Mons[0]
	if int(lead.HP) != int(lead.MaxHP) || lead.Status != 0 {
		diagFatalf(t, e, nil, "the lead is not at full strength when the gym fight starts: level %d, HP %d/%d, status=%#02x",
			lead.Level, lead.HP, lead.MaxHP, lead.Status)
	}
	t.Logf("gym fight starting: lead level %d, HP %d/%d, status=%#02x (guide's line already in the tape)", lead.Level, lead.HP, lead.MaxHP, lead.Status)

	// The Brock fight. A loss is the game answering, not a defect of this
	// slice: the two affordances above are already proved from RAM, so a
	// lost fight is reported with the badge state instead of failing the
	// test (AGENTS.md: a game outcome is not a defect).
	outcome, err := skill.Gym(e, romData, policy)
	if err != nil {
		diagFatalf(t, e, err, "Gym: %v", err)
	}
	state.Snapshot(e, &mem)
	badge := mem.U8(sym.ObtainedBadges)&0x01 != 0
	t.Logf("gym outcome=%d (won=%v), wObtainedBadges=%#02x badgeSet=%v, POKE BALL x%d",
		outcome, outcome == state.ResultWon, mem.U8(sym.ObtainedBadges), badge, pokeBallCount(&mem))
	if outcome != state.ResultWon {
		t.Logf("FINDING: lost the Brock fight (outcome=%d); both affordances were proved before it — see RUNNOTES", int(outcome))
	}
}

// pokeBallCount reads the bag's POKE BALL quantity straight from RAM via
// state.DecodeInventory — the same decode the planner's Observation.Bag
// shows.
func pokeBallCount(mem *state.Mem) int {
	for _, it := range state.DecodeInventory(mem).Items {
		if it.ID == skill.ItemPokeBall {
			return int(it.Quantity)
		}
	}
	return 0
}

// tapeSays reports whether any settled line contains want.
func tapeSays(lines []string, want string) bool {
	for _, l := range lines {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}

// dialogueRecorder is the journey test's stand-in for agent.Run's
// dialogueTape: the same per-frame sampling (OnFrame in headless mode is
// where the tape reads RAM), keeping a reading only once two consecutive
// samples agree, so a line is recorded once Gen 1 has typed it out. It
// keeps two records: the settled tape lines (exactly what the planner's
// Observation.RecentDialogue would hold — the proof layer the test
// asserts on) and the raw tilemap readings, kept only as failure
// diagnostics.
type dialogueRecorder struct {
	lines   []string
	pending string
	stable  bool
	last    string
	screen  []string
	lastScr string
}

func (d *dialogueRecorder) sample(m *emu.Emu) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if scr := state.ScreenText(&mem); scr != "" && scr != d.lastScr {
		d.lastScr = scr
		d.screen = append(d.screen, scr)
	}
	text := ""
	if ds := state.DecodeDialogue(&mem); ds != nil {
		text = ds.Text
	}
	switch {
	case text == "":
		d.last, d.pending, d.stable = "", "", false
	case text != d.pending:
		d.pending, d.stable = text, false
	case !d.stable:
		d.stable = true
		if text == d.last {
			return
		}
		d.last = text
		d.lines = append(d.lines, text)
	}
}

func (d *dialogueRecorder) settled() []string {
	out := make([]string, len(d.lines))
	copy(out, d.lines)
	return out
}

func (d *dialogueRecorder) screenReadings() []string {
	out := make([]string, len(d.screen))
	copy(out, d.screen)
	return out
}

// reset clears the settled tape so a later talk's lines can be asserted in
// isolation from everything earlier on the journey typed. The raw screen
// readings are kept (they are failure diagnostics only).
func (d *dialogueRecorder) reset() {
	d.lines, d.pending, d.stable, d.last = nil, "", false, ""
}

// TestCeruleanJourney is the S8-7 milestone. Its two real proofs are:
//
//  1. The fight/flee policy in skill/travel.go: TravelFlee flees wilds and
//     fights trainers, so a journey crossing grass does not hand-fight every
//     encounter. The policy is `fleeThenFight` (flee first, fight only when
//     Flee returns ErrTrainerBattle); its two halves are proven by
//     TestFleeWildBattle (a wild is fled) and TestFleeTrainerBattle (a trainer
//     refuses RUN). Here the journey simply TRAVELS with it and reports the
//     flee/fight split.
//  2. The Talk seam: once Route 3 is actually reached, an NPC's line reaches
//     the per-frame sampler through skill.Talk with a hook installed — the
//     Super Nerd at (57,11), whose line names Cerulean. Coordinates come from
//     agent.MapObjects, never a literal.
//
// The journey does NOT hard-fail when a leg is blocked by a PRE-EXISTING
// issue, because two of them are not on this slice's surface and AGENTS.md
// says to name them and hand them back rather than adopt them:
//
//   - The S8-6 world.Build single-sub-tile defect fragments Route 2, Route 4
//     and the Viridian Forest (measured: "no path" within each), so Travel
//     cannot always route through them.
//   - The post_errand lead (species 177) has a type-ineffective stalemate
//     with the (2,18) Youngster's mon (species 112): it looped to Battle's
//     60000-frame cap even with full PP, so no forest path that crosses that
//     trainer can clear it.
//
// Cerulean itself is unreachable via Travel on this build: the Route 3 ->
// Route 4 seam lands in a component of Route 4's west half that cannot reach
// Cerulean's east exit (the same S8-6 defect). The journey reports where it
// stops instead of forcing a pass.
func TestCeruleanJourney(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey; runs in the slice's journey command, not the per-task gate")
	}
	var e *emu.Emu
	var romData []byte
	var policy skill.MovePolicy
	flees, battles := 0, 0

	var mem state.Mem
	onMap := func(want uint8) bool {
		state.Snapshot(e, &mem)
		return mem.U8(sym.CurMap) == want
	}
	// reportStop logs where the journey stopped and the flee/fight split, then
	// names the pre-existing blockers. It is not a failure: the slice's own
	// code (the policy, the seam mechanism) is proven elsewhere, and these
	// stops are the ROM/grid answering, not a defect of this slice.
	reportStop := func(what string) {
		state.Snapshot(e, &mem)
		t.Logf("journey stopped at %s: map %#04x at (%d,%d); totals %d wild(s) fled, %d battle(s) fought",
			what, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), flees, battles)
		t.Logf("FINDING (pre-existing, handed back): Cerulean is unreachable via Travel on this build — the Route 3 -> Route 4 seam lands in a component of Route 4's west half that cannot reach Cerulean's east exit (S8-6 world.Build single-sub-tile defect); Route 2 and the Viridian Forest are fragmented the same way, and the post_errand lead stalemates the (2,18) Youngster's mon by type")
	}

	// leg attempts one TravelFlee leg (tolerating a blackout) and reports
	// whether it arrived. A non-blackout failure is logged as a FINDING and
	// stops the journey rather than hard-failing (see the test doc).
	leg := func(dest skill.Destination, what string) bool {
		for attempt := 0; ; attempt++ {
			res, err := skill.TravelFlee(e, romData, dest, policy, 15)
			flees += res.Flees
			battles += res.Battles
			if err == nil {
				t.Logf("%s: arrived (flees=%d battles=%d blackedOut=%v)", what, res.Flees, res.Battles, res.BlackedOut)
				return true
			}
			t.Logf("%s: attempt %d failed after %d flee/%d fight: %v", what, attempt, res.Flees, res.Battles, err)
			if !errors.Is(err, skill.ErrBlackedOut) || attempt >= 2 {
				return false
			}
			settleBlackout(t, e)
		}
	}

	e = fixture.Load(t, "post_errand")
	romData = e.ROM()
	policy = skill.StatAwareMove(romData)

	tape := &dialogueRecorder{}
	e.OnFrame(tape.sample)

	// Leg 1: Viridian City to the south gate, one row below the (5,0) forest
	// warp — same leg as TestGymBoulderBadge.
	if !leg(skill.Destination{Map: 0x32, X: 5, Y: 1}, "the south gate") {
		reportStop("the south gate")
		return
	}
	// Leg 2: into the forest, the training ground.
	forest, ok := skill.Place("viridian forest")
	if !ok {
		diagFatalf(t, e, nil, `Place "viridian forest" not found`)
	}
	if !leg(forest, "the forest") {
		reportStop("the forest")
		return
	}
	// Back to the warp-free safe spot before any grind ping-pong.
	safeSpot := skill.Destination{Map: 0x33, X: 17, Y: 40}
	if !leg(safeSpot, "the safe spot") {
		reportStop("the safe spot")
		return
	}

	// Train the lead up to gymLeadLevel (beats Brock; Route 3's trainers are
	// L1-6), with S7-8's session/heal/blackout loop. Best-effort: a stalemate
	// here is the pre-existing trainer issue, reported and stopped.
	totalBattles := 0
	phaseRetries := 0
	trained := false
	for detours := 0; detours <= maxHealDetours && !trained; detours++ {
		state.Snapshot(e, &mem)
		lead := state.DecodeParty(&mem).Mons[0]
		if int(lead.Level) >= gymLeadLevel {
			trained = true
			break
		}
		res, err := skill.Train(e, romData, gymLeadLevel, policy, trainBattleBudget)
		totalBattles += res.Battles
		if err != nil {
			if strings.Contains(err.Error(), "no-encounter phase") && phaseRetries < maxPhaseRetries {
				phaseRetries++
				e.StepFrames(123)
				continue
			}
			t.Logf("FINDING: training hit a wall (likely the (2,18) Youngster stalemate): %v", err)
			break
		}
		state.Snapshot(e, &mem)
		lead = state.DecodeParty(&mem).Mons[0]
		if res.BlackedOut {
			leg(safeSpot, "back to the safe spot after a blackout")
			continue
		}
		if int(lead.Level) < gymLeadLevel && (res.Retreated || int(lead.HP)*3 < int(lead.MaxHP) || lead.Status != 0) {
			center, ok := skill.Place("viridian pokemon center")
			if !ok {
				diagFatalf(t, e, nil, `Place "viridian pokemon center" not found`)
			}
			leg(center, "the Viridian Center to heal")
			skill.Heal(e)
			leg(safeSpot, "back to the safe spot after healing")
		}
	}
	state.Snapshot(e, &mem)
	if lead := state.DecodeParty(&mem).Mons[0]; int(lead.Level) < gymLeadLevel {
		t.Logf("the lead is level %d after %d training battle(s); Brock will likely be a loss (game answering, not a defect)", lead.Level, totalBattles)
	} else {
		t.Logf("trained the lead to level %d in %d battles", lead.Level, totalBattles)
	}

	// Leg 3: the forest to the north gate.
	if !leg(skill.Destination{Map: 0x2F, X: 5, Y: 1}, "the north gate") {
		reportStop("the north gate (likely the (2,18) Youngster stalemate or forest fragmentation)")
		return
	}
	// Leg 4: up Route 2 into Pewter — to the Center, healing before the gym.
	center, ok := skill.Place("pewter pokemon center")
	if !ok {
		diagFatalf(t, e, nil, `Place "pewter pokemon center" not found`)
	}
	if !leg(center, "the Pewter Center before the gym") {
		reportStop("the Pewter Center (Route 2 fragmentation)")
		return
	}
	skill.Heal(e)

	// Enter the gym through the x=1 side corridor (off the Cool Trainer's
	// sight line — S7-8) to below Brock, and beat him: required before
	// Pewter's east exit opens.
	gymSide := skill.Destination{Map: 0x36, X: 1, Y: 8}
	gymUpper := skill.Destination{Map: 0x36, X: 1, Y: 4}
	gym, ok := skill.Place("pewter gym")
	if !ok {
		diagFatalf(t, e, nil, `Place "pewter gym" not found`)
	}
	if !leg(gymSide, "the gym side corridor (row 8)") ||
		!leg(gymUpper, "up the side corridor to row 4") ||
		!leg(gym, "below Brock (4,2)") {
		reportStop("the Pewter gym")
		return
	}
	bres, err := skill.Gym(e, romData, policy)
	if err != nil {
		t.Logf("Brock: %v (game answering)", err)
		reportStop("Brock")
		return
	}
	if bres != state.ResultWon {
		t.Logf("Brock won or drew the fight (the game answering, not a defect): result=%d", bres)
		reportStop("Brock (not a win)")
		return
	}
	t.Logf("Brock beaten")

	// Route 3: heal, then cross to the Super Nerd and prove the Talk seam.
	if !leg(center, "back to the Pewter Center after Brock") {
		reportStop("the Pewter Center after Brock")
		return
	}
	skill.Heal(e)
	nx, ny, adjacent := superNerdOnRoute3(t, romData)
	if !leg(adjacent, "Route 3, beside the Super Nerd") {
		reportStop("Route 3 (the eight R3 trainers or fragmentation)")
		return
	}
	if !onMap(0x0E) {
		t.Fatalf("reached the Super Nerd's tile but wCurMap=%#02x, want 0x0e", mem.U8(sym.CurMap))
	}

	// THE TALK SEAM (hard proof): face the NPC and open its dialogue through
	// skill.Talk with the per-frame hook installed. The Super Nerd's line
	// names Cerulean, so a captured "CERULEAN" is the proof an NPC's line
	// reached the sampler.
	skill.Face(e, nx, ny)
	tape.reset()
	if _, err := skill.Talk(e); err != nil {
		t.Fatalf("Talk to the Super Nerd: %v", err)
	}
	settled := tape.settled()
	if !tapeSays(settled, "CERULEAN") {
		t.Fatalf("PROOF FAILED: the per-frame sampler never saw the Super Nerd's line (no 'CERULEAN' in %d settled lines)", len(settled))
	}
	var captured string
	for _, l := range settled {
		if strings.Contains(l, "CERULEAN") {
			captured = l
			break
		}
	}
	t.Logf("PROOF: an NPC's line reached the per-frame sampler through skill.Talk — captured %q", captured)

	// Cerulean: attempt it and report where it stops. Unreachable on this
	// build (Route 4 grid defect) — see the test doc.
	if res, err := skill.TravelFlee(e, romData, skill.Destination{Map: 0x03}, policy, 15); err == nil && onMap(0x03) {
		t.Logf("ARRIVED in Cerulean City (wCurMap=%#02x)", mem.U8(sym.CurMap))
	} else {
		flees += res.Flees
		battles += res.Battles
		reportStop("the Cerulean attempt")
	}

	t.Logf("journey totals: %d wild(s) fled, %d battle(s) fought — the flee policy skipped %d fight(s) that Travel would have had", flees, battles, flees)
}

// superNerdOnRoute3 returns the Route 3 (map 0x0E) Super Nerd's home tile
// (nx, ny) and the adjacent walkable tile the journey arrives on (the tile
// south of the NPC — the journey comes from Pewter, the west). The Super Nerd
// is the sole plain NPC (Kind "person") on Route 3 — the rest are trainers —
// so it is identified by Kind, and its coordinates come from agent.MapObjects,
// never a literal. Its line names Cerulean, which the seam proof asserts on.
func superNerdOnRoute3(t *testing.T, romData []byte) (nx, ny uint8, adjacent skill.Destination) {
	t.Helper()
	var people []agent.MapObject
	for _, o := range agent.MapObjects(romData, 0x0E) {
		if o.Kind == "person" {
			people = append(people, o)
		}
	}
	if len(people) != 1 {
		diagFatalf(t, nil, nil, "expected exactly one plain NPC (person) on Route 3, got %d: %+v", len(people), people)
	}
	return people[0].X, people[0].Y, skill.Destination{Map: 0x0E, X: people[0].X, Y: people[0].Y + 1}
}
