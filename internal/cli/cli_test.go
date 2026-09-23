package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	_ "image/jpeg"
	_ "image/png"

	_ "github.com/gen2brain/webp"

	"github.com/osint-builders/geoimg/internal/geo"
	"github.com/osint-builders/geoimg/internal/tiles/tilestest"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = Run(context.Background(), "test", args, strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

func readMeta(t *testing.T, path string) Metadata {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m Metadata
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func decodeFile(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return img
}

func TestPointEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		name   string
		beyond tilestest.Beyond
	}{
		{"404", tilestest.NotFound},
		{"placeholder", tilestest.Grey},
		{"upsampled", tilestest.Upsample},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := tilestest.New(18, tc.beyond)
			defer srv.Close()
			dir := t.TempDir()
			out := filepath.Join(dir, "wh.webp")
			code, stdout, stderr := run(t, "38.8977,-77.0365", "-r", "60", "-o", out, "-url", srv.Template(), "-quiet")
			if code != ExitOK {
				t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			m := readMeta(t, filepath.Join(dir, "wh.json"))
			if m.Zoom.Selected != 18 {
				t.Fatalf("selected zoom %d, want 18; attempts: %+v", m.Zoom.Selected, m.Zoom.Attempts)
			}
			if len(m.Output.Files) != 1 || m.Output.Files[0].SHA256 == "" {
				t.Fatalf("files: %+v", m.Output.Files)
			}
			img := decodeFile(t, out)
			if b := img.Bounds(); b.Dx() != m.Grid.PixelWidth || b.Dy() != m.Grid.PixelHeight {
				t.Fatalf("image %v, metadata %dx%d", b, m.Grid.PixelWidth, m.Grid.PixelHeight)
			}
			if !m.Complete || m.Tiles.Native != int64(m.Grid.TileCount) {
				t.Fatalf("tiles: %+v of %d", m.Tiles, m.Grid.TileCount)
			}
		})
	}
}

func TestFlagsAfterNegativeCoordinates(t *testing.T) {
	srv := tilestest.New(17, tilestest.NotFound)
	defer srv.Close()
	out := filepath.Join(t.TempDir(), "syd.jpg")
	code, _, stderr := run(t, "-33.8568,151.2153", "-r", "40", "-o", out, "-url", srv.Template(), "-quiet", "-world")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, f := range []string{"syd.jpg", "syd.json", "syd.jgw", "syd.prj"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(out), f)); err != nil {
			t.Error(err)
		}
	}
	if m := readMeta(t, strings.TrimSuffix(out, ".jpg")+".json"); m.Target.RadiusM != 40 || m.Output.Format != "jpeg" {
		t.Fatalf("%+v", m.Target)
	}
}

func TestChunkedBBox(t *testing.T) {
	srv := tilestest.New(17, tilestest.Grey)
	defer srv.Close()
	dir := t.TempDir()
	out := filepath.Join(dir, "grid.png")
	code, stdout, stderr := run(
		t,
		"40.7480,-73.9900,40.7530,-73.9820",
		"-o",
		out,
		"-url",
		srv.Template(),
		"-chunk",
		"512",
		"-quiet",
	)
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	m := readMeta(t, filepath.Join(dir, "grid.json"))
	if m.Grid.ChunkRows*m.Grid.ChunkCols != len(m.Output.Files) || len(m.Output.Files) < 4 || m.Output.Overview == nil {
		t.Fatalf("grid %+v files %d overview %v", m.Grid, len(m.Output.Files), m.Output.Overview)
	}
	var w int
	for _, f := range m.Output.Files {
		if f.Row == 0 {
			w += f.Width
		}
	}
	if w != m.Grid.PixelWidth || !strings.Contains(stdout, "images") {
		t.Fatalf("row width %d != %d; stdout %q", w, m.Grid.PixelWidth, stdout)
	}
	ov := decodeFile(t, m.Output.Overview.Path)
	if ov.Bounds().Dx() != m.Grid.PixelWidth { // area < 4096 px, so overview is 1:1
		t.Fatalf("overview %v", ov.Bounds())
	}
}

func TestClipGeoJSON(t *testing.T) {
	srv := tilestest.New(17, tilestest.NotFound)
	defer srv.Close()
	dir := t.TempDir()
	gj := filepath.Join(dir, "tri.geojson")
	os.WriteFile(
		gj,
		[]byte(
			`{"type":"Feature","geometry":{"type":"Polygon","coordinates":[[[-77.05,38.88],[-77.02,38.88],[-77.05,38.90],[-77.05,38.88]]]}}`,
		),
		0o644,
	)
	out := filepath.Join(dir, "tri.png")
	code, _, stderr := run(t, gj, "-clip", "-o", out, "-url", srv.Template(), "-quiet")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	m := readMeta(t, filepath.Join(dir, "tri.json"))
	if !m.Target.Clip || m.Tiles.Skipped == 0 {
		t.Fatalf("expected tiles outside the triangle to be skipped: %+v", m.Tiles)
	}
	img := decodeFile(t, out)
	b := img.Bounds()
	if _, _, _, a := img.At(b.Max.X-2, b.Min.Y+1).RGBA(); a != 0 { // NE corner is outside the triangle
		t.Fatal("outside pixel not transparent")
	}
	if _, _, _, a := img.At(b.Min.X+2, b.Max.Y-2).RGBA(); a == 0 { // SW corner is inside
		t.Fatal("inside pixel transparent")
	}
}

func TestDryRunAndBudgetMakeNoRequests(t *testing.T) {
	srv := tilestest.New(17, tilestest.NotFound)
	defer srv.Close()
	code, stdout, _ := run(t, "38.8977,-77.0365", "-n", "-url", srv.Template())
	if code != ExitOK || !strings.Contains(stdout, "m/px") || srv.Requests.Load() != 0 {
		t.Fatalf("dry run: exit %d requests %d\n%s", code, srv.Requests.Load(), stdout)
	}
	code, _, stderr := run(
		t,
		"30,-100,45,-80",
		"-url",
		srv.Template(),
		"-min-zoom",
		"15",
		"-o",
		filepath.Join(t.TempDir(), "x.webp"),
	)
	if code != ExitError || !strings.Contains(stderr, "too large") || srv.Requests.Load() != 0 {
		t.Fatalf("budget: exit %d requests %d: %s", code, srv.Requests.Load(), stderr)
	}
}

func TestPartialAndFatal(t *testing.T) {
	srv := tilestest.New(17, tilestest.NotFound)
	defer srv.Close()
	// One tile column has no imagery at any zoom: output is still written, exit 3.
	var holeX atomic.Int64
	holeX.Store(-1)
	srv.Hole = func(tl geo.Tile) bool {
		hx := holeX.Load()
		return hx >= 0 && tl.Z >= 13 && tl.Z <= 17 && int(hx)>>(17-tl.Z) == tl.X
	}
	b := geo.PointBox(38.8977, -77.0365, 150)
	tr := b.PixelWindow(17).Tiles()
	holeX.Store(int64(tr.X0 + 1))
	out := filepath.Join(t.TempDir(), "p.webp")
	code, _, stderr := run(
		t,
		"38.8977,-77.0365",
		"-r",
		"150",
		"-o",
		out,
		"-url",
		srv.Template(),
		"-quiet",
		"-min-zoom",
		"17",
	)
	if code != ExitPartial {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	m := readMeta(t, strings.TrimSuffix(out, ".webp")+".json")
	if m.Complete || m.Tiles.Empty == 0 || len(m.GapTiles) == 0 {
		t.Fatalf("%+v", m.Tiles)
	}

	code, _, stderr = run(
		t,
		"38.8977,-77.0365",
		"-url",
		"http://127.0.0.1:1/{z}/{y}/{x}",
		"-o",
		filepath.Join(t.TempDir(), "x.webp"),
		"-quiet",
		"-j",
		"1",
	)
	if code != ExitError {
		t.Fatalf("unreachable server: exit %d %s", code, stderr)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"1,2", "3,4"},
		{"38.9,-77", "-z", "30"},
		{"38.9,-77", "-o", "x.tif"},
		{"38.9,-77", "-clip"},
		{"38.9,-77", "-q", "0"},
		{"38.9,-77", "-url", "http://example.com/tiles"},
		{"38.9,-77", "-chunk", "20000"}, // beyond WebP limit
		{"not-a-file.geojson"},
		{"38.9,-77", "-bogus"},
	} {
		if code, _, _ := run(t, args...); code != ExitUsage {
			t.Errorf("%q: exit %d, want %d", args, code, ExitUsage)
		}
	}
	if code, out, _ := run(t, "-version"); code != ExitOK || !strings.Contains(out, "geoimg test") {
		t.Errorf("-version: %d %q", code, out)
	}
	if code, _, errOut := run(t, "-h"); code != ExitOK || !strings.Contains(errOut, "Usage:") {
		t.Errorf("-h: %d", code)
	}
}
