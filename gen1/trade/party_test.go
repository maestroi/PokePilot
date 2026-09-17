package trade

import (
	"bytes"
	"testing"
)

func TestTrainerLinkDataRoundTripRestoresNoDataBytes(t *testing.T) {
	party := []Mon{
		testMon(0x24), testMon(0x25), testMon(0x26),
		testMon(0x27), testMon(0x28), testMon(0x29),
	}
	party[0].Raw[10] = NoDataByte
	// Slot six starts after the 252-byte first patch-list segment, so this
	// exercises the second segment rather than only the common first one.
	party[5].Raw[5] = NoDataByte
	trainer := NewTrainer("POKEPILOT", party...)
	block, patch, err := trainer.LinkData()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(block[partyMonsOffset:partyMonsOffset+PartyLength*PartyMonSize], []byte{NoDataByte}) {
		t.Fatal("patched wire block still contains 0xfe")
	}
	decoded, err := ParseTrainerBlock(block, patch[:])
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Party) != len(party) {
		t.Fatalf("party len = %d, want %d", len(decoded.Party), len(party))
	}
	for i := range party {
		if decoded.Party[i] != party[i] {
			t.Fatalf("round trip changed slot %d\ngot  %#v\nwant %#v", i, decoded.Party[i], party[i])
		}
	}
}

func TestMachineStoresExactRemoteMonForTradeback(t *testing.T) {
	local := testMon(0x24)
	remote := testMon(0x26)
	remote.Raw[12] = NoDataByte
	remote.OT = EncodeText("PLAYER")
	remote.Nick = EncodeText("KADABRA")

	m, err := NewMachine(MachineConfig{Trainer: NewTrainer("POKEPILOT", local), Tradeback: true})
	if err != nil {
		t.Fatal(err)
	}
	remoteTrainer := NewTrainer("RED", remote)
	block, patch, err := remoteTrainer.LinkData()
	if err != nil {
		t.Fatal(err)
	}

	mustExchange(t, m, masterMagic)
	mustExchange(t, m, connectedMagic)
	mustExchange(t, m, selectTradeMagic)
	// CableClub_DoBattleOrTrade first performs nibble/zero synchronization.
	// The first 0xfd starts the random block, its first non-0xfd byte proves
	// random synchronization, and the next 0xfd starts trainer data.
	mustExchange(t, m, 0x60)
	mustExchange(t, m, 0)
	mustExchange(t, m, PreambleByte)
	mustExchange(t, m, PreambleByte)
	mustExchange(t, m, 0x12)
	mustExchange(t, m, 0x34)
	mustExchange(t, m, PreambleByte)

	for _, b := range block {
		mustExchange(t, m, b)
	}
	// The patch list is a separate Serial_ExchangeBytes call and therefore
	// has its own synchronization preamble before the 200 stored bytes.
	mustExchange(t, m, PreambleByte)
	for _, b := range patch {
		mustExchange(t, m, b)
	}

	mustExchange(t, m, firstPartyChoice)
	mustExchange(t, m, firstPartyChoice)
	mustExchange(t, m, 0)
	mustExchange(t, m, 0x62)

	if got := m.trainer.Party[0]; got != remote {
		t.Fatalf("stored tradeback mon changed bytes\ngot  %#v\nwant %#v", got, remote)
	}
	if m.state != stateSelectedTrade {
		t.Fatalf("after completed trade state=%d, want next-trade data exchange", m.state)
	}
}

func TestSelectedTradeDoesNotMistakeSyncZerosForRandomData(t *testing.T) {
	m, err := NewMachine(MachineConfig{Trainer: NewTrainer("PEER", testMon(0x24))})
	if err != nil {
		t.Fatal(err)
	}
	mustExchange(t, m, masterMagic)
	mustExchange(t, m, connectedMagic)
	mustExchange(t, m, selectTradeMagic)
	for i := 0; i < 12; i++ {
		mustExchange(t, m, 0)
	}
	if m.state != stateSelectedTrade {
		t.Fatalf("sync zeros advanced state to %d", m.state)
	}
	mustExchange(t, m, PreambleByte)
	if m.state != stateWaitingRandomSeed {
		t.Fatalf("random preamble state=%d", m.state)
	}
}

func TestBitBridgeShiftsMSBFirst(t *testing.T) {
	m, err := NewMachine(MachineConfig{Trainer: NewTrainer("PEER", testMon(0x24))})
	if err != nil {
		t.Fatal(err)
	}
	bridge := NewBitBridge(m)
	var got byte
	for bit := 7; bit >= 0; bit-- {
		in := masterMagic&(1<<uint(bit)) != 0
		out, err := bridge.ExchangeBit(in)
		if err != nil {
			t.Fatal(err)
		}
		got <<= 1
		if out {
			got |= 1
		}
	}
	if got != slaveMagic {
		t.Fatalf("first shifted response = %#02x, want %#02x", got, slaveMagic)
	}
}

func mustExchange(t *testing.T, m *Machine, in byte) byte {
	t.Helper()
	out, err := m.ExchangeByte(in)
	if err != nil {
		t.Fatalf("exchange %#02x: %v", in, err)
	}
	return out
}

func testMon(species byte) Mon {
	var mon Mon
	mon.Raw[0] = species
	mon.Raw[1] = 0
	mon.Raw[2] = 20
	mon.Raw[3] = 5
	mon.Raw[33] = 5
	mon.Raw[34] = 0
	mon.Raw[35] = 20
	mon.Raw[36] = 0
	mon.Raw[37] = 10
	mon.Raw[38] = 0
	mon.Raw[39] = 10
	mon.Raw[40] = 0
	mon.Raw[41] = 10
	mon.Raw[42] = 0
	mon.Raw[43] = 10
	mon.OT = EncodeText("OT")
	mon.Nick = EncodeText("MON")
	return mon
}
