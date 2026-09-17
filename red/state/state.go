package state

// GameState is the full decoded game state.
type GameState struct {
	Player    PlayerState
	World     WorldState
	Party     PartyState
	Inventory InventoryState
	Progress  ProgressState
	Pokedex   PokedexState
	Battle    *BattleState
	Menu      MenuState
	Dialogue  *DialogueState
}

// Decode turns a RAM snapshot into a GameState with the supported Pokémon Red
// image's addresses.
func Decode(m *Mem) GameState {
	return RedAddresses().Decode(m)
}

// Decode turns a RAM snapshot into a GameState at this address set.
func (a Addresses) Decode(m *Mem) GameState {
	return GameState{
		Player:    a.DecodePlayer(m),
		World:     a.DecodeWorld(m),
		Party:     a.DecodeParty(m),
		Inventory: a.DecodeInventory(m),
		Progress:  a.DecodeProgress(m),
		Pokedex:   a.DecodePokedex(m),
		Battle:    a.BattleAddresses.DecodeBattle(m),
		Menu:      a.DecodeMenu(m),
		Dialogue:  a.DecodeDialogue(m),
	}
}
