package agent

import (
	"fmt"
	"strings"
)

// InventoryCategory is the planner-facing role an item plays in the run.
// Categories describe why money or bag space might be spent; they do not make
// an item mandatory by themselves.
type InventoryCategory string

const (
	InventoryProgressionCritical InventoryCategory = "progression-critical"
	InventoryBattleConsumable    InventoryCategory = "battle-consumable"
	InventoryCapture             InventoryCategory = "capture"
	InventoryEvolution           InventoryCategory = "evolution"
	InventoryTravel              InventoryCategory = "travel"
	InventoryOptionalUtility     InventoryCategory = "optional-utility"
)

const (
	bagItemCapacity       = 20 // pokered/constants/menu_constants.asm
	targetCaptureStock    = 10
	minimumCaptureStock   = 5
	targetEmergencyHeals  = 2
	maxSafariEntryReserve = 1500 // skill.FuchsiaProgression allows at most 3 x ¥500 Safari sessions.
)

// ItemEconomySpec is the static economic meaning of one Red item. UnitPrice
// is the game's ItemPrices value. A zero price means the item is not a normal
// mart purchase; it can still be progression-critical inventory.
type ItemEconomySpec struct {
	Name      string
	ID        uint8
	Category  InventoryCategory
	UnitPrice uint32
}

// MoneyReservation is money the current implemented progression can prove it
// will need later. Reservations are deliberately narrow: no walkthrough
// guesses and no parsing planner-authored intent.
type MoneyReservation struct {
	Reason string
	Amount uint32
}

// InventoryEconomyItem is one current bag entry with its strategic category
// and normal mart price attached for planner diagnostics.
type InventoryEconomyItem struct {
	Name      string
	Quantity  int
	Category  InventoryCategory
	UnitPrice uint32
}

// PurchaseAdvice explains one item on the current mart shelf. CanAfford is
// intentionally separate from ShouldBuy: having enough cash is not a reason
// to spend it. CanAffordAfterReserve says whether even one unit can be bought
// without consuming money already reserved for known progression.
type PurchaseAdvice struct {
	Item                  string
	Category              InventoryCategory
	UnitPrice             uint32
	Owned                 int
	CategoryStock         int
	TargetStock           int
	SuggestedQty          int
	SuggestedCost         uint32
	CanStore              bool
	CanAfford             bool
	CanAffordAfterReserve bool
	ShouldBuy             bool
	Reason                string
}

// EconomyDecisionContext is the deterministic resource policy exposed to the
// planner. It is advice, not another planner: every number is derived from the
// observation and Red's fixed prices/capacities. The model still chooses among
// the objectives Offer made legal.
type EconomyDecisionContext struct {
	Money                    uint32
	MoneyAfterBlackout       uint32
	BlackoutLoss             uint32
	ReservedMoney            uint32
	SpendableMoney           uint32
	SpendableAfterBlackout   uint32
	ReserveSurvivesBlackout  bool
	BagSlotsUsed             int
	BagSlotsFree             int
	PreferFreeCenterHealing  bool
	ResupplyNeeded           bool
	ResupplyReason           string `json:",omitempty"`
	Reservations             []MoneyReservation `json:",omitempty"`
	Inventory                []InventoryEconomyItem `json:",omitempty"`
	Purchases                []PurchaseAdvice `json:",omitempty"`
}

// itemEconomy is Red's item-price table restricted to items that matter to
// strategic inventory decisions today. Prices come from
// pokered/data/items/prices.asm; zero-price key items are included so bag
// contents can still be classified correctly.
var itemEconomy = map[string]ItemEconomySpec{
	"master ball":    {Name: "master ball", ID: 0x01, Category: InventoryCapture, UnitPrice: 0},
	"ultra ball":     {Name: "ultra ball", ID: 0x02, Category: InventoryCapture, UnitPrice: 1200},
	"great ball":     {Name: "great ball", ID: 0x03, Category: InventoryCapture, UnitPrice: 600},
	"pokeball":       {Name: "pokeball", ID: 0x04, Category: InventoryCapture, UnitPrice: 200},
	"town map":       {Name: "town map", ID: 0x05, Category: InventoryProgressionCritical, UnitPrice: 0},
	"bicycle":        {Name: "bicycle", ID: 0x06, Category: InventoryProgressionCritical, UnitPrice: 0},
	"safari ball":    {Name: "safari ball", ID: 0x08, Category: InventoryCapture, UnitPrice: 1000},
	"pokedex":        {Name: "pokedex", ID: 0x09, Category: InventoryProgressionCritical, UnitPrice: 0},
	"moon stone":     {Name: "moon stone", ID: 0x0A, Category: InventoryEvolution, UnitPrice: 0},
	"antidote":       {Name: "antidote", ID: 0x0B, Category: InventoryBattleConsumable, UnitPrice: 100},
	"burn heal":      {Name: "burn heal", ID: 0x0C, Category: InventoryBattleConsumable, UnitPrice: 250},
	"ice heal":       {Name: "ice heal", ID: 0x0D, Category: InventoryBattleConsumable, UnitPrice: 250},
	"awakening":      {Name: "awakening", ID: 0x0E, Category: InventoryBattleConsumable, UnitPrice: 200},
	"parlyz heal":    {Name: "parlyz heal", ID: 0x0F, Category: InventoryBattleConsumable, UnitPrice: 200},
	"full restore":   {Name: "full restore", ID: 0x10, Category: InventoryBattleConsumable, UnitPrice: 3000},
	"max potion":     {Name: "max potion", ID: 0x11, Category: InventoryBattleConsumable, UnitPrice: 2500},
	"hyper potion":   {Name: "hyper potion", ID: 0x12, Category: InventoryBattleConsumable, UnitPrice: 1500},
	"super potion":   {Name: "super potion", ID: 0x13, Category: InventoryBattleConsumable, UnitPrice: 700},
	"potion":         {Name: "potion", ID: 0x14, Category: InventoryBattleConsumable, UnitPrice: 300},
	"escape rope":    {Name: "escape rope", ID: 0x1D, Category: InventoryTravel, UnitPrice: 550},
	"repel":          {Name: "repel", ID: 0x1E, Category: InventoryTravel, UnitPrice: 350},
	"old amber":      {Name: "old amber", ID: 0x1F, Category: InventoryProgressionCritical, UnitPrice: 0},
	"fire stone":     {Name: "fire stone", ID: 0x20, Category: InventoryEvolution, UnitPrice: 2100},
	"thunder stone":  {Name: "thunder stone", ID: 0x21, Category: InventoryEvolution, UnitPrice: 2100},
	"water stone":    {Name: "water stone", ID: 0x22, Category: InventoryEvolution, UnitPrice: 2100},
	"rare candy":     {Name: "rare candy", ID: 0x28, Category: InventoryOptionalUtility, UnitPrice: 4800},
	"dome fossil":    {Name: "dome fossil", ID: 0x29, Category: InventoryProgressionCritical, UnitPrice: 0},
	"helix fossil":   {Name: "helix fossil", ID: 0x2A, Category: InventoryProgressionCritical, UnitPrice: 0},
	"secret key":     {Name: "secret key", ID: 0x2B, Category: InventoryProgressionCritical, UnitPrice: 0},
	"bike voucher":   {Name: "bike voucher", ID: 0x2D, Category: InventoryProgressionCritical, UnitPrice: 0},
	"x accuracy":     {Name: "x accuracy", ID: 0x2E, Category: InventoryBattleConsumable, UnitPrice: 950},
	"leaf stone":     {Name: "leaf stone", ID: 0x2F, Category: InventoryEvolution, UnitPrice: 2100},
	"card key":       {Name: "card key", ID: 0x30, Category: InventoryProgressionCritical, UnitPrice: 0},
	"nugget":         {Name: "nugget", ID: 0x31, Category: InventoryOptionalUtility, UnitPrice: 10000},
	"poke doll":      {Name: "poke doll", ID: 0x33, Category: InventoryOptionalUtility, UnitPrice: 1000},
	"full heal":      {Name: "full heal", ID: 0x34, Category: InventoryBattleConsumable, UnitPrice: 600},
	"revive":         {Name: "revive", ID: 0x35, Category: InventoryBattleConsumable, UnitPrice: 1500},
	"max revive":     {Name: "max revive", ID: 0x36, Category: InventoryBattleConsumable, UnitPrice: 4000},
	"guard spec":     {Name: "guard spec", ID: 0x37, Category: InventoryBattleConsumable, UnitPrice: 700},
	"super repel":    {Name: "super repel", ID: 0x38, Category: InventoryTravel, UnitPrice: 500},
	"max repel":      {Name: "max repel", ID: 0x39, Category: InventoryTravel, UnitPrice: 700},
	"dire hit":       {Name: "dire hit", ID: 0x3A, Category: InventoryBattleConsumable, UnitPrice: 650},
	"fresh water":    {Name: "fresh water", ID: 0x3C, Category: InventoryProgressionCritical, UnitPrice: 200},
	"soda pop":       {Name: "soda pop", ID: 0x3D, Category: InventoryBattleConsumable, UnitPrice: 300},
	"lemonade":       {Name: "lemonade", ID: 0x3E, Category: InventoryBattleConsumable, UnitPrice: 350},
	"s.s. ticket":    {Name: "s.s. ticket", ID: 0x3F, Category: InventoryProgressionCritical, UnitPrice: 0},
	"gold teeth":     {Name: "gold teeth", ID: 0x40, Category: InventoryProgressionCritical, UnitPrice: 0},
	"x attack":       {Name: "x attack", ID: 0x41, Category: InventoryBattleConsumable, UnitPrice: 500},
	"x defend":       {Name: "x defend", ID: 0x42, Category: InventoryBattleConsumable, UnitPrice: 550},
	"x speed":        {Name: "x speed", ID: 0x43, Category: InventoryBattleConsumable, UnitPrice: 350},
	"x special":      {Name: "x special", ID: 0x44, Category: InventoryBattleConsumable, UnitPrice: 350},
	"coin case":      {Name: "coin case", ID: 0x45, Category: InventoryProgressionCritical, UnitPrice: 0},
	"oaks parcel":    {Name: "oaks parcel", ID: 0x46, Category: InventoryProgressionCritical, UnitPrice: 0},
	"itemfinder":     {Name: "itemfinder", ID: 0x47, Category: InventoryOptionalUtility, UnitPrice: 0},
	"silph scope":    {Name: "silph scope", ID: 0x48, Category: InventoryProgressionCritical, UnitPrice: 0},
	"poke flute":     {Name: "poke flute", ID: 0x49, Category: InventoryProgressionCritical, UnitPrice: 0},
	"lift key":       {Name: "lift key", ID: 0x4A, Category: InventoryProgressionCritical, UnitPrice: 0},
	"exp all":        {Name: "exp all", ID: 0x4B, Category: InventoryOptionalUtility, UnitPrice: 0},
	"old rod":        {Name: "old rod", ID: 0x4C, Category: InventoryProgressionCritical, UnitPrice: 0},
	"good rod":       {Name: "good rod", ID: 0x4D, Category: InventoryProgressionCritical, UnitPrice: 0},
	"super rod":      {Name: "super rod", ID: 0x4E, Category: InventoryProgressionCritical, UnitPrice: 0},
	"pp up":          {Name: "pp up", ID: 0x4F, Category: InventoryBattleConsumable, UnitPrice: 0},
	"ether":          {Name: "ether", ID: 0x50, Category: InventoryBattleConsumable, UnitPrice: 0},
	"max ether":      {Name: "max ether", ID: 0x51, Category: InventoryBattleConsumable, UnitPrice: 0},
	"elixer":         {Name: "elixer", ID: 0x52, Category: InventoryBattleConsumable, UnitPrice: 0},
	"max elixer":     {Name: "max elixer", ID: 0x53, Category: InventoryBattleConsumable, UnitPrice: 0},
	"hm03":           {Name: "hm03", ID: 0xC6, Category: InventoryProgressionCritical, UnitPrice: 0},
	"hm04":           {Name: "hm04", ID: 0xC7, Category: InventoryProgressionCritical, UnitPrice: 0},
}

// The agent's older argument vocabulary covered only the first slices of the
// game. Extending it here makes later mart stock (Ultra Balls, Revives,
// evolution stones, Repels, etc.) visible to Observation.MartStock instead of
// silently disappearing when Observe asks ItemName for the clerk's ROM list.
func init() {
	for name, spec := range itemEconomy {
		if _, exists := itemTable[name]; !exists {
			itemTable[name] = spec.ID
		}
		if _, exists := itemByID[spec.ID]; !exists {
			itemByID[spec.ID] = name
		}
	}
}

// ItemEconomy returns the known economic metadata for a normalized item name.
func ItemEconomy(name string) (ItemEconomySpec, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	spec, ok := itemEconomy[name]
	if ok {
		return spec, true
	}
	return ItemEconomySpec{Name: name, Category: InventoryOptionalUtility}, false
}

// EconomyContext derives planner-facing resource policy from one observation.
// It is ROM-free and deterministic so budget/reserve decisions can be tested
// without starting the emulator.
func EconomyContext(o Observation) *EconomyDecisionContext {
	relevant := o.Money > 0 || o.BlackedOut || len(o.Bag) > 0 || len(o.MartStock) > 0 ||
		(len(o.WildGrass) > 0 && normalBallStock(o) < minimumCaptureStock) || partyHurt(o) || pendingFuchsiaSpend(o)
	if !relevant {
		return nil
	}

	ctx := &EconomyDecisionContext{
		Money:                   o.Money,
		MoneyAfterBlackout:      o.Money / 2,
		BlackoutLoss:            o.Money - o.Money/2,
		BagSlotsUsed:            len(o.Bag),
		PreferFreeCenterHealing: partyHurt(o),
	}
	ctx.BagSlotsFree = bagItemCapacity - ctx.BagSlotsUsed
	if ctx.BagSlotsFree < 0 {
		ctx.BagSlotsFree = 0
	}

	if pendingFuchsiaSpend(o) {
		ctx.Reservations = append(ctx.Reservations, MoneyReservation{
			Reason: "reserve up to three ¥500 Safari Zone entries required by the current Fuchsia progression skill",
			Amount: maxSafariEntryReserve,
		})
	}
	for _, r := range ctx.Reservations {
		ctx.ReservedMoney += r.Amount
	}
	ctx.SpendableMoney = subtractFloor(o.Money, ctx.ReservedMoney)
	ctx.SpendableAfterBlackout = subtractFloor(ctx.MoneyAfterBlackout, ctx.ReservedMoney)
	ctx.ReserveSurvivesBlackout = ctx.MoneyAfterBlackout >= ctx.ReservedMoney

	ctx.Inventory = make([]InventoryEconomyItem, 0, len(o.Bag))
	for _, item := range o.Bag {
		spec, _ := ItemEconomy(item.Name)
		ctx.Inventory = append(ctx.Inventory, InventoryEconomyItem{
			Name: item.Name, Quantity: item.Quantity, Category: spec.Category, UnitPrice: spec.UnitPrice,
		})
	}

	balls := normalBallStock(o)
	heals := emergencyHealStock(o)
	bossRecovery := hasBossFailure(o)
	switch {
	case len(o.WildGrass) > 0 && balls < minimumCaptureStock:
		ctx.ResupplyNeeded = true
		ctx.ResupplyReason = fmt.Sprintf("capture stock is low (%d normal balls); target %d before committing to more catch attempts", balls, targetCaptureStock)
	case partyHurt(o) && heals == 0:
		ctx.ResupplyNeeded = true
		ctx.ResupplyReason = "party is hurt with no HP-healing stock; use free Center healing when practical, otherwise buy only a bounded emergency stock"
	case bossRecovery && heals == 0:
		ctx.ResupplyNeeded = true
		ctx.ResupplyReason = "a boss objective failed and no HP-healing stock remains; recover at a free Center first, then consider a bounded emergency stock"
	}

	ctx.Purchases = purchaseAdvice(o, ctx)
	return ctx
}

func pendingFuchsiaSpend(o Observation) bool {
	return bagHas(o, "poke flute") && (!hasBadge(o, 5) || !bagHas(o, "hm03") || !bagHas(o, "hm04"))
}

func subtractFloor(have, reserve uint32) uint32 {
	if have <= reserve {
		return 0
	}
	return have - reserve
}

func bagQuantity(o Observation, name string) int {
	for _, item := range o.Bag {
		if item.Name == name {
			return item.Quantity
		}
	}
	return 0
}

func normalBallStock(o Observation) int {
	total := 0
	for _, name := range []string{"pokeball", "great ball", "ultra ball"} {
		total += bagQuantity(o, name)
	}
	return total
}

var hpHealingItems = map[string]int{
	"potion": 20, "super potion": 50, "hyper potion": 200,
	"max potion": 10000, "full restore": 10000,
	"fresh water": 50, "soda pop": 60, "lemonade": 80,
}

func emergencyHealStock(o Observation) int {
	total := 0
	for name := range hpHealingItems {
		total += bagQuantity(o, name)
	}
	return total
}

func hasBossFailure(o Observation) bool {
	for _, f := range o.Failures {
		name := strings.ToLower(f.Objective)
		if strings.Contains(name, "gym leader") || strings.Contains(name, "rocket hideout") ||
			strings.Contains(name, "pokemon tower") || strings.Contains(name, "fuchsia") {
			return true
		}
	}
	return false
}

func purchaseAdvice(o Observation, ctx *EconomyDecisionContext) []PurchaseAdvice {
	if len(o.MartStock) == 0 {
		return nil
	}

	balls := normalBallStock(o)
	heals := emergencyHealStock(o)
	preferredBall := preferredCapturePurchase(o.MartStock, ctx.SpendableMoney)
	preferredHeal := preferredHealingPurchase(o, ctx.SpendableMoney)
	out := make([]PurchaseAdvice, 0, len(o.MartStock))
	for _, rawName := range o.MartStock {
		name := strings.ToLower(strings.TrimSpace(rawName))
		spec, ok := ItemEconomy(name)
		if !ok || spec.UnitPrice == 0 {
			continue
		}
		owned := bagQuantity(o, name)
		canStore := owned > 0 || ctx.BagSlotsFree > 0
		advice := PurchaseAdvice{
			Item: name, Category: spec.Category, UnitPrice: spec.UnitPrice, Owned: owned,
			CanStore: canStore,
			CanAfford: o.Money >= spec.UnitPrice,
			CanAffordAfterReserve: ctx.SpendableMoney >= spec.UnitPrice,
		}

		switch spec.Category {
		case InventoryCapture:
			advice.CategoryStock, advice.TargetStock = balls, targetCaptureStock
			need := targetCaptureStock - balls
			if need < 0 {
				need = 0
			}
			switch {
			case balls >= targetCaptureStock:
				advice.Reason = fmt.Sprintf("capture stock already meets the bounded target (%d/%d); do not overbuy", balls, targetCaptureStock)
			case preferredBall != name:
				advice.Reason = fmt.Sprintf("capture stock is %d/%d, but %s is the strongest stocked ball affordable without consuming reserved money", balls, targetCaptureStock, strings.ToUpper(preferredBall))
			case !canStore:
				advice.Reason = "bag has no free item slot for a new ball type"
			default:
				advice.SuggestedQty = boundedAffordableQty(need, ctx.SpendableMoney, spec.UnitPrice)
				advice.ShouldBuy = advice.SuggestedQty > 0
				advice.Reason = fmt.Sprintf("bounded capture resupply from %d toward %d while preserving ¥%d reserved money", balls, targetCaptureStock, ctx.ReservedMoney)
			}

		case InventoryBattleConsumable:
			if _, hpHeal := hpHealingItems[name]; hpHeal {
				advice.CategoryStock, advice.TargetStock = heals, targetEmergencyHeals
				need := targetEmergencyHeals - heals
				if need < 0 {
					need = 0
				}
				needNow := partyHurt(o) || hasBossFailure(o)
				switch {
				case !needNow:
					advice.Reason = "no immediate recovery pressure; prefer free Center healing and preserve money"
				case heals >= targetEmergencyHeals:
					advice.Reason = fmt.Sprintf("emergency healing stock already meets the bounded target (%d/%d); prefer free Center healing", heals, targetEmergencyHeals)
				case preferredHeal != name:
					advice.Reason = fmt.Sprintf("healing stock is %d/%d, but %s better matches current party HP for this bounded resupply", heals, targetEmergencyHeals, strings.ToUpper(preferredHeal))
				case !canStore:
					advice.Reason = "bag has no free item slot for a new healing item type"
				default:
					advice.SuggestedQty = boundedAffordableQty(need, ctx.SpendableMoney, spec.UnitPrice)
					advice.ShouldBuy = advice.SuggestedQty > 0
					advice.Reason = fmt.Sprintf("buy only a bounded emergency stock; Center healing is free when practical and ¥%d stays reserved", ctx.ReservedMoney)
				}
			} else if statusCureNeeded(o, name) && owned == 0 {
				advice.TargetStock = 1
				if canStore {
					advice.SuggestedQty = boundedAffordableQty(1, ctx.SpendableMoney, spec.UnitPrice)
					advice.ShouldBuy = advice.SuggestedQty == 1
				}
				advice.Reason = "party currently has the status this item cures; one is enough, and a free Center remains preferable when practical"
			} else {
				advice.Reason = "consumable is not needed by the current party state; preserve money and bag capacity"
			}

		case InventoryEvolution:
			advice.Reason = "do not speculate on evolution stones: buy only when an explicit evolution objective identifies the required stone"
		case InventoryTravel:
			advice.Reason = "travel utility is discretionary; current route has not proven this purchase is required"
		case InventoryProgressionCritical:
			advice.Reason = "progression-critical category, but the current observation does not prove this shelf item is required now"
		default:
			advice.Reason = "optional utility; preserve money and bag capacity unless a concrete objective requires it"
		}

		if !advice.CanAffordAfterReserve && advice.ShouldBuy {
			advice.ShouldBuy = false
			advice.SuggestedQty = 0
			advice.Reason = fmt.Sprintf("can afford from total cash, but spending would consume ¥%d reserved for known progression", ctx.ReservedMoney)
		}
		advice.SuggestedCost = uint32(advice.SuggestedQty) * spec.UnitPrice
		out = append(out, advice)
	}
	return out
}

func preferredCapturePurchase(stock []string, spendable uint32) string {
	for _, candidate := range []string{"ultra ball", "great ball", "pokeball"} {
		spec, _ := ItemEconomy(candidate)
		if spec.UnitPrice > spendable {
			continue
		}
		for _, stocked := range stock {
			if strings.EqualFold(strings.TrimSpace(stocked), candidate) {
				return candidate
			}
		}
	}
	return ""
}

func preferredHealingPurchase(o Observation, spendable uint32) string {
	wantHeal := 20
	for _, mon := range o.Party {
		if half := int(mon.MaxHP) / 2; half > wantHeal {
			wantHeal = half
		}
	}
	best := ""
	bestPrice := uint32(^uint32(0))
	for _, stocked := range o.MartStock {
		name := strings.ToLower(strings.TrimSpace(stocked))
		potency, ok := hpHealingItems[name]
		if !ok || potency < wantHeal {
			continue
		}
		spec, ok := ItemEconomy(name)
		if !ok || spec.UnitPrice == 0 || spec.UnitPrice > spendable {
			continue
		}
		if spec.UnitPrice < bestPrice {
			best, bestPrice = name, spec.UnitPrice
		}
	}
	if best != "" {
		return best
	}
	// If nothing on the shelf can heal half the largest party member, use the
	// strongest affordable option rather than recommending an unaffordable one.
	bestPotency := -1
	for _, stocked := range o.MartStock {
		name := strings.ToLower(strings.TrimSpace(stocked))
		potency, ok := hpHealingItems[name]
		if !ok || potency <= bestPotency {
			continue
		}
		spec, ok := ItemEconomy(name)
		if !ok || spec.UnitPrice == 0 || spec.UnitPrice > spendable {
			continue
		}
		best, bestPotency = name, potency
	}
	return best
}

func boundedAffordableQty(need int, spendable, unitPrice uint32) int {
	if need <= 0 || unitPrice == 0 {
		return 0
	}
	affordable := int(spendable / unitPrice)
	if affordable < need {
		need = affordable
	}
	if need > 99 {
		need = 99
	}
	if need < 0 {
		return 0
	}
	return need
}

func statusCureNeeded(o Observation, item string) bool {
	want, ok := map[string]string{
		"antidote": "poisoned", "burn heal": "burned", "ice heal": "frozen",
		"awakening": "asleep", "parlyz heal": "paralyzed",
	}[item]
	if item == "full heal" {
		for _, mon := range o.Party {
			if mon.Status != "" {
				return true
			}
		}
		return false
	}
	if !ok {
		return false
	}
	for _, mon := range o.Party {
		if mon.Status == want {
			return true
		}
	}
	return false
}
