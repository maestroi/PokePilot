package trade

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/maestroi/gomeboy/pkg/link"
)

const defaultBrokerTimeout = 10 * time.Second

type BrokerConfig struct {
	Address  string
	Session  string
	PeerID   string
	Metadata map[string]string
	Timeout  time.Duration
	// OnReady fires after the broker has accepted this peer. Service callers
	// use it to avoid advertising a session before an emulator can safely join
	// and begin clocking serial traffic.
	OnReady func()
}

// RunBroker joins one GomeBoy broker session as role=virtual and services
// clock pulses until the context is cancelled or the peer disconnects.
func RunBroker(ctx context.Context, cfg BrokerConfig, machine *Machine) error {
	if machine == nil {
		return errors.New("gen1 trade: nil machine")
	}
	if cfg.Address == "" || cfg.Session == "" || cfg.PeerID == "" {
		return errors.New("gen1 trade: broker address, session and peer id are required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultBrokerTimeout
	}
	conn, err := net.DialTimeout("tcp", cfg.Address, timeout)
	if err != nil {
		return fmt.Errorf("gen1 trade: dial broker: %w", err)
	}
	defer conn.Close()

	enc := link.NewEncoder(conn)
	dec := link.NewDecoder(conn)
	if err := enc.Encode(link.Message{
		Type:     link.MessageHello,
		Session:  cfg.Session,
		PeerID:   cfg.PeerID,
		Role:     "virtual",
		Metadata: cloneMetadata(cfg.Metadata),
	}); err != nil {
		return fmt.Errorf("gen1 trade: broker hello: %w", err)
	}
	ready, err := dec.Decode()
	if err != nil {
		return fmt.Errorf("gen1 trade: broker ready: %w", err)
	}
	if ready.Type == link.MessageError {
		return fmt.Errorf("gen1 trade: broker rejected peer: %s", ready.Error)
	}
	if ready.Type != link.MessageReady {
		return fmt.Errorf("gen1 trade: expected broker ready, got %q", ready.Type)
	}

	if len(cfg.Metadata) != 0 {
		// Hello metadata is forwarded only when the other peer is already
		// present. Publishing again is harmless and covers that race.
		_ = enc.Encode(link.Message{Type: link.MessageMetadata, Metadata: cloneMetadata(cfg.Metadata)})
	}
	if cfg.OnReady != nil {
		cfg.OnReady()
	}

	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closed:
		}
	}()
	defer close(closed)

	bridge := NewBitBridge(machine)
	for {
		msg, err := dec.Decode()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("gen1 trade: broker receive: %w", err)
		}
		switch msg.Type {
		case link.MessageClock:
			bit, exchangeErr := bridge.ExchangeBit(msg.Bit)
			if err := enc.Encode(link.Message{Type: link.MessageClockReply, Sequence: msg.Sequence, Bit: bit}); err != nil {
				return fmt.Errorf("gen1 trade: broker clock reply: %w", err)
			}
			if exchangeErr != nil {
				return exchangeErr
			}
		case link.MessageMetadata:
			if len(cfg.Metadata) != 0 {
				_ = enc.Encode(link.Message{Type: link.MessageMetadata, Metadata: cloneMetadata(cfg.Metadata)})
			}
		case link.MessageError:
			return fmt.Errorf("gen1 trade: broker error: %s", msg.Error)
		}
	}
}

func cloneMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
