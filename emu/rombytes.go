package emu

import "github.com/maestroi/gomeboy/pkg/gomeboy"

// OpenCGBBytes loads an in-memory ROM as a Game Boy Color session. It mirrors
// OpenCGB but lets experiment modes derive a ROM without writing copyrighted
// cartridge bytes to disk.
func OpenCGBBytes(rom []byte) (*Emu, error) {
	e, err := gomeboy.New(
		gomeboy.WithROMBytes(rom),
		gomeboy.Headless(),
		gomeboy.WithModel(gomeboy.ModelCGB),
	)
	if err != nil {
		return nil, err
	}
	return &Emu{e: e}, nil
}

// LoadROMBytes replaces the cartridge image and reinitializes the underlying
// emulator while keeping PokePilot's watch/trace wrapper attached. Farm workers
// use this between leases so a patched experiment never leaks into the next run.
func (m *Emu) LoadROMBytes(rom []byte, name string) error {
	return m.e.LoadROMBytes(rom, name)
}
