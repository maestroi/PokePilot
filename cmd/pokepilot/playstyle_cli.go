package main

import "flag"

// localPlayStyle is intentionally separate from -llm-profile: the latter
// chooses inference hardware, while this chooses gameplay priorities. Empty
// preserves the historical Speedrun behavior for old scripts/commands.
var localPlayStyle = flag.String("play-style", "", "gameplay policy for llm runs: speedrun, adventure, completionist, or team_builder")

func localPlayStyleName() string {
	if localPlayStyle == nil {
		return ""
	}
	return *localPlayStyle
}
