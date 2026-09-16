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
	type publicBagItem struct {
		Name     string `json:"name"`
		Quantity int    `json:"quantity"`
	}
	type publicPlayer struct {
		Money       uint32           `json:"money"`
		Badges      []string         `json:"badges,omitempty"`
		Party       []publicPartyMon `json:"party"`
		BagUsed     int              `json:"bag_used,omitempty"`
		BagCapacity int              `json:"bag_capacity,omitempty"`
		Bag         []publicBagItem  `json:"bag,omitempty"`
		DexOwned    int              `json:"dex_owned,omitempty"`
		DexSeen     int              `json:"dex_seen,omitempty"`
		DexTotal    int              `json:"dex_total,omitempty"`
		Milestones  []string         `json:"milestones,omitempty"`
	}
	type publicSprite struct {
		X uint8 `json:"x"`
		Y uint8 `json:"y"`
	}
	type publicRun struct {
		RunID          string          `json:"run_id"`
		Status         string          `json:"status"`
		Starter        string          `json:"starter,omitempty"`
		Dest           string          `json:"dest,omitempty"`
		Goal           string          `json:"goal,omitempty"`
		FPS            int             `json:"fps"`
		LLMProfile     string          `json:"llm_profile,omitempty"`
		PlayStyle      string          `json:"play_style,omitempty"`
		RiskTolerance  string          `json:"risk_tolerance,omitempty"`
		WildEncounters string          `json:"wild_encounters,omitempty"`
		QueuedAt       int64           `json:"queued_at,omitempty"`
		EndedAt        int64           `json:"ended_at,omitempty"`
		Frame          uint64          `json:"frame"`
		Map            uint8           `json:"map"`
		X              uint8           `json:"x"`
		Y              uint8           `json:"y"`
		Decision       string          `json:"decision,omitempty"`
		StopSoFar      string          `json:"stop_so_far,omitempty"`
		Stats          *spectatorStats `json:"stats,omitempty"`
		Player         *publicPlayer   `json:"player,omitempty"`
		Sprites        []publicSprite  `json:"sprites,omitempty"`
		Trail          [][2]uint8      `json:"trail,omitempty"`
		Attempts       int             `json:"attempts,omitempty"`
		Reason         string          `json:"reason,omitempty"`
		ReplayReady    bool            `json:"replay_ready,omitempty"`
		Highlight      string          `json:"highlight,omitempty"`
	}

	var player *publicPlayer
	if run.Player != nil {
		player = &publicPlayer{
			Money:       run.Player.Money,
			Badges:      append([]string(nil), run.Player.Badges...),
			Party:       make([]publicPartyMon, len(run.Player.Party)),
			BagUsed:     run.Player.BagUsed,
			BagCapacity: run.Player.BagCapacity,
			DexOwned:    run.Player.DexOwned,
			DexSeen:     run.Player.DexSeen,
			DexTotal:    run.Player.DexTotal,
			Milestones:  append([]string(nil), run.Player.Milestones...),
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
		if len(run.Player.Bag) > 0 {
			player.Bag = make([]publicBagItem, len(run.Player.Bag))
			for i, item := range run.Player.Bag {
				player.Bag[i] = publicBagItem{Name: item.Name, Quantity: item.Quantity}
			}
		}
	}

	sprites := make([]publicSprite, len(run.Sprites))
	for i, sp := range run.Sprites {
		sprites[i] = publicSprite{X: sp.X, Y: sp.Y}
	}
	presentation := spectatorPresentationPolicyForRun(run.RunID)

	return json.Marshal(publicRun{
		RunID:          run.RunID,
		Status:         run.Status,
		Starter:        run.Starter,
		Dest:           run.Dest,
		Goal:           run.Goal,
		FPS:            presentation.FPS,
		LLMProfile:     presentation.LLMProfile,
		PlayStyle:      presentation.PlayStyle,
		RiskTolerance:  presentation.RiskTolerance,
		WildEncounters: presentation.WildEncounters,
		QueuedAt:       run.QueuedAt,
		EndedAt:        run.EndedAt,
		Frame:          run.Frame,
		Map:            run.Map,
		X:              run.X,
		Y:              run.Y,
		Decision:       run.Decision,
		StopSoFar:      run.StopSoFar,
		Stats:          run.Stats,
		Player:         player,
		Sprites:        sprites,
		Trail:          append([][2]uint8(nil), run.Trail...),
		Attempts:       run.Attempts,
		Reason:         run.Reason,
		ReplayReady:    run.ReplayReady,
		Highlight:      run.Highlight,
	})
}
