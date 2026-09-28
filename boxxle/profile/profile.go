// Package profile implements the Game Boy Boxxle cartridge adapter.
//
// The base contract remains game-agnostic cartridge identity. Boxxle is a
// puzzle game, not a Pokémon title, so this slice registers identity only:
// board decoding, autonomous play, and state observation are later slices of
// the Boxxle epic and must not leak Pokémon-shaped observation fields here.
package profile

import "github.com/maestroi/pokepilot/game"

const (
	GameID   game.GameID     = "boxxle"
	Revision game.RevisionID = "us-eu-rev1"

	// ROMSHA1 and ROMSHA256 identify Boxxle (USA, Europe) (Rev 1), the
	// No-Intro-trusted 32 KiB dump (Pony Canyon, media serial DMG-SOE-1).
	// The bytes themselves are operator-supplied and never committed.
	ROMSHA1   = "f9d5287bc9d6eda9ec36e9b5a8dbc38cf2cffecf"
	ROMSHA256 = "c859503342db1f86dadeb7e6f3d8a8a2918e9b6a7c8756311b7cc7bb0a7e892f"
)

type Profile struct{}

func New() *Profile { return &Profile{} }

func (*Profile) ID() game.GameID           { return GameID }
func (*Profile) Revision() game.RevisionID { return Revision }

func (*Profile) Detect(info game.ROMInfo) bool {
	return info.SHA1 == ROMSHA1 && info.SHA256 == ROMSHA256
}

var _ game.CartridgeProfile = (*Profile)(nil)
