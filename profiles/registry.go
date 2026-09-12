// Package profiles owns the concrete profile registry used by PokePilot.
// Generic packages depend only on game.GameProfile; adding a game means
// registering its adapter here rather than branching throughout the runtime.
package profiles

import (
	"fmt"
	"sync"

	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
)

var (
	builtinOnce sync.Once
	builtin     *game.Registry
	builtinErr  error
)

func Builtin() (*game.Registry, error) {
	builtinOnce.Do(func() {
		builtin, builtinErr = game.NewRegistry(
			redprofile.New(),
		)
	})
	return builtin, builtinErr
}

func Detect(rom []byte) (game.GameProfile, game.ROMInfo, error) {
	registry, err := Builtin()
	if err != nil {
		return nil, game.ROMInfo{}, fmt.Errorf("profiles: build registry: %w", err)
	}
	profile, info, err := registry.DetectROM(rom)
	if err != nil {
		return nil, info, fmt.Errorf("profiles: %w", err)
	}
	return profile, info, nil
}
