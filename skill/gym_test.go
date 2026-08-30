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

// mistyLeadLevel is the level the lead must reach before Cerulean. Misty's
// team is a level 18 Staryu and a level 21 Starmie (data/trainers/
// parties.asm, MistyData). The lead is a water starter facing a water team,
// so there is no type edge the way water had against Brock's rock team: the
// target sits two levels above her ace rather than one.
const mistyLeadLevel = 24

// mistyMaxHealDetours bounds the Route 4 grind (level 12 -> 24) more loosely
// than maxHealDetours does the forest grind (7 -> 12): it is roughly twice
// as long, and the retreat line takes a detour every time the lead drops
// below half HP. A run that exhausts this many detours cannot reach the
// level and is reported, not retried.
const mistyMaxHealDetours = 12

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
	// detour is the approach the journey already made. Train stops a session
	// when the lead drops below its retreat line (half max HP), so the grind
	// runs in short sessions and the test heals the lead between them: a
	// session that left the lead hurt (retreated, under a third of its HP,
	// or statused) takes a detour to the Viridian Center, and a session that
	// blacked out is already healed by the blackout itself — travel back
	// into the forest and resume.
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
				// Pokemon Center: nothing to heal, just wait for the respawn
				// warp to land and get back into the grass.
				settleBlackout(t, e)
				if _, err := skill.Travel(e, romData, safeSpot, policy, 10); err != nil {
					diagFatalf(t, e, err, "Travel back to the forest after a blackout: %v", err)
				}
				t.Logf("grind session %d blacked out at level %d (detour %d): party healed by the blackout, resuming", detours+1, lead.Level, detours)
				continue
			}
			if int(lead.Level) < gymLeadLevel && (res.Retreated || int(lead.HP)*3 < int(lead.MaxHP) || lead.Status != 0) {
				center, ok := skill.Place("viridian pokemon center")
				if !ok {
					diagFatalf(t, e, nil, `Place "viridian pokemon center" not found`)
				}
				// A blackout ON the walk to the center is the same recovery as
				// an in-session one: the party respawns fully healed at the last
				// town. Measured on S9-9's first run of this test: the retreat
				// line takes the detour with the lead near half HP, and one wild
				// battle on the forest leg ended the walk (respawn at Pallet
				// Town, party healed).
				if _, err := skill.Travel(e, romData, center, policy, 5); err != nil {
					if !errors.Is(err, skill.ErrBlackedOut) {
						diagFatalf(t, e, err, "Travel to the Viridian Center to heal: %v", err)
					}
					settleBlackout(t, e)
				} else {
					if err := skill.Heal(e); err != nil {
						diagFatalf(t, e, err, "Heal at the Viridian Center: %v", err)
					}
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

// TestGymCascadeBadge is the Cerulean half of the gym generalisation: the
// same journey as TestGymBoulderBadge (which this test re-runs in full,
// because beating Brock is a hard prerequisite — Pewter's east exit stays
// locked until EVENT_BEAT_BROCK, S8-7), then across Route 3 and the
// Route 3 -> Route 4 seam to Cerulean, a grind on Route 4's grass to the
// level that beats her, and skill.Gym on map 0x41. It exists to answer two
// questions Pewter never could: does Travel carry the gym approach past the
// COOLTRAINER_F at (2,3) whose sight line runs along the approach tile's
// own row, and does a win set BadgeCascade (bit 1), not BadgeBoulder (bit 0)
// — the exact failure the table generalisation was meant to prevent.
//
// Like TestGymBoulderBadge it is a full journey (minutes of emulation and
// stochastic wild battles) and runs only outside -short. A lost fight is
// reported and NOT retried: the rDIV-seeded RNG makes re-runs a different
// game, so a retry would answer nothing.
func TestGymCascadeBadge(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey; runs in the slice's journey command, not the per-task gate")
	}
	e := fixture.Load(t, "post_errand")
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)

	// Count every battle the run enters (wIsInBattle 0->1). The counter is
	// the only record of what the approach resolved: Gym discards its
	// approach TravelResult, and the COOLTRAINER_F at (2,3) faces right
	// along row 3 — the same row as Place("cerulean gym") (4,3) — so the
	// approach is expected to engage her. The hook runs synchronously inside
	// StepFrame on this goroutine, so no synchronisation is needed.
	var inBattle bool
	battles := 0
	e.OnFrame(func(em *emu.Emu) {
		b := em.Peek8(sym.IsInBattle) != 0
		if b && !inBattle {
			battles++
		}
		inBattle = b
	})

	// The Pewter half: legs and grind are the proven TestGymBoulderBadge
	// scaffold, copied rather than refactored — this test's surface is
	// Cerulean, and a change to the proven half would make a failure here
	// ambiguous about which half moved.
	southGate := skill.Destination{Map: 0x32, X: 5, Y: 1}
	journeyLeg(t, e, romData, southGate, policy, "Travel to the south gate")
	forest, ok := skill.Place("viridian forest")
	if !ok {
		diagFatalf(t, e, nil, `Place "viridian forest" not found`)
	}
	journeyLeg(t, e, romData, forest, policy, "Travel to the forest")
	safeSpot := skill.Destination{Map: 0x33, X: 17, Y: 40}
	journeyLeg(t, e, romData, safeSpot, policy, "Travel to the safe spot")

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
				if strings.Contains(err.Error(), "no-encounter phase") && phaseRetries < maxPhaseRetries {
					phaseRetries++
					e.StepFrames(123)
					continue
				}
				diagFatalf(t, e, err, "Train: %v (start=%d end=%d battles=%d, reached=%v, blackedOut=%v)", err, res.StartLevel, res.EndLevel, res.Battles, res.Reached, res.BlackedOut)
			}
			totalBattles += res.Battles
			state.Snapshot(e, &mem)
			lead = state.DecodeParty(&mem).Mons[0]
			if res.BlackedOut {
				settleBlackout(t, e)
				if _, err := skill.Travel(e, romData, safeSpot, policy, 10); err != nil {
					diagFatalf(t, e, err, "Travel back to the forest after a blackout: %v", err)
				}
				continue
			}
			if int(lead.Level) < gymLeadLevel && (res.Retreated || int(lead.HP)*3 < int(lead.MaxHP) || lead.Status != 0) {
				center, ok := skill.Place("viridian pokemon center")
				if !ok {
					diagFatalf(t, e, nil, `Place "viridian pokemon center" not found`)
				}
				if _, err := skill.Travel(e, romData, center, policy, 5); err != nil {
					if !errors.Is(err, skill.ErrBlackedOut) {
						diagFatalf(t, e, err, "Travel to the Viridian Center to heal: %v", err)
					}
					settleBlackout(t, e)
				} else {
					if err := skill.Heal(e); err != nil {
						diagFatalf(t, e, err, "Heal at the Viridian Center: %v", err)
					}
				}
				if _, err := skill.Travel(e, romData, safeSpot, policy, 10); err != nil {
					diagFatalf(t, e, err, "Travel back to the forest after healing: %v", err)
				}
			}
		}
		state.Snapshot(e, &mem)
		if lead := state.DecodeParty(&mem).Mons[0]; int(lead.Level) < gymLeadLevel {
			diagFatalf(t, e, nil, "the lead is level %d after %d battles and %d heal detour(s), want >= %d to face Brock (HP %d/%d, status=%#02x)",
				lead.Level, totalBattles, maxHealDetours, gymLeadLevel, lead.HP, lead.MaxHP, lead.Status)
		}
	}
	// Leave the forest healthy (same guard as the Boulder test).
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
	}

	// North gate, then up Route 2 into Pewter and through the gym door.
	northGate := skill.Destination{Map: 0x2F, X: 5, Y: 1}
	journeyLeg(t, e, romData, northGate, policy, "Travel to the north gate")
	gym, ok := skill.Place("pewter gym")
	if !ok {
		diagFatalf(t, e, nil, "Place \"pewter gym\" not found")
	}
	journeyLeg(t, e, romData, gym, policy, "Travel to the Pewter gym")

	// Heal before Brock: full HP or the test fails before he moves.
	center, ok := skill.Place("pewter pokemon center")
	if !ok {
		diagFatalf(t, e, nil, `Place "pewter pokemon center" not found`)
	}
	if _, err := skill.Travel(e, romData, center, policy, 5); err != nil {
		diagFatalf(t, e, err, "Travel to the Pewter Center before the gym: %v", err)
	}
	if err := skill.Heal(e); err != nil {
		diagFatalf(t, e, err, "Heal at the Pewter Center: %v", err)
	}
	if _, err := skill.Travel(e, romData, gym, policy, 5); err != nil {
		diagFatalf(t, e, err, "Travel back to the Pewter gym door after healing: %v", err)
	}

	// Brock is a hard prerequisite, not an optional fight: Pewter's east
	// exit stays locked until EVENT_BEAT_BROCK (S8-7), so a loss here means
	// Cerulean is unreachable and the run stops — reported, not retried.
	if outcome, err := skill.Gym(e, romData, policy); err != nil {
		diagFatalf(t, e, err, "Gym (Brock): %v", err)
	} else if outcome != state.ResultWon {
		diagFatalf(t, e, nil, "Brock won or drew (the game answering, not a defect); Pewter's east exit stays locked, so Cerulean is unreachable: result=%d, battles so far=%d", int(outcome), battles)
	}
	state.Snapshot(e, &mem)
	t.Logf("Brock beaten; wObtainedBadges=%#02x, battles so far=%d", mem.U8(sym.ObtainedBadges), battles)

	// The party is hurt after Brock; heal before the Route 3 crossing.
	if _, err := skill.Travel(e, romData, center, policy, 5); err != nil {
		diagFatalf(t, e, err, "Travel to the Pewter Center after Brock: %v", err)
	}
	if err := skill.Heal(e); err != nil {
		diagFatalf(t, e, err, "Heal at the Pewter Center after Brock: %v", err)
	}

	// The Cerulean road: Pewter's east exit onto Route 3, across its eight
	// trainers, north at the east end into Route 4, and east to Cerulean.
	// This is the leg S8-7 reported as blocked (the seam allegedly landing
	// in Route 4's walled-off west half); the run answers it. The cap is
	// higher than journeyLeg's 10: eight trainers plus wilds on one leg.
	battlesBeforeRoad := battles
	route3, ok := skill.Place("route 3")
	if !ok {
		diagFatalf(t, e, nil, `Place "route 3" not found`)
	}
	for attempt := 0; ; attempt++ {
		_, err := skill.Travel(e, romData, route3, policy, 20)
		if err == nil {
			break
		}
		if !errors.Is(err, skill.ErrBlackedOut) || attempt >= 2 {
			diagFatalf(t, e, err, "Travel to Route 3: %v", err)
		}
		t.Logf("Route 3 leg blacked out (attempt %d): resuming", attempt+1)
		settleBlackout(t, e)
	}
	t.Logf("on Route 3: battles on the road so far=%d", battles-battlesBeforeRoad)

	// The training ground is Route 4's east half — the grass cells at
	// (60..73, 8..11) sit a few steps from (60,8), and its L9-12 wilds are
	// the fastest grind available before Cerulean. Reaching it crosses the
	// Route 3 -> Route 4 seam; if that seam is the wall S8-7 measured, this
	// leg is where the run stops.
	grindSpot := skill.Destination{Map: 0x0F, X: 60, Y: 8}
	for attempt := 0; ; attempt++ {
		_, err := skill.Travel(e, romData, grindSpot, policy, 10)
		if err == nil {
			break
		}
		if !errors.Is(err, skill.ErrBlackedOut) || attempt >= 2 {
			diagFatalf(t, e, err, "Travel to the Route 4 grind spot (the seam): %v", err)
		}
		t.Logf("Route 4 leg blacked out (attempt %d): resuming", attempt+1)
		settleBlackout(t, e)
	}
	state.Snapshot(e, &mem)
	if mem.U8(sym.CurMap) != 0x0F {
		diagFatalf(t, e, nil, "grind spot leg ended on map %#04x, want 0x0f (Route 4)", mem.U8(sym.CurMap))
	}
	t.Logf("on Route 4 east of the wall: battles on the road=%d", battles-battlesBeforeRoad)

	// Grind the lead from Brock level to Misty level on Route 4's grass,
	// healing at the Cerulean Center between hurt sessions. Same shape as
	// the forest grind, with its own (larger) detour budget.
	ceruleanCenter, ok := skill.Place("cerulean pokemon center")
	if !ok {
		diagFatalf(t, e, nil, `Place "cerulean pokemon center" not found`)
	}
	totalBattles := 0
	phaseRetries := 0
	for detours := 0; detours <= mistyMaxHealDetours; detours++ {
		state.Snapshot(e, &mem)
		lead = state.DecodeParty(&mem).Mons[0]
		if int(lead.Level) >= mistyLeadLevel {
			break
		}
		res, err := skill.Train(e, romData, mistyLeadLevel, policy, trainBattleBudget)
		if err != nil {
			if strings.Contains(err.Error(), "no-encounter phase") && phaseRetries < maxPhaseRetries {
				phaseRetries++
				e.StepFrames(123)
				continue
			}
			diagFatalf(t, e, err, "Train on Route 4: %v (start=%d end=%d battles=%d, reached=%v, blackedOut=%v)", err, res.StartLevel, res.EndLevel, res.Battles, res.Reached, res.BlackedOut)
		}
		totalBattles += res.Battles
		state.Snapshot(e, &mem)
		lead = state.DecodeParty(&mem).Mons[0]
		if res.BlackedOut {
			settleBlackout(t, e)
			if _, err := skill.Travel(e, romData, grindSpot, policy, 10); err != nil {
				diagFatalf(t, e, err, "Travel back to the Route 4 grind spot after a blackout: %v", err)
			}
			continue
		}
		if int(lead.Level) < mistyLeadLevel && (res.Retreated || int(lead.HP)*3 < int(lead.MaxHP) || lead.Status != 0) {
			if _, err := skill.Travel(e, romData, ceruleanCenter, policy, 5); err != nil {
				if !errors.Is(err, skill.ErrBlackedOut) {
					diagFatalf(t, e, err, "Travel to the Cerulean Center to heal: %v", err)
				}
				settleBlackout(t, e)
			} else {
				if err := skill.Heal(e); err != nil {
					diagFatalf(t, e, err, "Heal at the Cerulean Center: %v", err)
				}
			}
			if _, err := skill.Travel(e, romData, grindSpot, policy, 10); err != nil {
				diagFatalf(t, e, err, "Travel back to the Route 4 grind spot after healing: %v", err)
			}
			t.Logf("healed the lead at level %d (HP %d/%d, status=%#02x) (detour %d)", lead.Level, lead.HP, lead.MaxHP, lead.Status, detours)
		}
	}
	state.Snapshot(e, &mem)
	lead = state.DecodeParty(&mem).Mons[0]
	if int(lead.Level) < mistyLeadLevel {
		diagFatalf(t, e, nil, "the lead is level %d after %d Route 4 battles and %d heal detour(s), want >= %d to face Misty (HP %d/%d, status=%#02x)",
			lead.Level, totalBattles, mistyMaxHealDetours, mistyLeadLevel, lead.HP, lead.MaxHP, lead.Status)
	}
	t.Logf("trained the lead to level %d on Route 4 in %d battles (HP %d/%d, status=%#02x)", lead.Level, totalBattles, lead.HP, lead.MaxHP, lead.Status)

	// Leave the grind healthy: a statused or half-HP lead would black out
	// on the short walk to the gym and take the run down with it.
	if lead.Status != 0 || int(lead.HP)*2 < int(lead.MaxHP) {
		if _, err := skill.Travel(e, romData, ceruleanCenter, policy, 5); err != nil {
			diagFatalf(t, e, err, "Travel to the Cerulean Center before the gym: %v", err)
		}
		if err := skill.Heal(e); err != nil {
			diagFatalf(t, e, err, "Heal at the Cerulean Center before the gym: %v", err)
		}
	}

	// The approach: through the gym door to the stand-beside tile (4,3).
	// The COOLTRAINER_F at (2,3) faces right along that row, and the
	// SWIMMER at (8,7) faces left across the column the walk takes — this
	// Travel is expected to resolve them, and the battle counter records
	// how many it actually did.
	ceruleanGym, ok := skill.Place("cerulean gym")
	if !ok {
		diagFatalf(t, e, nil, `Place "cerulean gym" not found`)
	}
	battlesBeforeApproach := battles
	state.Snapshot(e, &mem)
	lead = state.DecodeParty(&mem).Mons[0]
	if int(lead.HP) != int(lead.MaxHP) || lead.Status != 0 {
		diagFatalf(t, e, nil, "the lead is not at full strength when the approach starts: level %d, HP %d/%d, status=%#02x",
			lead.Level, lead.HP, lead.MaxHP, lead.Status)
	}
	if _, err := skill.Travel(e, romData, ceruleanGym, policy, 5); err != nil {
		diagFatalf(t, e, err, "Travel to the Cerulean gym approach tile: %v", err)
	}
	t.Logf("approach done: battles resolved on the way in=%d (total=%d)", battles-battlesBeforeApproach, battles)

	// The fight, exactly once.
	outcome, err := skill.Gym(e, romData, policy)
	if err != nil {
		diagFatalf(t, e, err, "Gym (Misty): %v", err)
	}
	state.Snapshot(e, &mem)
	if outcome != state.ResultWon {
		diagFatalf(t, e, nil, "Misty won or drew the fight (the game answering, not a defect); NOT retried: result=%d, battles total=%d, wObtainedBadges=%#02x", int(outcome), battles, mem.U8(sym.ObtainedBadges))
	}

	// The badge bit Gym's postcondition names is Cascade (bit 1) — not
	// Boulder (bit 0), which the pre-generalisation code would have
	// checked. Poll for it the way the Boulder test does, and report the
	// RAW byte: bit 1 set is the answer, and anything else in the byte is
	// evidence the table's badge is wrong.
	for i := 0; i < 3000; i++ {
		e.StepFrame()
		state.Snapshot(e, &mem)
		if mem.U8(sym.ObtainedBadges)&0x02 != 0 {
			break
		}
	}
	state.Snapshot(e, &mem)
	if mem.U8(sym.ObtainedBadges)&0x02 == 0 {
		diagFatalf(t, e, nil, "wObtainedBadges = %#02x after the gym, want bit 1 (Cascade) set: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.ObtainedBadges), mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	if !state.Controllable(&mem) {
		diagFatalf(t, e, nil, "player not controllable after the gym: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	t.Logf("second badge won: lead level %d, wObtainedBadges=%#02x (raw), battles total=%d (approach resolved %d)",
		state.DecodeParty(&mem).Mons[0].Level, mem.U8(sym.ObtainedBadges), battles, battles-battlesBeforeApproach)
}
