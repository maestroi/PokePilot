// Command ramdiff compares two raw 64 KiB Game Boy address-space snapshots.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

const addressSpaceSize = 1 << 16

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("ramdiff", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	start := fs.Uint("start", 0, "first address to compare (decimal or 0xHEX)")
	end := fs.Uint("end", 0xffff, "last address to compare, inclusive (decimal or 0xHEX)")
	limit := fs.Int("limit", 256, "maximum changed addresses to print; 0 means unlimited")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("ramdiff: %w", err)
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("usage: ramdiff [-start 0xC000] [-end 0xDFFF] [-limit 256] BEFORE.ram AFTER.ram")
	}
	if *start > *end || *end >= addressSpaceSize {
		return fmt.Errorf("ramdiff: invalid range %#x..%#x; address space is 0x0000..0xFFFF", *start, *end)
	}
	if *limit < 0 {
		return fmt.Errorf("ramdiff: -limit must be >= 0")
	}

	before, err := loadRAM(fs.Arg(0))
	if err != nil {
		return err
	}
	after, err := loadRAM(fs.Arg(1))
	if err != nil {
		return err
	}

	changed, shown := 0, 0
	for addr := int(*start); addr <= int(*end); addr++ {
		if before[addr] == after[addr] {
			continue
		}
		changed++
		if *limit == 0 || shown < *limit {
			fmt.Fprintf(out, "0x%04X  %02X -> %02X\n", addr, before[addr], after[addr])
			shown++
		}
	}
	if changed == 0 {
		fmt.Fprintln(out, "no changes")
		return nil
	}
	if shown < changed {
		fmt.Fprintf(out, "... %d more changed address(es) not shown\n", changed-shown)
	}
	fmt.Fprintf(out, "%d changed address(es) in 0x%04X..0x%04X\n", changed, *start, *end)
	return nil
}

func loadRAM(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ramdiff: read %s: %w", path, err)
	}
	if len(b) != addressSpaceSize {
		return nil, fmt.Errorf("ramdiff: %s is %d bytes, want exactly %d", path, len(b), addressSpaceSize)
	}
	return b, nil
}
