package state

// Peeker is the subset of the emulator that a snapshot needs. *emu.Emu
// satisfies it.
type Peeker interface {
	PeekInto(addr uint16, dst []byte)
}

// Snapshot copies the whole address space into m. It allocates nothing when
// m is reused across frames, which is the intended usage.
func Snapshot(p Peeker, m *Mem) { p.PeekInto(0, m[:]) }

// Read is the convenience form: snapshot and decode in one call.
func Read(p Peeker, m *Mem) GameState {
	Snapshot(p, m)
	return Decode(m)
}
