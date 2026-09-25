package renderstate

import (
	"fmt"
	"strings"
	"unicode"

	reddata "github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	protocol "github.com/maestroi/pokepilot/renderstate"
)

func semanticBattle(romData []byte, mem *state.Mem) *protocol.BattleState {
	battle := state.DecodeBattle(mem)
	if battle == nil {
		return nil
	}

	kind := "wild"
	if battle.Kind == state.BattleTrainer {
		kind = "trainer"
	}
	phase := "action"
	if state.MenuUp(mem) {
		phase = "menu"
	}

	playerName, playerAppearance := semanticSpecies(battle.ActiveSpecies)
	enemyName, enemyAppearance := semanticSpecies(battle.EnemySpecies)
	out := &protocol.BattleState{
		Kind:  kind,
		Phase: phase,
		Actors: []protocol.BattleActor{
			{
				ID:         "player-active",
				Role:       "player",
				Name:       playerName,
				Appearance: playerAppearance,
				Level:      int(battle.ActiveLevel),
				HP:         int(battle.ActiveHP),
				MaxHP:      int(battle.ActiveMaxHP),
				Status:     state.StatusName(mem.U8(sym.BattleMonStatus)),
				Active:     true,
				Defeated:   battle.ActiveHP == 0,
			},
			{
				ID:         "opponent-active",
				Role:       "opponent",
				Name:       enemyName,
				Appearance: enemyAppearance,
				Level:      int(battle.EnemyLevel),
				HP:         int(battle.EnemyHP),
				MaxHP:      int(battle.EnemyMaxHP),
				Status:     state.StatusName(mem.U8(sym.EnemyMonStatus)),
				Active:     true,
				Defeated:   battle.EnemyHP == 0,
			},
		},
	}

	for _, move := range battle.Moves {
		if move.ID == 0 {
			continue
		}
		name, ok := reddata.MoveName(move.ID)
		if !ok {
			name = fmt.Sprintf("move %d", move.ID)
		}
		maxPP := 0
		if info, err := rom.LookupMove(romData, move.ID); err == nil {
			maxPP = int(info.PP)
		}
		out.Moves = append(out.Moves, protocol.BattleMove{
			ID:       semanticToken(name),
			Name:     titleCaseSemantic(name),
			PP:       int(move.PP),
			MaxPP:    maxPP,
			Disabled: move.Disabled,
		})
	}
	return out
}

func semanticDialogue(mem *state.Mem) *protocol.DialogueState {
	dialogue := state.DecodeDialogue(mem)
	if dialogue == nil {
		return nil
	}
	text := strings.TrimSpace(dialogue.Text)
	if text == "" {
		return nil
	}
	return &protocol.DialogueState{Text: text}
}

func semanticMenu(mem *state.Mem) *protocol.MenuState {
	if !state.MenuUp(mem) {
		return nil
	}
	cursor := int(mem.U8(sym.CurrentMenuItem))
	max := int(mem.U8(sym.MaxMenuItem))
	if max < 0 {
		max = 0
	}
	entries := make([]protocol.MenuEntry, 0, max+1)
	for i := 0; i <= max; i++ {
		entries = append(entries, protocol.MenuEntry{
			ID: fmt.Sprintf("option-%d", i),
		})
	}
	return &protocol.MenuState{
		ID:      "red-menu",
		Title:   strings.TrimSpace(state.ScreenText(mem)),
		Cursor:  &cursor,
		Entries: entries,
	}
}

func semanticSpecies(raw uint8) (name, appearance string) {
	if value, ok := reddata.SpeciesName(raw); ok {
		return titleCaseSemantic(value), semanticToken(value)
	}
	return "Unknown Pokémon", "unknown"
}

func semanticToken(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), " ", "-")
}

func titleCaseSemantic(value string) string {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	for i, word := range words {
		if word == "" {
			continue
		}
		runes := []rune(word)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}
