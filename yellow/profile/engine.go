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

func (*Profile) DecodePartyMenu(r game.MemoryReader) game.PartyMenuState {
	return engine.DecodePartyMenu(r)
}
