package world

import (
	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/worldmodel"
)

func testRedWorldProvider(romData []byte) worldmodel.MapHeaderProvider {
	return redrom.NewWorldProvider(romData)
}
