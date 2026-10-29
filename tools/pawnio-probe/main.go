// pawnio-probe: read-only diagnostic for the Go PawnIO backend.
// Run elevated; writes results to %TEMP%\go_pawnio_probe.txt.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
	"github.com/buglyz/ecc/internal/pawnio"
)

func main() {
	module := flag.String("module", "", "LpcACPIEC.bin path (default: assets/pawnio/LpcACPIEC.bin near the executable)")
	flag.Parse()

	modulePath := *module
	if modulePath == "" {
		modulePath = paths.Discover(controller.AppName).PawnIOModule
	}
	if modulePath == "" {
		fmt.Fprintln(os.Stderr, "LpcACPIEC.bin not found; pass -module")
		os.Exit(2)
	}

	outPath := filepath.Join(os.Getenv("TEMP"), "go_pawnio_probe.txt")
	out, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer out.Close()

	logger := log.New(out, "", 0)
	dev, err := pawnio.Open(*module, logger)
	if err != nil {
		fmt.Fprintf(out, "OPEN ERROR: %v\n", err)
		return
	}
	defer dev.Close()

	ctx := context.Background()
	for _, reg := range []byte{0x2C, 0x2D, 0x2E, 0x2F, 0x58, 0x59} {
		v, err := dev.ReadEC(ctx, reg)
		fmt.Fprintf(out, "0x%02X = 0x%02X (%d) err=%v\n", reg, v, v, err)
	}
	fmt.Fprintln(out, "DONE")
}
