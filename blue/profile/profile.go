// Package profile implements the Pokémon Blue adapter.
//
// Blue shares Gen I's engine with Red wholesale: identical RAM layout, symbol
// table, ROM table formats and map ids, differing only in identity and in data
// parsed from each image's own bytes (preset names, wild encounter sets,
// version-exclusive species). Every version-sensitive fact the runtime consults
// — encounters, fishing, mart stock, TM/HM, moves, experience, trades — is read
// out of romData at call time, so the same engine serves both images.
//
// This profile overrides identity and ROM detection and delegates everything
// else to the shared Gen I engine. Removal path: when that engine is factored
// out of red/, Red and Blue should both embed it instead of Blue delegating to
// Red's package.
package profile

import (
	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
)

const (
	GameID   game.GameID     = "pokemon-blue"
	Revision game.RevisionID = "en-us-rev0"

	// ROMSHA1 identifies the supported English Blue image.
	ROMSHA1 = "d7037c83e1ae5b39bde3c30787637ba1d4c48ce2"
)

// Profile is the Gen I adapter reporting Blue identity. It delegates layout,
// symbols, ROM parsing and observation to the shared engine.
type Profile struct {
	engine redprofile.Profile
}

func New() *Profile { return &Profile{} }

func (*Profile) ID() game.GameID               { return GameID }
func (*Profile) Revision() game.RevisionID     { return Revision }
func (*Profile) Detect(info game.ROMInfo) bool { return info.SHA1 == ROMSHA1 }

func (p *Profile) Symbols() game.SymbolTable      { return p.engine.Symbols() }
func (p *Profile) Features() game.ProfileFeatures { return p.engine.Features() }
func (p *Profile) ROMParser() game.ROMParser      { return p.engine.ROMParser() }
func (p *Profile) DecodeBootState(r game.MemoryReader) game.BootState {
	return p.engine.DecodeBootState(r)
}
func (p *Profile) DecodeObservation(r game.MemoryReader, rom []byte) (game.ProfileObservation, error) {
	return p.engine.DecodeObservation(r, rom)
}

func (p *Profile) BuildDexCatalog(rom []byte, owned, seen []game.SpeciesID) (game.DexCatalog, error) {
	return p.engine.BuildDexCatalog(rom, owned, seen)
}
