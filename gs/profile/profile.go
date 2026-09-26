// Package profile implements the shared Pokémon Gold/Silver adapter for the
// generic game profile contract. Revision-specific RAM addresses and raw ids
// stay under gs; planner-facing state is semantic.
package profile

import (
	"fmt"
	"math/bits"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	GoldGameID   game.GameID = "pokemon-gold"
	SilverGameID game.GameID = "pokemon-silver"
	Revision     game.RevisionID = "en-us-eu-rev0"

	ProgressJohtoBadges game.ProgressID = "johto-badges"
	ProgressKantoBadges game.ProgressID = "kanto-badges"
)

type Profile struct {
	id   game.GameID
	sha1 string
}

func New(ids ...game.GameID) *Profile {
	id := GoldGameID
	if len(ids) > 0 {
		id = ids[0]
	}
	switch id {
	case SilverGameID:
		return &Profile{id: SilverGameID, sha1: sym.SilverSHA1}
	case GoldGameID:
		return &Profile{id: GoldGameID, sha1: sym.GoldSHA1}
	default:
		return &Profile{id: id}
	}
}

func NewGold() *Profile   { return New(GoldGameID) }
func NewSilver() *Profile { return New(SilverGameID) }

func (p *Profile) ID() game.GameID           { return p.id }
func (*Profile) Revision() game.RevisionID   { return Revision }
func (p *Profile) Detect(info game.ROMInfo) bool {
	return p.sha1 != "" && info.SHA1 == p.sha1 && info.Size == sym.ROMSize
}
func (*Profile) ROMParser() game.ROMParser { return parser{} }

func (*Profile) Symbols() game.SymbolTable {
	bank := sym.WRAMBank
	return game.SymbolTable{
		"player.map":       {Name: "player.map", Address: sym.MapGroup, Bank: bank, Width: 2},
		"player.x":         {Name: "player.x", Address: sym.XCoord, Bank: bank, Width: 1},
		"player.y":         {Name: "player.y", Address: sym.YCoord, Bank: bank, Width: 1},
		"player.direction": {Name: "player.direction", Address: sym.PlayerDirection, Bank: bank, Width: 1},
		"party.count":      {Name: "party.count", Address: sym.PartyCount, Bank: bank, Width: 1},
		"party.members":    {Name: "party.members", Address: sym.PartyMon1, Bank: bank, Width: int(sym.PartyMonSize) * 6},
		"battle.mode":      {Name: "battle.mode", Address: sym.BattleMode, Bank: bank, Width: 1},
		"badges":           {Name: "badges", Address: sym.JohtoBadges, Bank: bank, Width: 2},
		"bag":              {Name: "bag", Address: sym.TMsHMs, Bank: bank, Width: int(sym.Balls + 1 + 2*sym.MaxBalls - sym.TMsHMs)},
		"money":            {Name: "money", Address: sym.Money, Bank: bank, Width: 3},
	}
}

func (*Profile) Features() game.ProfileFeatures {
	return game.ProfileFeatures{
		game.FeatureInventory:       true,
		game.FeatureStoryProgress:   true,
		game.FeatureBankedMemory:    true,
		game.FeatureSemanticSpecies: true,
	}
}

type parser struct{}

func (parser) MapName(raw uint16) (string, bool) {
	m, ok := gsdata.Map(raw)
	if !ok {
		return "", false
	}
	return m.Name, true
}

func (parser) Species(raw uint16) (game.SpeciesID, bool) {
	if raw > 0xff {
		return "", false
	}
	return gsdata.Species(uint8(raw))
}

var johtoBadgeNames = [...]string{
	"zephyr", "hive", "plain", "fog", "storm", "mineral", "glacier", "rising",
}
var kantoBadgeNames = [...]string{
	"boulder", "cascade", "thunder", "rainbow", "soul", "marsh", "volcano", "earth",
}

func (p *Profile) DecodeObservation(reader game.MemoryReader, romData []byte) (game.ProfileObservation, error) {
	_ = romData
	if reader == nil {
		return game.ProfileObservation{}, fmt.Errorf("gs profile: nil memory reader")
	}
	symbols := p.Symbols()
	read := func(name string, n int) ([]byte, error) {
		s, ok := symbols.Lookup(name)
		if !ok {
			return nil, fmt.Errorf("gs profile: missing symbol %q", name)
		}
		if s.Bank != 0 && s.Bank != sym.WRAMBank {
			return nil, fmt.Errorf("gs profile: unsupported WRAM bank %d for %s", s.Bank, name)
		}
		if n <= 0 {
			n = s.Width
		}
		out := make([]byte, n)
		reader.PeekInto(s.Address, out)
		return out, nil
	}

	mapBytes, err := read("player.map", 2)
	if err != nil {
		return game.ProfileObservation{}, err
	}
	nativeMap := gsdata.NativeMapID(mapBytes[0], mapBytes[1])
	mapInfo, _ := gsdata.Map(nativeMap)

	partyCount := int(reader.Peek8(sym.PartyCount))
	if partyCount < 0 || partyCount > 6 {
		return game.ProfileObservation{}, fmt.Errorf("gs profile: party count %d outside 0..6", partyCount)
	}

	moneyBytes, err := read("money", 3)
	if err != nil {
		return game.ProfileObservation{}, err
	}
	money, err := decodeBCD(moneyBytes)
	if err != nil {
		return game.ProfileObservation{}, fmt.Errorf("gs profile: money: %w", err)
	}

	obs := game.ProfileObservation{
		NativeMapID: nativeMap,
		Location:    mapInfo.Location,
		MapName:     mapInfo.Name,
		X:           reader.Peek8(sym.XCoord),
		Y:           reader.Peek8(sym.YCoord),
		Facing:      decodeFacing(reader.Peek8(sym.PlayerDirection)),
		// Phase 2 has no proven Gen-II overworld-control-state decoder yet.
		// Do not infer controllability from position or battle state.
		Controllable: false,
		InBattle:     reader.Peek8(sym.BattleMode) != 0,
		Party:        make([]game.ProfilePartyMon, partyCount),
		BagCapacity:  sym.MaxItems,
		Badges:       []string{},
		Money:        money,
		PokedexTotal: 251,
		Events:       []string{},
	}

	for i := 0; i < partyCount; i++ {
		base := sym.PartyMon1 + uint16(i)*sym.PartyMonSize
		raw := make([]byte, sym.PartyMonSize)
		reader.PeekInto(base, raw)
		rawSpecies := raw[0]
		species, ok := gsdata.Species(rawSpecies)
		if !ok {
			species = game.SpeciesID("unknown")
		}
		held, _ := gsdata.Item(raw[1])
		obs.Party[i] = game.ProfilePartyMon{
			Species:    species,
			HeldItem:   held,
			IsEgg:      gsdata.IsEgg(rawSpecies),
			Level:      raw[0x1f],
			Experience: uint32(raw[0x08])<<16 | uint32(raw[0x09])<<8 | uint32(raw[0x0a]),
			HP:         uint16(raw[0x22])<<8 | uint16(raw[0x23]),
			MaxHP:      uint16(raw[0x24])<<8 | uint16(raw[0x25]),
			Status:     decodeStatus(raw[0x20]),
		}
	}

	johto := reader.Peek8(sym.JohtoBadges)
	kanto := reader.Peek8(sym.KantoBadges)
	appendBadges := func(mask byte, names []string) {
		for i, name := range names {
			if mask&(1<<i) != 0 {
				obs.Badges = append(obs.Badges, name)
			}
		}
	}
	appendBadges(johto, johtoBadgeNames[:])
	appendBadges(kanto, kantoBadgeNames[:])
	obs.Story = game.ProgressState{
		{ID: ProgressJohtoBadges, Complete: johto == 0xff, Value: bits.OnesCount8(johto)},
		{ID: ProgressKantoBadges, Complete: kanto == 0xff, Value: bits.OnesCount8(kanto)},
	}

	obs.Bag = decodeBag(reader)
	return obs, nil
}

func decodeBag(reader game.MemoryReader) []game.ProfileItem {
	out := []game.ProfileItem{}
	appendPairs := func(countAddr, dataAddr uint16, max int) {
		count := int(reader.Peek8(countAddr))
		if count > max {
			count = max
		}
		for i := 0; i < count; i++ {
			id := reader.Peek8(dataAddr + uint16(i*2))
			qty := int(reader.Peek8(dataAddr + uint16(i*2+1)))
			item, ok := gsdata.Item(id)
			if ok && qty > 0 {
				out = append(out, game.ProfileItem{ID: item, Name: string(item), Quantity: qty})
			}
		}
	}
	appendPairs(sym.NumItems, sym.Items, sym.MaxItems)
	appendPairs(sym.NumBalls, sym.Balls, sym.MaxBalls)

	keyCount := int(reader.Peek8(sym.NumKeyItems))
	if keyCount > sym.MaxKeyItems {
		keyCount = sym.MaxKeyItems
	}
	for i := 0; i < keyCount; i++ {
		item, ok := gsdata.Item(reader.Peek8(sym.KeyItems + uint16(i)))
		if ok {
			out = append(out, game.ProfileItem{ID: item, Name: string(item), Quantity: 1})
		}
	}

	for i, rawID := range gsdata.MachineItems {
		if i >= sym.TMsHMsCount {
			break
		}
		qty := int(reader.Peek8(sym.TMsHMs + uint16(i)))
		if qty == 0 {
			continue
		}
		item, ok := gsdata.Item(rawID)
		if ok {
			out = append(out, game.ProfileItem{ID: item, Name: string(item), Quantity: qty})
		}
	}
	return out
}

func decodeFacing(raw byte) string {
	switch raw & 0x0c {
	case 0x00:
		return "down"
	case 0x04:
		return "up"
	case 0x08:
		return "left"
	case 0x0c:
		return "right"
	default:
		return "unknown"
	}
}

func decodeStatus(raw byte) string {
	switch {
	case raw&0x07 != 0:
		return "sleep"
	case raw&0x08 != 0:
		return "poison"
	case raw&0x10 != 0:
		return "burn"
	case raw&0x20 != 0:
		return "freeze"
	case raw&0x40 != 0:
		return "paralysis"
	default:
		return ""
	}
}

func decodeBCD(raw []byte) (uint32, error) {
	var out uint32
	for _, b := range raw {
		hi, lo := b>>4, b&0x0f
		if hi > 9 || lo > 9 {
			return 0, fmt.Errorf("invalid packed BCD byte %#02x", b)
		}
		out = out*100 + uint32(hi)*10 + uint32(lo)
	}
	return out, nil
}
