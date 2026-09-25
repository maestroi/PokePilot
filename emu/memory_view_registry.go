package emu

import "sync"

// MemoryViewResolver returns the canonical memory view a cartridge needs, or
// nil when its native layout is already canonical. Game packages register one
// for each revision whose RAM layout differs from the engine they share.
type MemoryViewResolver func(rom []byte) MemoryView

var (
	viewResolverMu sync.RWMutex
	viewResolvers  []MemoryViewResolver
)

// RegisterMemoryViewResolver makes every emulator that loads a matching
// cartridge bind the resolver's view. Binding at load time, rather than at
// each call site, is what keeps shared decoders from ever reading a
// non-canonical cartridge through raw addresses.
func RegisterMemoryViewResolver(r MemoryViewResolver) {
	if r == nil {
		return
	}
	viewResolverMu.Lock()
	viewResolvers = append(viewResolvers, r)
	viewResolverMu.Unlock()
}

// bindResolvedView binds the view registered for the semantic ROM, or clears
// any view left by a previously loaded cartridge.
func (m *Emu) bindResolvedView() {
	rom := m.semanticROM
	if rom == nil {
		rom = m.e.ROM()
	}
	viewResolverMu.RLock()
	resolvers := viewResolvers
	viewResolverMu.RUnlock()
	for _, resolve := range resolvers {
		if v := resolve(rom); v != nil {
			m.view = v
			return
		}
	}
	m.view = nil
}
