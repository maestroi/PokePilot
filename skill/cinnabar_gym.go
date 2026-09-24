package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
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
	Index           uint8
	TargetX         uint8
	TargetY         uint8
	AnswerMenuIndex uint8
}

// hidden_events.asm is the authoritative table for both terminal coordinates
// and the expected YesNoChoice cursor index. Its TRUE/FALSE macro argument is
// not semantic truth: it is compared directly with wCurrentMenuItem, where
// index 0 is YES and index 1 is NO. Keeping the raw index avoids accidentally
// inverting all six answers when driving the typed YES/NO controller.
var cinnabarQuizSpecs = [...]cinnabarQuizSpec{
	{Index: 1, TargetX: 15, TargetY: 7, AnswerMenuIndex: 0},
	{Index: 2, TargetX: 10, TargetY: 1, AnswerMenuIndex: 1},
	{Index: 3, TargetX: 9, TargetY: 7, AnswerMenuIndex: 1},
	{Index: 4, TargetX: 9, TargetY: 13, AnswerMenuIndex: 1},
	{Index: 5, TargetX: 1, TargetY: 13, AnswerMenuIndex: 0},
	{Index: 6, TargetX: 1, TargetY: 7, AnswerMenuIndex: 1},
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

func cinnabarQuizAnswerYes(quiz cinnabarQuizSpec) (bool, bool) {
	switch quiz.AnswerMenuIndex {
	case 0:
		return true, true
	case 1:
		return false, true
	default:
		return false, false
	}
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
// combat to Gym/Battle, and returns after a won Blaine battle. The Volcano
// Badge itself is the objective runtime's postcondition, not this skill's.
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
		if err := returnToCinnabarIsland(m, romData, policy); err != nil {
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
	if err := RequireTrainerBattleWin("gym:blaine", outcome); err != nil {
		return fmt.Errorf("skill: CinnabarProgression: %w", err)
	}
	// Badge ownership is the objective's semantic postcondition, verified once
	// by the objective runtime (#1655); this skill owns only the mechanics.
	return nil
}

// mansionB1FExitX/Y is the B1F stairs tile up to 1F.
const mansionB1FExitX, mansionB1FExitY uint8 = 23, 22

// openMansionBasementExit handles the normal handoff from the Secret Key
// objective, which ends beside the key on B1F. The global map graph knows the
// staircase but not which statue state currently exposes its corridor, so try
// each reachable live switch state until the 1F warp is pathable.
func openMansionBasementExit(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != pokemonMansionB1FMap {
		return nil
	}
	if mansionTileReachable(m, romData, mansionB1FExitX, mansionB1FExitY) {
		return nil
	}

	type attemptKey struct {
		SwitchOn bool
		X, Y     uint8
	}
	attempted := map[attemptKey]bool{}
	for step := 0; step < 6; step++ {
		if mansionTileReachable(m, romData, mansionB1FExitX, mansionB1FExitY) {
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
	if !mansionTileReachable(m, romData, mansionB1FExitX, mansionB1FExitY) {
		return fmt.Errorf("Mansion B1F exit warp (%d,%d) is unreachable in every reachable switch state", mansionB1FExitX, mansionB1FExitY)
	}
	return nil
}

// returnToCinnabarIsland leaves the Mansion. The B1F stairs land in a 1F
// pocket that is sealed in one of the two shared switch states, and the 1F
// statue sits outside that pocket; the key hunt usually leaves the switch in
// the sealing state. Flip it from wherever a statue is reachable (1F, else
// B1F without sealing the B1F stairs) and retry.
func returnToCinnabarIsland(m *emu.Emu, romData []byte, policy MovePolicy) error {
	island := Destination{Map: cinnabarIslandMap, X: 11, Y: 12}
	_, err := TravelFlee(m, romData, island, policy, mansionTravelBattles)
	if err == nil || m.Peek8(sym.CurMap) != pokemonMansion1FMap || !errors.Is(err, world.ErrNoRoute) {
		return err
	}
	want := !currentMansionSwitchOn(m)
	if mansionTileReachable(m, romData, mansion1FSwitch.StandX, mansion1FSwitch.StandY) {
		if err := setMansionSwitch(m, romData, mansion1FSwitch, want, policy); err != nil {
			return fmt.Errorf("flip Mansion 1F switch: %w", err)
		}
	} else {
		if _, err := TravelFlee(m, romData, Destination{Map: pokemonMansionB1FMap, X: 23, Y: 21}, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("Mansion 1F exit sealed; reach B1F switches: %w", err)
		}
		if err := setMansionSwitchKeepingBasementExit(m, romData, want, policy); err != nil {
			return err
		}
	}
	_, err = TravelFlee(m, romData, island, policy, mansionTravelBattles)
	return err
}

// setMansionSwitchKeepingBasementExit sets the shared switch from a B1F statue
// whose flip still leaves the 1F stairs reachable, undoing any flip that seals
// them (the player stays on that statue's stand, so it can always flip back).
func setMansionSwitchKeepingBasementExit(m *emu.Emu, romData []byte, want bool, policy MovePolicy) error {
	for _, sw := range mansionB1FSwitches {
		if !mansionTileReachable(m, romData, sw.StandX, sw.StandY) {
			continue
		}
		if err := setMansionSwitch(m, romData, sw, want, policy); err != nil {
			return fmt.Errorf("toggle B1F statue at (%d,%d): %w", sw.TargetX, sw.TargetY, err)
		}
		if mansionTileReachable(m, romData, mansionB1FExitX, mansionB1FExitY) {
			return nil
		}
		if err := setMansionSwitch(m, romData, sw, !want, policy); err != nil {
			return fmt.Errorf("undo B1F statue at (%d,%d): %w", sw.TargetX, sw.TargetY, err)
		}
	}
	return fmt.Errorf("no reachable B1F statue sets Mansion switch=%v with the B1F stairs (%d,%d) still reachable", want, mansionB1FExitX, mansionB1FExitY)
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

// OpenCinnabarGym answers each still-closed quiz from the ROM-declared menu
// index. Movement between machines uses the live WRAM collision grid, so each
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
		quiz := quiz
		stand := Destination{Map: cinnabarGymMap, X: quiz.TargetX, Y: quiz.TargetY + 1}
		_, err := executeTopologyInteraction(m, romData, policy, topologyInteraction{
			Name:       fmt.Sprintf("Cinnabar quiz %d", quiz.Index),
			Approach:   stand,
			TargetX:    quiz.TargetX,
			TargetY:    quiz.TargetY,
			MaxBattles: cinnabarGymTravelBattles,
			Budget:     cinnabarQuizDriveBudget,
			Complete: func(mm *state.Mem) bool {
				return cinnabarQuizGateOpen(mm, quiz.Index)
			},
			Interact: func() error {
				return answerCinnabarQuiz(m, quiz)
			},
		})
		if err != nil {
			return fmt.Errorf("skill: OpenCinnabarGym: %w", err)
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
	yes, ok := cinnabarQuizAnswerYes(quiz)
	if !ok {
		return fmt.Errorf("skill: OpenCinnabarGym: quiz %d has invalid answer menu index %d", quiz.Index, quiz.AnswerMenuIndex)
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
			if err := AnswerYesNo(m, yes); err != nil {
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
