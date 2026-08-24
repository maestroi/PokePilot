package state

// GameState is the full decoded game state.
type GameState struct {
	Player    PlayerState
	World     WorldState
	Party     PartyState
	Inventory InventoryState
	Progress  ProgressState
	Battle    *BattleState
	Menu      *MenuState
	Dialogue  *DialogueState
}

// Decode turns a RAM snapshot into a GameState.
func Decode(m *Mem) GameState {
	return GameState{
		Player:    DecodePlayer(m),
		World:     DecodeWorld(m),
		Party:     DecodeParty(m),
		Inventory: DecodeInventory(m),
		Progress:  DecodeProgress(m),
		Battle:    DecodeBattle(m),
		Menu:      DecodeMenu(m),
		Dialogue:  DecodeDialogue(m),
	}
}
