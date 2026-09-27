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
	type publicTetrisPiece struct {
		Piece    string `json:"piece,omitempty"`
		Rotation int    `json:"rotation,omitempty"`
		X        int    `json:"x,omitempty"`
		Y        int    `json:"y,omitempty"`
	}
	type publicTetrisState struct {
		Kind               string             `json:"kind,omitempty"`
		Mode               string             `json:"mode,omitempty"`
		Screen             string             `json:"screen,omitempty"`
		Board              []string           `json:"board,omitempty"`
		Level              int                `json:"level,omitempty"`
		Score              int                `json:"score,omitempty"`
		ScoreValid         bool               `json:"score_valid,omitempty"`
		LinesCleared       int                `json:"lines_cleared,omitempty"`
		LinesRemaining     int                `json:"lines_remaining,omitempty"`
		LineGoal           int                `json:"line_goal,omitempty"`
		Paused             bool               `json:"paused,omitempty"`
		Locking            bool               `json:"locking,omitempty"`
		Clearing           bool               `json:"clearing,omitempty"`
		GameOver           bool               `json:"game_over,omitempty"`
		Complete           bool               `json:"complete,omitempty"`
		ReadyForPieceInput bool               `json:"ready_for_piece_input,omitempty"`
		Active             *publicTetrisPiece `json:"active,omitempty"`
		Next               *publicTetrisPiece `json:"next,omitempty"`
	}
	type publicRun struct {
		RunID          string             `json:"run_id"`
		Status         string             `json:"status"`
		Game           string             `json:"game,omitempty"`
		Starter        string             `json:"starter,omitempty"`
		Dest           string             `json:"dest,omitempty"`
		Goal           string             `json:"goal,omitempty"`
		FPS            int                `json:"fps"`
		LLMProfile     string             `json:"llm_profile,omitempty"`
		PlayStyle      string             `json:"play_style,omitempty"`
		Purpose        string             `json:"purpose,omitempty"`
		RiskTolerance  string             `json:"risk_tolerance,omitempty"`
		WildEncounters string             `json:"wild_encounters,omitempty"`
		QueuedAt       int64              `json:"queued_at,omitempty"`
		EndedAt        int64              `json:"ended_at,omitempty"`
		Frame          uint64             `json:"frame"`
		Map            uint8              `json:"map"`
		X              uint8              `json:"x"`
		Y              uint8              `json:"y"`
		MapsVisited    int                `json:"maps_visited,omitempty"`
		PlannerWaiting bool               `json:"planner_waiting,omitempty"`
		PlannerOptions int                `json:"planner_options,omitempty"`
		Decision       string             `json:"decision,omitempty"`
		StopSoFar      string             `json:"stop_so_far,omitempty"`
		Stats          *spectatorStats    `json:"stats,omitempty"`
		Player         *publicPlayer      `json:"player,omitempty"`
		GameState      *publicTetrisState `json:"game_state,omitempty"`
		Sprites        []publicSprite     `json:"sprites,omitempty"`
		Trail          [][2]uint8         `json:"trail,omitempty"`
		Attempts       int                `json:"attempts,omitempty"`
		Reason         string             `json:"reason,omitempty"`
		ReplayReady    bool               `json:"replay_ready,omitempty"`
		Highlight      string             `json:"highlight,omitempty"`
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

	// game_state is a game-owned envelope on the private wall. Only copy the
	// small Tetris presentation contract that the public UI needs; arbitrary
	// keys (including future backend/model metadata) stay behind this boundary.
	var gameState *publicTetrisState
	if run.Game == "tetris" && len(run.GameState) > 0 {
		encoded, err := json.Marshal(run.GameState)
		if err != nil {
			return nil, err
		}
		var decoded publicTetrisState
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			return nil, err
		}
		if decoded.Kind == "tetris" {
			switch decoded.Mode {
			case "unknown", "type-a", "type-b", "versus":
			default:
				decoded.Mode = "unknown"
			}
			switch decoded.Screen {
			case "unknown", "intro", "title", "game-type-select", "music-select", "level-select", "height-select", "high-score", "starting", "playing", "versus-setup", "game-over", "ending":
			default:
				decoded.Screen = "unknown"
			}
			sanitizePiece := func(piece *publicTetrisPiece) {
				if piece == nil {
					return
				}
				switch piece.Piece {
				case "L", "J", "I", "O", "S", "Z", "T":
				default:
					piece.Piece = ""
				}
				if piece.Rotation < 0 || piece.Rotation > 3 {
					piece.Rotation = 0
				}
			}
			sanitizePiece(decoded.Active)
			sanitizePiece(decoded.Next)
			if len(decoded.Board) > 18 {
				decoded.Board = decoded.Board[:18]
			}
			for i, row := range decoded.Board {
				cells := []byte("..........")
				for x := 0; x < len(cells) && x < len(row); x++ {
					if row[x] == '#' {
						cells[x] = '#'
					}
				}
				decoded.Board[i] = string(cells)
			}
			gameState = &decoded
		}
	}
	presentation := spectatorPresentationPolicyForRun(run.RunID)

	return json.Marshal(publicRun{
		RunID:          run.RunID,
		Status:         run.Status,
		Game:           run.Game,
		Starter:        run.Starter,
		Dest:           run.Dest,
		Goal:           run.Goal,
		FPS:            presentation.FPS,
		LLMProfile:     presentation.LLMProfile,
		PlayStyle:      presentation.PlayStyle,
		Purpose:        presentation.Purpose,
		RiskTolerance:  presentation.RiskTolerance,
		WildEncounters: presentation.WildEncounters,
		QueuedAt:       run.QueuedAt,
		EndedAt:        run.EndedAt,
		Frame:          run.Frame,
		Map:            run.Map,
		X:              run.X,
		Y:              run.Y,
		MapsVisited:    run.MapsVisited,
		PlannerWaiting: run.PlannerWaiting,
		PlannerOptions: run.PlannerOptions,
		Decision:       run.Decision,
		StopSoFar:      run.StopSoFar,
		Stats:          run.Stats,
		Player:         player,
		GameState:      gameState,
		Sprites:        sprites,
		Trail:          append([][2]uint8(nil), run.Trail...),
		Attempts:       run.Attempts,
		Reason:         run.Reason,
		ReplayReady:    run.ReplayReady,
		Highlight:      run.Highlight,
	})
}
