// Package tilestest provides a synthetic XYZ tile server for tests. It mimics
// the behaviours geoimg must handle on Esri World Imagery: a native maximum
// zoom, missing tiles (404), grey "Map data not yet available" placeholders,
// server-side upsampling beyond native resolution, and rate limiting.
package tilestest

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/osint-builders/geoimg/internal/geo"
)

// Beyond says what the server does above its native zoom.
type Beyond int

const (
	// NotFound returns 404 above native zoom.
	NotFound Beyond = iota
	// Grey returns a placeholder tile above native zoom.
	Grey
	// Upsample returns imagery stretched from the native level.
	Upsample
)

// Server is a synthetic tile server.
type Server struct {
	*httptest.Server
	NativeZoom int
	Beyond     Beyond
	// Hole reports tiles that are missing even at native zoom (optional).
	Hole func(t geo.Tile) bool
	// Throttle makes the first N requests answer 429 with Retry-After: 0.
	Throttle atomic.Int64

	Requests atomic.Int64
	inFlight atomic.Int64
	MaxInFly atomic.Int64

	mu    sync.Mutex
	cache map[geo.Tile][]byte
}

// New starts a server whose URL template is s.URL + "/{z}/{y}/{x}".
func New(native int, beyond Beyond) *Server {
	s := &Server{NativeZoom: native, Beyond: beyond, cache: map[geo.Tile][]byte{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// Template returns the tile URL template for this server.
func (s *Server) Template() string { return s.URL + "/{z}/{y}/{x}" }

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.Requests.Add(1)
	n := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	for {
		m := s.MaxInFly.Load()
		if n <= m || s.MaxInFly.CompareAndSwap(m, n) {
			break
		}
	}
	if s.Throttle.Add(-1) >= 0 {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	z, _ := strconv.Atoi(parts[0])
	y, _ := strconv.Atoi(parts[1])
	x, _ := strconv.Atoi(parts[2])
	t := geo.Tile{Z: z, X: x, Y: y}
	if s.Hole != nil && s.Hole(t) {
		http.NotFound(w, r)
		return
	}
	if z > s.NativeZoom && s.Beyond == NotFound {
		http.NotFound(w, r)
		return
	}
	data := s.tile(t)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Write(data)
}

func (s *Server) tile(t geo.Tile) []byte {
	s.mu.Lock()
	if b, ok := s.cache[t]; ok {
		s.mu.Unlock()
		return b
	}
	s.mu.Unlock()
	var img image.Image
	switch {
	case t.Z > s.NativeZoom && s.Beyond == Grey:
		img = PlaceholderImage()
	default:
		img = Render(t, s.NativeZoom)
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	s.mu.Lock()
	s.cache[t] = buf.Bytes()
	s.mu.Unlock()
	return buf.Bytes()
}

// World returns the synthetic ground colour of native pixel (gx, gy). It is
// a sum of value-noise octaves of equal amplitude, which gives the roughly
// 1/f spectrum of real aerial imagery: every scale, down to single native
// pixels, carries detail.
func World(gx, gy int) color.RGBA {
	var v float64
	for k := 0; k < 8; k++ {
		v += valueNoise(gx, gy, k)
	}
	g := 128 + (v-4)*28
	return color.RGBA{R: clamp8(g * 0.9), G: clamp8(g), B: clamp8(g*0.75 + 20), A: 255}
}

func valueNoise(gx, gy, k int) float64 {
	s := 1 << k
	x0, y0 := floorDiv(gx, s), floorDiv(gy, s)
	fx := (float64(gx-x0*s) + 0.5) / float64(s)
	fy := (float64(gy-y0*s) + 0.5) / float64(s)
	a, b := lattice(x0, y0, k), lattice(x0+1, y0, k)
	c, d := lattice(x0, y0+1, k), lattice(x0+1, y0+1, k)
	return (a*(1-fx)+b*fx)*(1-fy) + (c*(1-fx)+d*fx)*fy
}

func lattice(x, y, k int) float64 {
	h := uint32(x)*73856093 ^ uint32(y)*19349663 ^ uint32(k)*83492791
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return float64(h&0xffff) / 0xffff
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

func clamp8(v float64) uint8 {
	return uint8(math.Max(0, math.Min(255, v)))
}

// Render draws tile t of a world whose finest real detail is at zoom native:
// below native it box-averages native pixels, above it bilinearly upsamples.
func Render(t geo.Tile, native int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, geo.TileSize, geo.TileSize))
	d := t.Z - native
	for py := 0; py < geo.TileSize; py++ {
		for px := 0; px < geo.TileSize; px++ {
			gx, gy := t.X*geo.TileSize+px, t.Y*geo.TileSize+py
			var c color.RGBA
			switch {
			case d == 0:
				c = World(gx, gy)
			case d < 0:
				k := 1 << -d
				var r, g, b int
				for j := 0; j < k; j++ {
					for i := 0; i < k; i++ {
						w := World(gx*k+i, gy*k+j)
						r, g, b = r+int(w.R), g+int(w.G), b+int(w.B)
					}
				}
				n := k * k
				c = color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), 255}
			default:
				k := float64(int(1) << d)
				fx, fy := (float64(gx)+0.5)/k-0.5, (float64(gy)+0.5)/k-0.5
				x0, y0 := int(fx), int(fy)
				ax, ay := fx-float64(x0), fy-float64(y0)
				c00, c10, c01, c11 := World(x0, y0), World(x0+1, y0), World(x0, y0+1), World(x0+1, y0+1)
				lerp := func(a, b, c, d uint8) uint8 {
					top := float64(a)*(1-ax) + float64(b)*ax
					bot := float64(c)*(1-ax) + float64(d)*ax
					return uint8(top*(1-ay) + bot*ay)
				}
				c = color.RGBA{
					lerp(c00.R, c10.R, c01.R, c11.R),
					lerp(c00.G, c10.G, c01.G, c11.G),
					lerp(c00.B, c10.B, c01.B, c11.B),
					255,
				}
			}
			img.SetRGBA(px, py, c)
		}
	}
	return img
}

// PlaceholderImage imitates Esri's grey "Map data not yet available" tile.
func PlaceholderImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, geo.TileSize, geo.TileSize))
	for y := 0; y < geo.TileSize; y++ {
		for x := 0; x < geo.TileSize; x++ {
			c := color.RGBA{204, 204, 204, 255}
			if y > 110 && y < 146 && x > 30 && x < 226 && (x/3+y/4)%3 == 0 {
				c = color.RGBA{110, 110, 110, 255} // "text"
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// String describes the server configuration.
func (s *Server) String() string {
	return fmt.Sprintf("tilestest(native=%d beyond=%d)", s.NativeZoom, s.Beyond)
}
