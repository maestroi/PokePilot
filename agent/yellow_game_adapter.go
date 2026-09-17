package agent

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

// yellowMapGraphAdapter builds Yellow's travel graph with Yellow's own ROM
// table addresses and valid-map set. Yellow shares Gen I's header/object
// format with Red but not its table layout, so it cannot reuse Red's tables:
// doing so parses only 33 of its 249 maps.
type yellowMapGraphAdapter struct{}

func (yellowMapGraphAdapter) BuildGraph(romData []byte) (*world.Graph, error) {
	return world.BuildGraphForTables(yellowrom.Tables(), romData)
}

func init() {
	registerMapGraphProvider(game.GameID(yellowprofile.GameID), yellowMapGraphAdapter{})
}
