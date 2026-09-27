// Package profiles owns the concrete profile registry used by PokePilot.
// Generic packages depend only on game.GameProfile; adding a game means
// registering its adapter here rather than branching throughout the runtime.
package profiles

import (
	"fmt"
	"sync"

	blueprofile "github.com/maestroi/pokepilot/blue/profile"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	redprofile "github.com/maestroi/pokepilot/red/profile"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	tetrisprofile "github.com/maestroi/pokepilot/tetris/profile"
)

var (
	builtinOnce sync.Once
	builtin     *game.Registry
	builtinErr  error

	cartridgeOnce sync.Once
	cartridges    *game.CartridgeRegistry
	cartridgeErr  error
)

func Builtin() (*game.Registry, error) {
	builtinOnce.Do(func() {
		builtin, builtinErr = game.NewRegistry(
			redprofile.New(),
			blueprofile.New(),
			yellowprofile.New(),
			gsprofile.NewGold(),
			gsprofile.NewSilver(),
		)
	})
	return builtin, builtinErr
}

 // Cartridges returns the game-agnostic ROM identity registry. Pokémon profiles
 // participate because game.GameProfile extends game.CartridgeProfile; Tetris
 // is registered here without pretending to implement Pokémon semantics.
func Cartridges() (*game.CartridgeRegistry, error) {
	cartridgeOnce.Do(func() {
		cartridges, cartridgeErr = game.NewCartridgeRegistry(
			redprofile.New(),
			blueprofile.New(),
			yellowprofile.New(),
			gsprofile.NewGold(),
			gsprofile.NewSilver(),
			tetrisprofile.New(),
		)
	})
	return cartridges, cartridgeErr
}

func DetectCartridge(rom []byte) (game.CartridgeProfile, game.ROMInfo, error) {
	registry, err := Cartridges()
	if err != nil {
		return nil, game.ROMInfo{}, fmt.Errorf("profiles: build cartridge registry: %w", err)
	}
	profile, info, err := registry.DetectROM(rom)
	if err != nil {
		return nil, info, fmt.Errorf("profiles: %w", err)
	}
	return profile, info, nil
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
