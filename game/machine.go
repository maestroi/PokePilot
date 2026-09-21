package game

import "errors"

// ErrMachineUnusable means execution left the backing machine unsafe to
// observe, step, settle, or save. The objective transaction must not run
// finish-boundary cleanup after this error: that work can step or snapshot a
// machine whose current frame still holds its lock.
var ErrMachineUnusable = errors.New("game: machine must not be used again")
