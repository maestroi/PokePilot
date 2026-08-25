package rom

import "fmt"

// The move table lives at Moves (bank 0x0E, 0x4000), six bytes per move:
//
//	+0  animation id, which is also the move's own id
//	+1  effect
//	+2  power, 0 for a move that deals no damage
//	+3  type
//	+4  accuracy
//	+5  PP
//
// Move ids are 1-based, so move n is the (n-1)th entry.
const (
	movesOffset  = 0x0E * 0x4000
	moveEntryLen = 6

	// DefenseDown1Effect is DEFENSE_DOWN1_EFFECT: the move lowers the
	// target's Defense by one stage. Tail Whip is the early example, and it
	// is the direct answer to an opponent that keeps lowering our Attack.
	DefenseDown1Effect uint8 = 19

	// AttackDown1Effect is ATTACK_DOWN1_EFFECT: the move lowers the target's
	// Attack by one stage. Growl is the early example. It is the other way
	// to win a damage race — cut what is coming in rather than raise what
	// goes out.
	AttackDown1Effect uint8 = 18
)

// Move is one entry of the ROM's move table.
type Move struct {
	ID     uint8
	Effect uint8
	Power  uint8 // 0 means the move deals no damage
	Type   uint8
	PP     uint8
}

// LookupMove reads move id from the ROM's move table. Move id 0 means "no
// move" and is not in the table, so it is an error.
func LookupMove(romData []byte, id uint8) (Move, error) {
	if id == 0 {
		return Move{}, fmt.Errorf("rom: move id 0 is the empty slot, not a move")
	}
	off := movesOffset + (int(id)-1)*moveEntryLen
	if off+moveEntryLen > len(romData) {
		return Move{}, fmt.Errorf("rom: move %d at offset %d exceeds ROM of %d bytes", id, off, len(romData))
	}
	e := romData[off : off+moveEntryLen]
	// Each entry's animation byte is the move's own id. Checking it turns a
	// mislocated table into a loud error instead of silent nonsense.
	if e[0] != id {
		return Move{}, fmt.Errorf("rom: move table entry %d has animation %d, want %d: the table is not where we think", id, e[0], id)
	}
	return Move{ID: id, Effect: e[1], Power: e[2], Type: e[3], PP: e[5]}, nil
}
