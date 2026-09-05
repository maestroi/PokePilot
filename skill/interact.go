package skill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// ErrNoDialogue reports that pressing A did not open a text box.
var ErrNoDialogue = errors.New("skill: A did not open a text box")

// ponytail: the budgets below are empirical, measured on this ROM. The
// A-press cadence in Talk matters: the same TV sign took 10 presses at a
// 40-frame cadence and 6 at a 100-frame cadence, so each press is followed
// by a settle interval that keeps the cadence in the cheaper regime.
const (
	faceTurnBudget = 60  // frames for a direction tap to register as a turn
	talkOpenBudget = 120 // frames for a text box to open after pressing A
	talkSettle     = 40  // frames stepped after each A press while the box is up
	talkPressCap   = 30  // A presses before Talk gives up on a stubborn box
)

// directionTo maps a tile orthogonally adjacent to (sx,sy) to the step
// toward it. ok is false when (tx,ty) is not exactly one tile away on one
// axis.
func directionTo(sx, sy, tx, ty uint8) (world.Step, bool) {
	switch {
	case int(tx) == int(sx) && int(ty) == int(sy)-1:
		return world.StepUp, true
	case int(tx) == int(sx) && int(ty) == int(sy)+1:
		return world.StepDown, true
	case int(tx) == int(sx)-1 && int(ty) == int(sy):
		return world.StepLeft, true
	case int(tx) == int(sx)+1 && int(ty) == int(sy):
		return world.StepRight, true
	}
	return world.Step{}, false
}

func facingFor(s world.Step) state.Facing {
	switch s {
	case world.StepUp:
		return state.FacingUp
	case world.StepDown:
		return state.FacingDown
	case world.StepLeft:
		return state.FacingLeft
	case world.StepRight:
		return state.FacingRight
	}
	return 0
}

// Face turns the player to look at the orthogonally adjacent tile (tx,ty).
// It returns an error if the tile is not orthogonally adjacent, or if the
// facing did not change within the budget.
//
// The completion predicate is the decoded facing, not the position: a tap
// toward an open tile may move the player onto it, and that is fine — the
// facing is what Face promises.
func Face(m *emu.Emu, tx, ty uint8) error {
	x, y := playerXY(m)
	step, ok := directionTo(x, y, tx, ty)
	if !ok {
		return fmt.Errorf("skill: Face: tile (%d,%d) is not orthogonally adjacent to (%d,%d)", tx, ty, x, y)
	}
	btn, ok := buttonFor(step)
	if !ok {
		return fmt.Errorf("skill: Face: invalid step %s", step)
	}
	want := facingFor(step)

	m.Tap(btn, 3, 7)
	var mem state.Mem
	if _, err := m.StepUntil(faceTurnBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return state.DecodePlayer(&mem).Facing == want
	}); err != nil {
		return fmt.Errorf("skill: Face: not facing %s within %d frames", want, faceTurnBudget)
	}
	return nil
}

// Talk presses A to open a text box, then keeps pressing A while a box is
// up until it closes. It returns the number of A presses spent.
//
// The open/closed signal is FontLoaded, never TextBoxID (which read 0x01
// before, during and after the measured dialogue). The press count is
// timing-dependent, so Talk is a bounded poll and callers must not assert
// a specific count.
func Talk(m *emu.Emu) (int, error) {
	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(talkOpenBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0
	}); err != nil {
		return 0, ErrNoDialogue
	}

	presses := 1
	for m.Peek8(sym.FontLoaded) != 0 {
		if presses >= talkPressCap {
			return presses, fmt.Errorf("skill: Talk: text box still open after %d A presses", talkPressCap)
		}
		m.Tap(emu.A, 3, 7)
		presses++
		m.StepFrames(talkSettle)
	}

	// The box is down, but the game may still be settling: wJoyIgnore can
	// clear a few frames after wFontLoaded. Wait for controllable rather
	// than asserting on the very next frame.
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		if _, err := m.StepUntil(talkSettle, func(m *emu.Emu) bool {
			state.Snapshot(m, &mem)
			return state.Controllable(&mem)
		}); err != nil {
			return presses, fmt.Errorf("skill: Talk: not controllable %d frames after the box closed", talkSettle)
		}
	}
	return presses, nil
}

// TalkAt approaches a map object by its ROM home coordinate, refreshes its
// live sprite position, then faces and talks to it. The ROM coordinate makes
// a map-wide objective possible; the sprite refresh keeps wandering NPCs
// from turning that objective into a stale-coordinate interaction.
//
// The approach crosses wild battles by fleeing them (talkBeside): policy is
// the move policy for the one case fleeing cannot cover — a trainer battle,
// which the game refuses to let you flee and which talkBeside fights.
func TalkAt(m *emu.Emu, romData []byte, homeX, homeY uint8, policy MovePolicy) (int, error) {
	cur := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return 0, fmt.Errorf("skill: TalkAt: parse map %#04x: %w", cur, err)
	}
	objectID := 0
	for i, object := range h.Objects {
		if object.X == homeX && object.Y == homeY {
			objectID = i + 1 // map object constants and sprite slots are 1-based
			break
		}
	}
	if objectID == 0 {
		return 0, fmt.Errorf("skill: TalkAt: no map object at (%d,%d) on map %#04x", homeX, homeY, cur)
	}

	const attempts = 4
	tx, ty := homeX, homeY
	for attempt := 1; attempt <= attempts; attempt++ {
		if liveX, liveY, ok := liveObjectPosition(m, objectID); ok {
			tx, ty = liveX, liveY
		}
		if err := talkBeside(m, romData, tx, ty, policy); err != nil {
			return 0, fmt.Errorf("skill: TalkAt: approach object %d at (%d,%d): %w", objectID, tx, ty, err)
		}

		// The NPC may have walked while the player approached. Re-read the
		// slot and retry if its current tile is no longer adjacent.
		if liveX, liveY, ok := liveObjectPosition(m, objectID); ok {
			tx, ty = liveX, liveY
		}
		px, py := playerXY(m)
		if _, ok := directionTo(px, py, tx, ty); !ok {
			m.StepFrames(npcWaitFrames)
			continue
		}
		if err := Face(m, tx, ty); err != nil {
			m.StepFrames(npcWaitFrames)
			continue
		}

		// Bill's first conversation flows directly into a scripted walk to
		// the Cell Separator. Generic Talk deliberately expects ordinary NPC
		// dialogue to settle within talkSettle frames after the box closes;
		// Bill violates that contract. Hand this one interaction to the story
		// driver, which has RAM-backed completion predicates for both scripted
		// handoffs and continues through the S.S. Ticket.
		if cur == billsHouseMap && homeX == billPokemonX && homeY == billPokemonY {
			if err := helpBill(m, romData, policy); err != nil {
				return 1, fmt.Errorf("skill: TalkAt: help Bill: %w", err)
			}
			return 1, nil
		}

		presses, err := Talk(m)
		if err != nil {
			if errors.Is(err, ErrNoDialogue) && attempt < attempts {
				m.StepFrames(npcWaitFrames)
				continue
			}
			return presses, fmt.Errorf("skill: TalkAt: object %d at (%d,%d): %w", objectID, tx, ty, err)
		}
		return presses, nil
	}
	return 0, fmt.Errorf("skill: TalkAt: object %d did not remain adjacent after %d approaches", objectID, attempts)
}

func liveObjectPosition(m *emu.Emu, objectID int) (uint8, uint8, bool) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	for _, sprite := range state.DecodeSprites(&mem) {
		if sprite.Slot == objectID && sprite.X >= 0 && sprite.Y >= 0 && sprite.X <= 255 && sprite.Y <= 255 {
			return uint8(sprite.X), uint8(sprite.Y), true
		}
	}
	return 0, 0, false
}

// besideDestination picks the walkable tile orthogonally adjacent to
// (targetX, targetY) on the current map that is closest (Manhattan) to the
// player. ok is false when the player already stands on such a tile: no
// journey is needed. It is the shared planning step of the "walk beside
// something" approaches — TalkAt and Pickup both need a tile to stand on, and
// Travel needs one destination, so the choice lives here rather than in each
// caller.
func besideDestination(m *emu.Emu, romData []byte, targetX, targetY uint8) (Destination, bool, error) {
	sx, sy := playerXY(m)
	if _, ok := directionTo(sx, sy, targetX, targetY); ok {
		return Destination{}, false, nil
	}
	cur := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return Destination{}, false, fmt.Errorf("parse map %#04x: %w", cur, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return Destination{}, false, fmt.Errorf("build map %#04x: %w", cur, err)
	}
	px, py := int(sx), int(sy)
	var best *struct{ x, y, d int }
	for _, s := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		nx, ny := int(targetX)+s.DX, int(targetY)+s.DY
		if !grid.InBounds(nx, ny) || !grid.Walkable(nx, ny) {
			continue
		}
		c := struct{ x, y, d int }{nx, ny, absInt(nx-px) + absInt(ny-py)}
		if best == nil || c.d < best.d {
			best = &c
		}
	}
	if best == nil {
		return Destination{}, false, fmt.Errorf("no walkable tile beside (%d,%d) on map %#04x", targetX, targetY, cur)
	}
	return Destination{Map: cur, X: uint8(best.x), Y: uint8(best.y)}, true, nil
}

const (
	museum1FMap            = 0x34
	maxTalkApproachChoices = 1
)

// talkApproachChoiceIndex classifies the tiny set of choices that are part of
// reaching a talk target rather than the target conversation itself. Keep this
// deliberately specific: blindly answering generic YES/NO prompts has lost
// caught Pokemon and made wrong move-learning decisions before.
//
// Museum 1F is one measured route gate. Walking from the entrance side to a
// person behind the counter crosses (9,4)/(10,4), where the admission script
// asks "Would you like to come in?". YES buys the ¥50 ticket, sets
// EVENT_BOUGHT_MUSEUM_TICKET and disables the gate script, so the interrupted
// approach can safely resume. No other prompt is answered here.
func talkApproachChoiceIndex(mapID uint8, text string) (int, bool) {
	if mapID != museum1FMap {
		return 0, false
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	if !strings.Contains(normalized, "would you like to come in?") {
		return 0, false
	}
	return 0, true // Museum1F YesNoChoice: index 0 = YES
}

// talkBeside walks to a walkable tile orthogonally adjacent to (tx,ty) on the
// current map, fleeing the wild encounters that interrupt the way. It is a
// no-op when the player is already adjacent.
//
// The approach uses TravelFlee, not a raw walk: talk targets stand in tall
// grass as often as in towns (the Route 1 old man), and a raw WalkPath aborts
// on the first wild battle by design — a talk objective there failed on every
// retry until the run's failure budget ran out (the make run-llm that died at
// round 18 on talk at (5,24), and three farm runs on talk at (15,13)). The
// walk is logistics for the conversation, not an end in itself: a fight here
// is damage and level-up math the objective never asked for, and a loss
// blackouts the party into a town, so the NPC's map may no longer be the
// current one when it returns. Fleeing never loses (S8-4 measured
// first-attempt success in this ROM), and a trainer battle — which the game
// refuses to let you flee — is fought with policy; a blackout from that
// fallback comes back as ErrBlackedOut for the caller to decide on.
func talkBeside(m *emu.Emu, romData []byte, tx, ty uint8, policy MovePolicy) error {
	dest, ok, err := besideDestination(m, romData, tx, ty)
	if err != nil {
		return fmt.Errorf("skill: TalkAt: %w", err)
	}
	if !ok {
		return nil
	}

	handledChoices := 0
	for {
		if _, err := TravelFlee(m, romData, dest, policy, 20); err != nil {
			var choice *ErrDialogueChoice
			if !errors.As(err, &choice) || handledChoices >= maxTalkApproachChoices {
				return fmt.Errorf("skill: TalkAt: approach beside (%d,%d) on map %#04x: %w", tx, ty, dest.Map, err)
			}
			index, recognized := talkApproachChoiceIndex(dest.Map, choice.Result.Text)
			if !recognized {
				return fmt.Errorf("skill: TalkAt: approach beside (%d,%d) on map %#04x: %w", tx, ty, dest.Map, err)
			}
			if err := selectTwoOption(m, index); err != nil {
				return fmt.Errorf("skill: TalkAt: answer Museum admission while approaching (%d,%d): %w", tx, ty, err)
			}
			handledChoices++

			// YES leaves the purchase/thank-you text up while the script sets
			// EVENT_BOUGHT_MUSEUM_TICKET. Close only that ordinary text, then
			// retry the same approach. If another menu appears, preserve the
			// safety rule instead of guessing a second answer.
			rec := RecoverDialogue(m, dialogueRecoveryBudget)
			switch rec.Stop {
			case DialogueRecovered:
				continue
			case DialogueChoiceRequired, DialogueMenuOpen:
				return fmt.Errorf("skill: TalkAt: Museum admission led to another unanswered choice: %w", &ErrDialogueChoice{Result: rec})
			case DialogueBudgetExhausted:
				return fmt.Errorf("skill: TalkAt: Museum admission text did not clear: %q", rec.Text)
			case DialogueUnexpectedMode:
				return fmt.Errorf("skill: TalkAt: Museum admission unexpectedly entered a battle")
			default:
				return fmt.Errorf("skill: TalkAt: Museum admission recovery stopped with %d", rec.Stop)
			}
		}
		return nil
	}
}
