// Package controller contains Pokémon Yellow-owned input controllers.
//
// These controllers may consume Yellow ROM/RAM semantics, but they expose
// semantic operations to the generic agent. They intentionally do not reuse
// Red skills whose native state decoders still read red/sym addresses.
package controller

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/world"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	mapPalletTown  uint8 = 0x00
	mapRedsHouse1F uint8 = 0x25
	mapRedsHouse2F uint8 = 0x26
	mapOaksLab     uint8 = 0x28

	openingFrameBudget = 90000
	battleFrameBudget  = 30000
	stepFrameBudget    = 90
	warpFrameBudget    = 240
)

type openingState struct {
	mapID        uint8
	x, y         uint8
	controllable bool
	inBattle     bool
	partyCount   int
	hasPikachu   bool
	starter      bool
	labRival     bool
}

type openingPhase string

const (
	openingPhaseDone              openingPhase = "done"
	openingPhaseBattle            openingPhase = "battle"
	openingPhaseNickname          openingPhase = "nickname"
	openingPhaseScript            openingPhase = "script"
	openingPhaseBedroomUpstairs   openingPhase = "bedroom-upstairs"
	openingPhaseBedroomDownstairs openingPhase = "bedroom-downstairs"
	openingPhaseOakGate           openingPhase = "oak-gate"
	openingPhaseEeveeBall         openingPhase = "eevee-ball"
	openingPhaseAwaitStarter      openingPhase = "await-starter"
	openingPhaseRivalTrigger      openingPhase = "rival-trigger"
	openingPhaseUnexpected        openingPhase = "unexpected"
)

func openingPhaseFor(state openingState, nickname bool) openingPhase {
	if state.labRival {
		if state.starter && state.hasPikachu && state.controllable && !state.inBattle {
			return openingPhaseDone
		}
		if !state.starter || !state.hasPikachu {
			return openingPhaseUnexpected
		}
	}
	if state.inBattle {
		return openingPhaseBattle
	}
	if nickname {
		return openingPhaseNickname
	}
	if !state.controllable {
		return openingPhaseScript
	}
	if !state.starter {
		switch state.mapID {
		case mapRedsHouse2F:
			return openingPhaseBedroomUpstairs
		case mapRedsHouse1F:
			return openingPhaseBedroomDownstairs
		case mapPalletTown:
			return openingPhaseOakGate
		case mapOaksLab:
			if state.partyCount == 0 {
				return openingPhaseEeveeBall
			}
			return openingPhaseAwaitStarter
		default:
			return openingPhaseUnexpected
		}
	}
	if state.mapID == mapOaksLab {
		return openingPhaseRivalTrigger
	}
	return openingPhaseUnexpected
}

func observeOpening(m *emu.Emu, romData []byte) (openingState, error) {
	obs, err := yellowprofile.New().DecodeObservation(m, romData)
	if err != nil {
		return openingState{}, err
	}
	hasPikachu := false
	for _, mon := range obs.Party {
		if mon.Species == "pikachu" {
			hasPikachu = true
			break
		}
	}
	return openingState{
		mapID:        uint8(obs.NativeMapID),
		x:            obs.X,
		y:            obs.Y,
		controllable: obs.Controllable,
		inBattle:     obs.InBattle,
		partyCount:   len(obs.Party),
		hasPikachu:   hasPikachu,
		starter:      obs.Story.Has(yellowprofile.ProgressYellowStarterReceived),
		labRival:     obs.Story.Has(yellowprofile.ProgressYellowLabRivalResolved),
	}, nil
}

// GetPikachuStarter drives Yellow's real opening story. Unlike Red there is no
// starter-ball choice: Oak catches the scripted wild Pikachu, the rival grabs
// Eevee when the player examines its ball, and Oak then gives Pikachu.
//
// The driver is state based and therefore resumable from a checkpoint anywhere
// in the opening. Success is the durable Yellow lab-rival progress fact plus a
// stable overworld boundary.
func GetPikachuStarter(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("yellow opening: nil emulator")
	}
	if info := game.InspectROM(romData); info.SHA1 != sym.ROMSHA1 {
		return fmt.Errorf("yellow opening: ROM sha1=%s, want %s", info.SHA1, sym.ROMSHA1)
	}

	start := m.FrameCount()
	for int(m.FrameCount()-start) <= openingFrameBudget {
		state, err := observeOpening(m, romData)
		if err != nil {
			return fmt.Errorf("yellow opening: observe: %w", err)
		}
		phase := openingPhaseFor(state, nicknamePrompt(m))
		switch phase {
		case openingPhaseDone:
			return nil
		case openingPhaseBattle:
			if state.starter {
				if _, err := Battle(m, romData); err != nil {
					return fmt.Errorf("yellow opening phase %s: resolve lab rival battle: %w", phase, err)
				}
			} else {
				if err := advanceScriptedCaptureBattle(m, romData); err != nil {
					return fmt.Errorf("yellow opening phase %s: resolve scripted Pikachu capture: %w", phase, err)
				}
			}
		case openingPhaseNickname:
			if err := declineNickname(m); err != nil {
				return fmt.Errorf("yellow opening phase %s: %w", phase, err)
			}
		case openingPhaseScript, openingPhaseAwaitStarter:
			advanceScriptFrame(m)
		case openingPhaseBedroomUpstairs:
			if err := takeWarp(m, romData, 7, 1, mapRedsHouse1F); err != nil {
				return fmt.Errorf("yellow opening phase %s: %w", phase, err)
			}
		case openingPhaseBedroomDownstairs:
			if err := takeWarp(m, romData, 2, 7, mapPalletTown); err != nil {
				return fmt.Errorf("yellow opening phase %s: %w", phase, err)
			}
		case openingPhaseOakGate:
			// PalletTownDefaultScript fires when the player reaches y=0.
			// x=10 is the canonical right-hand exit used by the script.
			if err := walkTo(m, romData, 10, 0, nil); err != nil {
				return fmt.Errorf("yellow opening phase %s: reach Oak gate: %w", phase, err)
			}
		case openingPhaseEeveeBall:
			// Once Oak's choose-mon speech releases control, examining the
			// Eevee ball at (7,3) is the only player-owned interaction.
			if err := interactAt(m, romData, 7, 3); err != nil {
				return fmt.Errorf("yellow opening phase %s: trigger Eevee-ball script: %w", phase, err)
			}
		case openingPhaseRivalTrigger:
			// Oak has given Pikachu. The lab rival challenges when the player
			// reaches row 6; (5,6) is open floor and works from either side of Oak.
			if err := walkTo(m, romData, 5, 6, nil); err != nil {
				// The rival challenge opens dialogue as the destination row is
				// reached. A text/script takeover after real movement is the
				// expected transition, so trust the live control state rather
				// than the stale walk error.
				after, obsErr := observeOpening(m, romData)
				if obsErr != nil || (after.controllable && !after.inBattle) {
					return fmt.Errorf("yellow opening phase %s: reach rival trigger: %w", phase, err)
				}
			}
		case openingPhaseUnexpected:
			return fmt.Errorf(
				"yellow opening phase %s: map=%#02x (%d,%d) controllable=%v battle=%v party=%d pikachu=%v starter=%v lab_rival=%v",
				phase, state.mapID, state.x, state.y, state.controllable, state.inBattle,
				state.partyCount, state.hasPikachu, state.starter, state.labRival)
		default:
			return fmt.Errorf("yellow opening: unknown phase %q", phase)
		}
	}

	state, _ := observeOpening(m, romData)
	return fmt.Errorf(
		"yellow opening: exceeded %d frames at map=%#02x (%d,%d) controllable=%v battle=%v party=%d starter=%v lab_rival=%v",
		openingFrameBudget, state.mapID, state.x, state.y, state.controllable,
		state.inBattle, state.partyCount, state.starter, state.labRival)
}

func advanceScriptFrame(m *emu.Emu) {
	if m.Peek8(sym.FontLoaded) != 0 {
		m.Tap(emu.A, 3, 7)
		return
	}
	m.StepFrame()
}

func screenText(m *emu.Emu) string {
	buf := make([]byte, sym.TileMapLen)
	m.PeekInto(sym.TileMap, buf)
	return gen1.NormalizeDisplayText(gen1.DecodeTiles(buf))
}

func nicknamePrompt(m *emu.Emu) bool {
	return m.Peek8(sym.MaxMenuItem) == 1 &&
		strings.Contains(strings.ToLower(screenText(m)), "give a nickname")
}

func declineNickname(m *emu.Emu) error {
	if !nicknamePrompt(m) {
		return fmt.Errorf("yellow opening: nickname prompt disappeared before selection")
	}
	if m.Peek8(sym.CurrentMenuItem) != 1 {
		m.Tap(emu.Down, 3, 7)
		if _, err := m.StepUntil(120, func(m *emu.Emu) bool {
			return m.Peek8(sym.CurrentMenuItem) == 1
		}); err != nil {
			return fmt.Errorf("yellow opening: nickname NO cursor did not settle: %w", err)
		}
	}
	m.Tap(emu.A, 3, 7)
	return nil
}

// confirmFirstMoveBattle is intentionally narrow: the opening has only the
// scripted Pikachu encounter and the one-Pokémon lab rival battle. Repeated A
// selects FIGHT and the first move from the default battle cursor, while also
// paging battle text. Losing the lab rival fight is a legal story outcome in
// Yellow; the durable progress flag, not the win bit, is the opening's goal.
func advanceScriptedCaptureBattle(m *emu.Emu, romData []byte) error {
	start := m.FrameCount()
	for int(m.FrameCount()-start) <= battleFrameBudget {
		state, err := observeOpening(m, romData)
		if err != nil {
			return fmt.Errorf("yellow opening scripted capture: observe: %w", err)
		}
		if !state.inBattle {
			return nil
		}
		// This is Oak's scripted capture, not a player battle: there is no
		// legal FIGHT/ITEM/PKMN/RUN decision to make. Advance only rendered
		// text pages; animation/script frames receive no input.
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("yellow opening scripted capture: exceeded %d frames", battleFrameBudget)
}

func staticObjectBlockers(h yellowrom.MapHeader, except *[2]int) map[[2]int]bool {
	blocked := make(map[[2]int]bool, len(h.Objects))
	for _, obj := range h.Objects {
		p := [2]int{int(obj.X), int(obj.Y)}
		if except != nil && p == *except {
			continue
		}
		blocked[p] = true
	}
	return blocked
}

func walkTo(m *emu.Emu, romData []byte, tx, ty int, except *[2]int) error {
	mapID := m.Peek8(sym.CurMap)
	h, err := yellowrom.ParseMap(romData, mapID)
	if err != nil {
		return fmt.Errorf("parse map %#02x: %w", mapID, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return fmt.Errorf("build map %#02x: %w", mapID, err)
	}
	sx, sy := int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord))
	steps, err := world.FindPath(grid, sx, sy, tx, ty, staticObjectBlockers(h, except))
	if err != nil {
		return fmt.Errorf("no path on map %#02x from (%d,%d) to (%d,%d): %w", mapID, sx, sy, tx, ty, err)
	}
	return walkPath(m, mapID, steps)
}

func interactAt(m *emu.Emu, romData []byte, tx, ty int) error {
	mapID := m.Peek8(sym.CurMap)
	h, err := yellowrom.ParseMap(romData, mapID)
	if err != nil {
		return err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return err
	}
	sx, sy := int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord))
	target := [2]int{tx, ty}
	steps, face, err := world.FindPathAdjacent(grid, sx, sy, tx, ty, staticObjectBlockers(h, &target))
	if err != nil {
		return err
	}
	if err := walkPath(m, mapID, steps); err != nil {
		return err
	}
	btn, ok := buttonFor(face)
	if !ok {
		return fmt.Errorf("invalid interaction direction %s", face)
	}
	// The blocked directional tap establishes facing; A owns the interaction.
	m.Tap(btn, 3, 7)
	m.Tap(emu.A, 3, 7)
	return nil
}

func takeWarp(m *emu.Emu, romData []byte, tx, ty int, wantMap uint8) error {
	mapID := m.Peek8(sym.CurMap)
	h, err := yellowrom.ParseMap(romData, mapID)
	if err != nil {
		return err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return err
	}
	sx, sy := int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord))
	target := [2]int{tx, ty}
	steps, push, err := world.FindPathAdjacent(grid, sx, sy, tx, ty, staticObjectBlockers(h, &target))
	if err != nil {
		return fmt.Errorf("path to warp (%d,%d): %w", tx, ty, err)
	}
	if err := walkPath(m, mapID, steps); err != nil {
		return err
	}
	btn, ok := buttonFor(push)
	if !ok {
		return fmt.Errorf("invalid warp direction %s", push)
	}
	m.Tap(btn, 3, 7)
	if m.Peek8(sym.CurMap) != wantMap {
		if _, err := m.StepUntil(warpFrameBudget, func(m *emu.Emu) bool {
			return m.Peek8(sym.CurMap) == wantMap
		}); err != nil {
			return fmt.Errorf("warp from map %#02x (%d,%d) did not reach %#02x: %w", mapID, tx, ty, wantMap, err)
		}
	}
	return nil
}

func walkPath(m *emu.Emu, mapID uint8, steps []world.Step) error {
	for _, step := range steps {
		if err := stepOnce(m, mapID, step); err != nil {
			return err
		}
	}
	return nil
}

func stepOnce(m *emu.Emu, mapID uint8, step world.Step) error {
	btn, ok := buttonFor(step)
	if !ok {
		return fmt.Errorf("invalid step %s", step)
	}
	startX, startY := int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord))
	wantX, wantY := startX+step.DX, startY+step.DY

	for attempt := 0; attempt < 2; attempt++ {
		m.Tap(btn, 3, 7)
		if m.Peek8(sym.CurMap) != mapID {
			return fmt.Errorf("map changed while walking %#02x", mapID)
		}
		if int(m.Peek8(sym.XCoord)) == wantX && int(m.Peek8(sym.YCoord)) == wantY {
			return nil
		}
		if _, err := m.StepUntil(stepFrameBudget, func(m *emu.Emu) bool {
			return m.Peek8(sym.CurMap) != mapID ||
				(int(m.Peek8(sym.XCoord)) == wantX && int(m.Peek8(sym.YCoord)) == wantY) ||
				m.Peek8(sym.IsInBattle) != 0 ||
				m.Peek8(sym.FontLoaded) != 0
		}); err == nil {
			if m.Peek8(sym.CurMap) != mapID {
				return fmt.Errorf("map changed while walking %#02x", mapID)
			}
			if m.Peek8(sym.IsInBattle) != 0 || m.Peek8(sym.FontLoaded) != 0 {
				return fmt.Errorf("movement interrupted on map %#02x at (%d,%d)", mapID,
					m.Peek8(sym.XCoord), m.Peek8(sym.YCoord))
			}
			if int(m.Peek8(sym.XCoord)) == wantX && int(m.Peek8(sym.YCoord)) == wantY {
				return nil
			}
		}
	}
	return fmt.Errorf("step %s blocked on map %#02x at (%d,%d)", step, mapID, startX, startY)
}

func buttonFor(step world.Step) (emu.Button, bool) {
	switch step {
	case world.StepUp:
		return emu.Up, true
	case world.StepDown:
		return emu.Down, true
	case world.StepLeft:
		return emu.Left, true
	case world.StepRight:
		return emu.Right, true
	default:
		return 0, false
	}
}
