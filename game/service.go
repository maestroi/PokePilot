package game

// ShopPhase is the semantic screen/state of a shop transaction. Concrete
// profiles own the RAM/menu layout used to recognize each phase.
type ShopPhase uint8

const (
	ShopPhaseClosed ShopPhase = iota
	ShopPhaseGreeting
	ShopPhaseActionMenu
	ShopPhaseItemList
	ShopPhaseQuantity
	ShopPhaseConfirmation
)

// ShopState is the portable transaction view used by the reusable shop driver.
type ShopState struct {
	Phase            ShopPhase
	Controllable     bool
	Cursor           MenuCursorState
	Items            []uint16
	Quantity         int
	MaxQuantity      int
	Total            int
	Text             string
	TradeUnavailable bool
	Unsellable       bool
}

// ShopDecoder hides game-specific shop menu recognition, stock layout and
// transaction scratch variables.
type ShopDecoder interface {
	DecodeShop(MemoryReader) ShopState
}

type ShopProfile interface {
	GameProfile
	ShopDecoder
}

// CenterState is the semantic state needed to drive a Pokemon Center heal.
type CenterState struct {
	PartyPresent bool
	PromptOpen   bool
	Recovered    bool
	Controllable bool
	TextOpen     bool
	MenuOpen     bool
}

// CenterDecoder hides game-specific nurse prompt and party-recovery layout.
type CenterDecoder interface {
	DecodeCenter(MemoryReader) CenterState
}

type CenterProfile interface {
	GameProfile
	CenterDecoder
}
