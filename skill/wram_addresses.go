package skill

import (
	redstate "github.com/maestroi/pokepilot/red/state"
	yellowstate "github.com/maestroi/pokepilot/yellow/state"
)

// wramAddresses is the WRAM address set for one image, and it is also the
// decoder address set: red/state.Addresses holds every address a Gen I decode
// reads, so the set the skill layer resolves for direct reads is the same set
// its decoders use. Red and Yellow share every decode rule but not the
// addresses, and Yellow shifted a contiguous WRAM region one byte lower, so
// reading or decoding with the wrong game's set is a silent failure, not a
// crash: the byte at Red's wPartyCount on a Yellow image is a real byte that
// decodes as a party of six.
//
// The skill layer therefore never names a game's sym package at a read site.
// It resolves the set once per ROM image (tablesForROM, cached) and both reads
// and decodes through it.
type wramAddresses = redstate.Addresses

// redWram is the address set for the supported Pokémon Red image.
func redWram() wramAddresses { return redstate.RedAddresses() }

// yellowWram is the address set for the supported Pokémon Yellow image.
// Every value comes from yellow/sym, generated from pokeyellow's symbol map.
func yellowWram() wramAddresses { return yellowstate.YellowAddresses() }
