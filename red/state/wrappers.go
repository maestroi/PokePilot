package state

// Free-function wrappers keep the package API the Red-only callers (and the
// red/state tests) already use: each decodes with the supported Pokemon Red
// image's addresses. Callers that may hold a different image should resolve
// an Addresses (see profiles) and call the method form instead, or the decode
// silently reads the wrong game's RAM.
func Controllable(m *Mem) bool                  { return RedAddresses().Controllable(m) }
func DecodeBox(m *Mem) BoxState                 { return RedAddresses().DecodeBox(m) }
func DecodeCoins(mem *Mem) int                  { return RedAddresses().DecodeCoins(mem) }
func DecodeDialogue(m *Mem) *DialogueState      { return RedAddresses().DecodeDialogue(m) }
func DecodeInteraction(m *Mem) InteractionState { return RedAddresses().DecodeInteraction(m) }
func DecodeInventory(m *Mem) InventoryState     { return RedAddresses().DecodeInventory(m) }
func DecodeMenu(m *Mem) MenuState               { return RedAddresses().DecodeMenu(m) }
func DecodeParty(m *Mem) PartyState             { return RedAddresses().DecodeParty(m) }
func DecodePlayer(m *Mem) PlayerState           { return RedAddresses().DecodePlayer(m) }
func DecodePokedex(m *Mem) PokedexState         { return RedAddresses().DecodePokedex(m) }
func DecodeProgress(m *Mem) ProgressState       { return RedAddresses().DecodeProgress(m) }
func DecodeSprites(m *Mem) []SpriteState        { return RedAddresses().DecodeSprites(m) }
func DecodeStoryFacts(m *Mem, inv InventoryState) StoryFacts {
	return RedAddresses().DecodeStoryFacts(m, inv)
}
func DecodeTwoOptionMenu(m *Mem) *TwoOptionMenu { return RedAddresses().DecodeTwoOptionMenu(m) }
func DecodeWorld(m *Mem) WorldState             { return RedAddresses().DecodeWorld(m) }
func HasEvent(m *Mem, e Event) bool             { return RedAddresses().HasEvent(m, e) }
func TookStarterBall(m *Mem) bool               { return RedAddresses().TookStarterBall(m) }
func MenuUp(m *Mem) bool                        { return RedAddresses().MenuUp(m) }
