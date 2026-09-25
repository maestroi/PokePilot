package game

// BattleExecutionPhase identifies the battle-owned UI surface that currently
// owns input. It deliberately describes intent rather than a cartridge's text
// markers or RAM bytes so shared execution code can wait on semantic states.
type BattleExecutionPhase string

const (
	BattleExecutionNone             BattleExecutionPhase = ""
	BattleExecutionMainMenu         BattleExecutionPhase = "main_menu"
	BattleExecutionMoveMenu         BattleExecutionPhase = "move_menu"
	BattleExecutionMoveDisabled     BattleExecutionPhase = "move_disabled"
	BattleExecutionUseNextPrompt    BattleExecutionPhase = "use_next_prompt"
	BattleExecutionTryLearnPrompt   BattleExecutionPhase = "try_learn_prompt"
	BattleExecutionAbandonLearn     BattleExecutionPhase = "abandon_learn_prompt"
	BattleExecutionTrainerSwitch    BattleExecutionPhase = "trainer_switch_prompt"
	BattleExecutionForgetMove       BattleExecutionPhase = "forget_move_menu"
	BattleExecutionHMForgetRejected BattleExecutionPhase = "hm_forget_rejected"
	BattleExecutionSwitchBox        BattleExecutionPhase = "switch_box"
)

// BattleMoveLearnerState is the portable state needed while a level-up move is
// being offered. Native type/move ids are uint16 so Gen II is not forced into
// Gen I's byte-shaped public execution contract.
type BattleMoveLearnerState struct {
	Valid     bool
	PartySlot int
	Type1     uint16
	Type2     uint16
	Moves     [4]uint16
}

// BattleExecutionState is the profile-owned projection of battle UI and
// move-learning execution state. Strategy is intentionally not part of this
// contract: the skill decides what to do; the profile says what the game is
// currently asking and exposes enough state to verify the result.
type BattleExecutionState struct {
	InBattle     bool
	Phase        BattleExecutionPhase
	ForgetCursor MenuCursorState
	ForgetReady  bool
	OfferedMove  uint16
	Learner      BattleMoveLearnerState
	PartyMoves   [][4]uint16
}

// MoveLearned is the positive postcondition used by shared execution after a
// replacement is selected.
func (s BattleExecutionState) MoveLearned(partySlot, moveSlot int, move uint16) bool {
	if partySlot < 0 || partySlot >= len(s.PartyMoves) || moveSlot < 0 || moveSlot >= 4 {
		return false
	}
	return s.PartyMoves[partySlot][moveSlot] == move
}

// BattleExecutionDecoder hides game-specific text markers, menu-shape bytes,
// move-learning scratch variables and party move layout from shared drivers.
type BattleExecutionDecoder interface {
	DecodeBattleExecution(MemoryReader) BattleExecutionState
}

// BattleExecutionProfile is a game profile that exposes semantic battle
// execution state.
type BattleExecutionProfile interface {
	GameProfile
	BattleExecutionDecoder
}
