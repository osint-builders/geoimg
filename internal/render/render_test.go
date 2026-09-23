package render

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/osint-builders/geoimg/internal/geo"
	"github.com/osint-builders/geoimg/internal/tiles"
	"github.com/osint-builders/geoimg/internal/tiles/tilestest"
)

func TestPlanChunks(t *testing.T) {
	w := geo.Window{Z: 18, X0: 1000, Y0: 2000, X1: 1000 + 5000, Y1: 2000 + 3000}
	single := PlanChunks(w, 8192)
	if len(single.Chunks) != 1 || single.Chunks[0].Window != w || single.ChunkName(single.Chunks[0]) != "" {
		t.Fatalf("%+v", single)
	}
	l := PlanChunks(w, 2048)
	area, tilesSeen := 0, map[geo.Tile]bool{}
	for _, c := range l.Chunks {
		if c.Window.Width() > 2048+256 || c.Window.Height() > 2048+256 {
			t.Fatalf("chunk too big: %+v", c.Window)
		}
		area += c.Window.Width() * c.Window.Height()
		for y := c.Tiles.Y0; y <= c.Tiles.Y1; y++ {
			for x := c.Tiles.X0; x <= c.Tiles.X1; x++ {
				tl := geo.Tile{Z: 18, X: x, Y: y}
				if tilesSeen[tl] {
					t.Fatalf("tile %v in two chunks", tl)
				}
				tilesSeen[tl] = true
			}
		}
	}
	if area != w.Width()*w.Height() || len(tilesSeen) != w.Tiles().Count() {
		t.Fatalf("chunks cover %d px / %d tiles, want %d / %d", area, len(tilesSeen), w.Width()*w.Height(), w.Tiles().Count())
	}
	if l.Rows*l.Cols != len(l.Chunks) || l.ChunkName(l.Chunks[len(l.Chunks)-1]) != "_r01_c02" {
		t.Fatalf("%dx%d %q", l.Rows, l.Cols, l.ChunkName(l.Chunks[len(l.Chunks)-1]))
	}
}

func TestMask(t *testing.T) {
	// A square with a square hole, projected at z0 where pixel = degrees/360*256.
	sq := func(a, b float64) []geo.LonLat {
		return []geo.LonLat{{a, a}, {b, a}, {b, b}, {a, b}, {a, a}}
	}
	m := NewMask([]geo.Polygon{{sq(-40, 40), sq(-10, 10)}}, 0)
	w := geo.Window{Z: 0, X0: 0, Y0: 0, X1: 256, Y1: 256}
	a := m.Render(w)
	at := func(lon, lat float64) uint8 {
		return a.AlphaAt(int(geo.LonToPixelX(lon, 0)), int(geo.LatToPixelY(lat, 0))).A
	}
	if at(-30, 30) != 255 || at(0, 0) != 0 || at(60, 60) != 0 {
		t.Fatalf("inside=%d hole=%d outside=%d", at(-30, 30), at(0, 0), at(60, 60))
	}
	if !Any(a, image.Rect(100, 100, 110, 110)) || Any(a, image.Rect(0, 0, 20, 20)) {
		t.Fatal("Any")
	}
}

func TestRenderStitchesExactPixels(t *testing.T) {
	srv := tilestest.New(17, tilestest.NotFound)
	defer srv.Close()
	src := tiles.NewSource(tiles.NewFetcher(srv.Template(), "", "test", 8))
	// A window deliberately not aligned to tile edges.
	w := geo.Window{Z: 17, X0: 37488*256 + 37, Y0: 50144*256 + 201, X1: 37488*256 + 37 + 700, Y1: 50144*256 + 201 + 450}
	dir := t.TempDir()
	out := filepath.Join(dir, "x.png")
	res, err := Render(context.Background(), src, PlanChunks(w, 8192), Options{Out: out, Format: PNG, Quality: 90, Workers: 4, WorldFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Native != int64(w.Tiles().Count()) || len(res.Files) != 1 {
		t.Fatalf("%+v", res)
	}
	f, _ := os.Open(out)
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil || img.Bounds().Dx() != 700 || img.Bounds().Dy() != 450 {
		t.Fatalf("%v %v", img.Bounds(), err)
	}
	// Compare sampled pixels to the source world (tiles are JPEG, so allow noise).
	for _, p := range []image.Point{{0, 0}, {218, 54}, {219, 55}, {699, 449}, {350, 300}} {
		gx, gy := w.X0+p.X, w.Y0+p.Y
		tl := geo.Tile{Z: 17, X: gx / 256, Y: gy / 256}
		want := tilestest.Render(tl, 17).RGBAAt(gx%256, gy%256)
		r, g, b, _ := img.At(p.X, p.Y).RGBA()
		if d := absDiff(int(r>>8), int(want.R)) + absDiff(int(g>>8), int(want.G)) + absDiff(int(b>>8), int(want.B)); d > 45 {
			t.Errorf("pixel %v = %d,%d,%d want %v", p, r>>8, g>>8, b>>8, want)
		}
	}
	// World file: pixel size and top-left pixel center in EPSG:3857.
	data, err := os.ReadFile(filepath.Join(dir, "x.pgw"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Fields(string(data))
	res0, _ := strconv.ParseFloat(lines[0], 64)
	cx, _ := strconv.ParseFloat(lines[4], 64)
	wantX, _ := geo.PixelToMercator(float64(w.X0)+0.5, 0, 17)
	if abs(res0-geo.MetersPerPixel(17)) > 1e-6 || abs(cx-wantX) > 1e-3 {
		t.Fatalf("world file %q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.prj")); err != nil {
		t.Fatal(err)
	}
}

func TestRenderChunksAndOverview(t *testing.T) {
	srv := tilestest.New(16, tilestest.NotFound)
	defer srv.Close()
	src := tiles.NewSource(tiles.NewFetcher(srv.Template(), "", "test", 8))
	w := geo.Window{Z: 16, X0: 18744*256 + 10, Y0: 25072*256 + 10, X1: 18744*256 + 10 + 1300, Y1: 25072*256 + 10 + 700}
	out := filepath.Join(t.TempDir(), "big.webp")
	l := PlanChunks(w, 512)
	res, err := Render(context.Background(), src, l, Options{Out: out, Format: WebP, Quality: 80, Workers: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != len(l.Chunks) || len(l.Chunks) < 6 || res.Overview == nil {
		t.Fatalf("files %d chunks %d overview %v", len(res.Files), len(l.Chunks), res.Overview)
	}
	for _, f := range res.Files {
		if _, err := os.Stat(f.Path); err != nil || len(f.SHA256) != 64 {
			t.Fatalf("%+v %v", f, err)
		}
	}
	if res.Overview.Width != 1300 || res.Overview.Height != 700 {
		t.Fatalf("overview %+v", res.Overview)
	}
}

func TestFormatFromPath(t *testing.T) {
	for in, want := range map[string]Format{"a.webp": WebP, "b.JPG": JPEG, "c.jpeg": JPEG, "d.png": PNG} {
		if f, err := FormatFromPath(in); err != nil || f != want {
			t.Errorf("%s: %v %v", in, f, err)
		}
	}
	if _, err := FormatFromPath("x.tif"); err == nil {
		t.Error("tif should be rejected")
	}
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
