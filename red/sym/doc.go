// Package sym contains generated Pokemon Red RAM/ROM addresses.
//
// The constants here are verified against testdata/pokered.sym, a committed
// snapshot of the RAM and HRAM symbols emitted by an RGBDS build of the
// pokered decompilation. Regenerate it against a newer decomp with:
//
//	mkdir -p red/sym/testdata
//	{
//	  printf '; RAM and HRAM symbols from the pokered decompilation, RGBDS .sym format.\n'
//	  printf '; Filtered to addresses >= 0xC000; see doc.go for the regeneration command.\n'
//	  printf '; pokered commit: %s\n' "$(git -C ~/.cache/pokered rev-parse HEAD)"
//	  grep -E '^[0-9a-fA-F]{2}:[C-Fc-f]' ~/.cache/pokered/pokered.sym
//	} > red/sym/testdata/pokered.sym
//
// Prefer deriving struct sizes and field offsets from symbol differences
// (PartyMonSize is wPartyMon2 - wPartyMon1) over copying magic numbers.
package sym
