// Package gs exposes constructors for the supported Gold/Silver profiles.
package gs

import (
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

const (
	GoldGameID   = gsprofile.GoldGameID
	SilverGameID = gsprofile.SilverGameID
)

func New(ids ...game.GameID) *gsprofile.Profile { return gsprofile.New(ids...) }
func Gold() *gsprofile.Profile                  { return gsprofile.NewGold() }
func Silver() *gsprofile.Profile                { return gsprofile.NewSilver() }
