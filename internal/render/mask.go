package render

import (
	"image"
	"math"
	"sort"

	"github.com/osint-builders/geoimg/internal/geo"
)

type edge struct{ x0, y0, x1, y1 float64 }

// Mask rasterizes GeoJSON polygons into pixel space at one zoom level. Each
// polygon is filled with the even-odd rule (so holes work) and polygons are
// unioned.
type Mask struct {
	polys [][]edge
}

// NewMask projects polygons to global pixel coordinates at zoom z.
func NewMask(polys []geo.Polygon, z int) *Mask {
	m := &Mask{}
	for _, p := range polys {
		var es []edge
		for _, ring := range p {
			for i := 0; i+1 < len(ring); i++ {
				a, b := ring[i], ring[i+1]
				es = append(es, edge{
					geo.LonToPixelX(a[0], z), geo.LatToPixelY(a[1], z),
					geo.LonToPixelX(b[0], z), geo.LatToPixelY(b[1], z),
				})
			}
			if n := len(ring); n > 2 && ring[0] != ring[n-1] { // tolerate unclosed rings
				a, b := ring[n-1], ring[0]
				es = append(es, edge{
					geo.LonToPixelX(a[0], z), geo.LatToPixelY(a[1], z),
					geo.LonToPixelX(b[0], z), geo.LatToPixelY(b[1], z),
				})
			}
		}
		m.polys = append(m.polys, es)
	}
	return m
}

// Render returns an alpha mask for window w (255 = inside).
func (m *Mask) Render(w geo.Window) *image.Alpha {
	a := image.NewAlpha(image.Rect(0, 0, w.Width(), w.Height()))
	xs := make([]float64, 0, 64)
	for row := 0; row < w.Height(); row++ {
		y := float64(w.Y0+row) + 0.5
		line := a.Pix[row*a.Stride : row*a.Stride+w.Width()]
		for _, es := range m.polys {
			xs = xs[:0]
			for _, e := range es {
				if (e.y0 <= y) != (e.y1 <= y) {
					xs = append(xs, e.x0+(y-e.y0)*(e.x1-e.x0)/(e.y1-e.y0))
				}
			}
			sort.Float64s(xs)
			for i := 0; i+1 < len(xs); i += 2 {
				from := int(math.Ceil(xs[i]-0.5)) - w.X0
				to := int(math.Ceil(xs[i+1]-0.5)) - w.X0
				from, to = max(from, 0), min(to, w.Width())
				for x := from; x < to; x++ {
					line[x] = 255
				}
			}
		}
	}
	return a
}

// Any reports whether any mask pixel inside r is set.
func Any(a *image.Alpha, r image.Rectangle) bool {
	r = r.Intersect(a.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := a.Pix[a.PixOffset(r.Min.X, y) : a.PixOffset(r.Min.X, y)+r.Dx()]
		for _, v := range row {
			if v != 0 {
				return true
			}
		}
	}
	return false
}

// Apply clears every canvas pixel outside the mask (transparent for formats
// with alpha, black for JPEG).
func Apply(canvas *image.RGBA, a *image.Alpha) {
	for y := 0; y < a.Rect.Dy(); y++ {
		mrow := a.Pix[y*a.Stride : y*a.Stride+a.Rect.Dx()]
		crow := canvas.Pix[y*canvas.Stride:]
		for x, v := range mrow {
			if v == 0 {
				copy(crow[x*4:x*4+4], []byte{0, 0, 0, 0})
			}
		}
	}
}
