package rom

// Ledge is a directed two-tile hop. HandleLedges compares the bottom-left
// tiles under and in front of the player, and runs two simulated inputs.
type Ledge struct {
	DX, DY     int
	From, Over byte
}

func Ledges(data []byte, tileset uint8) []Ledge {
	if tileset != 0 {
		return nil
	} // HandleLedges only runs in OVERWORLD.
	const offset = 6*0x4000 + 0x66cf - 0x4000 // pokered.sym: LedgeTiles
	var out []Ledge
	for at := offset; at+3 < len(data) && data[at] != 0xff; at += 4 {
		l := Ledge{From: data[at+1], Over: data[at+2]}
		switch data[at] {
		case 0:
			l.DY = 1
		case 4:
			l.DY = -1
		case 8:
			l.DX = -1
		case 12:
			l.DX = 1
		default:
			continue
		}
		out = append(out, l)
	}
	return out
}
