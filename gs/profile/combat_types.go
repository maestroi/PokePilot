package profile

// Native Gold/Silver type ids from pret/pokegold constants/type_constants.asm
// at gs/data.SourceRevision.
const (
	gsTypeNormal   uint8 = 0x00
	gsTypeFighting uint8 = 0x01
	gsTypeFlying   uint8 = 0x02
	gsTypePoison   uint8 = 0x03
	gsTypeGround   uint8 = 0x04
	gsTypeRock     uint8 = 0x05
	gsTypeBug      uint8 = 0x07
	gsTypeGhost    uint8 = 0x08
	gsTypeSteel    uint8 = 0x09
	gsTypeFire     uint8 = 0x14
	gsTypeWater    uint8 = 0x15
	gsTypeGrass    uint8 = 0x16
	gsTypeElectric uint8 = 0x17
	gsTypePsychic  uint8 = 0x18
	gsTypeIce      uint8 = 0x19
	gsTypeDragon   uint8 = 0x1a
	gsTypeDark     uint8 = 0x1b
)

const (
	gsTypeNoEffect         = 0
	gsTypeNotVeryEffective = 5
	gsTypeNeutralEffect    = 10
	gsTypeSuperEffective   = 20
)

// gsTypeMatchups is pret/pokegold data/types/type_matchups.asm including the
// Ghost immunities that Foresight would skip. This slice does not model
// Foresight yet; ordinary battles apply the immunities.
var gsTypeMatchups = [][3]uint8{
	{gsTypeNormal, gsTypeRock, gsTypeNotVeryEffective},
	{gsTypeNormal, gsTypeSteel, gsTypeNotVeryEffective},
	{gsTypeFire, gsTypeFire, gsTypeNotVeryEffective},
	{gsTypeFire, gsTypeWater, gsTypeNotVeryEffective},
	{gsTypeFire, gsTypeGrass, gsTypeSuperEffective},
	{gsTypeFire, gsTypeIce, gsTypeSuperEffective},
	{gsTypeFire, gsTypeBug, gsTypeSuperEffective},
	{gsTypeFire, gsTypeRock, gsTypeNotVeryEffective},
	{gsTypeFire, gsTypeDragon, gsTypeNotVeryEffective},
	{gsTypeFire, gsTypeSteel, gsTypeSuperEffective},
	{gsTypeWater, gsTypeFire, gsTypeSuperEffective},
	{gsTypeWater, gsTypeWater, gsTypeNotVeryEffective},
	{gsTypeWater, gsTypeGrass, gsTypeNotVeryEffective},
	{gsTypeWater, gsTypeGround, gsTypeSuperEffective},
	{gsTypeWater, gsTypeRock, gsTypeSuperEffective},
	{gsTypeWater, gsTypeDragon, gsTypeNotVeryEffective},
	{gsTypeElectric, gsTypeWater, gsTypeSuperEffective},
	{gsTypeElectric, gsTypeElectric, gsTypeNotVeryEffective},
	{gsTypeElectric, gsTypeGrass, gsTypeNotVeryEffective},
	{gsTypeElectric, gsTypeGround, gsTypeNoEffect},
	{gsTypeElectric, gsTypeFlying, gsTypeSuperEffective},
	{gsTypeElectric, gsTypeDragon, gsTypeNotVeryEffective},
	{gsTypeGrass, gsTypeFire, gsTypeNotVeryEffective},
	{gsTypeGrass, gsTypeWater, gsTypeSuperEffective},
	{gsTypeGrass, gsTypeGrass, gsTypeNotVeryEffective},
	{gsTypeGrass, gsTypePoison, gsTypeNotVeryEffective},
	{gsTypeGrass, gsTypeGround, gsTypeSuperEffective},
	{gsTypeGrass, gsTypeFlying, gsTypeNotVeryEffective},
	{gsTypeGrass, gsTypeBug, gsTypeNotVeryEffective},
	{gsTypeGrass, gsTypeRock, gsTypeSuperEffective},
	{gsTypeGrass, gsTypeDragon, gsTypeNotVeryEffective},
	{gsTypeGrass, gsTypeSteel, gsTypeNotVeryEffective},
	{gsTypeIce, gsTypeWater, gsTypeNotVeryEffective},
	{gsTypeIce, gsTypeGrass, gsTypeSuperEffective},
	{gsTypeIce, gsTypeIce, gsTypeNotVeryEffective},
	{gsTypeIce, gsTypeGround, gsTypeSuperEffective},
	{gsTypeIce, gsTypeFlying, gsTypeSuperEffective},
	{gsTypeIce, gsTypeDragon, gsTypeSuperEffective},
	{gsTypeIce, gsTypeSteel, gsTypeNotVeryEffective},
	{gsTypeIce, gsTypeFire, gsTypeNotVeryEffective},
	{gsTypeFighting, gsTypeNormal, gsTypeSuperEffective},
	{gsTypeFighting, gsTypeIce, gsTypeSuperEffective},
	{gsTypeFighting, gsTypePoison, gsTypeNotVeryEffective},
	{gsTypeFighting, gsTypeFlying, gsTypeNotVeryEffective},
	{gsTypeFighting, gsTypePsychic, gsTypeNotVeryEffective},
	{gsTypeFighting, gsTypeBug, gsTypeNotVeryEffective},
	{gsTypeFighting, gsTypeRock, gsTypeSuperEffective},
	{gsTypeFighting, gsTypeDark, gsTypeSuperEffective},
	{gsTypeFighting, gsTypeSteel, gsTypeSuperEffective},
	{gsTypePoison, gsTypeGrass, gsTypeSuperEffective},
	{gsTypePoison, gsTypePoison, gsTypeNotVeryEffective},
	{gsTypePoison, gsTypeGround, gsTypeNotVeryEffective},
	{gsTypePoison, gsTypeRock, gsTypeNotVeryEffective},
	{gsTypePoison, gsTypeGhost, gsTypeNotVeryEffective},
	{gsTypePoison, gsTypeSteel, gsTypeNoEffect},
	{gsTypeGround, gsTypeFire, gsTypeSuperEffective},
	{gsTypeGround, gsTypeElectric, gsTypeSuperEffective},
	{gsTypeGround, gsTypeGrass, gsTypeNotVeryEffective},
	{gsTypeGround, gsTypePoison, gsTypeSuperEffective},
	{gsTypeGround, gsTypeFlying, gsTypeNoEffect},
	{gsTypeGround, gsTypeBug, gsTypeNotVeryEffective},
	{gsTypeGround, gsTypeRock, gsTypeSuperEffective},
	{gsTypeGround, gsTypeSteel, gsTypeSuperEffective},
	{gsTypeFlying, gsTypeElectric, gsTypeNotVeryEffective},
	{gsTypeFlying, gsTypeGrass, gsTypeSuperEffective},
	{gsTypeFlying, gsTypeFighting, gsTypeSuperEffective},
	{gsTypeFlying, gsTypeBug, gsTypeSuperEffective},
	{gsTypeFlying, gsTypeRock, gsTypeNotVeryEffective},
	{gsTypeFlying, gsTypeSteel, gsTypeNotVeryEffective},
	{gsTypePsychic, gsTypeFighting, gsTypeSuperEffective},
	{gsTypePsychic, gsTypePoison, gsTypeSuperEffective},
	{gsTypePsychic, gsTypePsychic, gsTypeNotVeryEffective},
	{gsTypePsychic, gsTypeDark, gsTypeNoEffect},
	{gsTypePsychic, gsTypeSteel, gsTypeNotVeryEffective},
	{gsTypeBug, gsTypeFire, gsTypeNotVeryEffective},
	{gsTypeBug, gsTypeGrass, gsTypeSuperEffective},
	{gsTypeBug, gsTypeFighting, gsTypeNotVeryEffective},
	{gsTypeBug, gsTypePoison, gsTypeNotVeryEffective},
	{gsTypeBug, gsTypeFlying, gsTypeNotVeryEffective},
	{gsTypeBug, gsTypePsychic, gsTypeSuperEffective},
	{gsTypeBug, gsTypeGhost, gsTypeNotVeryEffective},
	{gsTypeBug, gsTypeDark, gsTypeSuperEffective},
	{gsTypeBug, gsTypeSteel, gsTypeNotVeryEffective},
	{gsTypeRock, gsTypeFire, gsTypeSuperEffective},
	{gsTypeRock, gsTypeIce, gsTypeSuperEffective},
	{gsTypeRock, gsTypeFighting, gsTypeNotVeryEffective},
	{gsTypeRock, gsTypeGround, gsTypeNotVeryEffective},
	{gsTypeRock, gsTypeFlying, gsTypeSuperEffective},
	{gsTypeRock, gsTypeBug, gsTypeSuperEffective},
	{gsTypeRock, gsTypeSteel, gsTypeNotVeryEffective},
	{gsTypeGhost, gsTypeNormal, gsTypeNoEffect},
	{gsTypeGhost, gsTypePsychic, gsTypeSuperEffective},
	{gsTypeGhost, gsTypeDark, gsTypeNotVeryEffective},
	{gsTypeGhost, gsTypeSteel, gsTypeNotVeryEffective},
	{gsTypeGhost, gsTypeGhost, gsTypeSuperEffective},
	{gsTypeDragon, gsTypeDragon, gsTypeSuperEffective},
	{gsTypeDragon, gsTypeSteel, gsTypeNotVeryEffective},
	{gsTypeDark, gsTypeFighting, gsTypeNotVeryEffective},
	{gsTypeDark, gsTypePsychic, gsTypeSuperEffective},
	{gsTypeDark, gsTypeGhost, gsTypeSuperEffective},
	{gsTypeDark, gsTypeDark, gsTypeNotVeryEffective},
	{gsTypeDark, gsTypeSteel, gsTypeNotVeryEffective},
	{gsTypeSteel, gsTypeFire, gsTypeNotVeryEffective},
	{gsTypeSteel, gsTypeWater, gsTypeNotVeryEffective},
	{gsTypeSteel, gsTypeElectric, gsTypeNotVeryEffective},
	{gsTypeSteel, gsTypeIce, gsTypeSuperEffective},
	{gsTypeSteel, gsTypeRock, gsTypeSuperEffective},
	{gsTypeSteel, gsTypeSteel, gsTypeNotVeryEffective},
	{gsTypeNormal, gsTypeGhost, gsTypeNoEffect},
	{gsTypeFighting, gsTypeGhost, gsTypeNoEffect},
}

func gsTypePairEffect(moveType, defType uint8) int {
	for _, row := range gsTypeMatchups {
		if row[0] == moveType && row[1] == defType {
			return int(row[2])
		}
	}
	return gsTypeNeutralEffect
}

func gsTypeEffectiveness(moveType, def1, def2 uint8) int {
	m1 := gsTypePairEffect(moveType, def1)
	if def2 == def1 {
		return m1
	}
	return m1 * gsTypePairEffect(moveType, def2) / gsTypeNeutralEffect
}
