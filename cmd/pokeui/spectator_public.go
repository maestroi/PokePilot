package main

import "encoding/json"

// MarshalJSON is deliberately explicit at the public trust boundary. In
// particular, spectatorRun currently holds farm.Player internally so it can
// decode the wall's dashboard without another conversion pass; enumerating the
// public player fields here prevents future additions to farm.Player from
// becoming public by accident.
func (run spectatorRun) MarshalJSON() ([]byte, error) {
	type publicPartyMon struct {
		Name   string `json:"name"`
		Level  uint8  `json:"level"`
		HP     uint16 `json:"hp"`
		MaxHP  uint16 `json:"max_hp"`
		Status string `json:"status,omitempty"`
	}
	type publicPlayer struct {
		Money  uint32           `json:"money"`
		Badges []string         `json:"badges,omitempty"`
		Party  []publicPartyMon `json:"party"`
	}
	type publicRun struct {
		RunID     string          `json:"run_id"`
		Status    string          `json:"status"`
		Starter   string          `json:"starter,omitempty"`
		Dest      string          `json:"dest,omitempty"`
		Goal      string          `json:"goal,omitempty"`
		QueuedAt  int64           `json:"queued_at,omitempty"`
		EndedAt   int64           `json:"ended_at,omitempty"`
		Frame     uint64          `json:"frame"`
		Map       uint8           `json:"map"`
		X         uint8           `json:"x"`
		Y         uint8           `json:"y"`
		Decision  string          `json:"decision,omitempty"`
		StopSoFar string          `json:"stop_so_far,omitempty"`
		Stats     *spectatorStats `json:"stats,omitempty"`
		Player    *publicPlayer   `json:"player,omitempty"`
		Attempts  int             `json:"attempts,omitempty"`
		Reason    string          `json:"reason,omitempty"`
	}

	var player *publicPlayer
	if run.Player != nil {
		player = &publicPlayer{
			Money:  run.Player.Money,
			Badges: append([]string(nil), run.Player.Badges...),
			Party:  make([]publicPartyMon, len(run.Player.Party)),
		}
		for i, mon := range run.Player.Party {
			player.Party[i] = publicPartyMon{
				Name:   mon.Name,
				Level:  mon.Level,
				HP:     mon.HP,
				MaxHP:  mon.MaxHP,
				Status: mon.Status,
			}
		}
	}

	return json.Marshal(publicRun{
		RunID:     run.RunID,
		Status:    run.Status,
		Starter:   run.Starter,
		Dest:      run.Dest,
		Goal:      run.Goal,
		QueuedAt:  run.QueuedAt,
		EndedAt:   run.EndedAt,
		Frame:     run.Frame,
		Map:       run.Map,
		X:         run.X,
		Y:         run.Y,
		Decision:  run.Decision,
		StopSoFar: run.StopSoFar,
		Stats:     run.Stats,
		Player:    player,
		Attempts:  run.Attempts,
		Reason:    run.Reason,
	})
}
