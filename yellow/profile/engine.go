package profile

import (
	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
)

// Yellow runs a revision of the Gen-I engine Red and Blue share. Its emulator
// binds the canonical memory view (native.go) and its ROM tables are bound
// per cartridge (yellow/rom.Tables), so the shared Gen-I runtime decoders
// below read Yellow correctly without a forked controller. Yellow keeps its
// own identity, boot, observation, story projection and Pikachu facts, which
// read native RAM.
//
// Removal path: as for Blue, when the Gen-I engine is factored out of red/,
// Yellow should embed it directly instead of delegating to Red's profile.
var engine redprofile.Profile

func (*Profile) DecodeBattleExecution(r game.MemoryReader) game.BattleExecutionState {
	return engine.DecodeBattleExecution(r)
}

func (*Profile) DecodeBattleMainMenu(r game.MemoryReader) game.BattleMainMenuState {
	return engine.DecodeBattleMainMenu(r)
}

func (*Profile) BattleMainMenuEntryPosition(entry game.BattleMenuEntry) (game.BattleMenuPosition, bool) {
	return engine.BattleMainMenuEntryPosition(entry)
}

// DecodeBattleEscapeMenu delegates the RUN-capable battle menu for the same
// reason DecodeBattleMainMenu does: Yellow runs the Gen-I engine Red and Blue
// share, and the ordinary and Safari command menus are engine facts, not
// cartridge facts. Omitting this delegation made Flee hard-fail on Yellow with
// "profile pokemon-yellow@en-us-rev0 does not expose escape-menu semantics"
// before a single button was pressed (run-1wsyy1f75ssxsheu3o4xpui4).
func (*Profile) DecodeBattleEscapeMenu(r game.MemoryReader) game.BattleEscapeMenuState {
	return engine.DecodeBattleEscapeMenu(r)
}

func (*Profile) BattleEscapeRunPosition(kind game.BattleEscapeMenuKind) (game.BattleMenuPosition, bool) {
	return engine.BattleEscapeRunPosition(kind)
}

func (*Profile) DecodeBattleResources(r game.MemoryReader) game.BattleResourcesState {
	return engine.DecodeBattleResources(r)
}

func (*Profile) DecodeBattleRuntime(r game.MemoryReader) game.BattleRuntimeState {
	return engine.DecodeBattleRuntime(r)
}

func (*Profile) DecodeBattleState(r game.MemoryReader) (game.BattleState, bool) {
	return engine.DecodeBattleState(r)
}

func (*Profile) DecodeBattleResult(r game.MemoryReader) game.BattleResult {
	return engine.DecodeBattleResult(r)
}

func (*Profile) DecodeInventory(r game.MemoryReader) game.InventoryState {
	return engine.DecodeInventory(r)
}

func (*Profile) DecodeCapture(r game.MemoryReader) game.CaptureState {
	return engine.DecodeCapture(r)
}

func (*Profile) CaptureDexNumber(romData []byte, species uint16) (uint16, bool) {
	return engine.CaptureDexNumber(romData, species)
}

func (*Profile) OrdinaryCaptureBallOrder() []uint16 {
	return engine.OrdinaryCaptureBallOrder()
}

func (*Profile) DecodeFieldAction(r game.MemoryReader) game.FieldActionState {
	return engine.DecodeFieldAction(r)
}

func (*Profile) DecodeListMenu(r game.MemoryReader) game.ListMenuState {
	return engine.DecodeListMenu(r)
}

func (*Profile) DecodeMenuCursor(r game.MemoryReader) game.MenuCursorState {
	return engine.DecodeMenuCursor(r)
}

func (*Profile) DecodeTwoOption(r game.MemoryReader) (game.TwoOptionState, bool) {
	return engine.DecodeTwoOption(r)
}

func (*Profile) DecodeStartMenu(r game.MemoryReader) game.StartMenuState {
	return engine.DecodeStartMenu(r)
}

func (*Profile) StartMenuEntryIndex(r game.MemoryReader, entry game.StartMenuEntry) (int, bool) {
	return engine.StartMenuEntryIndex(r, entry)
}

func (*Profile) DecodeOverworld(r game.MemoryReader) game.OverworldState {
	return engine.DecodeOverworld(r)
}

func (*Profile) DecodeOverworldBlackout(r game.MemoryReader) game.OverworldBlackoutState {
	return engine.DecodeOverworldBlackout(r)
}

func (*Profile) DecodePartyMenu(r game.MemoryReader) game.PartyMenuState {
	return engine.DecodePartyMenu(r)
}

// Every capability the shared Gen-I runtime resolves from a profile must be
// delegated explicitly. A missing method is invisible at compile time and only
// surfaces at runtime as a "profile does not expose X semantics" stall, so the
// assertions below turn that omission into a build failure.
var (
	_ game.BattleMenuDecoder       = (*Profile)(nil)
	_ game.BattleEscapeMenuDecoder = (*Profile)(nil)
	_ game.BattleCombatStrategy    = (*Profile)(nil)
)

func (*Profile) EvaluateCombatMove(
	romData []byte,
	attacker, defender game.BattleCombatant,
	nativeMoveID uint16,
	currentPP uint8,
) (game.BattleMoveEvaluation, game.BattleMoveRole, error) {
	return engine.EvaluateCombatMove(romData, attacker, defender, nativeMoveID, currentPP)
}

func (*Profile) IncomingTypeRisk(romData []byte, enemy, candidate game.BattleCombatant) int {
	return engine.IncomingTypeRisk(romData, enemy, candidate)
}

func (*Profile) PreferSetupMove(
	b game.BattleState,
	setup, attack game.BattleMoveEvaluation,
) bool {
	return engine.PreferSetupMove(b, setup, attack)
}

func (*Profile) IsFieldMove(nativeMoveID uint16) bool {
	return engine.IsFieldMove(nativeMoveID)
}
