package skill_test

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// gymLeadLevel is the level the lead must reach before Pewter. Two fights
// stand in the way: the rival's forced cutscene battle on the forest leg
// (the player blacked out at level 8) and Brock himself, whose team is a
// level 12 Geodude and a level 14 Onix. The forest grass is sparse (about
// one battle per thirty steps), so the target stays within reach of a
// single training loop; a water lead at 12 is super-effective against the
// rock team and clears both. If a battle still loses, this is the knob.
const gymLeadLevel = 12

// trainBattleBudget bounds one grind session to a few battles, so the test
// regains control of the lead's HP before it can faint: Train itself has no
// HP awareness, and a one-mon party that grinds 25 battles blacks out at
// level 8. The heal detours below are what make the grind to 12 survive.
const trainBattleBudget = 4

// maxHealDetours bounds how many times the grind may leave the forest to
// heal (or come back from a blackout) before the test gives up and reports
// that the lead cannot reach the level. It must cover the blackouts the
// game itself forces: a one-mon party loses wild battles on the way up
// (measured: blackouts at levels 8, 10 and 11 in one run), each of which
// costs a detour, and there are still grind sessions to fight after them.
const maxHealDetours = 6

// maxPhaseRetries bounds retries after a no-encounter phase. The wild
// encounter roll mixes in the frame counter (rDIV), and a blackout's fade
// frames can shift the grind legs into a cycle where no encounter ever
// fires (measured: 141 legs, 2 encounters; a fresh Train from the same
// state fought on its first leg). Stepping a few frames breaks the phase.
const maxPhaseRetries = 3

// diagFatalf fails the test with the failure diagnostic bundle appended to
// the message: map id, player tile, controllable, in battle, the decoded
// sprite slots and their tiles, the blocker set the planner derives from
// that same snapshot, and the typed error chain. Sprite timing is
// nondeterministic, so a bare "blocked at (x,y)" cannot be diagnosed after
// the fact — the sprite that caused it has moved by the time anyone reads
// the log — and this bundle is the evidence a failed run leaves behind.
func diagFatalf(t *testing.T, e *emu.Emu, err error, format string, args ...any) {
	t.Helper()
	t.Fatalf(format+"\n%s", append(args, diagnosticBundle(e, err))...)
}

// journeyLeg walks one Travel leg, tolerating a blackout: a lost wild battle
// ends the walk with ErrBlackedOut, but the party comes back fully healed at
// a center, so the leg is simply resumed from where the respawn landed. This
// is recovery, not retrying a result — the leg has no outcome of its own,
// and the gym fight below is still made exactly once. Any other failure is
// fatal exactly as before; the blackout retries are bounded.
func journeyLeg(t *testing.T, e *emu.Emu, romData []byte, dest skill.Destination, policy skill.MovePolicy, what string) {
	t.Helper()
	for attempt := 0; ; attempt++ {
		_, err := skill.Travel(e, romData, dest, policy, 10)
		if err == nil {
			return
		}
		if !errors.Is(err, skill.ErrBlackedOut) || attempt >= 2 {
			diagFatalf(t, e, err, "%s: %v", what, err)
		}
		t.Logf("%s blacked out (attempt %d): party healed by the blackout, resuming", what, attempt+1)
		settleBlackout(t, e)
	}
}

// settleBlackout steps frames without input until a blackout's respawn warp
// has landed: the party fully healed and the player controllable. Travel
// returns ErrBlackedOut before the respawn completes (measured: the warp
// lands within about 50 frames of the loss), and walking in that gap is
// worse than waiting — the next plan is made where the battle was lost, but
// the player is about to be teleported, so stale steps replay from the wrong
// map (measured: lost at Pewter (18,31), respawned at Pallet (5,6), stale
// steps ended blocked at Pallet (9,8)).
func settleBlackout(t *testing.T, e *emu.Emu) {
	t.Helper()
	for i := 0; i < 200; i++ {
		e.StepFrames(25)
		var mem state.Mem
		state.Snapshot(e, &mem)
		if !state.Controllable(&mem) {
			continue
		}
		lead := state.DecodeParty(&mem).Mons[0]
		if int(lead.HP) == int(lead.MaxHP) && lead.Status == 0 {
			return
		}
	}
	diagFatalf(t, e, nil, "settleBlackout: the respawn warp did not land within 5000 frames")
}

// diagnosticBundle snapshots the emulator once and renders the diagnostic
// block. The sprite dump and the blocker set come from the same snapshot,
// so the block is self-consistent: every blocked tile names the sprite slot
// that stands on it, the same derivation the planner's walkAround snapshot
// uses.
func diagnosticBundle(e *emu.Emu, err error) string {
	var mem state.Mem
	state.Snapshot(e, &mem)
	var b strings.Builder
	fmt.Fprintf(&b, "  map=%#04x player=(%d,%d) controllable=%v battle=%v\n",
		mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
		state.Controllable(&mem), state.DecodeBattle(&mem) != nil)
	sprites := state.DecodeSprites(&mem)
	if len(sprites) == 0 {
		b.WriteString("  sprites: (none live)\n")
	}
	var blocked []string
	for _, s := range sprites {
		fmt.Fprintf(&b, "  sprite slot %2d: tile (%2d,%2d) pic=%#02x\n", s.Slot, s.X, s.Y, s.PictureID)
		blocked = append(blocked, fmt.Sprintf("(%d,%d)", s.X, s.Y))
	}
	sort.Strings(blocked)
	fmt.Fprintf(&b, "  blockers: %s\n", strings.Join(blocked, " "))
	if err == nil {
		b.WriteString("  error: (none)\n")
	}
	for i, link := 0, err; link != nil; i, link = i+1, errors.Unwrap(link) {
		fmt.Fprintf(&b, "  error[%d]: %T: %s\n", i, link, link.Error())
	}
	return b.String()
}

// TestGymBoulderBadge is the journey milestone: from the fresh post_errand
// checkpoint (the player has just delivered Oak's parcel and is back in
// Viridian City) it travels to the Pewter Gym, trains the lead first if it
// is under the level that beats Brock, fights the gym, and proves the
// Boulder Badge is set in RAM with the player controllable again.
//
// The route has four travel legs. The gates (maps 0x32 south, 0x2F north)
// are small bridge maps between Route 2 and the forest, and each exposes
// two warp tiles to the same destination: (4,0) is non-walkable in this
// ROM and the pathfinder's only route to it crosses the (5,0) warp, so
// Traverse selects the reachable (5,0) tile from where the player stands.
//
//  1. 0x01 -> 0x32 (5,1)    city to Route 2, up the band to the south gate
//  2. 0x32 -> 0x33 (17,43)  south gate -> forest, the training ground
//  3. 0x33 -> 0x2F (5,1)    through the forest to the north gate
//  4. 0x2F -> gym           north gate -> Route 2's north band -> Pewter City, through the door
// It is a full journey — minutes of emulation and stochastic wild battles
// — so it runs only outside -short, in the slice's journey command, not the
// per-task gate. The whole journey and its diagnostic wiring stay intact
// below the guard: slice 6 starts from this test, and a deleted milestone
// is how TestTravelToPewter was nearly lost.
func TestGymBoulderBadge(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey; runs in the slice's journey command, not the per-task gate")
	}
	e := fixture.Load(t, "post_errand")
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)

	// Leg 1: Viridian City to the south gate, stopping one row below the
	// (5,0) forest warp. The gate is entered from Route 2's warp (3,43).
	southGate := skill.Destination{Map: 0x32, X: 5, Y: 1}
	journeyLeg(t, e, romData, southGate, policy, "Travel to the south gate")

	// Leg 2: the south gate into the forest. Traverse picks the reachable
	// (5,0) warp tile itself; the landing (17,47) is the training ground's
	// own row, so the detour Train makes is the approach the journey
	// already took.
	forest, ok := skill.Place("viridian forest")
	if !ok {
		diagFatalf(t, e, nil, `Place "viridian forest" not found`)
	}
	journeyLeg(t, e, romData, forest, policy, "Travel to the forest")

	// The gate drops the player at (17,47), inside the pocket that holds the
	// four south warps (15,47)-(18,47). A grind ping-pong there steps on a
	// warp and gets carried back to Route 2's dead-end south band, so walk up
	// into the open forest first: the grass the trainer finds is a few steps
	// from here, and it is clear of every warp on the map.
	safeSpot := skill.Destination{Map: 0x33, X: 17, Y: 40}
	journeyLeg(t, e, romData, safeSpot, policy, "Travel to the safe spot")

	// Train in the forest itself if the lead is under the level that
	// beats Brock: its grass is a few steps from the travel target, so the
	// detour is the approach the journey already made. Train has no HP
	// awareness, so the grind runs in short sessions and the test heals the
	// lead between them: a session that leaves the lead under a third of its
	// HP (or statused) takes a detour to the Viridian Center, and a session
	// that blacked out is already healed by the blackout itself — travel
	// back into the forest and resume.
	var mem state.Mem
	state.Snapshot(e, &mem)
	if lead := state.DecodeParty(&mem).Mons[0].Level; int(lead) < gymLeadLevel {
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
				if strings.Contains(err.Error(), "without enough encounters") && phaseRetries < maxPhaseRetries {
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
				// Pokemon Center: nothing to heal, just wait for the respawn
				// warp to land and get back into the grass.
				settleBlackout(t, e)
				if _, err := skill.Travel(e, romData, safeSpot, policy, 10); err != nil {
					diagFatalf(t, e, err, "Travel back to the forest after a blackout: %v", err)
				}
				t.Logf("grind session %d blacked out at level %d (detour %d): party healed by the blackout, resuming", detours+1, lead.Level, detours)
				continue
			}
			if int(lead.Level) < gymLeadLevel && (int(lead.HP)*3 < int(lead.MaxHP) || lead.Status != 0) {
				center, ok := skill.Place("viridian pokemon center")
				if !ok {
					diagFatalf(t, e, nil, `Place "viridian pokemon center" not found`)
				}
				if _, err := skill.Travel(e, romData, center, policy, 5); err != nil {
					diagFatalf(t, e, err, "Travel to the Viridian Center to heal: %v", err)
				}
				if err := skill.Heal(e); err != nil {
					diagFatalf(t, e, err, "Heal at the Viridian Center: %v", err)
				}
				if _, err := skill.Travel(e, romData, safeSpot, policy, 10); err != nil {
					diagFatalf(t, e, err, "Travel back to the forest after healing: %v", err)
				}
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
	}
	// Leave the forest healthy: the grind ends the moment the level is
	// reached, which can be with a statused lead (measured: level 12 at
	// 22/36 HP, paralyzed), and a lead like that blacks out on the forest
	// leg and the journey dies with it. The detour is taken only when the
	// lead is actually in danger — statused or under half HP — because a
	// long walk back through the gates is where the journey can go wrong,
	// and a 33/36 lead needs no heal (the Pewter Center one before the gym
	// is unconditional either way). Same detour as inside the loop: Viridian
	// Center, heal, back to the safe spot.
	state.Snapshot(e, &mem)
	lead := state.DecodeParty(&mem).Mons[0]
	if lead.Status != 0 || int(lead.HP)*2 < int(lead.MaxHP) {
		center, ok := skill.Place("viridian pokemon center")
		if !ok {
			diagFatalf(t, e, nil, `Place "viridian pokemon center" not found`)
		}
		if _, err := skill.Travel(e, romData, center, policy, 5); err != nil {
			diagFatalf(t, e, err, "Travel to the Viridian Center before leaving the forest: %v", err)
		}
		if err := skill.Heal(e); err != nil {
			diagFatalf(t, e, err, "Heal at the Viridian Center before leaving the forest: %v", err)
		}
		if _, err := skill.Travel(e, romData, safeSpot, policy, 10); err != nil {
			diagFatalf(t, e, err, "Travel back to the forest after the pre-journey heal: %v", err)
		}
		t.Logf("left the forest healed: level %d (was HP %d/%d, status=%#02x)", lead.Level, lead.HP, lead.MaxHP, lead.Status)
	}
	state.Snapshot(e, &mem)
	t.Logf("post-train position: map=%#04x at (%d,%d) controllable=%v",
		mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), state.Controllable(&mem))

	// Leg 3: the forest to the north gate.
	northGate := skill.Destination{Map: 0x2F, X: 5, Y: 1}
	journeyLeg(t, e, romData, northGate, policy, "Travel to the north gate")

	// Leg 4: the north gate into Route 2's north band, then up Route 2
	// into Pewter City and through the gym door. The gate crossing is an
	// ordinary leg: Traverse picks the reachable (5,0) warp tile itself.
	gym, ok := skill.Place("pewter gym")
	if !ok {
		diagFatalf(t, e, nil, "Place \"pewter gym\" not found")
	}
	journeyLeg(t, e, romData, gym, policy, "Travel to the gym")

	// Heal before the gym: the previous attempt fought Brock at 24/36 HP and
	// lost. The Pewter Center is a step off the gym door, so this detour is
	// cheap, and the assertion below makes the precondition positive — the
	// fight must start with the lead at full HP and no status, or the test
	// fails before Brock ever moves.
	center, ok := skill.Place("pewter pokemon center")
	if !ok {
		diagFatalf(t, e, nil, `Place "pewter pokemon center" not found`)
	}
	if _, err := skill.Travel(e, romData, center, policy, 5); err != nil {
		diagFatalf(t, e, err, "Travel to the Pewter Center before the gym: %v", err)
	}
	if err := skill.Heal(e); err != nil {
		diagFatalf(t, e, err, "Heal at the Pewter Center before the gym: %v", err)
	}
	if _, err := skill.Travel(e, romData, gym, policy, 5); err != nil {
		diagFatalf(t, e, err, "Travel back to the gym door after healing: %v", err)
	}
	state.Snapshot(e, &mem)
	lead = state.DecodeParty(&mem).Mons[0]
	if int(lead.HP) != int(lead.MaxHP) || lead.Status != 0 {
		diagFatalf(t, e, nil, "the lead is not at full strength when the gym fight starts: level %d, HP %d/%d, status=%#02x",
			lead.Level, lead.HP, lead.MaxHP, lead.Status)
	}
	t.Logf("gym fight starting: lead level %d, HP %d/%d, status=%#02x", lead.Level, lead.HP, lead.MaxHP, lead.Status)

	outcome, err := skill.Gym(e, romData, policy)
	if err != nil {
		diagFatalf(t, e, err, "Gym: %v", err)
	}
	if outcome != state.ResultWon {
		diagFatalf(t, e, nil, "Gym outcome = %d, want ResultWon", int(outcome))
	}

	// Gym's postcondition is the badge itself, so this poll is expected to
	// succeed on the first snapshot; it stays as the trip wire that would
	// catch a Gym that returned before the write landed.
	for i := 0; i < 3000; i++ {
		e.StepFrame()
		state.Snapshot(e, &mem)
		if mem.U8(sym.ObtainedBadges)&0x01 != 0 {
			break
		}
	}
	state.Snapshot(e, &mem)
	if mem.U8(sym.ObtainedBadges)&0x01 == 0 {
		diagFatalf(t, e, nil, "wObtainedBadges = %#02x after the gym, want bit 0 (Boulder) set: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.ObtainedBadges), mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	if !state.Controllable(&mem) {
		diagFatalf(t, e, nil, "player not controllable after the gym: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	t.Logf("first badge won: lead level %d, wObtainedBadges=%#02x", state.DecodeParty(&mem).Mons[0].Level, mem.U8(sym.ObtainedBadges))
}
