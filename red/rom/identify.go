package rom

import (
	"crypto/sha1"
	"encoding/hex"

	"github.com/maestroi/pokepilot/red/sym"
)

// ErrUnknownROM reports a ROM that PokePilot does not know how to decode.
type ErrUnknownROM struct{ GotSHA1 string }

func (e *ErrUnknownROM) Error() string {
	return "unknown ROM: sha1 " + e.GotSHA1
}

// SHA1Hex returns the lowercase hex sha1 of a ROM image.
func SHA1Hex(rom []byte) string {
	sum := sha1.Sum(rom)
	return hex.EncodeToString(sum[:])
}

// Verify checks that rom is the Pokemon Red image PokePilot supports.
func Verify(rom []byte) error {
	if got := SHA1Hex(rom); got != sym.ROMSHA1 {
		return &ErrUnknownROM{GotSHA1: got}
	}
	return nil
}
