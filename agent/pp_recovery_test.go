package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestLeadOutOfPPRequiresHardExhaustion(t *testing.T) {
	if leadOutOfPP(Observation{}) {
		t.Fatal("empty PP observation should not be treated as exhausted")
	}
	if leadOutOfPP(Observation{LeadPP: []uint8{0, 3}}) {
		t.Fatal("one usable move should keep the lead out of hard exhaustion")
	}
	if !leadOutOfPP(Observation{LeadPP: []uint8{0, 0}}) {
		t.Fatal("all-zero PP should be treated as hard exhaustion")
	}
}

func TestOfferPPItemOnlyAtHardExhaustion(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{})
	base := Observation{
		Map:        0xfe,
		MapName:    "ROUTE_TEST",
		PartyCount: 1,
		Party:      []PartyMon{{HP: 20, MaxHP: 20}},
		Bag:        []Item{{Name: "ether", Quantity: 1}},
	}

	usable := base
	usable.LeadPP = []uint8{0, 2}
	for _, o := range Offer(usable, known) {
		if o.Kind == KindUseItem && o.Item == 0x50 {
			t.Fatalf("ether offered before hard exhaustion: %+v", o)
		}
	}

	exhausted := base
	exhausted.LeadPP = []uint8{0, 0}
	found := false
	for _, o := range Offer(exhausted, known) {
		if o.Kind == KindUseItem && o.Item == 0x50 && o.Slot == 0 {
			found = true
			if !strings.Contains(o.Note, "finite PP recovery") {
				t.Fatalf("ether objective missing finite-resource note: %+v", o)
			}
		}
	}
	if !found {
		t.Fatal("ether was not offered at hard PP exhaustion")
	}
}

func TestOfferPrefersFreeCenterWhenAlreadyThere(t *testing.T) {
	obs := Observation{
		Map:        0xfe,
		MapName:    "VIRIDIAN_POKECENTER",
		PartyCount: 1,
		Party:      []PartyMon{{HP: 20, MaxHP: 20}},
		LeadPP:     []uint8{0, 0},
		Bag:        []Item{{Name: "ether", Quantity: 1}},
	}
	foundHeal := false
	for _, o := range Offer(obs, NewKnowledge(map[uint8][]uint8{})) {
		if o.Kind == KindUseItem && o.Item == 0x50 {
			t.Fatalf("finite ether offered while already in a Center: %+v", o)
		}
		if o.Kind == KindHeal {
			foundHeal = true
			if !strings.Contains(o.Note, "without spending finite items") {
				t.Fatalf("Center heal missing PP preservation note: %+v", o)
			}
		}
	}
	if !foundHeal {
		t.Fatal("Center heal not offered for PP exhaustion")
	}
}

func TestOfferKnownCenterForHealthyPPExhaustedParty(t *testing.T) {
	center, ok := skill.Place("viridian pokemon center")
	if !ok {
		t.Fatal("viridian pokemon center place missing")
	}
	fieldMap := uint8(0xfe)
	if fieldMap == center.Map {
		fieldMap = 0xfd
	}
	known := NewKnowledge(map[uint8][]uint8{
		fieldMap:   []uint8{center.Map},
		center.Map: []uint8{fieldMap},
	})
	known.Visited[center.Map] = true
	obs := Observation{
		Map:        fieldMap,
		MapName:    "ROUTE_TEST",
		PartyCount: 1,
		Party:      []PartyMon{{HP: 20, MaxHP: 20}},
		LeadPP:     []uint8{0, 0},
	}
	found := false
	for _, o := range Offer(obs, known) {
		if o.Kind == KindHeal && o.Place != "" {
			found = true
			if !strings.Contains(o.Note, "lead has no PP") {
				t.Fatalf("PP recovery heal missing reason: %+v", o)
			}
		}
	}
	if !found {
		t.Fatal("healthy PP-exhausted party was not offered a known Center recovery")
	}
}

func TestPPRecoveryDueWithholdsTrainAndGym(t *testing.T) {
	known := NewKnowledge(nil)
	candidates := []Objective{
		{Kind: KindHeal, Note: "(lead has no PP; Center restores PP without spending finite items)"},
		{Kind: KindTrain, Level: 24},
		{Kind: KindGym, Place: "pewter gym"},
		{Kind: KindTalk, X: 1, Y: 2},
	}
	got := filterTrainerLossBlocked(candidates, known)
	for _, o := range got {
		if o.Kind == KindTrain || o.Kind == KindGym {
			t.Fatalf("combat objective survived PP-recovery gate: %+v (all=%+v)", o, got)
		}
	}
	if !ppRecoveryDue(got) {
		t.Fatalf("PP recovery option disappeared with combat objectives: %+v", got)
	}
}

func TestFinitePPRecoveryAlsoWithholdsCombat(t *testing.T) {
	known := NewKnowledge(nil)
	candidates := []Objective{
		{Kind: KindUseItem, Item: ppRestoreItems["ether"], Slot: 0, Note: "(finite PP recovery)"},
		{Kind: KindTrain, Level: 24},
		{Kind: KindGym, Place: "pewter gym"},
	}
	got := filterTrainerLossBlocked(candidates, known)
	if len(got) != 1 || got[0].Kind != KindUseItem {
		t.Fatalf("finite PP recovery should be the only surviving candidate, got %+v", got)
	}
}

func TestPPItemsAreExecutableVocabulary(t *testing.T) {
	for name, want := range map[string]uint8{
		"ether": 0x50, "max ether": 0x51, "elixer": 0x52, "max elixer": 0x53,
	} {
		if got, ok := ItemByName(name); !ok || got != want {
			t.Fatalf("ItemByName(%q) = %#02x,%v; want %#02x,true", name, got, ok, want)
		}
	}
}
