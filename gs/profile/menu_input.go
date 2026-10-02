package profile

import "github.com/maestroi/pokepilot/game"

// gen2MenuPressHoldFrames is the shortest press Gold/Silver's menus reliably
// sample. Measured on retail Gold from the run-1p7ixxdreiam630odlwlg1xf35
// checkpoint (ILEX_FOREST, teaching HM01 through the native TM/HM menu):
//
//	hold 3   START-menu cursor: dropped        PACK pocket switch: dropped
//	hold 4   one step                          exactly one pocket
//	hold 8   one step                          exactly one pocket
//	hold 12  one step                          exactly one pocket
//	hold 20  one step                          exactly one pocket
//	hold 24  one step                          two pockets
//
// and the TM/HM pocket cursor steps exactly once up to hold 12, twice by 20.
// 8 sits in the middle of the window every one of those menus reads as a single
// step, so a press is neither lost nor double-counted.
const gen2MenuPressHoldFrames = 8

// MenuPressHoldFrames makes Gold/Silver's slower menu input cadence explicit to
// the reusable menu driver instead of leaving it to a frame count tuned for
// Gen-I. See game.MenuPressTiming.
func (*Profile) MenuPressHoldFrames() int { return gen2MenuPressHoldFrames }

var _ game.MenuPressTiming = (*Profile)(nil)
