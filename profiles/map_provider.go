package profiles

import (
	"fmt"

	"github.com/maestroi/pokepilot/worldmodel"
)

// mapHeaderSource is implemented by profiles whose world uses the uint8
// MapHeaderProvider graph (Gen I today).
type mapHeaderSource interface {
	MapProvider([]byte) worldmodel.MapHeaderProvider
}

// MapProvider resolves rom to its game profile and returns that profile's map
// header provider. It is the single raw-ROM entry point into world graphs: the
// selected profile, not a generic detector, owns which provider applies.
func MapProvider(rom []byte) (worldmodel.MapHeaderProvider, error) {
	profile, _, err := Detect(rom)
	if err != nil {
		return nil, err
	}
	source, ok := profile.(mapHeaderSource)
	if !ok {
		return nil, fmt.Errorf("profiles: %s@%s has no map header provider", profile.ID(), profile.Revision())
	}
	provider := source.MapProvider(rom)
	if provider == nil {
		return nil, fmt.Errorf("profiles: %s@%s returned nil map header provider", profile.ID(), profile.Revision())
	}
	return provider, nil
}
