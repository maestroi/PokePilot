// Package state decodes Pokémon Yellow's RAM into the shared Gen I state
// types.
//
// Yellow shares the Gen I engine *shape* with Red: the party_struct,
// battle_struct, inventory layout, badge bitfield and event-flag encoding are
// all byte-identical. Only the WRAM addresses moved (most by one byte lower).
// This package therefore reuses red/state's types and decoders and supplies
// Yellow's addresses, rather than duplicating the decoding logic. A fix to a
// shared struct serves both games.
//
// Do not copy values from red/sym; use yellow/sym. Do not hand-count event
// flags: replay pokeyellow's const_def counter the way
// red/state/event_constants_test.go does for Red.
package state

import (
	redstate "github.com/maestroi/pokepilot/red/state"
	yellowsym "github.com/maestroi/pokepilot/yellow/sym"
)

// Mem is a full 64 KiB snapshot of the Game Boy address space, indexed by
// absolute address so decoders use yellow/sym constants directly. It is the
// shared Gen I memory surface.
type Mem = redstate.Mem

// GameState is the full decoded game state. Shared Gen I types.
type GameState = redstate.GameState

// YellowAddresses returns the WRAM address set Pokémon Yellow's decoders read.
// Every value comes from yellow/sym (itself generated from pokeyellow's symbol
// map). The decode rules are shared with Red via redstate.Addresses methods;
// only the addresses differ, so this is the whole of Yellow's decode
// specificity.
func YellowAddresses() redstate.Addresses {
	return redstate.Addresses{
		BattleAddresses: redstate.BattleAddresses{
			IsInBattle:          yellowsym.IsInBattle,
			BattleResult:        yellowsym.BattleResult,
			EnemyMonSpecies:     yellowsym.EnemyMonSpecies,
			EnemyMonHP:          yellowsym.EnemyMonHP,
			EnemyMonMaxHP:       yellowsym.EnemyMonMaxHP,
			EnemyMonLevel:       yellowsym.EnemyMonLevel,
			EnemyMonAttack:      yellowsym.EnemyMonAttack,
			EnemyMonDefense:     yellowsym.EnemyMonDefense,
			EnemyMonSpecial:     yellowsym.EnemyMonSpecial,
			EnemyMonType1:       yellowsym.EnemyMonType1,
			EnemyMonType2:       yellowsym.EnemyMonType2,
			BattleMonSpecies:    yellowsym.BattleMonSpecies,
			BattleMonLevel:      yellowsym.BattleMonLevel,
			BattleMonHP:         yellowsym.BattleMonHP,
			BattleMonMaxHP:      yellowsym.BattleMonMaxHP,
			BattleMonAttack:     yellowsym.BattleMonAttack,
			BattleMonDefense:    yellowsym.BattleMonDefense,
			BattleMonSpecial:    yellowsym.BattleMonSpecial,
			BattleMonType1:      yellowsym.BattleMonType1,
			BattleMonType2:      yellowsym.BattleMonType2,
			BattleMonMoves:      yellowsym.BattleMonMoves,
			BattleMonPP:         yellowsym.BattleMonPP,
			PlayerDisabledMove:  yellowsym.PlayerDisabledMove,
			PlayerMonAttackMod:  yellowsym.PlayerMonAttackMod,
			PlayerMonDefenseMod: yellowsym.PlayerMonDefenseMod,
			EnemyMonAttackMod:   yellowsym.EnemyMonAttackMod,
			EnemyMonDefenseMod:  yellowsym.EnemyMonDefenseMod,
		},
		CurMap:                  yellowsym.CurMap,
		CurMapTileset:           yellowsym.CurMapTileset,
		CurMapHeight:            yellowsym.CurMapHeight,
		CurMapWidth:             yellowsym.CurMapWidth,
		XCoord:                  yellowsym.XCoord,
		YCoord:                  yellowsym.YCoord,
		SpritePlayerFacing:      yellowsym.SpritePlayerFacing,
		PartyCount:              yellowsym.PartyCount,
		PartyMon1:               yellowsym.PartyMon1,
		BoxCount:                yellowsym.BoxCount,
		BoxMon1:                 yellowsym.BoxMon1,
		CurrentBoxNum:           yellowsym.CurrentBoxNum,
		PlayerMoney:             yellowsym.PlayerMoney,
		PlayerCoins:             yellowsym.PlayerCoins,
		NumBagItems:             yellowsym.NumBagItems,
		BagItems:                yellowsym.BagItems,
		ObtainedBadges:          yellowsym.ObtainedBadges,
		EventFlags:              yellowsym.EventFlags,
		StatusFlags1:            yellowsym.StatusFlags1,
		StatusFlags4:            yellowsym.StatusFlags4,
		PokedexSeen:             yellowsym.PokedexSeen,
		PokedexOwned:            yellowsym.PokedexOwned,
		FontLoaded:              yellowsym.FontLoaded,
		TextBoxID:               yellowsym.TextBoxID,
		TileMap:                 yellowsym.TileMap,
		TwoOptionMenuID:         yellowsym.TwoOptionMenuID,
		ListMenuID:              yellowsym.ListMenuID,
		ListScrollOffset:        yellowsym.ListScrollOffset,
		MenuWatchedKeys:         yellowsym.MenuWatchedKeys,
		CurrentMenuItem:         yellowsym.CurrentMenuItem,
		MaxMenuItem:             yellowsym.MaxMenuItem,
		TopMenuItemX:            yellowsym.TopMenuItemX,
		TopMenuItemY:            yellowsym.TopMenuItemY,
		SpritePlayerStateData1:  yellowsym.SpritePlayerStateData1,
		SpriteStateData2:        yellowsym.SpriteStateData2,
		ToggleableObjectFlags:   yellowsym.ToggleableObjectFlags,
		ToggleableObjectList:    yellowsym.ToggleableObjectList,
		WalkCounter:             yellowsym.WalkCounter,
		MtMoonB2FCurScript:      yellowsym.MtMoonB2FCurScript,
		MapPalOffset:            yellowsym.MapPalOffset,
		PartySpecies:            yellowsym.PartySpecies,
		PlayerName:              yellowsym.PlayerName,
		RivalName:               yellowsym.RivalName,
		WalkBikeSurfState:       yellowsym.WalkBikeSurfState,
		TileInFrontOfPlayer:     yellowsym.TileInFrontOfPlayer,
		JoyIgnore:               yellowsym.JoyIgnore,
		FirstLockTrashCanIndex:  yellowsym.FirstLockTrashCanIndex,
		SecondLockTrashCanIndex: yellowsym.SecondLockTrashCanIndex,
		NumSafariBalls:          yellowsym.NumSafariBalls,
		NumRunAttempts:          yellowsym.NumRunAttempts,
		MoveNum:                 yellowsym.MoveNum,
		NumberOfWarps:           yellowsym.NumberOfWarps,
		WarpEntries:             yellowsym.WarpEntries,
		MoveMenuType:            yellowsym.MoveMenuType,
		PlayerMonNumber:         yellowsym.PlayerMonNumber,
		WhichPokemon:            yellowsym.WhichPokemon,
		ItemList:                yellowsym.ItemList,
		ItemQuantity:            yellowsym.ItemQuantity,
		MaxItemQuantity:         yellowsym.MaxItemQuantity,
		BoxMonCounts:            yellowsym.BoxMonCounts,
		FieldMoves:              yellowsym.FieldMoves,
		RodResponse:             yellowsym.RodResponse,
		OverworldMap:            yellowsym.OverworldMap,
	}
}

// Decode turns a Yellow RAM snapshot into a GameState.
func Decode(m *Mem) GameState {
	a := YellowAddresses()
	return GameState{
		Player:    a.DecodePlayer(m),
		World:     a.DecodeWorld(m),
		Party:     a.DecodeParty(m),
		Inventory: a.DecodeInventory(m),
		Progress:  a.DecodeProgress(m),
		Pokedex:   a.DecodePokedex(m),
		Battle:    a.DecodeBattle(m),
		Menu:      a.DecodeMenu(m),
		Dialogue:  a.DecodeDialogue(m),
	}
}

// The decoders below are the shared Gen I rules with Yellow's addresses. They
// exist as thin wrappers so a caller holding a *Mem without knowing the image
// still has one named place to reach the right decode; callers that know the
// image can call YellowAddresses() methods directly.

func DecodePlayer(m *Mem) redstate.PlayerState       { return YellowAddresses().DecodePlayer(m) }
func DecodeWorld(m *Mem) redstate.WorldState         { return YellowAddresses().DecodeWorld(m) }
func DecodeParty(m *Mem) redstate.PartyState         { return YellowAddresses().DecodeParty(m) }
func DecodeInventory(m *Mem) redstate.InventoryState { return YellowAddresses().DecodeInventory(m) }
func DecodeProgress(m *Mem) redstate.ProgressState   { return YellowAddresses().DecodeProgress(m) }
func DecodePokedex(m *Mem) redstate.PokedexState     { return YellowAddresses().DecodePokedex(m) }
func DecodeBattle(m *Mem) *redstate.BattleState      { return YellowAddresses().DecodeBattle(m) }
func DecodeMenu(m *Mem) redstate.MenuState           { return YellowAddresses().DecodeMenu(m) }
func DecodeDialogue(m *Mem) *redstate.DialogueState  { return YellowAddresses().DecodeDialogue(m) }
func DecodeBox(m *Mem) redstate.BoxState             { return YellowAddresses().DecodeBox(m) }
func DecodeCoins(m *Mem) int                         { return YellowAddresses().DecodeCoins(m) }
func DecodeSprites(m *Mem) []redstate.SpriteState    { return YellowAddresses().DecodeSprites(m) }
func DecodeTwoOptionMenu(m *Mem) *redstate.TwoOptionMenu {
	return YellowAddresses().DecodeTwoOptionMenu(m)
}
func DecodeInteraction(m *Mem) redstate.InteractionState {
	return YellowAddresses().DecodeInteraction(m)
}
func DecodeStoryFacts(m *Mem, inv redstate.InventoryState) redstate.StoryFacts {
	return YellowAddresses().DecodeStoryFacts(m, inv)
}
func HasEvent(m *Mem, e redstate.Event) bool { return YellowAddresses().HasEvent(m, e) }
func Controllable(m *Mem) bool               { return YellowAddresses().Controllable(m) }
func MenuUp(m *Mem) bool                     { return YellowAddresses().MenuUp(m) }
func TookStarterBall(m *Mem) bool            { return YellowAddresses().TookStarterBall(m) }
