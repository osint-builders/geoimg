package geo

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestKnownPixels(t *testing.T) {
	if x, y := LonToPixelX(0, 1), LatToPixelY(0, 1); x != 256 || !near(y, 256, 1e-9) {
		t.Fatalf("(0,0)@z1 = %v,%v want 256,256", x, y)
	}
	if y := LatToPixelY(MaxLat, 3); !near(y, 0, 1e-6) {
		t.Fatalf("MaxLat → %v, want 0", y)
	}
	// Washington Monument at z19: tile x=149953, y=200578 (cross-checked with the OSM slippy-map formula).
	w := PointBox(38.8895, -77.0353, 1).PixelWindow(19)
	tr := w.Tiles()
	if tr.X0 != 149953 || tr.Y0 != 200578 {
		t.Fatalf("tile = %d,%d", tr.X0, tr.Y0)
	}
}

func TestRoundTrip(t *testing.T) {
	for _, z := range []int{0, 5, 12, 19, 23} {
		for _, lat := range []float64{-85, -45.5, 0, 12.34, 38.8977, 85} {
			for _, lon := range []float64{-180, -77.0365, 0, 151.2093, 179.999} {
				gotLat := PixelYToLat(LatToPixelY(lat, z), z)
				gotLon := PixelXToLon(LonToPixelX(lon, z), z)
				if !near(gotLat, lat, 1e-9) || !near(gotLon, lon, 1e-9) {
					t.Fatalf("z%d (%v,%v) → (%v,%v)", z, lat, lon, gotLat, gotLon)
				}
			}
		}
	}
}

func TestGroundResolution(t *testing.T) {
	if r := GroundResolution(0, 19); !near(r, 0.2986, 1e-3) {
		t.Fatalf("equator z19 = %v m/px", r)
	}
	if r := GroundResolution(60, 19); !near(r, 0.1493, 1e-3) {
		t.Fatalf("60° z19 = %v m/px", r)
	}
}

func TestWindowCoversBBox(t *testing.T) {
	b := BBox{MinLat: 40.748, MinLon: -74.00, MaxLat: 40.750, MaxLon: -73.98}
	for z := 10; z <= 20; z++ {
		w := b.PixelWindow(z)
		got := w.BBox()
		if got.MinLat > b.MinLat || got.MaxLat < b.MaxLat || got.MinLon > b.MinLon || got.MaxLon < b.MaxLon {
			t.Fatalf("z%d window %+v does not cover %+v", z, got, b)
		}
		// No more than one pixel of slack on any side.
		px := 360 / WorldPixels(z)
		if b.MinLon-got.MinLon > px || got.MaxLon-b.MaxLon > px {
			t.Fatalf("z%d window too loose: %+v", z, got)
		}
		tr := w.Tiles()
		if tr.X0*TileSize > w.X0 || (tr.X1+1)*TileSize < w.X1 {
			t.Fatalf("z%d tiles %+v do not cover window %+v", z, tr, w)
		}
	}
}

func TestPointBoxRadius(t *testing.T) {
	b := PointBox(38.8977, -77.0365, 200)
	lat, _ := b.Center()
	height := (b.MaxLat - b.MinLat) * 111320
	width := (b.MaxLon - b.MinLon) * 111320 * math.Cos(lat*math.Pi/180)
	if !near(height, 400, 1) || !near(width, 400, 1) {
		t.Fatalf("box %v × %v m, want 400 × 400", width, height)
	}
}

func TestTileParent(t *testing.T) {
	p := Tile{Z: 19, X: 149953, Y: 200578}.Parent(2)
	if p != (Tile{Z: 17, X: 37488, Y: 50144}) {
		t.Fatalf("parent = %v", p)
	}
}

func TestParseTargetCoordinates(t *testing.T) {
	cases := []struct {
		in   string
		kind string
		err  string
	}{
		{"38.8977,-77.0365", KindPoint, ""},
		{"-33.8568, 151.2153", KindPoint, ""},
		{"40.750,-73.98,40.748,-74.00", KindBBox, ""}, // corners in any order
		{"40.748 -74.00 40.750 -73.98", KindBBox, ""},
		{"-77.0365,38.8977", KindPoint, ""}, // valid numbers, just a different place
		{"91,0", "", "latitude"},
		{"10,200", "", "longitude"},
		{"1,2,3", "", "expected 2 numbers"},
		{"40,-74,40,-73", "", "zero area"},
		{"nope.geojson", "", "neither coordinates"},
	}
	for _, c := range cases {
		tg, err := ParseTarget(c.in, 100, nil)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%q: err = %v, want %q", c.in, err, c.err)
			}
			continue
		}
		if err != nil || tg.Kind != c.kind {
			t.Errorf("%q: kind %q err %v", c.in, tg.Kind, err)
		}
	}
}

func TestParseGeoJSON(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	poly := write("poly.geojson", `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{},
		"geometry":{"type":"Polygon","coordinates":[[[-77.04,38.88],[-77.03,38.88],[-77.03,38.89],[-77.04,38.89],[-77.04,38.88]]]}}]}`)
	tg, err := ParseTarget(poly, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tg.Kind != KindGeoJSON || len(tg.Polygons) != 1 {
		t.Fatalf("%+v", tg)
	}
	if !near(tg.BBox.MinLon, -77.04, 1e-12) || !near(tg.BBox.MaxLat, 38.89, 1e-12) {
		t.Fatalf("bbox %+v (polygons must not be padded)", tg.BBox)
	}

	multi := write("multi.json", `{"type":"MultiPolygon","coordinates":[
		[[[0,0],[1,0],[1,1],[0,1],[0,0]]],
		[[[2,2],[3,2],[3,3],[2,3],[2,2]],[[2.2,2.2],[2.8,2.2],[2.8,2.8],[2.2,2.8],[2.2,2.2]]]]}`)
	tg, err = ParseTarget(multi, 100, nil)
	if err != nil || len(tg.Polygons) != 2 || len(tg.Polygons[1]) != 2 || tg.BBox.MaxLon != 3 {
		t.Fatalf("%+v %v", tg, err)
	}

	pt := write("pt.geojson", `{"type":"Feature","geometry":{"type":"Point","coordinates":[-77.0365,38.8977,12.5]}}`)
	tg, err = ParseTarget(pt, 200, nil)
	if err != nil || tg.BBox.MaxLat-tg.BBox.MinLat < 0.003 {
		t.Fatalf("point should be padded by radius: %+v %v", tg.BBox, err)
	}

	tg, err = ParseTarget("-", 100, strings.NewReader(`{"type":"LineString","coordinates":[[10,10],[10.01,10.02]]}`))
	if err != nil || tg.Input != "stdin" {
		t.Fatalf("%+v %v", tg, err)
	}

	for body, want := range map[string]string{
		`{"type":"Polygon","coordinates":[[[38.8,-77.0],[38.9,-77.0],[38.9,-200],[38.8,-77.0]]]}`: "lon, lat",
		`{"type":"Circle","coordinates":[0,0]}`:                                                   "unsupported",
		`{"type":"FeatureCollection","features":[]}`:                                              "no coordinates",
		`not json`: "invalid GeoJSON",
	} {
		if _, err := ParseTarget(write("bad.geojson", body), 100, nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", body, err, want)
		}
	}
}
