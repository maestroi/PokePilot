package trade

import (
	"errors"
	"fmt"
	"strings"
)

const (
	NameLength       = 11
	PartyLength      = 6
	PartyMonSize     = 44
	TrainerBlockSize = 0x1a8
	PatchListSize    = 200

	PreambleByte  byte = 0xfd
	NoDataByte    byte = 0xfe
	PatchTermByte byte = 0xff

	trainerPreambleLength = 6
	trainerNameOffset     = trainerPreambleLength
	partyCountOffset      = trainerNameOffset + NameLength
	partySpeciesOffset    = partyCountOffset + 1
	partyMonsOffset       = partySpeciesOffset + PartyLength + 1
	partyOTOffset         = partyMonsOffset + PartyLength*PartyMonSize
	partyNickOffset       = partyOTOffset + PartyLength*NameLength
	patchEntriesOffset    = 10
	patchPart1Length      = 252 // 0xfd is reserved as a serial preamble value.
)

// Mon is the exact Generation-I party representation carried over the link.
// Raw is the 44-byte party struct. OT and Nick are the fixed-width encoded
// original-trainer and nickname strings that follow the party structs.
type Mon struct {
	Raw  [PartyMonSize]byte
	OT   [NameLength]byte
	Nick [NameLength]byte
}

func (m Mon) Species() byte { return m.Raw[0] }

// Trainer is the virtual peer's link-visible trainer identity and party.
type Trainer struct {
	Name  [NameLength]byte
	Party []Mon
}

// NewTrainer builds a trainer from ordinary ASCII-ish names. Generation-I
// names are at most ten visible characters plus the 0x50 terminator.
func NewTrainer(name string, party ...Mon) Trainer {
	return Trainer{Name: EncodeText(name), Party: append([]Mon(nil), party...)}
}

// EncodeText encodes the subset of the Generation-I character map needed by
// generated trainer/Pokemon names. Unsupported runes become spaces instead of
// leaking arbitrary bytes into the link payload.
func EncodeText(s string) [NameLength]byte {
	var out [NameLength]byte
	for i := range out {
		out[i] = 0x50
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	pos := 0
	for _, r := range s {
		if pos >= NameLength-1 {
			break
		}
		var b byte
		switch {
		case r >= 'A' && r <= 'Z':
			b = 0x80 + byte(r-'A')
		case r >= '0' && r <= '9':
			b = 0xf6 + byte(r-'0')
		case r == ' ':
			b = 0x7f
		default:
			b = 0x7f
		}
		out[pos] = b
		pos++
	}
	out[pos] = 0x50
	return out
}

// LinkData returns the exact 424-byte trainer block and 200-byte patch list
// used by the Red/Blue Cable Club. 0xfe bytes inside party structs are escaped
// to 0xff and restored by the receiver from the two-part patch list.
func (t Trainer) LinkData() ([TrainerBlockSize]byte, [PatchListSize]byte, error) {
	var block [TrainerBlockSize]byte
	var patch [PatchListSize]byte
	if len(t.Party) < 1 || len(t.Party) > PartyLength {
		return block, patch, fmt.Errorf("gen1 trade: party size %d outside 1..%d", len(t.Party), PartyLength)
	}
	for i := 0; i < trainerPreambleLength; i++ {
		block[i] = PreambleByte
	}
	copy(block[trainerNameOffset:trainerNameOffset+NameLength], t.Name[:])
	block[partyCountOffset] = byte(len(t.Party))
	for i := 0; i < PartyLength+1; i++ {
		block[partySpeciesOffset+i] = PatchTermByte
	}
	for i, mon := range t.Party {
		if mon.Species() == 0 {
			return block, patch, fmt.Errorf("gen1 trade: party slot %d has species 0", i)
		}
		block[partySpeciesOffset+i] = mon.Species()
		copy(block[partyMonsOffset+i*PartyMonSize:], mon.Raw[:])
		copy(block[partyOTOffset+i*NameLength:], mon.OT[:])
		copy(block[partyNickOffset+i*NameLength:], mon.Nick[:])
	}

	patched, patch, err := PatchTrainerBlock(block)
	if err != nil {
		return block, patch, err
	}
	return patched, patch, nil
}

// PatchTrainerBlock applies the Cable Club's SERIAL_NO_DATA_BYTE escaping to
// the party-mon section and emits the exact two-part offset patch list.
func PatchTrainerBlock(block [TrainerBlockSize]byte) ([TrainerBlockSize]byte, [PatchListSize]byte, error) {
	var patch [PatchListSize]byte
	for i := 0; i < 3; i++ {
		patch[i] = PreambleByte
	}
	write := patchEntriesOffset
	add := func(v byte) error {
		if write >= len(patch) {
			return errors.New("gen1 trade: party patch list overflow")
		}
		patch[write] = v
		write++
		return nil
	}

	for i := 0; i < patchPart1Length; i++ {
		off := partyMonsOffset + i
		if block[off] != NoDataByte {
			continue
		}
		block[off] = PatchTermByte
		if err := add(byte(i + 1)); err != nil {
			return block, patch, err
		}
	}
	if err := add(PatchTermByte); err != nil {
		return block, patch, err
	}

	for i := patchPart1Length; i < PartyLength*PartyMonSize; i++ {
		off := partyMonsOffset + i
		if block[off] != NoDataByte {
			continue
		}
		block[off] = PatchTermByte
		if err := add(byte(i - patchPart1Length + 1)); err != nil {
			return block, patch, err
		}
	}
	if err := add(PatchTermByte); err != nil {
		return block, patch, err
	}
	return block, patch, nil
}

// UnpatchTrainerBlock restores escaped 0xfe bytes in a received trainer block.
func UnpatchTrainerBlock(block *[TrainerBlockSize]byte, patch []byte) error {
	if block == nil {
		return errors.New("gen1 trade: nil trainer block")
	}
	part := 0
	for _, v := range patch {
		switch v {
		case 0, PreambleByte, NoDataByte:
			continue
		case PatchTermByte:
			part++
			if part == 2 {
				return nil
			}
			continue
		}
		if part > 1 {
			return errors.New("gen1 trade: patch entry after second terminator")
		}
		index := int(v) - 1
		if part == 1 {
			index += patchPart1Length
		}
		if index < 0 || index >= PartyLength*PartyMonSize {
			return fmt.Errorf("gen1 trade: patch offset %d outside party data", index)
		}
		block[partyMonsOffset+index] = NoDataByte
	}
	return errors.New("gen1 trade: patch list missing two terminators")
}

// ParseTrainerBlock decodes a trainer block after applying its patch list.
func ParseTrainerBlock(block [TrainerBlockSize]byte, patch []byte) (Trainer, error) {
	if err := UnpatchTrainerBlock(&block, patch); err != nil {
		return Trainer{}, err
	}
	count := int(block[partyCountOffset])
	if count < 1 || count > PartyLength {
		return Trainer{}, fmt.Errorf("gen1 trade: received party count %d outside 1..%d", count, PartyLength)
	}
	var t Trainer
	copy(t.Name[:], block[trainerNameOffset:trainerNameOffset+NameLength])
	t.Party = make([]Mon, count)
	for i := 0; i < count; i++ {
		mon := &t.Party[i]
		copy(mon.Raw[:], block[partyMonsOffset+i*PartyMonSize:partyMonsOffset+(i+1)*PartyMonSize])
		copy(mon.OT[:], block[partyOTOffset+i*NameLength:partyOTOffset+(i+1)*NameLength])
		copy(mon.Nick[:], block[partyNickOffset+i*NameLength:partyNickOffset+(i+1)*NameLength])
		listed := block[partySpeciesOffset+i]
		if listed != mon.Species() {
			return Trainer{}, fmt.Errorf("gen1 trade: slot %d species list %#02x != struct %#02x", i, listed, mon.Species())
		}
	}
	return t, nil
}

func cloneTrainer(t Trainer) Trainer {
	out := t
	out.Party = append([]Mon(nil), t.Party...)
	return out
}
