package geo

import (
	"errors"
	"fmt"
	"math"
)

// BBox is a WGS84 bounding box in decimal degrees.
type BBox struct {
	MinLat float64 `json:"min_lat"`
	MinLon float64 `json:"min_lon"`
	MaxLat float64 `json:"max_lat"`
	MaxLon float64 `json:"max_lon"`
}

// Validate reports whether b is a usable, non-degenerate box.
func (b BBox) Validate() error {
	for _, v := range []float64{b.MinLat, b.MinLon, b.MaxLat, b.MaxLon} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("coordinates must be finite numbers")
		}
	}
	if b.MinLat < -90 || b.MaxLat > 90 {
		return fmt.Errorf("latitude out of range [-90, 90]: %g..%g", b.MinLat, b.MaxLat)
	}
	if b.MinLon < -180 || b.MaxLon > 180 {
		return fmt.Errorf("longitude out of range [-180, 180]: %g..%g", b.MinLon, b.MaxLon)
	}
	if b.MinLat >= b.MaxLat || b.MinLon >= b.MaxLon {
		return errors.New("bounding box has zero area (check coordinate order: lat,lon)")
	}
	if b.MinLat <= -MaxLat || b.MaxLat >= MaxLat {
		return fmt.Errorf("latitude beyond Web Mercator limit ±%.4f", MaxLat)
	}
	return nil
}

// Center returns the midpoint of the box.
func (b BBox) Center() (lat, lon float64) {
	return (b.MinLat + b.MaxLat) / 2, (b.MinLon + b.MaxLon) / 2
}

// Extend grows b to include the point.
func (b *BBox) Extend(lat, lon float64) {
	b.MinLat = math.Min(b.MinLat, lat)
	b.MaxLat = math.Max(b.MaxLat, lat)
	b.MinLon = math.Min(b.MinLon, lon)
	b.MaxLon = math.Max(b.MaxLon, lon)
}

// EmptyBBox returns an inverted box ready to be grown with Extend.
func EmptyBBox() BBox {
	return BBox{MinLat: math.Inf(1), MinLon: math.Inf(1), MaxLat: math.Inf(-1), MaxLon: math.Inf(-1)}
}

// Pad grows the box by meters on every side (ground distance).
func (b BBox) Pad(meters float64) BBox {
	lat, _ := b.Center()
	dLat := meters / 111320.0
	dLon := meters / (111320.0 * math.Max(math.Cos(lat*math.Pi/180), 1e-6))
	return BBox{
		MinLat: math.Max(-MaxLat+1e-9, b.MinLat-dLat),
		MaxLat: math.Min(MaxLat-1e-9, b.MaxLat+dLat),
		MinLon: math.Max(-180, b.MinLon-dLon),
		MaxLon: math.Min(180, b.MaxLon+dLon),
	}
}

// PointBox returns a square box of the given radius (meters) around a point.
func PointBox(lat, lon, radius float64) BBox {
	return BBox{MinLat: lat, MaxLat: lat, MinLon: lon, MaxLon: lon}.Pad(radius)
}

// Window is a half-open rectangle of global pixel coordinates at zoom Z.
type Window struct {
	Z      int `json:"z"`
	X0, Y0 int `json:"-"`
	X1, Y1 int `json:"-"`
}

// PixelWindow returns the exact pixel window covering b at zoom z.
func (b BBox) PixelWindow(z int) Window {
	world := int(WorldPixels(z))
	w := Window{
		Z:  z,
		X0: int(math.Floor(LonToPixelX(b.MinLon, z))),
		X1: int(math.Ceil(LonToPixelX(b.MaxLon, z))),
		Y0: int(math.Floor(LatToPixelY(b.MaxLat, z))),
		Y1: int(math.Ceil(LatToPixelY(b.MinLat, z))),
	}
	w.X0, w.Y0 = clampInt(w.X0, 0, world-1), clampInt(w.Y0, 0, world-1)
	w.X1, w.Y1 = clampInt(w.X1, w.X0+1, world), clampInt(w.Y1, w.Y0+1, world)
	return w
}

// Width in pixels.
func (w Window) Width() int { return w.X1 - w.X0 }

// Height in pixels.
func (w Window) Height() int { return w.Y1 - w.Y0 }

// Tiles returns the inclusive tile index range covering the window.
func (w Window) Tiles() TileRange {
	return TileRange{Z: w.Z, X0: w.X0 / TileSize, Y0: w.Y0 / TileSize, X1: (w.X1 - 1) / TileSize, Y1: (w.Y1 - 1) / TileSize}
}

// BBox returns the exact geographic extent of the window's pixel edges.
func (w Window) BBox() BBox {
	return BBox{
		MinLon: PixelXToLon(float64(w.X0), w.Z),
		MaxLon: PixelXToLon(float64(w.X1), w.Z),
		MaxLat: PixelYToLat(float64(w.Y0), w.Z),
		MinLat: PixelYToLat(float64(w.Y1), w.Z),
	}
}

// TileRange is an inclusive range of tile indexes at one zoom level.
type TileRange struct {
	Z  int `json:"z"`
	X0 int `json:"x_min"`
	Y0 int `json:"y_min"`
	X1 int `json:"x_max"`
	Y1 int `json:"y_max"`
}

// Cols is the number of tile columns.
func (r TileRange) Cols() int { return r.X1 - r.X0 + 1 }

// Rows is the number of tile rows.
func (r TileRange) Rows() int { return r.Y1 - r.Y0 + 1 }

// Count is the total number of tiles.
func (r TileRange) Count() int { return r.Cols() * r.Rows() }

// Tile addresses one XYZ tile.
type Tile struct {
	Z int `json:"z"`
	X int `json:"x"`
	Y int `json:"y"`
}

func (t Tile) String() string { return fmt.Sprintf("%d/%d/%d", t.Z, t.X, t.Y) }

// Parent returns the ancestor of t that is `levels` zoom levels up.
func (t Tile) Parent(levels int) Tile {
	return Tile{Z: t.Z - levels, X: t.X >> levels, Y: t.Y >> levels}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
