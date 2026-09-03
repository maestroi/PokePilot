package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/red/state"
)

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
	bagItemCapacity       = 20
	targetCaptureStock    = 10
	minimumCaptureStock   = 5
	targetEmergencyHeals  = 2
	maxSafariEntryReserve = 1500 // FuchsiaProgression permits at most 3 x ¥500 Safari sessions.
)

type ItemEconomySpec struct {
	Name      string
	ID        uint8
	Category  InventoryCategory
	UnitPrice uint32
}

type MoneyReservation struct {
	Reason string
	Amount uint32
}

type InventoryEconomyItem struct {
	Name      string
	Quantity  int
	Category  InventoryCategory
	UnitPrice uint32
}

// PurchaseAdvice deliberately separates affordability from desirability.
// SuggestedQty is bounded by the stock target, available cash after reserves,
// and the existing KindBuy quantity range.
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

type EconomyDecisionContext struct {
	Money                   uint32
	MoneyAfterBlackout      uint32
	BlackoutLoss            uint32
	ReservedMoney           uint32
	SpendableMoney          uint32
	SpendableAfterBlackout  uint32
	ReserveSurvivesBlackout bool
	BagSlotsUsed            int
	BagSlotsFree            int
	PreferFreeCenterHealing bool
	ResupplyNeeded          bool
	ResupplyReason          string                 `json:",omitempty"`
	Reservations            []MoneyReservation     `json:",omitempty"`
	Inventory               []InventoryEconomyItem `json:",omitempty"`
	Purchases               []PurchaseAdvice       `json:",omitempty"`
}

// Prices are the fixed values in pokered/data/items/prices.asm. Zero-price
// key items are present only so current inventory is classified correctly.
var itemEconomy = map[string]ItemEconomySpec{
	"ultra ball":    {Name: "ultra ball", ID: 0x02, Category: InventoryCapture, UnitPrice: 1200},
	"great ball":    {Name: "great ball", ID: 0x03, Category: InventoryCapture, UnitPrice: 600},
	"pokeball":      {Name: "pokeball", ID: 0x04, Category: InventoryCapture, UnitPrice: 200},
	"antidote":      {Name: "antidote", ID: 0x0B, Category: InventoryBattleConsumable, UnitPrice: 100},
	"burn heal":     {Name: "burn heal", ID: 0x0C, Category: InventoryBattleConsumable, UnitPrice: 250},
	"ice heal":      {Name: "ice heal", ID: 0x0D, Category: InventoryBattleConsumable, UnitPrice: 250},
	"awakening":     {Name: "awakening", ID: 0x0E, Category: InventoryBattleConsumable, UnitPrice: 200},
	"parlyz heal":   {Name: "parlyz heal", ID: 0x0F, Category: InventoryBattleConsumable, UnitPrice: 200},
	"full restore":  {Name: "full restore", ID: 0x10, Category: InventoryBattleConsumable, UnitPrice: 3000},
	"max potion":    {Name: "max potion", ID: 0x11, Category: InventoryBattleConsumable, UnitPrice: 2500},
	"hyper potion":  {Name: "hyper potion", ID: 0x12, Category: InventoryBattleConsumable, UnitPrice: 1500},
	"super potion":  {Name: "super potion", ID: 0x13, Category: InventoryBattleConsumable, UnitPrice: 700},
	"potion":        {Name: "potion", ID: 0x14, Category: InventoryBattleConsumable, UnitPrice: 300},
	"escape rope":   {Name: "escape rope", ID: 0x1D, Category: InventoryTravel, UnitPrice: 550},
	"repel":         {Name: "repel", ID: 0x1E, Category: InventoryTravel, UnitPrice: 350},
	"fire stone":    {Name: "fire stone", ID: 0x20, Category: InventoryEvolution, UnitPrice: 2100},
	"thunder stone": {Name: "thunder stone", ID: 0x21, Category: InventoryEvolution, UnitPrice: 2100},
	"water stone":   {Name: "water stone", ID: 0x22, Category: InventoryEvolution, UnitPrice: 2100},
	"leaf stone":    {Name: "leaf stone", ID: 0x2F, Category: InventoryEvolution, UnitPrice: 2100},
	"poke doll":     {Name: "poke doll", ID: 0x33, Category: InventoryOptionalUtility, UnitPrice: 1000},
	"full heal":     {Name: "full heal", ID: 0x34, Category: InventoryBattleConsumable, UnitPrice: 600},
	"revive":        {Name: "revive", ID: 0x35, Category: InventoryBattleConsumable, UnitPrice: 1500},
	"max revive":    {Name: "max revive", ID: 0x36, Category: InventoryBattleConsumable, UnitPrice: 4000},
	"guard spec":    {Name: "guard spec", ID: 0x37, Category: InventoryBattleConsumable, UnitPrice: 700},
	"super repel":   {Name: "super repel", ID: 0x38, Category: InventoryTravel, UnitPrice: 500},
	"max repel":     {Name: "max repel", ID: 0x39, Category: InventoryTravel, UnitPrice: 700},
	"dire hit":      {Name: "dire hit", ID: 0x3A, Category: InventoryBattleConsumable, UnitPrice: 650},
	"x attack":      {Name: "x attack", ID: 0x41, Category: InventoryBattleConsumable, UnitPrice: 500},
	"x defend":      {Name: "x defend", ID: 0x42, Category: InventoryBattleConsumable, UnitPrice: 550},
	"x speed":       {Name: "x speed", ID: 0x43, Category: InventoryBattleConsumable, UnitPrice: 350},
	"x special":     {Name: "x special", ID: 0x44, Category: InventoryBattleConsumable, UnitPrice: 350},
	"silph scope":   {Name: "silph scope", ID: 0x48, Category: InventoryProgressionCritical},
	"poke flute":    {Name: "poke flute", ID: 0x49, Category: InventoryProgressionCritical},
	"pp up":         {Name: "pp up", ID: 0x4F, Category: InventoryBattleConsumable},
	"ether":         {Name: "ether", ID: 0x50, Category: InventoryBattleConsumable},
	"max ether":     {Name: "max ether", ID: 0x51, Category: InventoryBattleConsumable},
	"elixer":        {Name: "elixer", ID: 0x52, Category: InventoryBattleConsumable},
	"max elixer":    {Name: "max elixer", ID: 0x53, Category: InventoryBattleConsumable},
	"hm03":          {Name: "hm03", ID: 0xC6, Category: InventoryProgressionCritical},
	"hm04":          {Name: "hm04", ID: 0xC7, Category: InventoryProgressionCritical},
}

// These purchasable items were absent from the old planner vocabulary, which
// meant Observe silently dropped them from later MartStock lists.
var economyVocabulary = map[string]uint8{
	"ultra ball": 0x02, "fire stone": 0x20, "thunder stone": 0x21,
	"water stone": 0x22, "leaf stone": 0x2F, "poke doll": 0x33,
	"full heal": 0x34, "revive": 0x35, "max revive": 0x36,
	"guard spec": 0x37, "super repel": 0x38, "max repel": 0x39,
	"dire hit": 0x3A, "x attack": 0x41, "x defend": 0x42,
	"x speed": 0x43, "x special": 0x44, "pp up": 0x4F,
	"ether": 0x50, "max ether": 0x51, "elixer": 0x52, "max elixer": 0x53,
}

func init() {
	for name, id := range economyVocabulary {
		if _, exists := itemTable[name]; !exists {
			itemTable[name] = id
		}
		if _, exists := itemByID[id]; !exists {
			itemByID[id] = name
		}
	}
}

func ItemEconomy(name string) (ItemEconomySpec, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	spec, ok := itemEconomy[name]
	if ok {
		return spec, true
	}
	return ItemEconomySpec{Name: name, Category: InventoryOptionalUtility}, false
}

// EconomyContext is a pure policy function. It contains no emulator reads and
// no route walkthrough: all decisions come from the current Observation plus
// fixed Red prices and capacities.
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
	for _, reserve := range ctx.Reservations {
		ctx.ReservedMoney += reserve.Amount
	}
	ctx.SpendableMoney = subtractFloor(o.Money, ctx.ReservedMoney)
	ctx.SpendableAfterBlackout = subtractFloor(ctx.MoneyAfterBlackout, ctx.ReservedMoney)
	ctx.ReserveSurvivesBlackout = ctx.MoneyAfterBlackout >= ctx.ReservedMoney

	for _, item := range o.Bag {
		spec, _ := ItemEconomy(item.Name)
		ctx.Inventory = append(ctx.Inventory, InventoryEconomyItem{
			Name: item.Name, Quantity: item.Quantity, Category: spec.Category, UnitPrice: spec.UnitPrice,
		})
	}

	balls, heals := normalBallStock(o), emergencyHealStock(o)
	switch {
	case len(o.WildGrass) > 0 && balls < minimumCaptureStock:
		ctx.ResupplyNeeded = true
		ctx.ResupplyReason = fmt.Sprintf("capture stock is low (%d normal balls); target %d before more catch attempts", balls, targetCaptureStock)
	case partyHurt(o) && heals == 0:
		ctx.ResupplyNeeded = true
		ctx.ResupplyReason = "party is hurt with no HP-healing stock; prefer free Center healing, otherwise buy a bounded emergency stock"
	case hasBossFailure(o) && heals == 0:
		ctx.ResupplyNeeded = true
		ctx.ResupplyReason = "a boss objective failed with no HP-healing stock; recover at a free Center first, then consider bounded emergency stock"
	}

	ctx.Purchases = purchaseAdvice(o, ctx)
	return ctx
}

func pendingFuchsiaSpend(o Observation) bool {
	return bagHas(o, "poke flute") && (!hasBadge(o, state.BadgeSoul) || !bagHas(o, "hm03") || !bagHas(o, "hm04"))
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
	return bagQuantity(o, "pokeball") + bagQuantity(o, "great ball") + bagQuantity(o, "ultra ball")
}

var hpHealingItems = map[string]int{
	"potion": 20, "super potion": 50, "hyper potion": 200,
	"max potion": 10000, "full restore": 10000,
}

func emergencyHealStock(o Observation) int {
	total := 0
	for name := range hpHealingItems {
		total += bagQuantity(o, name)
	}
	return total
}

func hasBossFailure(o Observation) bool {
	for _, failure := range o.Failures {
		name := strings.ToLower(failure.Objective)
		if strings.Contains(name, "gym leader") || strings.Contains(name, "rocket hideout") ||
			strings.Contains(name, "pokemon tower") || strings.Contains(name, "fuchsia") {
			return true
		}
	}
	return false
}

func purchaseAdvice(o Observation, ctx *EconomyDecisionContext) []PurchaseAdvice {
	balls, heals := normalBallStock(o), emergencyHealStock(o)
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
			Item:                  name,
			Category:              spec.Category,
			UnitPrice:             spec.UnitPrice,
			Owned:                 owned,
			CanStore:              canStore,
			CanAfford:             o.Money >= spec.UnitPrice,
			CanAffordAfterReserve: ctx.SpendableMoney >= spec.UnitPrice,
		}

		switch spec.Category {
		case InventoryCapture:
			advice.CategoryStock, advice.TargetStock = balls, targetCaptureStock
			need := maxInt(0, targetCaptureStock-balls)
			switch {
			case balls >= targetCaptureStock:
				advice.Reason = fmt.Sprintf("capture stock already meets the bounded target (%d/%d); do not overbuy", balls, targetCaptureStock)
			case preferredBall == "":
				advice.Reason = "no stocked ball is affordable without consuming reserved money"
			case preferredBall != name:
				advice.Reason = fmt.Sprintf("prefer %s, the strongest stocked ball affordable after reserves", strings.ToUpper(preferredBall))
			case !canStore:
				advice.Reason = "bag has no free item slot for a new ball type"
			default:
				advice.SuggestedQty = boundedAffordableQty(need, ctx.SpendableMoney, spec.UnitPrice)
				advice.ShouldBuy = advice.SuggestedQty > 0
				advice.Reason = fmt.Sprintf("bounded capture resupply toward %d while preserving ¥%d reserved money", targetCaptureStock, ctx.ReservedMoney)
			}

		case InventoryBattleConsumable:
			if _, isHeal := hpHealingItems[name]; isHeal {
				advice.CategoryStock, advice.TargetStock = heals, targetEmergencyHeals
				need := maxInt(0, targetEmergencyHeals-heals)
				needNow := partyHurt(o) || hasBossFailure(o)
				switch {
				case !needNow:
					advice.Reason = "no immediate recovery pressure; prefer free Center healing and preserve money"
				case heals >= targetEmergencyHeals:
					advice.Reason = "emergency healing stock already meets its bounded target; prefer free Center healing"
				case preferredHeal == "":
					advice.Reason = "no stocked medicine is affordable without consuming reserved money"
				case preferredHeal != name:
					advice.Reason = fmt.Sprintf("prefer %s for the current party HP", strings.ToUpper(preferredHeal))
				case !canStore:
					advice.Reason = "bag has no free item slot for a new healing item type"
				default:
					advice.SuggestedQty = boundedAffordableQty(need, ctx.SpendableMoney, spec.UnitPrice)
					advice.ShouldBuy = advice.SuggestedQty > 0
					advice.Reason = "buy only bounded emergency stock; Center healing is free when practical"
				}
			} else if statusCureNeeded(o, name) && owned == 0 && canStore {
				advice.TargetStock = 1
				advice.SuggestedQty = boundedAffordableQty(1, ctx.SpendableMoney, spec.UnitPrice)
				advice.ShouldBuy = advice.SuggestedQty == 1
				advice.Reason = "current party status matches this cure; one is enough and a free Center remains preferable"
			} else {
				advice.Reason = "no current need for this consumable; preserve money and bag capacity"
			}

		case InventoryEvolution:
			advice.Reason = "do not speculate on stones; wait for an explicit evolution objective naming the required stone"
		case InventoryTravel:
			advice.Reason = "travel utility is discretionary unless the current route proves it is required"
		default:
			advice.Reason = "optional utility; preserve money and bag capacity without a concrete objective"
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
		wantHeal = maxInt(wantHeal, int(mon.MaxHP)/2)
	}
	best, bestPrice := "", ^uint32(0)
	for _, stocked := range o.MartStock {
		name := strings.ToLower(strings.TrimSpace(stocked))
		potency, ok := hpHealingItems[name]
		spec, known := ItemEconomy(name)
		if !ok || !known || potency < wantHeal || spec.UnitPrice > spendable {
			continue
		}
		if spec.UnitPrice < bestPrice {
			best, bestPrice = name, spec.UnitPrice
		}
	}
	if best != "" {
		return best
	}

	bestPotency := -1
	for _, stocked := range o.MartStock {
		name := strings.ToLower(strings.TrimSpace(stocked))
		potency, ok := hpHealingItems[name]
		spec, known := ItemEconomy(name)
		if !ok || !known || spec.UnitPrice > spendable || potency <= bestPotency {
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
	need = minInt(need, int(spendable/unitPrice))
	return minInt(need, 99)
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
