package game

// MenuPressTiming is an optional profile contract for games whose menus sample
// input more slowly than the generic default.
//
// Generic menu navigation presses a button for a fixed number of frames. Every
// game reaches its joypad mirror through its own pipeline (VBlank latch ->
// GetJoypad deltas -> the open screen's input loop), and a press that ends
// before that pipeline reads it leaves no trace at all: the menu looks exactly
// as if it had chosen to ignore the key. Length alone is therefore not a style
// preference, it is the minimum a game needs to see a press.
//
// Gen-I menus drop a 3-frame press after a menu opens but recover on the next
// tap, because generic retries there drift in phase. Gen-II's PACK retries at a
// fixed phase, so a dropped press is dropped on every retry and the pocket can
// never change. Profiles that need a longer press than the default declare it
// here; profiles that do not stay frame-identical.
type MenuPressTiming interface {
	// MenuPressHoldFrames is the shortest button hold this game's menus
	// reliably sample. A non-positive value means "use the generic default".
	MenuPressHoldFrames() int
}

// MenuPressTimingProfile is a game profile that declares its menu press hold.
type MenuPressTimingProfile interface {
	GameProfile
	MenuPressTiming
}
