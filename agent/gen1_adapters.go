package agent

import (
	blueprofile "github.com/maestroi/pokepilot/blue/profile"
	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
)

// gen1Games are the games sharing the Gen I engine: identical RAM layout,
// symbol table, ROM table formats and map ids, differing only in identity and
// in data parsed from their own ROM bytes. The Red adapter implementation
// serves all of them, bound per game at registration so per-game vocabulary
// (location ids, catalogs) never crosses versions.
//
// Adding a Gen I revision means adding its profile here and in
// profiles.Builtin; it does not mean branching the runtime.
var gen1Games = []game.GameID{
	redprofile.GameID,
	blueprofile.GameID,
}
