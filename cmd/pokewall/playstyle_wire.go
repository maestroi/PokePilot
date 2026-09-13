package main

import (
	"encoding/json"

	"github.com/maestroi/pokepilot/farm"
)

func playStyleForWallRun(runID, resumeFrom string) string {
	if style := farm.PlayStyleForRun(runID); style != "" {
		return style
	}
	if resumeFrom != "" {
		farm.CopyPlayStyle(resumeFrom, runID)
		return farm.PlayStyleForRun(runID)
	}
	return ""
}

// MarshalJSON adds the optional gameplay policy to dashboard rows while
// keeping tileRow's historical Go shape source-compatible.
func (r tileRow) MarshalJSON() ([]byte, error) {
	type plain tileRow
	return json.Marshal(struct {
		plain
		PlayStyle string `json:"play_style,omitempty"`
	}{plain: plain(r), PlayStyle: playStyleForWallRun(r.RunID, r.ResumeFromRunID)})
}

// persistedTile carries play_style through a wall restart. Old state files
// simply omit it and retain legacy Speedrun behavior.
func (p persistedTile) MarshalJSON() ([]byte, error) {
	type plain persistedTile
	return json.Marshal(struct {
		plain
		PlayStyle string `json:"play_style,omitempty"`
	}{plain: plain(p), PlayStyle: playStyleForWallRun(p.RunID, p.ResumeFromRunID)})
}

func (p *persistedTile) UnmarshalJSON(data []byte) error {
	type plain persistedTile
	var in struct {
		plain
		PlayStyle string `json:"play_style,omitempty"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	*p = persistedTile(in.plain)
	farm.RememberPlayStyle(p.RunID, in.PlayStyle)
	return nil
}
