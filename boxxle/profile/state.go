package profile

import (
	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/game"
)

// DecodeBoxxleState exposes the Boxxle board state through the cartridge
// capability. It delegates to the boxxle package so the profile stays a thin
// identity adapter.
func (*Profile) DecodeBoxxleState(reader game.MemoryReader) (boxxle.State, error) {
	return boxxle.DecodeState(reader)
}

var _ boxxle.StateProfile = (*Profile)(nil)
