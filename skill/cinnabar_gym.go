package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	cinnabarGymMap uint8 = 0xa6

	cinnabarGymEntranceX uint8 = 16
	cinnabarGymEntranceY uint8 = 16
	cinnabarGymLeaderX   uint8 = 3
	cinnabarGymLeaderY   uint8 = 3

	cinnabarGymTravelBattles = 40
	cinnabarQuizDriveBudget  = 6000

	// event_constants.asm: EVENT_2A7 is followed by the seven gate bits.
	// The six quiz machines use gate indices 1..6, so gate 0 is only the
	// spare first-trainer bit used by CinnabarGymOpenGateScript.
	cinnabarGymGate0Event state.Event = 0x2a8
)

type cinnabarQuizSpec struct {
	Index         uint8
	TargetX       uint8
	TargetY       uint8
	CorrectAnswer bool
}

// hidden_events.asm is the authoritative table for both terminal coordinates
// and the correct YES/NO answer. Keeping those together prevents question text
// changes from turning the controller into a blind text parser.
var cinnabarQuizSpecs = [...]cinnabarQuizSpec{
	{Index: 1, TargetX: 15, TargetY: 7, CorrectAnswer: false},
	{Index: 2, TargetX: 10, TargetY: 1, CorrectAnswer: true},
	{Index: 3, TargetX: 9, TargetY: 7, CorrectAnswer: true},
	{Index: 4, TargetX: 9, TargetY: 13, CorrectAnswer: true},
	{Index: 5, TargetX: 1, TargetY: 13, CorrectAnswer: false},
	{Index: 6, TargetX: 1, TargetY: 7, CorrectAnswer: true},
}

var blaineGym = GymInfo{
	Map:     cinnabarGymMap,
	Place:   "cinnabar gym",
	LeaderX: cinnabarGymLeaderX,
	LeaderY: cinnabarGymLeaderY,
	Badge:   state.BadgeVolcano,
	Leader:  "BLAINE",
}

func init() {
	places["cinnabar gym"] = Destination{Map: cinnabarGymMap, X: cinnabarGymLeaderX, Y: cinnabarGymLeaderY + 1}
	gyms[cinnabarGymMap] = blaineGym
}

func cinnabarQuizGateEvent(index uint8) (state.Event, bool) {
	if index < 1 || index > uint8(len(cinnabarQuizSpecs)) {
		return 0, false
	}
	return cinnabarGymGate0Event + state.Event(index), true
}

func cinnabarQuizGateOpen(mem *state.Mem, index uint8) bool {
	event, ok := cinnabarQuizGateEvent(index)
	return ok && state.HasEvent(mem, event)
}

// CinnabarGymReady is the durable Secret Key prerequisite. The island map
// script uses the same bag check at (18,4), immediately south of the gym warp.
func CinnabarGymReady(mem *state.Mem) bool {
	return CinnabarSecretKeyOwned(mem)
}

// CinnabarGymOpen reports whether every quiz-controlled gate is already open.
// Trainer wins may set the same bits, so a resumed run naturally skips any
// gate solved either by a correct quiz answer or by the associated battle.
func CinnabarGymOpen(mem *state.Mem) bool {
	for _, quiz := range cinnabarQuizSpecs {
		if !cinnabarQuizGateOpen(mem, quiz.Index) {
			return false
		}
	}
	return true
}

// CinnabarProgression is the final #35 executor. It resumes from the Mansion,
// Cinnabar Island, or the Gym, opens only the remaining quiz gates, delegates
// combat to Gym/Battle, and returns only after the Volcano Badge is present.
func CinnabarProgression(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: CinnabarProgression: nil policy")
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeProgress(&mem).Has(state.BadgeVolcano) {
		return nil
	}
	if !CinnabarGymReady(&mem) {
		return fmt.Errorf("skill: CinnabarProgression: Secret Key is not owned")
	}

	if m.Peek8(sym.CurMap) == pokemonMansionB1FMap {
		if err := openMansionBasementExit(m, romData, policy); err != nil {
			return fmt.Errorf("skill: CinnabarProgression: leave Mansion basement: %w", err)
		}
	}
	if m.Peek8(sym.CurMap) != cinnabarGymMap {
		if _, err := TravelFlee(m, romData, Destination{Map: cinnabarIslandMap, X: 11, Y: 12}, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("skill: CinnabarProgression: return to Cinnabar Island: %w", err)
		}
		if err := EnterCinnabarGym(m, romData, policy); err != nil {
			return err
		}
	}

	outcome, err := Gym(m, romData, policy)
	if err != nil {
		return fmt.Errorf("skill: CinnabarProgression: %w", err)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: CinnabarProgression: Blaine battle outcome %v, want won", outcome)
	}
	state.Snapshot(m, &mem)
	if !state.DecodeProgress(&mem).Has(state.BadgeVolcano) {
		return fmt.Errorf("skill: CinnabarProgression: Blaine win returned without Volcano Badge")
	}
	return nil
}

// openMansionBasementExit handles the normal handoff from the Secret Key
// objective, which ends beside the key on B1F. The global map graph knows the
// staircase but not which statue state currently exposes its corridor, so try
// each reachable live switch state until the 1F warp is pathable.
func openMansionBasementExit(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != pokemonMansionB1FMap {
		return nil
	}
	const exitX, exitY uint8 = 23, 22
	if mansionTileReachable(m, romData, exitX, exitY) {
		return nil
	}

	type attemptKey struct {
		SwitchOn bool
		X, Y     uint8
	}
	attempted := map[attemptKey]bool{}
	for step := 0; step < 6; step++ {
		if mansionTileReachable(m, romData, exitX, exitY) {
			return nil
		}
		before := currentMansionSwitchOn(m)
		toggled := false
		for _, sw := range mansionB1FSwitches {
			key := attemptKey{SwitchOn: before, X: sw.TargetX, Y: sw.TargetY}
			if attempted[key] || !mansionTileReachable(m, romData, sw.StandX, sw.StandY) {
				continue
			}
			attempted[key] = true
			if err := setMansionSwitch(m, romData, sw, !before, policy); err != nil {
				return err
			}
			toggled = true
			break
		}
		if !toggled {
			break
		}
	}
	if !mansionTileReachable(m, romData, exitX, exitY) {
		return fmt.Errorf("Mansion B1F exit warp (%d,%d) is unreachable in every reachable switch state", exitX, exitY)
	}
	return nil
}

func EnterCinnabarGym(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) == cinnabarGymMap {
		return nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !CinnabarGymReady(&mem) {
		return fmt.Errorf("skill: EnterCinnabarGym: Secret Key is not owned")
	}
	if _, err := TravelFlee(m, romData, Destination{Map: cinnabarGymMap, X: cinnabarGymEntranceX, Y: cinnabarGymEntranceY}, policy, cinnabarGymTravelBattles); err != nil {
		return fmt.Errorf("skill: EnterCinnabarGym: %w", err)
	}
	if m.Peek8(sym.CurMap) != cinnabarGymMap {
		return fmt.Errorf("skill: EnterCinnabarGym: stopped on map %#04x", m.Peek8(sym.CurMap))
	}
	return nil
}

// OpenCinnabarGym answers each still-closed quiz from the ROM-declared answer
// bit. Movement between machines uses the live WRAM collision grid, so each
// newly replaced gate block becomes ordinary topology immediately. If a
// trainer intercepts the route and its win opens that gate, the event check
// simply skips the corresponding quiz.
func OpenCinnabarGym(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: OpenCinnabarGym: nil policy")
	}
	if m.Peek8(sym.CurMap) != cinnabarGymMap {
		return fmt.Errorf("skill: OpenCinnabarGym: on map %#04x, want %#04x", m.Peek8(sym.CurMap), cinnabarGymMap)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !CinnabarGymReady(&mem) {
		return fmt.Errorf("skill: OpenCinnabarGym: Secret Key is not owned")
	}
	if CinnabarGymOpen(&mem) {
		return nil
	}

	for _, quiz := range cinnabarQuizSpecs {
		state.Snapshot(m, &mem)
		if cinnabarQuizGateOpen(&mem, quiz.Index) {
			continue
		}
		stand := Destination{Map: cinnabarGymMap, X: quiz.TargetX, Y: quiz.TargetY + 1}
		res, err := Travel(m, romData, stand, policy, cinnabarGymTravelBattles)
		if err != nil {
			return fmt.Errorf("skill: OpenCinnabarGym: reach quiz %d at (%d,%d): %w", quiz.Index, quiz.TargetX, quiz.TargetY, err)
		}
		if res.BlackedOut {
			return fmt.Errorf("skill: OpenCinnabarGym: %w reaching quiz %d", ErrBlackedOut, quiz.Index)
		}

		// The route may have crossed the associated trainer, whose post-battle
		// script sets the same gate bit. Re-read before touching the terminal.
		state.Snapshot(m, &mem)
		if cinnabarQuizGateOpen(&mem, quiz.Index) {
			continue
		}
		if err := answerCinnabarQuiz(m, quiz); err != nil {
			return err
		}
	}

	state.Snapshot(m, &mem)
	if !CinnabarGymOpen(&mem) {
		return fmt.Errorf("skill: OpenCinnabarGym: returned with one or more quiz gates still closed")
	}
	return nil
}

func answerCinnabarQuiz(m *emu.Emu, quiz cinnabarQuizSpec) error {
	px, py := playerXY(m)
	if px != quiz.TargetX || py != quiz.TargetY+1 {
		return fmt.Errorf("skill: OpenCinnabarGym: quiz %d requires stand (%d,%d), at (%d,%d)", quiz.Index, quiz.TargetX, quiz.TargetY+1, px, py)
	}
	if err := Face(m, quiz.TargetX, quiz.TargetY); err != nil {
		return fmt.Errorf("skill: OpenCinnabarGym: face quiz %d: %w", quiz.Index, err)
	}
	if px, py = playerXY(m); px != quiz.TargetX || py != quiz.TargetY+1 {
		return fmt.Errorf("skill: OpenCinnabarGym: facing quiz %d moved player to (%d,%d)", quiz.Index, px, py)
	}

	gateEvent, _ := cinnabarQuizGateEvent(quiz.Index)
	m.Tap(emu.A, 3, 7)
	answered := false
	for frame := 0; frame < cinnabarQuizDriveBudget; frame++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if answered && state.HasEvent(&mem, gateEvent) && state.Controllable(&mem) {
			m.StepFrames(2)
			return nil
		}
		if state.DecodeBattle(&mem) != nil {
			return fmt.Errorf("skill: OpenCinnabarGym: quiz %d answer unexpectedly started a battle", quiz.Index)
		}
		interaction := state.DecodeInteraction(&mem)
		switch interaction.Kind {
		case state.InteractionTwoOption:
			if answered {
				return fmt.Errorf("skill: OpenCinnabarGym: quiz %d opened a second unexpected two-option prompt", quiz.Index)
			}
			if err := AnswerYesNo(m, quiz.CorrectAnswer); err != nil {
				return fmt.Errorf("skill: OpenCinnabarGym: answer quiz %d: %w", quiz.Index, err)
			}
			answered = true
		case state.InteractionDialogue:
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
		case state.InteractionNone:
			m.StepFrame()
		default:
			return fmt.Errorf("skill: OpenCinnabarGym: quiz %d entered unexpected interaction %q text=%q", quiz.Index, interaction.Kind, interaction.Text)
		}
	}
	return fmt.Errorf("skill: OpenCinnabarGym: quiz %d did not set gate event %#x within %d frames", quiz.Index, uint16(gateEvent), cinnabarQuizDriveBudget)
}
