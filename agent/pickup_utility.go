package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

type pickupPriority uint8

const (
	pickupOptional pickupPriority = iota
	pickupUseful
	pickupHighValue
)

// enhancePickupObjectives gives the strategist the missing reason to spend a
// round on a free map item. Pickups remain optional semantic objectives: this
// layer only explains their current utility and rough detour size, and filters
// a new bag entry that cannot fit. It never turns the run into a collect-all
// policy and never chooses a pickup for the planner.
func enhancePickupObjectives(romData []byte, party state.PartyState, obs Observation, out []Objective) []Objective {
	enhanced := make([]Objective, 0, len(out))
	for _, o := range out {
		if o.Kind != KindPickup {
			enhanced = append(enhanced, o)
			continue
		}
		if !pickupCanStore(obs, o.Item) {
			// Red's 20-entry bag cannot accept a new item kind. Existing kinds
			// still stack, so a full bag only blocks a pickup not already owned.
			continue
		}

		priority, reason := pickupUtility(obs, o.Item)
		if raw, ok := redItemID(o.Item); ok {
			if machine, err := rom.LookupTMHM(romData, raw); err == nil {
				machinePriority, machineReason := machinePickupUtility(romData, party, raw, machine)
				if machinePriority > priority {
					priority = machinePriority
				}
				if machineReason != "" {
					reason = machineReason
				}
			}
		}

		note := fmt.Sprintf("(%s; %s; %s)", pickupPriorityLabel(priority), pickupDetourClass(obs, o), reason)
		if o.Note == "" {
			o.Note = note
		} else {
			o.Note += " " + note
		}
		enhanced = append(enhanced, o)
	}
	return enhanced
}

func pickupCanStore(obs Observation, item ItemID) bool {
	if bagQuantity(obs, string(item)) > 0 {
		return true
	}
	return len(obs.Bag) < bagItemCapacity
}

func pickupUtility(obs Observation, item ItemID) (pickupPriority, string) {
	name := strings.ToLower(strings.TrimSpace(string(item)))
	owned := bagQuantity(obs, name)
	spec, known := ItemEconomy(name)
	price := ""
	if known && spec.UnitPrice > 0 {
		price = fmt.Sprintf("; free pickup normally costs ¥%d", spec.UnitPrice)
	}

	switch spec.Category {
	case InventoryProgressionCritical:
		return pickupHighValue, "progression-critical resource" + price

	case InventoryCapture:
		stock := normalBallStock(obs)
		if stock < minimumCaptureStock {
			return pickupHighValue, fmt.Sprintf("capture stock low (%d/%d minimum); free resupply%s", stock, minimumCaptureStock, price)
		}
		return pickupUseful, fmt.Sprintf("free capture stock; %d balls currently owned%s", stock, price)

	case InventoryEvolution:
		if owned == 0 {
			return pickupUseful, "new evolution resource for future party development" + price
		}
		return pickupOptional, fmt.Sprintf("additional evolution resource; %d already owned%s", owned, price)

	case InventoryTravel:
		if owned == 0 {
			return pickupUseful, "new travel/encounter-management resource" + price
		}
		return pickupOptional, fmt.Sprintf("extra travel/encounter-management stock; %d already owned%s", owned, price)

	case InventoryBattleConsumable:
		if _, hpHeal := hpHealingItems[name]; hpHeal {
			heals := emergencyHealStock(obs)
			switch {
			case heals == 0:
				return pickupHighValue, "no HP-healing stock; free emergency healing resource" + price
			case partyHurt(obs) || hasBossFailure(obs) || heals < targetEmergencyHeals:
				return pickupHighValue, fmt.Sprintf("healing stock is thin (%d/%d emergency target)%s", heals, targetEmergencyHeals, price)
			default:
				return pickupUseful, fmt.Sprintf("finite healing stock; %d heals currently owned%s", heals, price)
			}
		}
		if _, ppRestore := ppRestoreItems[name]; ppRestore {
			if leadOutOfPP(obs) {
				return pickupHighValue, "lead is out of usable PP; finite PP recovery can prevent a Center detour" + price
			}
			return pickupUseful, "finite PP recovery resource" + price
		}
		if name == "revive" || name == "max revive" {
			if partyHasFaintedMember(obs) {
				return pickupHighValue, "party has a fainted member; free revival resource" + price
			}
			return pickupUseful, "finite revival resource" + price
		}
		if name == "full heal" {
			if partyHasStatus(obs, "") {
				return pickupHighValue, "party has a status condition; broad status recovery is immediately useful" + price
			}
			return pickupUseful, "broad status recovery resource" + price
		}
		if want, statusCure := fieldMedStatus[name]; statusCure && want != "" {
			if partyHasStatus(obs, want) {
				return pickupHighValue, fmt.Sprintf("party currently has %s; matching status cure%s", want, price)
			}
			return pickupOptional, "situational status recovery" + price
		}
		if name == "pp up" {
			return pickupUseful, "permanent PP-capacity resource" + price
		}
		return pickupOptional, "situational battle consumable" + price
	}

	if owned == 0 {
		return pickupOptional, "free optional resource not currently tied to a known shortage" + price
	}
	return pickupOptional, fmt.Sprintf("extra optional resource; %d already owned%s", owned, price)
}

func machinePickupUtility(romData []byte, party state.PartyState, raw uint8, machine rom.Machine) (pickupPriority, string) {
	name := fmt.Sprintf("TM%02d", machine.Number)
	base := "one-time move resource"
	priority := pickupUseful
	if machine.HM {
		name = fmt.Sprintf("HM%02d", machine.Number-rom.NumTMs)
		base = "reusable field/move resource"
		priority = pickupHighValue
	}

	decision, err := skill.DecideTMHM(romData, party, raw, false)
	if err == nil {
		if decision.Existing {
			return priority, fmt.Sprintf("%s %s; current party already knows move %d", name, base, machine.Move)
		}
		return pickupHighValue, fmt.Sprintf("%s %s; usable now on party slot %d with move-set score %d->%d", name, base, decision.PartySlot, decision.BeforeScore, decision.AfterScore)
	}
	return priority, fmt.Sprintf("%s %s; no current material move-set upgrade, but it remains a free future option", name, base)
}

func pickupPriorityLabel(priority pickupPriority) string {
	switch priority {
	case pickupHighValue:
		return "high-value preparation"
	case pickupUseful:
		return "useful preparation"
	default:
		return "optional pickup"
	}
}

// pickupDetourClass is deliberately approximate. Observation has semantic map
// coordinates but not a route-cost contract, so Manhattan tile offset is used
// only to distinguish "nearby" from progressively larger same-map detours; it
// is never presented as an exact walking distance.
func pickupDetourClass(obs Observation, o Objective) string {
	dx := absInt(int(obs.X) - int(o.X))
	dy := absInt(int(obs.Y) - int(o.Y))
	switch d := dx + dy; {
	case d <= 3:
		return "nearby"
	case d <= 8:
		return "small local detour"
	case d <= 16:
		return "local detour"
	default:
		return "longer same-map detour"
	}
}

func partyHasFaintedMember(obs Observation) bool {
	for _, mon := range obs.Party {
		if mon.MaxHP > 0 && mon.HP == 0 {
			return true
		}
	}
	return false
}

// want == "" means any non-empty status.
func partyHasStatus(obs Observation, want string) bool {
	for _, mon := range obs.Party {
		if want == "" {
			if mon.Status != "" {
				return true
			}
			continue
		}
		if mon.Status == want {
			return true
		}
	}
	return false
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
