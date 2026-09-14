package emu

import "github.com/maestroi/gomeboy/pkg/gomeboy"

// OpenCGBBytes loads an in-memory ROM as a Game Boy Color session. It mirrors
// OpenCGB but lets callers boot a cartridge image without writing it to disk.
func OpenCGBBytes(rom []byte) (*Emu, error) {
	e, err := gomeboy.New(
		gomeboy.WithROMBytes(rom),
		gomeboy.Headless(),
		gomeboy.WithModel(gomeboy.ModelCGB),
	)
	if err != nil {
		return nil, err
	}
	return &Emu{e: e, semanticROM: e.ROM()}, nil
}

// LoadROMBytes replaces the cartridge image and makes the new image the
// semantic ROM as well. This is the ordinary non-derived reload behavior.
func (m *Emu) LoadROMBytes(rom []byte, name string) error {
	if err := m.e.LoadROMBytes(rom, name); err != nil {
		return err
	}
	m.semanticROM = m.e.ROM()
	return nil
}

// LoadDerivedROM replaces the cartridge with derived while keeping base as
// the semantic ROM returned by ROM(). This lets deterministic experiment
// patches execute in GomeBoy without weakening PokePilot's exact base-ROM
// profile checks or making every parser understand a synthetic ROM identity.
func (m *Emu) LoadDerivedROM(base, derived []byte, name string) error {
	if err := m.e.LoadROMBytes(derived, name); err != nil {
		return err
	}
	m.semanticROM = append(m.semanticROM[:0], base...)
	return nil
}
