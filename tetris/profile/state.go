package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/tetris"
)

func (*Profile) DecodeTetrisState(reader game.MemoryReader) (tetris.State, error) {
	return tetris.DecodeState(reader)
}

var _ tetris.StateProfile = (*Profile)(nil)
