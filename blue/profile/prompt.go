package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodePrompt(r game.MemoryReader) game.PromptState {
	return p.engine.DecodePrompt(r)
}
