package sym

// OverworldMap is wOverworldMap, the current map's block buffer after map
// scripts and ReplaceTileBlock have modified it. The buffer includes a
// three-block connection border on every side and spans 1300 bytes in WRAM.
const (
	OverworldMap    uint16 = 0xC6E8
	OverworldMapLen        = 1300
)
