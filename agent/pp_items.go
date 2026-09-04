package agent

// ppRestoreItems are the finite field PP restorers in Pokemon Red. The ROM
// constants intentionally spell ELIXER this way. Keep the same names in the
// executable item vocabulary so map pickups, bag observations, and
// KindUseItem objectives all resolve to one deterministic ID.
var ppRestoreItems = map[string]uint8{
	"ether":      0x50,
	"max ether":  0x51,
	"elixer":     0x52,
	"max elixer": 0x53,
}

func init() {
	for name, id := range ppRestoreItems {
		itemTable[name] = id
		itemByID[id] = name
	}
}
