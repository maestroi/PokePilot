package trade

import (
	"bytes"
	"testing"
)

func TestTrainerLinkDataRoundTripRestoresNoDataBytes(t *testing.T) {
	mon := testMon(0x26)
	mon.Raw[10] = NoDataByte
	mon.Raw[260%PartyMonSize] = NoDataByte
	trainer := NewTrainer("POKEPILOT", mon)
	block, patch, err := trainer.LinkData()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(block[partyMonsOffset:partyMonsOffset+PartyMonSize], []byte{NoDataByte}) {
		t.Fatal("patched wire block still contains 0xfe")
	}
	decoded, err := ParseTrainerBlock(block, patch[:])
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Party) != 1 {
		t.Fatalf("party len = %d, want 1", len(decoded.Party))
	}
	if decoded.Party[0] != mon {
		t.Fatalf("round trip changed mon\ngot  %#v\nwant %#v", decoded.Party[0], mon)
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
	mustExchange(t, m, PreambleByte)
	mustExchange(t, m, 0x12)
	mustExchange(t, m, PreambleByte)

	trainerWire := append([]byte{PreambleByte}, block[:]...)
	for _, b := range trainerWire {
		mustExchange(t, m, b)
	}
	patchWire := append([]byte{PreambleByte}, patch[:]...)
	for _, b := range patchWire {
		mustExchange(t, m, b)
	}

	mustExchange(t, m, firstPartyChoice)
	mustExchange(t, m, firstPartyChoice)
	mustExchange(t, m, 0)
	mustExchange(t, m, 0x62)

	if got := m.trainer.Party[0]; got != remote {
		t.Fatalf("stored tradeback mon changed bytes\ngot  %#v\nwant %#v", got, remote)
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
