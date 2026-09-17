package trade

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/maestroi/gomeboy/pkg/link"
)

func TestRunBrokerRepliesToClockBits(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	brokerDone := make(chan error, 1)
	go func() { brokerDone <- link.NewBroker().Serve(listener) }()

	machine, err := NewMachine(MachineConfig{Trainer: NewTrainer("PEER", testMon(0x24))})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	peerDone := make(chan error, 1)
	go func() {
		peerDone <- RunBroker(ctx, BrokerConfig{
			Address: listener.Addr().String(), Session: "test", PeerID: "virtual", Timeout: time.Second,
		}, machine)
	}()

	conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	enc := link.NewEncoder(conn)
	dec := link.NewDecoder(conn)
	if err := enc.Encode(link.Message{Type: link.MessageHello, Session: "test", PeerID: "rom", Role: "emulator"}); err != nil {
		t.Fatal(err)
	}
	ready, err := dec.Decode()
	if err != nil || ready.Type != link.MessageReady {
		t.Fatalf("ready = %#v, %v", ready, err)
	}

	var response byte
	for i := 0; i < 8; i++ {
		seq := uint64(i + 1)
		bit := masterMagic&(0x80>>uint(i)) != 0
		if err := enc.Encode(link.Message{Type: link.MessageClock, Sequence: seq, Bit: bit}); err != nil {
			t.Fatal(err)
		}
		msg, err := dec.Decode()
		if err != nil {
			t.Fatal(err)
		}
		for msg.Type == link.MessageMetadata {
			msg, err = dec.Decode()
			if err != nil {
				t.Fatal(err)
			}
		}
		if msg.Type != link.MessageClockReply || msg.Sequence != seq {
			t.Fatalf("clock reply %d = %#v", i, msg)
		}
		response <<= 1
		if msg.Bit {
			response |= 1
		}
	}
	if response != slaveMagic {
		t.Fatalf("response byte = %#02x, want %#02x", response, slaveMagic)
	}

	cancel()
	select {
	case err := <-peerDone:
		if err != nil && err != context.Canceled {
			t.Fatalf("peer exit: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("virtual peer did not stop after cancellation")
	}
}
