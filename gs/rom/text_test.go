package rom

import "testing"

func TestDecodeTextEnglishAlphabetAndTerminator(t *testing.T) {
	raw := encodeEnglish("BULBASAUR")
	if got := DecodeText(raw); got != "BULBASAUR" {
		t.Fatalf("DecodeText(BULBASAUR)=%q", got)
	}
	if got := DecodeText(append(raw, 0x80, 0x81)); got != "BULBASAUR" {
		t.Fatalf("DecodeText should stop at @, got %q", got)
	}
}

func TestDecodeTextGen2Punctuation(t *testing.T) {
	// PKMN symbol, male/female, hyphen, digits.
	raw := []byte{0x54, 0xe3, 0xf7, 0xef, 0xf5, Terminator}
	if got := DecodeText(raw); got != "POKé-1♂♀" {
		t.Fatalf("DecodeText punctuation = %q", got)
	}
}
