package profile

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

// Yellow's cartridge binds the canonical Gen-I memory view on every emulator
// that loads it, so shared Gen-I decoders read Yellow through Red's
// coordinates without ever seeing Yellow's native addresses.
func init() {
	emu.RegisterMemoryViewResolver(func(rom []byte) emu.MemoryView {
		if yellowrom.IsCartridge(rom) {
			return &sym.Canonical
		}
		return nil
	})
}

// nativeMemory is the part of a canonical-view emulator that still reads the
// cartridge as the CPU sees it (emu.Emu's Peek*Native).
type nativeMemory interface {
	Peek8Native(addr uint16) byte
	PeekIntoNative(addr uint16, dst []byte)
}

type nativeReader struct{ m nativeMemory }

func (r nativeReader) Peek8(addr uint16) byte           { return r.m.Peek8Native(addr) }
func (r nativeReader) PeekInto(addr uint16, dst []byte) { r.m.PeekIntoNative(addr, dst) }

// native returns a reader of Yellow's own RAM layout. The Yellow-owned
// decoders in this package address yellow/sym, so they must bypass a bound
// canonical view; a plain reader (a test fake) is already native.
func native(reader game.MemoryReader) game.MemoryReader {
	if m, ok := reader.(nativeMemory); ok {
		return nativeReader{m: m}
	}
	return reader
}
