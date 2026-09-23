// Package geo implements the spatial math geoimg needs: WGS84 (EPSG:4326)
// to Web Mercator (EPSG:3857) conversions, tile/pixel addressing for the
// standard XYZ tile pyramid, bounding boxes and target parsing.
package geo

import "math"

const (
	// TileSize is the edge length in pixels of one XYZ tile.
	TileSize = 256
	// MaxLat is the latitude limit of the Web Mercator projection.
	MaxLat = 85.05112877980659
	// EarthRadius is the WGS84 semi-major axis used by EPSG:3857, in meters.
	EarthRadius = 6378137.0
	// OriginShift is half the EPSG:3857 world width in meters.
	OriginShift = math.Pi * EarthRadius
)

// WorldPixels returns the width (and height) of the whole world in pixels at zoom z.
func WorldPixels(z int) float64 {
	return float64(TileSize) * math.Exp2(float64(z))
}

// LonToPixelX converts a longitude to a global pixel X coordinate at zoom z.
func LonToPixelX(lon float64, z int) float64 {
	return (lon + 180) / 360 * WorldPixels(z)
}

// LatToPixelY converts a latitude to a global pixel Y coordinate at zoom z.
func LatToPixelY(lat float64, z int) float64 {
	lat = clampLat(lat)
	s := math.Sin(lat * math.Pi / 180)
	return (0.5 - math.Log((1+s)/(1-s))/(4*math.Pi)) * WorldPixels(z)
}

// PixelXToLon is the inverse of LonToPixelX.
func PixelXToLon(px float64, z int) float64 {
	return px/WorldPixels(z)*360 - 180
}

// PixelYToLat is the inverse of LatToPixelY.
func PixelYToLat(py float64, z int) float64 {
	n := math.Pi - 2*math.Pi*py/WorldPixels(z)
	return 180 / math.Pi * math.Atan(math.Sinh(n))
}

// PixelToMercator converts global pixel coordinates at zoom z to EPSG:3857 meters.
func PixelToMercator(px, py float64, z int) (x, y float64) {
	res := MetersPerPixel(z)
	return px*res - OriginShift, OriginShift - py*res
}

// MetersPerPixel is the EPSG:3857 pixel size at zoom z (true at the equator).
func MetersPerPixel(z int) float64 {
	return 2 * OriginShift / WorldPixels(z)
}

// GroundResolution is the true ground distance covered by one pixel at the
// given latitude and zoom, in meters.
func GroundResolution(lat float64, z int) float64 {
	return MetersPerPixel(z) * math.Cos(clampLat(lat)*math.Pi/180)
}

func clampLat(lat float64) float64 {
	return math.Max(-MaxLat, math.Min(MaxLat, lat))
}
