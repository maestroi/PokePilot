package rom

import "fmt"

const trainerPicMoneyEntry = 5 // dw pic, bcd3 money

// TrainerPrizeMoney is the ¥ an ordinary trainer pays when beaten
// (pokered/data/trainers/pic_pointers_money.asm): the class base money times
// the level of the party's last Pokémon. class is 1-based, as in TrainerParty.
func TrainerPrizeMoney(romData []byte, class, set uint8) (uint32, error) {
	if class == 0 || class > trainerClassCount {
		return 0, fmt.Errorf("rom: trainer class %d outside 1..%d", class, trainerClassCount)
	}
	party, err := TrainerParty(romData, class, set)
	if err != nil {
		return 0, err
	}
	if len(party) == 0 {
		return 0, fmt.Errorf("rom: trainer class %d set %d has an empty party", class, set)
	}
	base, err := Tables(romData).TrainerPicAndMoneyPointers.Offset()
	if err != nil {
		return 0, fmt.Errorf("rom: TrainerPicAndMoneyPointers: %w", err)
	}
	off := base + int(class-1)*trainerPicMoneyEntry + 2
	if off+3 > len(romData) {
		return 0, fmt.Errorf("rom: trainer class %d money at %#x exceeds ROM", class, off)
	}
	var money uint32
	for _, b := range romData[off : off+3] {
		money = money*100 + uint32(b>>4)*10 + uint32(b&0x0F)
	}
	// bcd3 stores base×100 (YOUNGSTER 1500 = ¥15 per level).
	return money / 100 * uint32(party[len(party)-1].Level), nil
}
