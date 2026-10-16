package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/gs/sym"
)

type pokegearFixtureMemory map[uint16]byte

func (m pokegearFixtureMemory) Peek8(addr uint16) byte { return m[addr] }
func (m pokegearFixtureMemory) PeekInto(addr uint16, dst []byte) {
	for i := range dst {
		dst[i] = m[addr+uint16(i)]
	}
}

func TestGSWorldOpportunitiesAppearAsPlannerPrerequisitesAndRematches(t *testing.T) {
	mem := pokegearFixtureMemory{
		sym.PokegearFlags: 0x04, // phone, but no radio or expansion card
		sym.PhoneList: 15, // Joey is registered
	}
	mem[sym.EventFlags+628/8] |= 1 << uint(628%8) // Joey ready to rematch
	var obs Observation
	applyWorldOpportunities(&obs, gsprofile.NewGold().DecodeWorldOpportunities(mem))

	if len(obs.RouteBlockages) != 1 || len(obs.RouteBlockages[0].Missing) != 2 {
		t.Fatalf("Kanto gate must remain blocked without radio and expansion: %+v", obs.RouteBlockages)
	}
	if len(obs.PhoneContacts) != 1 || obs.PhoneContacts[0].ID != "joey" ||
		len(obs.BattleSources) != 1 || !obs.BattleSources[0].Ready ||
		obs.BattleSources[0].Kind != game.BattleSourcePhoneRematch {
		t.Fatalf("registered rematch not visible: contact=%+v sources=%+v", obs.PhoneContacts, obs.BattleSources)
	}
	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"phone_contacts"`, `"battle_sources"`, `"kind":"phone_rematch"`, `"ready":true`, `"route_gates"`, `"route_blockages"`, `"pokegear_radio_card"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("planner view %s missing %s", data, want)
		}
	}
	// The planner does not infer radio ownership from the mere existence of
	// the phone card; the actual ownership bit must appear.
	if strings.Contains(string(data), `"capabilities"`) {
		t.Fatalf("invented radio capability without ownership: %s", data)
	}

	mem[sym.PokegearFlags] |= 0x0a // both radio and expansion bits
	var unlocked Observation
	applyWorldOpportunities(&unlocked, gsprofile.NewSilver().DecodeWorldOpportunities(mem))
	if len(unlocked.RouteBlockages) != 0 || len(unlocked.RouteGates) != 1 || !unlocked.RouteGates[0].Available {
		t.Fatalf("unlocked gate still blocked: %+v", unlocked)
	}
	if len(unlocked.Capabilities) != 2 {
		t.Fatalf("radio and expansion ownership not published: %+v", unlocked.Capabilities)
	}
}

func TestClocklessRedShapeDoesNotInventPokegearFacts(t *testing.T) {
	data, err := json.Marshal(Observation{GameID: "pokemon-red", Location: "pallet-town"})
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{`"phone_contacts"`, `"battle_sources"`, `"route_gates"`, `"capabilities"`} {
		if strings.Contains(string(data), absent) {
			t.Fatalf("Red observation grew an unsupported phone or radio field: %s", data)
		}
	}
}
