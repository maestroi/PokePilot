package agent

// ppRestoreItems is the planner-facing semantic vocabulary for finite PP
// restorers. The Red bytes stay in ppRestoreRedItems and are registered in the
// adapter translation table below.
var ppRestoreItems = map[string]ItemID{
	"ether":      "ether",
	"max ether":  "max ether",
	"elixer":     "elixer",
	"max elixer": "max elixer",
}

var ppRestoreRedItems = map[string]uint8{
	"ether":      0x50,
	"max ether":  0x51,
	"elixer":     0x52,
	"max elixer": 0x53,
}

func init() {
	for name, id := range ppRestoreRedItems {
		itemTable[name] = id
		itemByID[id] = name
	}
}
