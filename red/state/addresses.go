package state

import "github.com/maestroi/pokepilot/red/sym"

// Addresses are the WRAM addresses Gen I state decoders read. Pokémon Red and
// Pokémon Yellow share every decode *rule* — the party_struct, box_struct,
// BCD money, badge bitfield, menu registers and event-flag encoding are all
// byte-identical — but they do not share the WRAM addresses those rules read.
// Yellow inserted a byte above a contiguous WRAM region and shifted the whole
// region one byte lower, so most of these differ by one between the two
// games, and a handful (sprite data, JoyIgnore) happen to coincide.
//
// Reading a decoder with the wrong game's addresses is a silent failure, not
// a crash: the byte at Red's wPartyCount on a Yellow image is a real byte
// that decodes as a party of six. So every decode goes through an Addresses
// instance resolved for the image whose RAM is being read, never through a
// game's sym package directly at a read site.
//
// Addresses embeds BattleAddresses, so the battle decode takes the same set
// as every other decode rather than a second, parallel one.
//
// Constants that are genuinely Gen I rather than per-game are NOT fields here
// and stay in red/sym: party_struct and box_struct field offsets
// (MonSpecies and its neighbours), the buffer lengths (TileMapLen,
// PartyMonSize, BoxMonSize), and the fixed counts (PokedexCount) are
// identical in both games, so the shared decode bodies read them directly.
type Addresses struct {
	BattleAddresses

	// Overworld position and current map.
	CurMap             uint16
	CurMapTileset      uint16
	CurMapHeight       uint16
	CurMapWidth        uint16
	XCoord             uint16
	YCoord             uint16
	SpritePlayerFacing uint16

	// Party and boxes.
	PartyCount    uint16
	PartyMon1     uint16
	BoxCount      uint16
	BoxMon1       uint16
	CurrentBoxNum uint16

	// Inventory and money.
	PlayerMoney uint16
	PlayerCoins uint16
	NumBagItems uint16
	BagItems    uint16

	// Progress.
	ObtainedBadges uint16
	EventFlags     uint16
	StatusFlags1   uint16
	StatusFlags4   uint16

	// Pokedex.
	PokedexSeen  uint16
	PokedexOwned uint16

	// Menus and text.
	FontLoaded       uint16
	TextBoxID        uint16
	TileMap          uint16
	TwoOptionMenuID  uint16
	ListMenuID       uint16
	ListScrollOffset uint16
	MenuWatchedKeys  uint16
	CurrentMenuItem  uint16
	MaxMenuItem      uint16
	TopMenuItemX     uint16
	TopMenuItemY     uint16

	// Overworld sprites.
	SpritePlayerStateData1 uint16
	SpriteStateData2       uint16

	// Hidden objects.
	ToggleableObjectFlags uint16
	ToggleableObjectList  uint16

	// Script / walk state read by decoders.
	WalkCounter        uint16
	MtMoonB2FCurScript uint16

	// Addresses the skill layer reads that no current decoder needs. Kept
	// here so one resolved set serves both decoders and direct skill reads.
	MapPalOffset            uint16
	PartySpecies            uint16
	PlayerName              uint16
	RivalName               uint16
	WalkBikeSurfState       uint16
	TileInFrontOfPlayer     uint16
	JoyIgnore               uint16
	FirstLockTrashCanIndex  uint16
	SecondLockTrashCanIndex uint16
	NumSafariBalls          uint16
	NumRunAttempts          uint16
	MoveNum                 uint16
	NumberOfWarps           uint16
	WarpEntries             uint16
	MoveMenuType            uint16
	PlayerMonNumber         uint16
	WhichPokemon            uint16
	ItemList                uint16
	ItemQuantity            uint16
	MaxItemQuantity         uint16
	BoxMonCounts            uint16
	FieldMoves              uint16
	RodResponse             uint16
	OverworldMap            uint16
}

// RedAddresses returns the address set for the supported Pokémon Red image.
func RedAddresses() Addresses {
	return Addresses{
		BattleAddresses: RedBattleAddresses(),

		CurMap:                  sym.CurMap,
		CurMapTileset:           sym.CurMapTileset,
		CurMapHeight:            sym.CurMapHeight,
		CurMapWidth:             sym.CurMapWidth,
		XCoord:                  sym.XCoord,
		YCoord:                  sym.YCoord,
		SpritePlayerFacing:      sym.SpritePlayerFacing,
		PartyCount:              sym.PartyCount,
		PartyMon1:               sym.PartyMon1,
		BoxCount:                sym.BoxCount,
		BoxMon1:                 sym.BoxMon1,
		CurrentBoxNum:           sym.CurrentBoxNum,
		PlayerMoney:             sym.PlayerMoney,
		PlayerCoins:             sym.PlayerCoins,
		NumBagItems:             sym.NumBagItems,
		BagItems:                sym.BagItems,
		ObtainedBadges:          sym.ObtainedBadges,
		EventFlags:              sym.EventFlags,
		StatusFlags1:            sym.StatusFlags1,
		StatusFlags4:            sym.StatusFlags4,
		PokedexSeen:             sym.PokedexSeen,
		PokedexOwned:            sym.PokedexOwned,
		FontLoaded:              sym.FontLoaded,
		TextBoxID:               sym.TextBoxID,
		TileMap:                 sym.TileMap,
		TwoOptionMenuID:         sym.TwoOptionMenuID,
		ListMenuID:              sym.ListMenuID,
		ListScrollOffset:        sym.ListScrollOffset,
		MenuWatchedKeys:         sym.MenuWatchedKeys,
		CurrentMenuItem:         sym.CurrentMenuItem,
		MaxMenuItem:             sym.MaxMenuItem,
		TopMenuItemX:            sym.TopMenuItemX,
		TopMenuItemY:            sym.TopMenuItemY,
		SpritePlayerStateData1:  sym.SpritePlayerStateData1,
		SpriteStateData2:        sym.SpriteStateData2,
		ToggleableObjectFlags:   sym.ToggleableObjectFlags,
		ToggleableObjectList:    sym.ToggleableObjectList,
		WalkCounter:             sym.WalkCounter,
		MtMoonB2FCurScript:      sym.MtMoonB2FCurScript,
		MapPalOffset:            sym.MapPalOffset,
		PartySpecies:            sym.PartySpecies,
		PlayerName:              sym.PlayerName,
		RivalName:               sym.RivalName,
		WalkBikeSurfState:       sym.WalkBikeSurfState,
		TileInFrontOfPlayer:     sym.TileInFrontOfPlayer,
		JoyIgnore:               sym.JoyIgnore,
		FirstLockTrashCanIndex:  sym.FirstLockTrashCanIndex,
		SecondLockTrashCanIndex: sym.SecondLockTrashCanIndex,
		NumSafariBalls:          sym.NumSafariBalls,
		NumRunAttempts:          sym.NumRunAttempts,
		MoveNum:                 sym.MoveNum,
		NumberOfWarps:           sym.NumberOfWarps,
		WarpEntries:             sym.WarpEntries,
		MoveMenuType:            sym.MoveMenuType,
		PlayerMonNumber:         sym.PlayerMonNumber,
		WhichPokemon:            sym.WhichPokemon,
		ItemList:                sym.ItemList,
		ItemQuantity:            sym.ItemQuantity,
		MaxItemQuantity:         sym.MaxItemQuantity,
		BoxMonCounts:            sym.BoxMonCounts,
		FieldMoves:              sym.FieldMoves,
		RodResponse:             sym.RodResponse,
		OverworldMap:            sym.OverworldMap,
	}
}
