package state

// Mem is a full 64 KiB snapshot of the Game Boy address space. Index it by
// absolute address; that keeps every decoder able to use red/sym constants
// directly with no offset arithmetic.
type Mem [0x10000]byte

// U8 reads a single byte at addr.
func (m *Mem) U8(addr uint16) byte {
	return m[addr]
}

// U16LE reads a little-endian 16-bit value: CPU pointers.
func (m *Mem) U16LE(addr uint16) uint16 {
	return uint16(m[addr]) | uint16(m[addr+1])<<8
}

// U16BE reads a big-endian 16-bit value: Pokemon data structs.
func (m *Mem) U16BE(addr uint16) uint16 {
	return uint16(m[addr])<<8 | uint16(m[addr+1])
}

// Slice returns n bytes starting at addr. It panics if the range exceeds
// the 64 KiB address space.
func (m *Mem) Slice(addr uint16, n int) []byte {
	return m[addr : addr+uint16(n)]
}
