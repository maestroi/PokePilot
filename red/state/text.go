package state

import (
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/red/sym"
)

// DecodeName retains the Red state API while delegating the shared English
// Gen-I character encoding to the neutral gen1 package.
func DecodeName(buf []byte) string { return gen1.DecodeName(buf) }

// DecodeTiles retains the Red state API for existing callers.
func DecodeTiles(tiles []byte) string { return gen1.DecodeTiles(tiles) }

// NormalizeDisplayText retains presentation compatibility for Red callers.
func NormalizeDisplayText(s string) string { return gen1.NormalizeDisplayText(s) }

// ScreenText returns the text currently rendered on Red's tilemap. The address
// remains Red-owned; only the byte encoding is shared.
func ScreenText(m *Mem) string {
	return gen1.NormalizeDisplayText(gen1.DecodeTiles(m.Slice(sym.TileMap, sym.TileMapLen)))
}
