package rom

import "strings"

// Terminator is the Gen 2 string terminator (pokegold constants/charmap.asm).
const Terminator = 0x50

// English printable characters from the Gold/Silver font. Control bytes and
// the Japanese overlays that reuse the same indexes are not decoded here.
var gen2English = [256]string{
	0x00: " ",
	0x54: "POKé",
	0x75: "…",
	0x7f: " ",
	0x80: "A", 0x81: "B", 0x82: "C", 0x83: "D", 0x84: "E",
	0x85: "F", 0x86: "G", 0x87: "H", 0x88: "I", 0x89: "J",
	0x8a: "K", 0x8b: "L", 0x8c: "M", 0x8d: "N", 0x8e: "O",
	0x8f: "P", 0x90: "Q", 0x91: "R", 0x92: "S", 0x93: "T",
	0x94: "U", 0x95: "V", 0x96: "W", 0x97: "X", 0x98: "Y",
	0x99: "Z",
	0x9a: "(", 0x9b: ")", 0x9c: ":", 0x9d: ";", 0x9e: "[", 0x9f: "]",
	0xa0: "a", 0xa1: "b", 0xa2: "c", 0xa3: "d", 0xa4: "e",
	0xa5: "f", 0xa6: "g", 0xa7: "h", 0xa8: "i", 0xa9: "j",
	0xaa: "k", 0xab: "l", 0xac: "m", 0xad: "n", 0xae: "o",
	0xaf: "p", 0xb0: "q", 0xb1: "r", 0xb2: "s", 0xb3: "t",
	0xb4: "u", 0xb5: "v", 0xb6: "w", 0xb7: "x", 0xb8: "y",
	0xb9: "z",
	0xc0: "Ä", 0xc1: "Ö", 0xc2: "Ü", 0xc3: "ä", 0xc4: "ö", 0xc5: "ü",
	0xd0: "'d", 0xd1: "'l", 0xd2: "'m", 0xd3: "'r", 0xd4: "'s", 0xd5: "'t", 0xd6: "'v",
	0xdf: "←",
	0xe0: "'", 0xe3: "-",
	0xe6: "?", 0xe7: "!", 0xe8: ".", 0xe9: "&",
	0xea: "é", 0xeb: "→",
	0xef: "♂", 0xf0: "¥", 0xf1: "×", 0xf2: ".", 0xf3: "/", 0xf4: ",", 0xf5: "♀",
	0xf6: "0", 0xf7: "1", 0xf8: "2", 0xf9: "3", 0xfa: "4",
	0xfb: "5", 0xfc: "6", 0xfd: "7", 0xfe: "8", 0xff: "9",
}

// DecodeText maps a Gen 2 encoded string to UTF-8, stopping at @ or raw.
func DecodeText(raw []byte) string {
	var b strings.Builder
	for _, c := range raw {
		if c == Terminator {
			break
		}
		if s := gen2English[c]; s != "" {
			b.WriteString(s)
			continue
		}
	}
	return b.String()
}

func encodeEnglish(s string) []byte {
	out := make([]byte, 0, len(s)+1)
	for _, r := range s {
		found := byte(0)
		for i, ch := range gen2English {
			if ch == string(r) && i >= 0x80 {
				found = byte(i)
				break
			}
		}
		if found != 0 {
			out = append(out, found)
		}
	}
	out = append(out, Terminator)
	return out
}
