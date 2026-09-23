// Package cli implements the geoimg command line.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/osint-builders/geoimg/internal/geo"
	"github.com/osint-builders/geoimg/internal/render"
	"github.com/osint-builders/geoimg/internal/tiles"
)

// Exit codes.
const (
	ExitOK      = 0
	ExitError   = 1
	ExitUsage   = 2
	ExitPartial = 3 // output written, but some tiles had no imagery at all
)

// Defaults. The zoom defaults favour the sharpest imagery available: probing
// starts at Esri's deepest level and steps down only when imagery is missing,
// a placeholder, or merely an upsampled copy of a coarser level.
const (
	DefaultMaxZoom     = 23
	DefaultMinZoom     = 1
	DefaultRadius      = 200.0
	DefaultQuality     = 90
	DefaultWorkers     = 16
	DefaultMaxTiles    = 20000
	DefaultChunk       = 8192
	DefaultCoverage    = 0.5
	DefaultDetailRatio = 0.2
)

type config struct {
	out, url, token                     string
	radius, coverage, detail            float64
	maxZoom, minZoom, quality, workers  int
	maxTiles, chunk                     int
	clip, world, dryRun, quiet, version bool
}

// Run executes the CLI and returns the process exit code.
func Run(ctx context.Context, version string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var c config
	fs := flag.NewFlagSet("geoimg", flag.ContinueOnError)
	fs.SetOutput(stderr)
	str := func(p *string, def, usage string, names ...string) {
		for _, n := range names {
			fs.StringVar(p, n, def, usage)
		}
	}
	num := func(p *int, def int, usage string, names ...string) {
		for _, n := range names {
			fs.IntVar(p, n, def, usage)
		}
	}
	boolean := func(p *bool, usage string, names ...string) {
		for _, n := range names {
			fs.BoolVar(p, n, false, usage)
		}
	}
	str(&c.out, "", "output file; extension picks the format (.webp .jpg .png)", "o", "out")
	fs.Float64Var(&c.radius, "r", DefaultRadius, "radius in meters around a point target")
	fs.Float64Var(&c.radius, "radius", DefaultRadius, "radius in meters around a point target")
	num(&c.maxZoom, DefaultMaxZoom, "highest zoom to try", "z", "zoom")
	num(&c.quality, DefaultQuality, "quality 1-100 (.webp at 100 = lossless)", "q", "quality")
	num(&c.workers, DefaultWorkers, "concurrent downloads", "j", "workers")
	boolean(&c.clip, "mask the image to GeoJSON polygons", "clip")
	boolean(&c.world, "write world file + .prj for GIS", "world")
	boolean(&c.dryRun, "show what would be downloaded, then exit", "n", "dry-run")
	boolean(&c.quiet, "no progress output", "quiet")
	boolean(&c.version, "print version and exit", "version")
	num(&c.minZoom, DefaultMinZoom, "lowest acceptable zoom", "min-zoom")
	num(&c.maxTiles, DefaultMaxTiles, "safety ceiling on tiles per run", "max-tiles")
	num(&c.chunk, DefaultChunk, "max pixels per image side before splitting into chunks", "chunk")
	fs.Float64Var(&c.coverage, "coverage", DefaultCoverage, "fraction of probe tiles that must have imagery to accept a zoom")
	fs.Float64Var(&c.detail, "detail", DefaultDetailRatio, "upsampling detector threshold (0 = off)")
	str(&c.url, tiles.DefaultURL, "tile URL template with {z} {x} {y}", "url")
	str(&c.token, "", "ArcGIS API key / token (default $ARCGIS_API_KEY)", "token")
	fs.Usage = func() { fmt.Fprint(stderr, usage) }

	flagArgs, pos := splitArgs(fs, args)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitUsage
	}
	if c.version {
		fmt.Fprintln(stdout, "geoimg", version)
		return ExitOK
	}
	if len(pos) != 1 {
		fmt.Fprint(stderr, usage)
		if len(pos) > 1 {
			fmt.Fprintf(stderr, "\nerror: expected one target, got %d: %s\n(no spaces inside coordinates, or quote them)\n", len(pos), strings.Join(pos, " "))
		}
		return ExitUsage
	}
	if c.token == "" {
		c.token = os.Getenv("ARCGIS_API_KEY")
	}
	if err := c.validate(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitUsage
	}
	target, err := geo.ParseTarget(pos[0], c.radius, stdin)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitUsage
	}
	if c.clip && len(target.Polygons) == 0 {
		fmt.Fprintln(stderr, "error: -clip needs a GeoJSON target containing Polygon or MultiPolygon geometry")
		return ExitUsage
	}
	if c.out == "" {
		c.out = "geoimg-" + time.Now().Format("20060102-150405") + ".webp"
	}
	format, err := render.FormatFromPath(c.out)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitUsage
	}
	if c.chunk > format.MaxDim() {
		fmt.Fprintf(stderr, "error: -chunk %d exceeds the %s limit of %d px per side\n", c.chunk, format, format.MaxDim())
		return ExitUsage
	}
	if c.dryRun {
		dryRun(stdout, c, target)
		return ExitOK
	}
	code, err := execute(ctx, c, version, target, format, stdout, stderr)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(stderr, "\ninterrupted")
		} else {
			fmt.Fprintln(stderr, "\nerror:", err)
		}
	}
	return code
}

func (c *config) validate() error {
	switch {
	case c.maxZoom < 0 || c.maxZoom > 23:
		return fmt.Errorf("-zoom must be 0-23, got %d", c.maxZoom)
	case c.minZoom < 0 || c.minZoom > c.maxZoom:
		return fmt.Errorf("-min-zoom must be between 0 and -zoom (%d), got %d", c.maxZoom, c.minZoom)
	case c.quality < 1 || c.quality > 100:
		return fmt.Errorf("-q must be 1-100, got %d", c.quality)
	case c.workers < 1 || c.workers > 64:
		return fmt.Errorf("-j must be 1-64, got %d", c.workers)
	case c.maxTiles < 1:
		return errors.New("-max-tiles must be positive")
	case c.chunk < geo.TileSize:
		return fmt.Errorf("-chunk must be at least %d", geo.TileSize)
	case c.coverage <= 0 || c.coverage > 1:
		return errors.New("-coverage must be in (0, 1]")
	case c.detail < 0:
		return errors.New("-detail must be >= 0")
	case !strings.Contains(c.url, "{z}") || !strings.Contains(c.url, "{x}") || !strings.Contains(c.url, "{y}"):
		return errors.New("-url must contain {z}, {x} and {y}")
	}
	return nil
}

func execute(ctx context.Context, c config, version string, target geo.Target, format render.Format, stdout, stderr io.Writer) (int, error) {
	start := time.Now()
	if dir := filepath.Dir(c.out); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return ExitError, err
		}
	}
	logf := func(format string, a ...any) {
		if !c.quiet {
			fmt.Fprintf(stderr, format, a...)
		}
	}

	ua := fmt.Sprintf("geoimg/%s (+https://github.com/osint-builders/geoimg)", version)
	fetcher := tiles.NewFetcher(c.url, c.token, ua, c.workers)
	src := tiles.NewSource(fetcher)

	logf("probing zoom levels %d→%d …\n", c.maxZoom, c.minZoom)
	zoom, attempts, err := tiles.SelectZoom(ctx, src, target.BBox, tiles.ZoomOptions{
		MaxZoom: c.maxZoom, MinZoom: c.minZoom, MaxTiles: c.maxTiles,
		MinCoverage: c.coverage, DetailRatio: c.detail, Workers: c.workers, Batch: 3,
	})
	if err != nil {
		return ExitError, err
	}
	src.Forget(func(t geo.Tile) bool { return t.Z <= zoom })

	win := target.BBox.PixelWindow(zoom)
	layout := render.PlanChunks(win, c.chunk)
	lat, _ := target.BBox.Center()
	logf("zoom %d (%.2f m/px) · %d×%d px · %d tiles · %d image(s)\n",
		zoom, geo.GroundResolution(lat, zoom), win.Width(), win.Height(), win.Tiles().Count(), len(layout.Chunks))

	var mask *render.Mask
	if c.clip {
		mask = render.NewMask(target.Polygons, zoom)
	}
	prog := newProgress(stderr, c.quiet)
	res, err := render.Render(ctx, src, layout, render.Options{
		Out: c.out, Format: format, Quality: c.quality, WorldFile: c.world,
		Mask: mask, Workers: c.workers, Progress: prog.update,
	})
	prog.finish()
	if err != nil {
		return ExitError, err
	}

	bounds := win.BBox()
	minx, miny := geo.PixelToMercator(float64(win.X0), float64(win.Y1), zoom)
	maxx, maxy := geo.PixelToMercator(float64(win.X1), float64(win.Y0), zoom)
	m := Metadata{
		Tool: "geoimg", Version: version, CreatedUTC: start.UTC().Format(time.RFC3339),
		Source: MetaSource{Provider: "Esri World Imagery", URLTemplate: c.url, Attribution: tiles.Attribution},
		Target: MetaTarget{Kind: target.Kind, Input: target.Input, BBox: target.BBox, Polygons: len(target.Polygons), Clip: c.clip},
		Zoom:   MetaZoom{Selected: zoom, MaxTried: c.maxZoom, Min: c.minZoom, Attempts: attempts},
		Resolution: MetaResolution{
			GroundMetersPerPixel:   round(geo.GroundResolution(lat, zoom), 4),
			MercatorMetersPerPixel: round(geo.MetersPerPixel(zoom), 6),
		},
		Grid: MetaGrid{Tiles: win.Tiles(), TileCount: win.Tiles().Count(), PixelWidth: win.Width(), PixelHeight: win.Height(),
			ChunkRows: layout.Rows, ChunkCols: layout.Cols, ChunkMaxSide: c.chunk},
		Bounds:     MetaBounds{WGS84: bounds, EPSG3857: [4]float64{round(minx, 3), round(miny, 3), round(maxx, 3), round(maxy, 3)}},
		Output:     MetaOutput{Format: string(format), Quality: c.quality, Files: res.Files, Overview: res.Overview},
		Tiles:      res.Stats,
		GapTiles:   res.GapTiles,
		Network:    MetaNetwork{Requests: fetcher.Requests.Load(), Bytes: fetcher.Bytes.Load(), Workers: c.workers},
		ElapsedSec: round(time.Since(start).Seconds(), 3),
		Complete:   res.Stats.Empty == 0,
	}
	if target.Kind == geo.KindPoint {
		m.Target.RadiusM = c.radius
	}
	metaPath := strings.TrimSuffix(c.out, filepath.Ext(c.out)) + ".json"
	if err := writeJSON(metaPath, m); err != nil {
		return ExitError, err
	}

	var total int64
	for _, f := range res.Files {
		total += f.Bytes
	}
	if len(res.Files) == 1 {
		fmt.Fprintf(stdout, "%s  %d×%d  %s\n", res.Files[0].Path, res.Files[0].Width, res.Files[0].Height, humanBytes(total))
	} else {
		fmt.Fprintf(stdout, "%d images (%d×%d grid)  %s total\n", len(res.Files), layout.Rows, layout.Cols, humanBytes(total))
		if res.Overview != nil {
			fmt.Fprintf(stdout, "%s  %d×%d  overview\n", res.Overview.Path, res.Overview.Width, res.Overview.Height)
		}
	}
	fmt.Fprintf(stdout, "%s  metadata\n", metaPath)
	s := res.Stats
	logf("zoom %d · %d native, %d gap-filled, %d placeholder, %d empty · %s downloaded in %.1fs\n",
		zoom, s.Native, s.GapFilled, s.Placeholder, s.Empty, humanBytes(m.Network.Bytes), time.Since(start).Seconds())
	if s.Empty > 0 {
		fmt.Fprintf(stderr, "warning: %d tile(s) had no imagery at any zoom (see gap_tiles in %s)\n", s.Empty, metaPath)
		return ExitPartial, nil
	}
	return ExitOK, nil
}

func dryRun(w io.Writer, c config, t geo.Target) {
	b := t.BBox
	lat, _ := b.Center()
	fmt.Fprintf(w, "target   %s (%s)\n", t.Input, t.Kind)
	fmt.Fprintf(w, "bbox     lat %.6f…%.6f  lon %.6f…%.6f\n", b.MinLat, b.MaxLat, b.MinLon, b.MaxLon)
	fmt.Fprintf(w, "output   %s (chunks ≤ %d px)\n\n", c.out, c.chunk)
	fmt.Fprintf(w, "%4s  %9s  %8s  %13s  %7s  %s\n", "zoom", "m/px", "tiles", "pixels", "images", "note")
	for z := c.maxZoom; z >= c.minZoom; z-- {
		win := b.PixelWindow(z)
		n := win.Tiles().Count()
		l := render.PlanChunks(win, c.chunk)
		note := ""
		if n > c.maxTiles {
			note = "over -max-tiles budget"
		}
		fmt.Fprintf(w, "%4d  %9.3f  %8d  %6d×%-6d  %7d  %s\n", z, geo.GroundResolution(lat, z), n, win.Width(), win.Height(), len(l.Chunks), note)
		if n <= 1 {
			break
		}
	}
	fmt.Fprintln(w, "\nThe highest zoom within budget is probed first; geoimg steps down only where imagery is missing or upsampled.")
}

type progress struct {
	w       io.Writer
	enabled bool
	mu      sync.Mutex
	last    time.Time
	shown   bool
}

func newProgress(w io.Writer, quiet bool) *progress {
	p := &progress{w: w}
	if f, ok := w.(*os.File); ok && !quiet {
		if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
			p.enabled = true
		}
	}
	return p
}

func (p *progress) update(done, total int) {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if done != total && time.Since(p.last) < 100*time.Millisecond {
		return
	}
	p.last, p.shown = time.Now(), true
	fmt.Fprintf(p.w, "\r  tiles %d/%d (%d%%)", done, total, done*100/max(total, 1))
}

func (p *progress) finish() {
	if p.shown {
		fmt.Fprintln(p.w)
	}
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func round(v float64, places int) float64 {
	p := 1.0
	for range places {
		p *= 10
	}
	return float64(int64(v*p+0.5*sign(v))) / p
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

const usage = `geoimg — download and stitch high-resolution Esri World Imagery

Usage:
  geoimg [flags] <target>

Target (one of):
  38.8977,-77.0365                 point: lat,lon (area = -r meters around it)
  40.748,-74.00,40.750,-73.98      box: two opposite corners, lat,lon,lat,lon
  area.geojson                     GeoJSON file (Polygon, MultiPolygon, Point, …)
  -                                GeoJSON from stdin

Common flags:
  -o FILE      output image; extension picks format: .webp (default) .jpg .png
  -r METERS    radius around a point target (default 200)
  -z ZOOM      highest zoom to try, 0-23 (default 23 = sharpest available)
  -q QUALITY   1-100 (default 90; .webp with 100 = lossless)
  -clip        mask output to the GeoJSON polygon(s)
  -world       also write a world file + .prj (EPSG:3857) for GIS tools
  -n           dry run: show zoom/tile/size estimates, download nothing
  -j N         concurrent downloads (default 16)
  -quiet       no progress output
  -version     print version

Advanced flags:
  -min-zoom Z      lowest acceptable zoom (default 1)
  -max-tiles N     safety ceiling on tiles per run (default 20000);
                   zoom is lowered automatically to stay under it
  -chunk PX        split output into a grid of images when larger than
                   PX on a side (default 8192)
  -coverage F      fraction of probe tiles needing imagery to accept a zoom (default 0.5)
  -detail F        upsampling detector: min fine-detail ratio for a zoom to
                   count as native imagery (default 0.2, 0 = off)
  -url TEMPLATE    tile URL with {z} {x} {y} (default Esri World Imagery)
  -token KEY       ArcGIS API key (default $ARCGIS_API_KEY)

Outputs:
  <out>            the image (or <out>_rNN_cNN.* chunks + <out>_overview.* if large)
  <out>.json       metadata: zoom decisions, bounds, resolution, sha256, provenance

Exit codes: 0 ok, 1 error, 2 usage, 3 written but some tiles had no imagery.
Imagery © Esri, Maxar, Earthstar Geographics, and the GIS User Community.
`
