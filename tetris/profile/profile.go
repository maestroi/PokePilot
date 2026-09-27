// Package profile implements the Game Boy Tetris cartridge adapter.
//
// The base contract remains game-agnostic cartridge identity. Tetris gameplay
// state is exposed through the optional tetris.StateProfile capability rather
// than through Pokémon-specific observation fields.
package profile

import "github.com/maestroi/pokepilot/game"

const (
	GameID   game.GameID     = "tetris"
	Revision game.RevisionID = "world-rev1"

	// ROMSHA1 and ROMSHA256 identify Tetris (World) (Rev 1).
	ROMSHA1   = "74591cc9501af93873f9a5d3eb12da12c0723bbc"
	ROMSHA256 = "0d6535aef23969c7e5af2b077acaddb4a445b3d0df7bf34c8acef07b51b015c3"
)

type Profile struct{}

func New() *Profile { return &Profile{} }

func (*Profile) ID() game.GameID           { return GameID }
func (*Profile) Revision() game.RevisionID { return Revision }

func (*Profile) Detect(info game.ROMInfo) bool {
	return info.SHA1 == ROMSHA1 && info.SHA256 == ROMSHA256
}

var _ game.CartridgeProfile = (*Profile)(nil)
