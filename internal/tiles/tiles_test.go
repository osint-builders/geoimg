package tiles

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/osint-builders/geoimg/internal/geo"
	"github.com/osint-builders/geoimg/internal/tiles/tilestest"
)

func jpegRoundTrip(img image.Image) image.Image {
	var b bytes.Buffer
	jpeg.Encode(&b, img, &jpeg.Options{Quality: 85})
	out, _ := jpeg.Decode(&b)
	return out
}

// deep returns a tile at zoom z over the same ground for every z.
func deep(z int) geo.Tile {
	return geo.Tile{Z: z, X: (75000 << 6) >> (23 - z), Y: (50000 << 6) >> (23 - z)}
}

func TestPlaceholderDetection(t *testing.T) {
	if !IsPlaceholder(jpegRoundTrip(tilestest.PlaceholderImage())) {
		t.Fatal("placeholder not detected")
	}
	for z := 14; z <= 20; z++ {
		if IsPlaceholder(jpegRoundTrip(tilestest.Render(deep(z), 18))) {
			t.Fatalf("imagery at z%d flagged as placeholder", z)
		}
	}
	flat := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for i := range flat.Pix {
		flat.Pix[i] = []byte{20, 60, 140, 255}[i%4] // uniform ocean blue
	}
	if IsPlaceholder(flat) {
		t.Fatal("saturated uniform tile flagged as placeholder")
	}
}

func TestSharpnessVerdicts(t *testing.T) {
	const th = 0.2
	for z := 15; z <= 18; z++ {
		if v := Measure(jpegRoundTrip(tilestest.Render(deep(z), 18))).Verdict(th); v != NativeRes {
			t.Errorf("native z%d judged %d", z, v)
		}
	}
	for z := 19; z <= 23; z++ {
		if v := Measure(jpegRoundTrip(tilestest.Render(deep(z), 18))).Verdict(th); v != Upsampled {
			t.Errorf("upsampled z%d judged %d (%+v)", z, v, Measure(jpegRoundTrip(tilestest.Render(deep(z), 18))))
		}
	}
	flat := image.NewRGBA(image.Rect(0, 0, 256, 256))
	if v := Measure(flat).Verdict(th); v != Inconclusive {
		t.Errorf("flat tile judged %d", v)
	}
}

func box() geo.BBox { return geo.PointBox(38.8977, -77.0365, 80) }

func TestSelectZoom(t *testing.T) {
	for _, beyond := range []tilestest.Beyond{tilestest.NotFound, tilestest.Grey, tilestest.Upsample} {
		srv := tilestest.New(17, beyond)
		src := NewSource(NewFetcher(srv.Template(), "", "test", 8))
		z, attempts, err := SelectZoom(context.Background(), src, box(), ZoomOptions{
			MaxZoom: 23, MinZoom: 10, MaxTiles: 5000, MinCoverage: 0.5, DetailRatio: 0.2, Workers: 8, Batch: 3,
		})
		srv.Close()
		if err != nil || z != 17 {
			t.Fatalf("beyond=%d: zoom %d err %v attempts %+v", beyond, z, err, attempts)
		}
	}
}

func TestSelectZoomBudgetAndCoverage(t *testing.T) {
	srv := tilestest.New(20, tilestest.NotFound)
	defer srv.Close()
	src := NewSource(NewFetcher(srv.Template(), "", "test", 8))
	b := box()
	budget := b.PixelWindow(18).Tiles().Count()
	z, attempts, err := SelectZoom(context.Background(), src, b, ZoomOptions{
		MaxZoom: 22, MinZoom: 10, MaxTiles: budget, MinCoverage: 1, Workers: 8, Batch: 2,
	})
	if err != nil || z != 18 {
		t.Fatalf("zoom %d err %v", z, err)
	}
	if len(attempts) < 4 || attempts[0].Accepted || !strings.Contains(attempts[0].Reason, "budget") {
		t.Fatalf("attempts %+v", attempts)
	}

	// Holes in part of the area: strict coverage steps down, lenient keeps it.
	srv.Hole = func(tl geo.Tile) bool { return tl.Z == 20 && tl.X%2 == 0 }
	for _, tc := range []struct {
		cov  float64
		want int
	}{{1, 19}, {0.3, 20}} {
		src := NewSource(NewFetcher(srv.Template(), "", "test", 8))
		z, _, err := SelectZoom(
			context.Background(),
			src,
			b,
			ZoomOptions{MaxZoom: 20, MinZoom: 10, MaxTiles: 1e6, MinCoverage: tc.cov, Workers: 8, Batch: 3},
		)
		if err != nil || z != tc.want {
			t.Fatalf("coverage %v: zoom %d err %v", tc.cov, z, err)
		}
	}

	if _, _, err := SelectZoom(context.Background(), src, b, ZoomOptions{MaxZoom: 20, MinZoom: 19, MaxTiles: 1}); err == nil ||
		!strings.Contains(err.Error(), "too large") {
		t.Fatalf("budget error: %v", err)
	}
}

func TestRetriesAndFatal(t *testing.T) {
	srv := tilestest.New(18, tilestest.NotFound)
	defer srv.Close()
	srv.Throttle.Store(3)
	f := NewFetcher(srv.Template(), "", "test", 2)
	if _, st, err := f.Fetch(context.Background(), deep(18)); err != nil || st != OK {
		t.Fatalf("after 429s: status %v err %v", st, err)
	}
	if f.Requests.Load() != 4 {
		t.Fatalf("requests %d, want 4", f.Requests.Load())
	}

	var calls atomic.Int64
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.Contains(r.URL.RawQuery, "token=s3cret") || !strings.HasPrefix(r.UserAgent(), "geoimg/") {
			t.Errorf("bad request %s ua=%q", r.URL, r.UserAgent())
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer forbidden.Close()
	f = NewFetcher(forbidden.URL+"/{z}/{y}/{x}", "s3cret", "geoimg/test", 2)
	_, _, err := f.Fetch(context.Background(), deep(18))
	if !errors.Is(err, ErrFatal) || calls.Load() != 1 || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("err %v calls %d", err, calls.Load())
	}
}

func TestGapFill(t *testing.T) {
	srv := tilestest.New(18, tilestest.NotFound)
	defer srv.Close()
	missing := deep(18)
	srv.Hole = func(tl geo.Tile) bool { return tl == missing }
	src := NewSource(NewFetcher(srv.Template(), "", "test", 4))
	img, prov, err := src.Take(context.Background(), missing)
	if err != nil || prov.Kind != GapFill || prov.FromZoom != 17 || img.Bounds().Dx() != 256 {
		t.Fatalf("prov %+v err %v", prov, err)
	}
	// Gap fill should resemble the real tile (blurred), not be blank.
	want := tilestest.Render(missing, 18)
	var diff float64
	for i := 0; i < len(want.Pix); i += 4 {
		d := float64(want.Pix[i+1]) - float64(img.(*image.RGBA).Pix[i+1])
		diff += d * d
	}
	if rms := diff / float64(len(want.Pix)/4); rms > 30*30 {
		t.Fatalf("gap fill too different: mean sq %v", rms)
	}

	src.GapFill = false
	if _, prov, _ := src.Take(context.Background(), missing); prov.Kind != Empty {
		t.Fatalf("without gap fill: %+v", prov)
	}
}

func TestForEachBoundsConcurrency(t *testing.T) {
	var cur, peak atomic.Int64
	items := make([]int, 200)
	err := ForEach(context.Background(), 5, items, func(ctx context.Context, _ int) error {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		cur.Add(-1)
		return nil
	})
	if err != nil || peak.Load() > 5 {
		t.Fatalf("err %v peak %d", err, peak.Load())
	}
	boom := errors.New("boom")
	var ran atomic.Int64
	err = ForEach(context.Background(), 2, items, func(ctx context.Context, i int) error {
		ran.Add(1)
		return boom
	})
	if !errors.Is(err, boom) || ran.Load() > 10 {
		t.Fatalf("err %v ran %d", err, ran.Load())
	}
}

func TestTileURL(t *testing.T) {
	f := NewFetcher(DefaultURL, "", "x", 1)
	got := f.TileURL(geo.Tile{Z: 19, X: 149953, Y: 200578})
	if !strings.HasSuffix(got, "/tile/19/200578/149953") {
		t.Fatalf("Esri order is z/y/x: %s", got)
	}
	f.Token = "a b"
	f.URL = "https://h/{z}/{x}/{y}.png?style=1"
	if got := f.TileURL(geo.Tile{Z: 1, X: 2, Y: 3}); got != "https://h/1/2/3.png?style=1&token=a+b" {
		t.Fatal(got)
	}
}
