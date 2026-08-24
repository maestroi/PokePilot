package main

import (
	"fmt"
	"os"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// Gen 1 WRAM/IO addresses not in the sym package.
const (
	wFadeoutMode uint16 = 0xD838
	wMapStatus   uint16 = 0xD828
	ioLY         uint16 = 0xFF46
	ioSTA        uint16 = 0xFF47
	ioBKPD       uint16 = 0xFF80
	ioM5L        uint16 = 0xFF49
)

func dump(label string, e *emu.Emu) {
	fmt.Printf("%-12s map=%02x pos=(%d,%d) walk=%d joy=%02x fade=%02x mapStatus=%02x m5l=%02x sta=%02x ly=%02x bgp=%02x\n",
		label,
		e.Peek8(sym.CurMap), e.Peek8(sym.XCoord), e.Peek8(sym.YCoord),
		e.Peek8(sym.WalkCounter), e.Peek8(sym.JoyIgnore),
		e.Peek8(wFadeoutMode), e.Peek8(wMapStatus),
		e.Peek8(ioM5L), e.Peek8(ioSTA), e.Peek8(ioLY), e.Peek8(ioBKPD))
}

func main() {
	romPath := os.Getenv("POKEMON_RED_ROM")
	e, err := emu.Open(romPath)
	if err != nil {
		panic(err)
	}
	defer e.Close()
	if _, err := skill.BootToOverworld(e); err != nil {
		panic(err)
	}
	romData := e.ROM()
	g, err := world.BuildGraph(romData)
	if err != nil {
		panic(err)
	}
	for _, from := range []uint8{0x26, 0x25} {
		route, err := world.FindRoute(g, from, 0x00)
		if err != nil {
			panic(err)
		}
		if err := skill.Traverse(e, romData, route[0]); err != nil {
			panic(fmt.Sprintf("Traverse %02x->00: %v", from, err))
		}
	}
	dump("arrival", e)

	// Dump raw Pallet Town map header to decode connection data.
	bankOff := 3*0x4000 + (0x423D - 0x4000)
	bank := romData[bankOff]
	ptrOff := 0x01AE
	ptr := uint16(romData[ptrOff]) | uint16(romData[ptrOff+1])<<8
	hdrOff := int(bank)*0x4000 + int(ptr-0x4000)
	fmt.Printf("map00 header: bank=%02x ptr=%04x off=%04x\n", bank, ptr, hdrOff)
	var hdr [32]byte
	copy(hdr[:], romData[hdrOff:hdrOff+32])
	fmt.Printf("hdr bytes: ")
	for _, b := range hdr {
		fmt.Printf("%02x ", b)
	}
	fmt.Println()

	h, err := rom.ParseMap(romData, 0x00)
	if err != nil {
		panic(err)
	}
	fmt.Printf("connections: ")
	for _, c := range h.Connections {
		fmt.Printf("(dir=%d->map=%02x) ", c.Dir, c.MapID)
	}
	fmt.Println()

	grid, err := world.Build(romData, h)
	if err != nil {
		panic(err)
	}
	ax, ay := int(e.Peek8(sym.XCoord)), int(e.Peek8(sym.YCoord))
	steps, err := world.FindPath(grid, ax, ay, 11, 2, nil)
	if err != nil {
		panic(err)
	}
	if err := skill.WalkPath(e, steps); err != nil {
		panic(err)
	}
	dump("at (11,2)", e)
	if err := skill.StepOnce(e, world.StepUp); err != nil {
		panic(err)
	}
	dump("at (11,1)", e)

	// Press Up and trace per-frame.
	e.Press(emu.Up)
	for i := 0; i < 40; i++ {
		e.StepFrame()
		if i < 12 || i%10 == 0 {
			fmt.Printf("  f%2d pos=(%d,%d) walk=%d joy=%02x fade=%02x m5l=%02x sta=%02x ly=%02x bgp=%02x\n",
				i, e.Peek8(sym.XCoord), e.Peek8(sym.YCoord),
				e.Peek8(sym.WalkCounter), e.Peek8(sym.JoyIgnore),
				e.Peek8(wFadeoutMode), e.Peek8(ioM5L), e.Peek8(ioSTA), e.Peek8(ioLY), e.Peek8(ioBKPD))
		}
	}
	e.Release(emu.Up)
	dump("after up", e)
}
