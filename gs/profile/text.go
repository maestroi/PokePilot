package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

// gsScreenText decodes the rendered 20x18 Gold/Silver tilemap, not script
// source bytes. The battle text engine has already expanded names and control
// codes by the time they reach wTileMap, so a compact display-glyph decoder is
// sufficient for stable UI markers.
func gsScreenText(reader game.MemoryReader) string {
	if reader == nil {
		return ""
	}
	raw := make([]byte, sym.TileMapLen)
	reader.PeekInto(sym.TileMap, raw)
	var b strings.Builder
	for _, ch := range raw {
		switch {
		case ch >= 0x80 && ch <= 0x99:
			b.WriteByte('A' + ch - 0x80)
		case ch >= 0xa0 && ch <= 0xb9:
			b.WriteByte('a' + ch - 0xa0)
		case ch >= 0xf6:
			b.WriteByte('0' + ch - 0xf6)
		default:
			switch ch {
			case 0x7f:
				b.WriteByte(' ')
			case 0x9a:
				b.WriteByte('(')
			case 0x9b:
				b.WriteByte(')')
			case 0x9c:
				b.WriteByte(':')
			case 0x9d:
				b.WriteByte(';')
			case 0xe0:
				b.WriteByte('\'')
			case 0xe3:
				b.WriteByte('-')
			case 0xe6:
				b.WriteByte('?')
			case 0xe7:
				b.WriteByte('!')
			case 0xe8, 0xf2:
				b.WriteByte('.')
			case 0xe9:
				b.WriteByte('&')
			case 0xea:
				b.WriteString("é")
			case 0xf3:
				b.WriteByte('/')
			case 0xf4:
				b.WriteByte(',')
			default:
				b.WriteByte(' ')
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
