package trade

import (
	"errors"
	"fmt"
)

const (
	masterMagic       byte = 0x01
	slaveMagic        byte = 0x02
	connectedMagic    byte = 0x60
	selectTradeMagic  byte = 0xd4
	selectBattleMagic byte = 0xd5
	selectCancelMagic byte = 0xd6
	tradeMenuClosed   byte = 0x6f
	firstPartyChoice  byte = 0x60
	lastPartyChoice   byte = 0x65
	tradeCancelled    byte = 0x61
)

type machineState uint8

const (
	stateConnecting machineState = iota
	stateLinkMenu
	stateWaitingRandom
	stateSendingTrainer
	stateSendingPatch
	stateTradeMenu
	stateTradeInitiated
	stateTradeConfirmation
	stateTradeCancelled
)

// EventKind is a semantic transition observed by the virtual trade peer.
type EventKind string

const (
	EventConnected       EventKind = "connected"
	EventTradeCenter     EventKind = "trade_center"
	EventPartyReceived   EventKind = "party_received"
	EventTradeSelected   EventKind = "trade_selected"
	EventTradeCompleted  EventKind = "trade_completed"
	EventTradeCancelled  EventKind = "trade_cancelled"
	EventTradebackStored EventKind = "tradeback_stored"
)

// Event is intentionally generation-specific. Generic runtime code can record
// it as provenance without learning Gen-I serial bytes or party layouts.
type Event struct {
	Kind       EventKind
	RemoteSlot int
	LocalSlot  int
	Species    byte
}

type MachineConfig struct {
	Trainer   Trainer
	Tradeback bool
	OnEvent   func(Event)
}

// Machine implements the byte-level Pokemon Red/Blue Cable Club protocol.
// The network transport is bit-level; BitBridge below performs the serial
// shift-register adaptation and leaves this state machine easy to test.
type Machine struct {
	cfg     MachineConfig
	trainer Trainer
	state   machineState

	trainerWire []byte
	patchWire   []byte
	trainerPos  int
	patchPos    int

	sawRandomData bool
	remoteBlock   []byte
	remotePatch   []byte
	remoteTrainer *Trainer
	remoteSlot    int
}

func NewMachine(cfg MachineConfig) (*Machine, error) {
	m := &Machine{cfg: cfg, trainer: cloneTrainer(cfg.Trainer), remoteSlot: -1}
	if err := m.rebuildWireData(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Machine) InitialByte() byte { return slaveMagic }

// ExchangeByte consumes the byte just shifted out by the ROM and returns the
// byte to place in the virtual peer's shift register for the next exchange.
// Link-menu polling and transfer preambles are deliberately repeated by the
// games, which makes this one-byte pipeline equivalent to a physical slave.
func (m *Machine) ExchangeByte(in byte) (byte, error) {
	switch m.state {
	case stateConnecting:
		switch in {
		case masterMagic:
			return slaveMagic, nil
		case 0:
			return 0, nil
		case connectedMagic:
			m.state = stateLinkMenu
			m.emit(Event{Kind: EventConnected})
			return connectedMagic, nil
		default:
			return in, nil
		}

	case stateLinkMenu:
		switch in {
		case connectedMagic:
			return connectedMagic, nil
		case selectTradeMagic:
			m.state = stateWaitingRandom
			m.sawRandomData = false
			m.emit(Event{Kind: EventTradeCenter})
			return selectTradeMagic, nil
		case selectBattleMagic:
			return selectCancelMagic, errors.New("gen1 trade: virtual peer does not implement link battles")
		case selectCancelMagic, masterMagic:
			m.resetSession()
			return selectCancelMagic, nil
		default:
			return in, nil
		}

	case stateWaitingRandom:
		// The random-number transfer is safe to echo. Once non-preamble data
		// has flowed, the next 0xfd starts the trainer-data transfer.
		if in != PreambleByte {
			m.sawRandomData = true
		} else if m.sawRandomData {
			m.state = stateSendingTrainer
			m.trainerPos = 0
			m.remoteBlock = m.remoteBlock[:0]
		}
		return in, nil

	case stateSendingTrainer:
		m.remoteBlock = append(m.remoteBlock, in)
		if m.trainerPos >= len(m.trainerWire) {
			return PreambleByte, errors.New("gen1 trade: trainer stream exhausted unexpectedly")
		}
		out := m.trainerWire[m.trainerPos]
		m.trainerPos++
		if m.trainerPos == len(m.trainerWire) {
			m.state = stateSendingPatch
			m.patchPos = 0
			m.remotePatch = m.remotePatch[:0]
		}
		return out, nil

	case stateSendingPatch:
		m.remotePatch = append(m.remotePatch, in)
		if m.patchPos >= len(m.patchWire) {
			return PreambleByte, errors.New("gen1 trade: patch stream exhausted unexpectedly")
		}
		out := m.patchWire[m.patchPos]
		m.patchPos++
		if m.patchPos == len(m.patchWire) {
			m.state = stateTradeMenu
			if err := m.decodeRemoteTrainer(); err != nil {
				return out, err
			}
		}
		return out, nil

	case stateTradeMenu:
		switch {
		case in == tradeMenuClosed:
			m.state = stateWaitingRandom
			m.sawRandomData = false
			return tradeMenuClosed, nil
		case in >= firstPartyChoice && in <= lastPartyChoice:
			m.remoteSlot = int(in - firstPartyChoice)
			if m.remoteTrainer == nil || m.remoteSlot >= len(m.remoteTrainer.Party) {
				return firstPartyChoice, fmt.Errorf("gen1 trade: remote selected party slot %d but received party has %d mon(s)", m.remoteSlot, m.remotePartyLen())
			}
			m.state = stateTradeInitiated
			m.emit(Event{Kind: EventTradeSelected, RemoteSlot: m.remoteSlot, LocalSlot: 0, Species: m.remoteTrainer.Party[m.remoteSlot].Species()})
			return firstPartyChoice, nil
		default:
			return in, nil
		}

	case stateTradeInitiated:
		if in != 0 {
			return firstPartyChoice, nil
		}
		m.state = stateTradeConfirmation
		return 0, nil

	case stateTradeConfirmation:
		if in == tradeCancelled {
			m.state = stateTradeCancelled
			m.emit(Event{Kind: EventTradeCancelled, RemoteSlot: m.remoteSlot, LocalSlot: 0})
			return tradeCancelled, nil
		}
		if in != 0 {
			if err := m.completeTrade(); err != nil {
				return in, err
			}
			m.resetSession()
		}
		return in, nil

	case stateTradeCancelled:
		if in == 0 {
			m.state = stateTradeMenu
		}
		return in, nil
	}
	return NoDataByte, fmt.Errorf("gen1 trade: unknown machine state %d", m.state)
}

func (m *Machine) rebuildWireData() error {
	block, patch, err := m.trainer.LinkData()
	if err != nil {
		return err
	}
	// Serial_ExchangeBytes discards the first received preamble and repeats
	// its local byte. Prefixing one extra 0xfd yields exactly 424/200 stored
	// bytes after synchronization, matching the real two-Game-Boy exchange.
	m.trainerWire = make([]byte, 1, 1+len(block))
	m.trainerWire[0] = PreambleByte
	m.trainerWire = append(m.trainerWire, block[:]...)
	m.patchWire = make([]byte, 1, 1+len(patch))
	m.patchWire[0] = PreambleByte
	m.patchWire = append(m.patchWire, patch[:]...)
	return nil
}

func (m *Machine) decodeRemoteTrainer() error {
	block, err := findTrainerBlock(m.remoteBlock)
	if err != nil {
		return err
	}
	patch, err := findPatchList(m.remotePatch)
	if err != nil {
		return err
	}
	remote, err := ParseTrainerBlock(block, patch[:])
	if err != nil {
		return fmt.Errorf("gen1 trade: decode remote trainer: %w", err)
	}
	m.remoteTrainer = &remote
	m.emit(Event{Kind: EventPartyReceived})
	return nil
}

func (m *Machine) completeTrade() error {
	if m.remoteTrainer == nil || m.remoteSlot < 0 || m.remoteSlot >= len(m.remoteTrainer.Party) {
		return errors.New("gen1 trade: cannot complete trade without a valid remote selection")
	}
	if len(m.trainer.Party) == 0 {
		return errors.New("gen1 trade: virtual trainer has no party")
	}
	incoming := m.remoteTrainer.Party[m.remoteSlot]
	m.trainer.Party[0] = incoming // exact bytes: this is the trade-back store.
	if err := m.rebuildWireData(); err != nil {
		return err
	}
	m.emit(Event{Kind: EventTradeCompleted, RemoteSlot: m.remoteSlot, LocalSlot: 0, Species: incoming.Species()})
	if m.cfg.Tradeback {
		m.emit(Event{Kind: EventTradebackStored, RemoteSlot: m.remoteSlot, LocalSlot: 0, Species: incoming.Species()})
	}
	return nil
}

func (m *Machine) resetSession() {
	m.state = stateConnecting
	m.sawRandomData = false
	m.trainerPos = 0
	m.patchPos = 0
	m.remoteBlock = m.remoteBlock[:0]
	m.remotePatch = m.remotePatch[:0]
	m.remoteTrainer = nil
	m.remoteSlot = -1
}

func (m *Machine) remotePartyLen() int {
	if m.remoteTrainer == nil {
		return 0
	}
	return len(m.remoteTrainer.Party)
}

func (m *Machine) emit(event Event) {
	if m.cfg.OnEvent != nil {
		m.cfg.OnEvent(event)
	}
}

func findTrainerBlock(stream []byte) ([TrainerBlockSize]byte, error) {
	var zero [TrainerBlockSize]byte
	for start := 0; start+TrainerBlockSize <= len(stream); start++ {
		ok := true
		for i := 0; i < trainerPreambleLength; i++ {
			if stream[start+i] != PreambleByte {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		count := int(stream[start+partyCountOffset])
		if count < 1 || count > PartyLength {
			continue
		}
		for i := 0; i < count; i++ {
			if stream[start+partySpeciesOffset+i] == 0 || stream[start+partySpeciesOffset+i] == PatchTermByte {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		var block [TrainerBlockSize]byte
		copy(block[:], stream[start:start+TrainerBlockSize])
		return block, nil
	}
	return zero, fmt.Errorf("gen1 trade: could not locate %d-byte remote trainer block in %d received bytes", TrainerBlockSize, len(stream))
}

func findPatchList(stream []byte) ([PatchListSize]byte, error) {
	var zero [PatchListSize]byte
	for start := 0; start+PatchListSize <= len(stream); start++ {
		if stream[start] != PreambleByte || stream[start+1] != PreambleByte || stream[start+2] != PreambleByte {
			continue
		}
		var patch [PatchListSize]byte
		copy(patch[:], stream[start:start+PatchListSize])
		var dummy [TrainerBlockSize]byte
		if err := UnpatchTrainerBlock(&dummy, patch[:]); err == nil {
			return patch, nil
		}
	}
	return zero, fmt.Errorf("gen1 trade: could not locate %d-byte remote patch list in %d received bytes", PatchListSize, len(stream))
}

// BitBridge adapts GomeBoy's MSB-first serial clock pulses to Machine bytes.
type BitBridge struct {
	machine *Machine
	out     byte
	outBit  uint8
	in      byte
	inBits  uint8
}

func NewBitBridge(machine *Machine) *BitBridge {
	b := &BitBridge{machine: machine, out: NoDataByte}
	if machine != nil {
		b.out = machine.InitialByte()
	}
	return b
}

func (b *BitBridge) ExchangeBit(in bool) (bool, error) {
	if b.machine == nil {
		return true, errors.New("gen1 trade: nil machine")
	}
	mask := byte(0x80 >> b.outBit)
	out := b.out&mask != 0
	b.in <<= 1
	if in {
		b.in |= 1
	}
	b.outBit++
	b.inBits++
	if b.inBits < 8 {
		return out, nil
	}

	next, err := b.machine.ExchangeByte(b.in)
	b.out = next
	b.outBit = 0
	b.in = 0
	b.inBits = 0
	return out, err
}
