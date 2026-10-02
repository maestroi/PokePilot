// Package rom parses static Gold/Silver data out of the ROM image.
//
// Table addresses are located from structure, not a committed .sym, so Gold
// and Silver share the decoders. Generic code consumes the results only
// through worldmodel.NativeMapTopologyProvider and game.ROMParser.
package rom
