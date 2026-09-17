package emu

import (
	"time"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/maestroi/gomeboy/pkg/link"
)

// NetworkLink is PokePilot's game-agnostic handle for one network-backed Game
// Boy serial connection. Pokemon-specific protocol logic belongs above emu.
type NetworkLink struct {
	inner *gomeboy.NetworkLink
}

// ConnectBrokerLink joins a named GomeBoy broker session and attaches it to
// this already-loaded emulator. A caller that resets the underlying emulator
// must establish/attach a link again because Reset rebuilds the serial device.
func (m *Emu) ConnectBrokerLink(address, session, peerID, role string, metadata map[string]string, timeout time.Duration) (*NetworkLink, error) {
	inner, err := m.e.ConnectBrokerLink(address, session, peerID, role, metadata, timeout)
	if err != nil {
		return nil, err
	}
	return &NetworkLink{inner: inner}, nil
}

func (l *NetworkLink) PublishMetadata(metadata map[string]string) error {
	if l == nil || l.inner == nil {
		return nil
	}
	return l.inner.PublishMetadata(metadata)
}

func (l *NetworkLink) Metadata() <-chan link.Message {
	if l == nil || l.inner == nil {
		return nil
	}
	return l.inner.Metadata()
}

func (l *NetworkLink) Close() error {
	if l == nil || l.inner == nil {
		return nil
	}
	return l.inner.Close()
}
