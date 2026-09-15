package agent

import "errors"

// errGymLeaderLost is the typed compatibility signal for the legacy
// Knowledge.Failed(Objective, error) API. Live runtime policy uses the
// structured BattleEvidence on ObjectiveResult instead.
var errGymLeaderLost = errors.New("lost to the gym leader")
