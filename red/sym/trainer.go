package sym

// Trainer interaction state. StoreTrainerHeaderPointer writes the trainer
// header pointer high-byte first; wTrainerHeaderFlagBit is the bit index used
// by TrainerFlagAction for the active header.
const (
	TrainerHeaderPtr     uint16 = 0xDA30 // wTrainerHeaderPtr
	TrainerHeaderFlagBit uint16 = 0xCC55 // wTrainerHeaderFlagBit
)
