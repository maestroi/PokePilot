package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

// Gold/Silver wPokegearFlags from pokegold/ram/wram.asm. Map=bit0,
// Radio=bit1, Phone=bit2, and the Kanto Expansion Card=bit3.
const (
	gsPokegearRadioCardMask     byte = 1 << 1
	gsPokegearExpansionCardMask byte = 1 << 3
	gsPokegearPhoneCardMask     byte = 1 << 2

	gsCapRadioCard     game.CapabilityID = "pokegear_radio_card"
	gsCapExpansionCard game.CapabilityID = "pokegear_expansion_card"
	gsSnorlaxGateID    string = "gs:vermilion_snorlax"
	gsEventFoughtSnorlax uint16 = 1872
)

// Phone list indexes from pokegold/constants/phone_constants.asm and
// ready-for-rematch flags from constants/event_flags.asm at SourceRevision.
// These are cartridge-owned facts, not generic planner script names.
type gsRematchContact struct {
	Index byte
	ID string
	Name string
	MapName string
	ReadyEvent uint16
}

var gsRematchContacts = [...]gsRematchContact{
	{5, "jack", "Jack", "NATIONAL_PARK", 608},
	{6, "beverly", "Beverly", "NATIONAL_PARK", 610},
	{7, "huey", "Huey", "OLIVINE_LIGHTHOUSE_2F", 612},
	{11, "gaven", "Gaven", "ROUTE_26", 620},
	{12, "beth", "Beth", "ROUTE_26", 622},
	{13, "jose", "Jose", "ROUTE_27", 624},
	{14, "reena", "Reena", "ROUTE_27", 626},
	{15, "joey", "Joey", "ROUTE_30", 628},
	{16, "wade", "Wade", "ROUTE_31", 630},
	{17, "ralph", "Ralph", "ROUTE_32", 632},
	{18, "liz", "Liz", "ROUTE_32", 634},
	{19, "anthony", "Anthony", "ROUTE_33", 636},
	{20, "todd", "Todd", "ROUTE_34", 638},
	{21, "gina", "Gina", "ROUTE_34", 640},
	{22, "irwin", "Irwin", "ROUTE_35", 642},
	{23, "arnie", "Arnie", "ROUTE_35", 644},
	{24, "alan", "Alan", "ROUTE_36", 646},
	{26, "dana", "Dana", "ROUTE_38", 650},
	{27, "chad", "Chad", "ROUTE_38", 652},
	{28, "derek", "Derek", "ROUTE_39", 654},
	{29, "chris", "Chris", "ROUTE_42", 656},
	{30, "brent", "Brent", "ROUTE_43", 658},
	{31, "tiffany", "Tiffany", "ROUTE_43", 660},
	{32, "vance", "Vance", "ROUTE_44", 662},
	{33, "wilton", "Wilton", "ROUTE_44", 664},
	{34, "kenji", "Kenji", "ROUTE_45", 666},
	{35, "parry", "Parry", "ROUTE_45", 668},
	{36, "erin", "Erin", "ROUTE_46", 670},
}

func gsPhoneRegistered(reader game.MemoryReader, index byte) bool {
	for i := uint16(0); i < sym.PhoneListSize; i++ {
		if reader.Peek8(sym.PhoneList+i) == index {
			return true
		}
	}
	return false
}

func gsLocation(name string) game.PlaceID {
	m, ok := gsdata.MapByName(name)
	if !ok {
		return ""
	}
	return m.Location
}

// DecodeWorldOpportunities projects radio-card ownership and registered phone
// contacts into shared vocabulary. A trainer rematch is Ready only if the
// number is registered AND its cartridge event has been raised by a call.
// The Snorlax route gate needs both Radio and Kanto Expansion cards; the
// battle script additionally requires tuning the Poké Flute station while
// standing next to Snorlax, which execution must perform separately.
func (*Profile) DecodeWorldOpportunities(reader game.MemoryReader) game.WorldOpportunities {
	if reader == nil {
		return game.WorldOpportunities{}
	}
	var out game.WorldOpportunities
	flags := reader.Peek8(sym.PokegearFlags)
	caps := game.NewCapabilitySet()
	if flags&gsPokegearRadioCardMask != 0 {
		out.Capabilities = append(out.Capabilities, gsCapRadioCard)
		caps[gsCapRadioCard] = true
	}
	if flags&gsPokegearExpansionCardMask != 0 {
		out.Capabilities = append(out.Capabilities, gsCapExpansionCard)
		caps[gsCapExpansionCard] = true
	}
	if !hasGSEvent(reader, gsEventFoughtSnorlax) {
		from, to := gsLocation("VERMILION_CITY"), gsLocation("DIGLETTS_CAVE")
		if from != "" && to != "" {
			out.RouteGates = append(out.RouteGates, game.AssessRouteGate(game.Transition{
				ID: gsSnorlaxGateID,
				From: from, To: to,
				Requires: []game.CapabilityID{gsCapRadioCard, gsCapExpansionCard},
				Gate: true, ChoiceRequired: true,
			}, caps))
		}
	}
	if flags&gsPokegearPhoneCardMask == 0 {
		return out
	}
	for _, contact := range gsRematchContacts {
		if !gsPhoneRegistered(reader, contact.Index) {
			continue
		}
		location := gsLocation(contact.MapName)
		out.PhoneContacts = append(out.PhoneContacts, game.PhoneContact{
			ID: contact.ID, Name: contact.Name, Location: location,
		})
		out.BattleSources = append(out.BattleSources, game.BattleSource{
			ID: "phone_rematch:" + contact.ID,
			Kind: game.BattleSourcePhoneRematch,
			Trainer: contact.Name,
			Location: location,
			Ready: hasGSEvent(reader, contact.ReadyEvent),
		})
	}
	return out
}

var _ game.WorldOpportunityDecoder = (*Profile)(nil)
