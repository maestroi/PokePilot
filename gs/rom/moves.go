package rom

import "fmt"

// Move is one 7-byte Gen 2 move table entry.
type Move struct {
	ID       uint8
	Effect   uint8
	Power    uint8
	Type     uint8
	Accuracy uint8
	PP       uint8
	Chance   uint8
}

// TypeDark and TypeSteel are the Gen 2 type bytes (pokegold type_constants.asm).
const (
	TypeNormal   uint8 = 0x00
	TypeFighting uint8 = 0x01
	TypeFlying   uint8 = 0x02
	TypePoison   uint8 = 0x03
	TypeGround   uint8 = 0x04
	TypeRock     uint8 = 0x05
	TypeBug      uint8 = 0x07
	TypeGhost    uint8 = 0x08
	TypeSteel    uint8 = 0x09
	TypeFire     uint8 = 0x14
	TypeWater    uint8 = 0x15
	TypeGrass    uint8 = 0x16
	TypeElectric uint8 = 0x17
	TypePsychic  uint8 = 0x18
	TypeIce      uint8 = 0x19
	TypeDragon   uint8 = 0x1a
	TypeDark     uint8 = 0x1b
)

// LookupMove reads move id from the ROM's move table. Id 0 is not a move.
func LookupMove(rom []byte, id uint8) (Move, error) {
	if id == 0 {
		return Move{}, fmt.Errorf("gs/rom: move id 0 is the empty slot")
	}
	base, err := locateMoves(rom)
	if err != nil {
		return Move{}, err
	}
	off := base + int(id-1)*moveEntryLen
	if err := mustInROM(rom, off, moveEntryLen, "move"); err != nil {
		return Move{}, err
	}
	return Move{
		ID:       rom[off],
		Effect:   rom[off+1],
		Power:    rom[off+2],
		Type:     rom[off+3],
		Accuracy: rom[off+4],
		PP:       rom[off+5],
		Chance:   rom[off+6],
	}, nil
}

// TMHMMove returns the move taught by TM01..TM50 / HM01..HM07 (1-based).
func TMHMMove(rom []byte, machineNumber int) (uint8, error) {
	if machineNumber < 1 || machineNumber > numMachines {
		return 0, fmt.Errorf("gs/rom: machine number %d is outside 1..%d", machineNumber, numMachines)
	}
	base, err := locateTMHMMoves(rom)
	if err != nil {
		return 0, err
	}
	return rom[base+machineNumber-1], nil
}
