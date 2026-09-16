package agent

import (
	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
)

// Red scenario tests drive the Gen I engine that Red and Blue share, so their
// synthetic observations are Red games. The per-GameID dispatch tables stopped
// falling back on an unidentified observation as soon as Blue registered, so
// every synthetic caller has to name its game instead of relying on there
// being exactly one.
const testGameID game.GameID = redprofile.GameID

// testKnowledge resolves native Gen I adjacency through the Red game's topology
// so the resulting Knowledge carries the same semantic LocationIDs the catalog
// offers, rather than the raw legacy fallback ids no catalog matches.
func testKnowledge(adjacency map[uint8][]uint8) *Knowledge {
	return NewKnowledge(knowledgeTopologyFor(testGameID, adjacency))
}
