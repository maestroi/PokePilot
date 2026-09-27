// Package sym owns the verified retail Gold/Silver RAM layout used by the GS profile.
// Addresses correspond to the supported USA/Europe rev0 ROMs and pret/pokegold 0f087a51e36cbd38f33e5055754614578246ceff.
package sym

const (
	GoldSHA1   = "d8b8a3600a465308c9953dfa04f0081c05bdcb94"
	SilverSHA1 = "49b163f7e57702bc939d642a18f591de55d92dae"
	ROMSize    = 2 * 1024 * 1024
	WRAMBank   = 1

	OverworldMap    uint16 = 0xC800
	OverworldMapLen        = 1300

	PlayerStepFlags uint16 = 0xD150
	MapHeight       uint16 = 0xD19E
	MapWidth        uint16 = 0xD19F
	PlayerDirection uint16 = 0xD205

	MapStatus      uint16 = 0xD432
	MapEventStatus uint16 = 0xD433
	ScriptFlags    uint16 = 0xD434
	ScriptMode     uint16 = 0xD437
	ScriptRunning  uint16 = 0xD438

	ObjectStructs    uint16 = 0xD4D6
	ObjectStructLen         = 0x28
	NumObjectStructs        = 13

	PlayerState uint16 = 0xD95D

	MapGroup  uint16 = 0xDA00
	MapNumber uint16 = 0xDA01
	YCoord    uint16 = 0xDA02
	XCoord    uint16 = 0xDA03

	PartyCount   uint16 = 0xDA22
	PartySpecies uint16 = 0xDA23
	PartyMon1    uint16 = 0xDA2A
	PartyMonSize uint16 = 0x30

	BattleMode  uint16 = 0xD116
	Money       uint16 = 0xD573
	JohtoBadges uint16 = 0xD57C
	KantoBadges uint16 = 0xD57D
	TMsHMs      uint16 = 0xD57E
	TMsHMsCount        = 57
	NumItems    uint16 = 0xD5B7
	Items       uint16 = 0xD5B8
	MaxItems           = 20
	NumKeyItems uint16 = 0xD5E1
	KeyItems    uint16 = 0xD5E2
	MaxKeyItems        = 25
	NumBalls    uint16 = 0xD5FC
	Balls       uint16 = 0xD5FD
	MaxBalls           = 12
)
