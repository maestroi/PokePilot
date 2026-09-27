package profile

import (
	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/worldmodel"
)

// NativeMapProvider exposes the verified wide-id Gold/Silver topology slice.
// The historical MapProvider contract remains uint8 and cannot represent the
// Gen-II (map group, map number) namespace without collisions.
func (*Profile) NativeMapProvider(romData []byte) worldmodel.NativeMapTopologyProvider {
	return gsrom.NewFirstBadgeWorldProvider(romData)
}
