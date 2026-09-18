package gen1

import "testing"

type fakeRAM [0x10000]byte

func (m *fakeRAM) Peek8(addr uint16) byte { return m[int(addr)] }
func (m *fakeRAM) PeekInto(addr uint16, dst []byte) {
	copy(dst, m[int(addr):])
}

func TestSharedRAMDecodersUseSuppliedLayout(t *testing.T) {
	const (
		partyCount = 0xd000
		partyMon1  = 0xd100
		bagCount   = 0xd300
		bagItems   = 0xd301
		money      = 0xd400
		badges     = 0xd500
	)
	layout := RAMLayout{
		PartyCount: partyCount, PartyMon1: partyMon1, PartyMonSize: 0x2c,
		NumBagItems: bagCount, BagItems: bagItems, PlayerMoney: money, ObtainedBadges: badges,
	}
	var mem fakeRAM
	mem[partyCount] = 1
	mem[partyMon1+monSpecies] = 0x54
	mem[partyMon1+monHP], mem[partyMon1+monHP+1] = 0x00, 0x20
	mem[partyMon1+monStatus] = 1 << 3
	mem[partyMon1+monExp], mem[partyMon1+monExp+1], mem[partyMon1+monExp+2] = 1, 2, 3
	mem[partyMon1+monLevel] = 10
	mem[partyMon1+monMaxHP], mem[partyMon1+monMaxHP+1] = 0x00, 0x2a

	mem[money], mem[money+1], mem[money+2] = 0x12, 0x34, 0x56
	mem[bagCount] = 2
	mem[bagItems], mem[bagItems+1] = 0x14, 3
	mem[bagItems+2], mem[bagItems+3] = 0x31, 1
	mem[badges] = 0b10000001

	party := DecodeParty(&mem, layout)
	if len(party) != 1 || party[0].Species != "pikachu" || party[0].Experience != 0x010203 {
		t.Fatalf("party = %+v", party)
	}
	if party[0].HP != 0x20 || party[0].MaxHP != 0x2a || party[0].Status != "poisoned" {
		t.Fatalf("party mon = %+v", party[0])
	}
	if got := DecodeMoney(&mem, layout); got != 123456 {
		t.Fatalf("money = %d, want 123456", got)
	}
	items := DecodeBag(&mem, layout)
	if len(items) != 2 || items[0].ID != 0x14 || items[0].Quantity != 3 || items[1].ID != 0x31 {
		t.Fatalf("bag = %+v", items)
	}
	gotBadges := DecodeBadges(&mem, layout)
	if len(gotBadges) != 2 || gotBadges[0] != "Boulder" || gotBadges[1] != "Earth" {
		t.Fatalf("badges = %v", gotBadges)
	}
}
