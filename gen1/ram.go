package gen1

import "github.com/maestroi/pokepilot/game"

// RAMLayout contains only addresses whose *contents* use common Generation-I
// structures. Each concrete profile supplies its own base addresses.
type RAMLayout struct {
	PartyCount     uint16
	PartyMon1      uint16
	PartyMonSize   uint16
	NumBagItems    uint16
	BagItems       uint16
	PlayerMoney    uint16
	ObtainedBadges uint16
}

type BagItem struct {
	ID       uint8
	Quantity uint8
}

const BagCapacity = 20

const (
	monSpecies = 0x00
	monHP      = 0x01
	monStatus  = 0x04
	monExp     = 0x0e
	monLevel   = 0x21
	monMaxHP   = 0x22
)

func readBE16(r game.MemoryReader, addr uint16) uint16 {
	return uint16(r.Peek8(addr))<<8 | uint16(r.Peek8(addr+1))
}

func DecodeParty(r game.MemoryReader, layout RAMLayout) []game.ProfilePartyMon {
	if r == nil || layout.PartyMonSize == 0 {
		return []game.ProfilePartyMon{}
	}
	count := int(r.Peek8(layout.PartyCount))
	if count > 6 {
		count = 6
	}
	if count < 0 {
		count = 0
	}
	out := make([]game.ProfilePartyMon, count)
	for i := 0; i < count; i++ {
		base := layout.PartyMon1 + uint16(i)*layout.PartyMonSize
		raw := r.Peek8(base + monSpecies)
		species, ok := Species(raw)
		if !ok {
			species = "unknown"
		}
		exp := uint32(r.Peek8(base+monExp))<<16 |
			uint32(r.Peek8(base+monExp+1))<<8 |
			uint32(r.Peek8(base+monExp+2))
		out[i] = game.ProfilePartyMon{
			Species:    species,
			Level:      r.Peek8(base + monLevel),
			Experience: exp,
			HP:         readBE16(r, base+monHP),
			MaxHP:      readBE16(r, base+monMaxHP),
			Status:     statusName(r.Peek8(base + monStatus)),
		}
	}
	return out
}

func statusName(status uint8) string {
	switch {
	case status&0x07 != 0:
		return "asleep"
	case status&(1<<3) != 0:
		return "poisoned"
	case status&(1<<4) != 0:
		return "burned"
	case status&(1<<5) != 0:
		return "frozen"
	case status&(1<<6) != 0:
		return "paralyzed"
	default:
		return ""
	}
}

func DecodeMoney(r game.MemoryReader, layout RAMLayout) uint32 {
	if r == nil {
		return 0
	}
	var money uint32
	for i := uint16(0); i < 3; i++ {
		b := r.Peek8(layout.PlayerMoney + i)
		money = money*100 + uint32(b>>4)*10 + uint32(b&0x0f)
	}
	return money
}

func DecodeBag(r game.MemoryReader, layout RAMLayout) []BagItem {
	if r == nil {
		return []BagItem{}
	}
	count := int(r.Peek8(layout.NumBagItems))
	if count == 0xff {
		count = 0
	}
	if count > BagCapacity {
		count = BagCapacity
	}
	out := make([]BagItem, count)
	for i := 0; i < count; i++ {
		at := layout.BagItems + uint16(i)*2
		out[i] = BagItem{ID: r.Peek8(at), Quantity: r.Peek8(at + 1)}
	}
	return out
}

var badgeNames = [...]string{
	"Boulder", "Cascade", "Thunder", "Rainbow",
	"Soul", "Marsh", "Volcano", "Earth",
}

func DecodeBadges(r game.MemoryReader, layout RAMLayout) []string {
	if r == nil {
		return []string{}
	}
	raw := r.Peek8(layout.ObtainedBadges)
	out := make([]string, 0, 8)
	for i, name := range badgeNames {
		if raw&(1<<uint8(i)) != 0 {
			out = append(out, name)
		}
	}
	return out
}
