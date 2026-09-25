// Package canon presents a Generation-I game's work RAM in the canonical
// Gen-I engine layout.
//
// Red and Blue share one engine and one RAM layout; the Gen-I adapter code in
// red/ (state decoders, skills, observation) addresses RAM through red/sym.
// Yellow runs a revision of that engine: most WRAM symbols sit one byte lower,
// a handful are resized, some are gone, and the event and toggleable-object
// bit arrays are renumbered. A View is the per-game fact that bridges the
// difference: it answers a read addressed in canonical (red/sym) coordinates
// from the game's native RAM, so shared Gen-I decoders read another Gen-I
// revision without learning its addresses.
//
// A View is generated from the vendored decompilations (see cmd/gen1canongen)
// and owned by the game profile whose layout it describes. Red and Blue need
// none: their native layout is canonical.
//
// Canonical bytes that have no native counterpart read as zero rather than
// aliasing a neighbouring native variable. Reads outside work RAM and HRAM
// (ROM, VRAM, cartridge RAM, I/O registers) pass through unchanged.
package canon

import "sort"

// Span maps Len canonical bytes starting at Canon to native bytes starting at
// Native.
type Span struct {
	Canon  uint16
	Native uint16
	Len    uint16
}

// FlagArray remaps a bit array whose indices are numbered differently by the
// canonical and native games (event flags, toggleable-object flags).
// CanonToNative[i] is the native bit holding canonical bit i, or -1 when the
// native game has no such flag.
type FlagArray struct {
	Canon         uint16
	Native        uint16
	CanonToNative []int16
}

// IndexList is a byte list whose entries at ValueOffset (stepping Stride, up
// to Len bytes, stopping at Terminator) are indices into a FlagArray and must
// be renumbered into canonical indices. NativeToCanon[i] is the canonical
// index for native index i.
type IndexList struct {
	Canon         uint16
	Native        uint16
	Len           uint16
	Stride        uint16
	ValueOffset   uint16
	Terminator    byte
	NativeToCanon []int16
}

// View is one game's canonical Gen-I memory mapping.
type View struct {
	Spans []Span // sorted by Canon, non-overlapping
	Flags []FlagArray
	Lists []IndexList
}

const (
	wramStart = 0xC000
	echoStart = 0xE000
	echoEnd   = 0xFE00
	hramStart = 0xFF80
	hramEnd   = 0xFFFF // IE register at 0xFFFF is hardware, not HRAM
)

// mapped reports whether canonical addr is translated by this view (work RAM
// or HRAM) rather than passed through.
func mapped(addr int) bool {
	return (addr >= wramStart && addr < echoEnd) || (addr >= hramStart && addr < hramEnd)
}

// Native returns the native address holding canonical addr, and false when
// the canonical byte has no native counterpart. Addresses outside work RAM
// and HRAM translate to themselves.
func (v *View) Native(addr uint16) (uint16, bool) {
	a := int(addr)
	if !mapped(a) {
		return addr, true
	}
	if a >= echoStart && a < echoEnd {
		a -= echoStart - wramStart
	}
	i := sort.Search(len(v.Spans), func(i int) bool {
		s := v.Spans[i]
		return int(s.Canon)+int(s.Len) > a
	})
	if i == len(v.Spans) || int(v.Spans[i].Canon) > a {
		return 0, false
	}
	s := v.Spans[i]
	return s.Native + uint16(a-int(s.Canon)), true
}

// ReadInto fills dst with canonical bytes starting at canonical addr, reading
// the game through native. It never writes through native.
func (v *View) ReadInto(native func(addr uint16, dst []byte), addr uint16, dst []byte) {
	if len(dst) == 0 {
		return
	}
	end := int(addr) + len(dst)
	if !v.touches(int(addr), end) {
		native(addr, dst)
		return
	}
	if len(dst) <= smallRead {
		r := nativeByte{read: native}
		for i := range dst {
			dst[i] = v.byteAt(&r, int(addr)+i)
		}
		return
	}
	var raw [0x10000]byte
	native(0, raw[:])
	v.compose(&raw, int(addr), dst)
}

// smallRead is the largest read served byte by byte. Larger reads (snapshots,
// tile maps) take one native snapshot and compose the canonical image.
const smallRead = 64

type nativeByte struct {
	read func(addr uint16, dst []byte)
	buf  [1]byte
}

func (r *nativeByte) at(addr int) byte {
	r.read(uint16(addr), r.buf[:])
	return r.buf[0]
}

// byteAt is the single-byte form of compose; tests hold the two equivalent.
func (v *View) byteAt(r *nativeByte, addr int) byte {
	if addr > 0xffff {
		return 0
	}
	if !mapped(addr) {
		return r.at(addr)
	}
	if addr >= echoStart && addr < echoEnd {
		addr -= echoStart - wramStart
	}
	for _, f := range v.Flags {
		if addr < int(f.Canon) || addr >= int(f.Canon)+(len(f.CanonToNative)+7)/8 {
			continue
		}
		var out byte
		base := (addr - int(f.Canon)) * 8
		for bit := 0; bit < 8 && base+bit < len(f.CanonToNative); bit++ {
			nat := f.CanonToNative[base+bit]
			if nat >= 0 && r.at(int(f.Native)+int(nat)/8)&(1<<(uint(nat)%8)) != 0 {
				out |= 1 << uint(bit)
			}
		}
		return out
	}
	for _, l := range v.Lists {
		if addr < int(l.Canon) || addr >= int(l.Canon)+int(l.Len) {
			continue
		}
		off := uint16(addr - int(l.Canon))
		for head := uint16(0); head+l.ValueOffset < l.Len && head <= off; head += l.Stride {
			if r.at(int(l.Native)+int(head)) == l.Terminator {
				return r.at(int(l.Native) + int(off))
			}
			if head+l.ValueOffset == off {
				val := r.at(int(l.Native) + int(off))
				if int(val) < len(l.NativeToCanon) && l.NativeToCanon[val] >= 0 {
					return byte(l.NativeToCanon[val])
				}
				return 0xff
			}
		}
		return r.at(int(l.Native) + int(off))
	}
	nat, ok := v.Native(uint16(addr))
	if !ok {
		return 0
	}
	return r.at(int(nat))
}

// touches reports whether canonical [start,end) needs translation.
func (v *View) touches(start, end int) bool {
	if end > 0x10000 {
		end = 0x10000
	}
	return (start < echoEnd && end > wramStart) || (start < hramEnd && end > hramStart)
}

// compose writes canonical [start, start+len(dst)) into dst from a full
// native snapshot.
func (v *View) compose(raw *[0x10000]byte, start int, dst []byte) {
	var canon [0x10000]byte
	copy(canon[:wramStart], raw[:wramStart])
	copy(canon[hramEnd:], raw[hramEnd:])
	copy(canon[echoEnd:hramStart], raw[echoEnd:hramStart])
	for _, s := range v.Spans {
		copy(canon[s.Canon:int(s.Canon)+int(s.Len)], raw[s.Native:int(s.Native)+int(s.Len)])
	}
	for _, f := range v.Flags {
		n := (len(f.CanonToNative) + 7) / 8
		for i := 0; i < n; i++ {
			canon[int(f.Canon)+i] = 0
		}
		for c, nat := range f.CanonToNative {
			if nat < 0 {
				continue
			}
			if raw[int(f.Native)+int(nat)/8]&(1<<(uint(nat)%8)) != 0 {
				canon[int(f.Canon)+c/8] |= 1 << (uint(c) % 8)
			}
		}
	}
	for _, l := range v.Lists {
		for off := uint16(0); off+l.ValueOffset < l.Len; off += l.Stride {
			head := raw[int(l.Native)+int(off)]
			if head == l.Terminator {
				break
			}
			val := raw[int(l.Native)+int(off+l.ValueOffset)]
			out := byte(0xff)
			if int(val) < len(l.NativeToCanon) && l.NativeToCanon[val] >= 0 {
				out = byte(l.NativeToCanon[val])
			}
			canon[int(l.Canon)+int(off+l.ValueOffset)] = out
		}
	}
	copy(canon[echoStart:echoEnd], canon[wramStart:wramStart+(echoEnd-echoStart)])
	copy(dst, canon[start:])
}
