// Command geoimg downloads and stitches high-resolution Esri World Imagery
// for a point, bounding box or GeoJSON area. Run `geoimg -h` for usage.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/osint-builders/geoimg/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, version, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
