package tiles

import (
	"context"
	"errors"
	"image"
	"image/draw"
	"sync"

	"github.com/osint-builders/geoimg/internal/geo"
)

// Provenance says where the pixels for a tile came from.
type Provenance struct {
	Kind     string `json:"kind"`                // native | gapfill | placeholder | empty
	FromZoom int    `json:"from_zoom,omitempty"` // for gapfill: ancestor zoom used
}

// Provenance kinds.
const (
	Native      = "native"
	GapFill     = "gapfill"
	Filler      = "placeholder"
	Empty       = "empty"
	maxAncestor = 4
)

type result struct {
	img    image.Image
	status Status
	err    error
}

type entry struct {
	once sync.Once
	res  result
}

// Source resolves tiles to 256×256 images, deduplicating concurrent requests
// and filling holes (missing, placeholder or failed tiles) by upscaling the
// nearest ancestor tile that has real imagery.
type Source struct {
	f     *Fetcher
	mu    sync.Mutex
	cache map[geo.Tile]*entry
	// GapFill enables ancestor-based hole filling.
	GapFill bool
}

// NewSource wraps a Fetcher.
func NewSource(f *Fetcher) *Source {
	return &Source{f: f, cache: make(map[geo.Tile]*entry), GapFill: true}
}

// fetch returns a (memoized) fetch result for t.
func (s *Source) fetch(ctx context.Context, t geo.Tile) result {
	s.mu.Lock()
	e, ok := s.cache[t]
	if !ok {
		e = &entry{}
		s.cache[t] = e
	}
	s.mu.Unlock()
	e.once.Do(func() {
		img, st, err := s.f.Fetch(ctx, t)
		e.res = result{img: img, status: st, err: err}
	})
	return e.res
}

// Probe fetches t (memoized) and reports its status.
func (s *Source) Probe(ctx context.Context, t geo.Tile) (image.Image, Status, error) {
	r := s.fetch(ctx, t)
	return r.img, r.status, r.err
}

// Forget drops every cached tile for which keep returns false.
func (s *Source) Forget(keep func(geo.Tile) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for t := range s.cache {
		if !keep(t) {
			delete(s.cache, t)
		}
	}
}

// Take returns the image for t and drops it from the cache (tiles at the
// target zoom are drawn exactly once). Ancestors stay cached since many
// children share them.
func (s *Source) Take(ctx context.Context, t geo.Tile) (image.Image, Provenance, error) {
	r := s.fetch(ctx, t)
	s.mu.Lock()
	delete(s.cache, t)
	s.mu.Unlock()
	if r.err != nil && isFatal(ctx, r.err) {
		return nil, Provenance{}, r.err
	}
	if r.err == nil && r.status == OK {
		return r.img, Provenance{Kind: Native}, nil
	}
	if s.GapFill {
		for up := 1; up <= maxAncestor && t.Z-up >= 0; up++ {
			p := t.Parent(up)
			pr := s.fetch(ctx, p)
			if pr.err != nil && isFatal(ctx, pr.err) {
				return nil, Provenance{}, pr.err
			}
			if pr.err == nil && pr.status == OK {
				return Upscale(pr.img, t, up), Provenance{Kind: GapFill, FromZoom: p.Z}, nil
			}
		}
	}
	if r.img != nil { // a placeholder beats a hole
		return r.img, Provenance{Kind: Filler}, nil
	}
	return nil, Provenance{Kind: Empty}, nil
}

func isFatal(ctx context.Context, err error) bool {
	return errors.Is(err, ErrFatal) || ctx.Err() != nil
}

// Upscale crops the region of ancestor img (which is `up` levels above t)
// that covers t and bilinearly resamples it to a full tile.
func Upscale(img image.Image, t geo.Tile, up int) *image.RGBA {
	n := 1 << up
	b := img.Bounds()
	cell := float64(b.Dx()) / float64(n)
	ox := float64(b.Min.X) + float64(t.X%n)*cell
	oy := float64(b.Min.Y) + float64(t.Y%n)*cell

	src := image.NewRGBA(b)
	draw.Draw(src, b, img, b.Min, draw.Src)

	out := image.NewRGBA(image.Rect(0, 0, geo.TileSize, geo.TileSize))
	scale := cell / geo.TileSize
	for y := 0; y < geo.TileSize; y++ {
		sy := oy + (float64(y)+0.5)*scale - 0.5
		y0, fy := splitCoord(sy, b.Min.Y, b.Max.Y-1)
		y1 := min(y0+1, b.Max.Y-1)
		for x := 0; x < geo.TileSize; x++ {
			sx := ox + (float64(x)+0.5)*scale - 0.5
			x0, fx := splitCoord(sx, b.Min.X, b.Max.X-1)
			x1 := min(x0+1, b.Max.X-1)
			i00, i10 := src.PixOffset(x0, y0), src.PixOffset(x1, y0)
			i01, i11 := src.PixOffset(x0, y1), src.PixOffset(x1, y1)
			o := out.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				top := float64(src.Pix[i00+c])*(1-fx) + float64(src.Pix[i10+c])*fx
				bot := float64(src.Pix[i01+c])*(1-fx) + float64(src.Pix[i11+c])*fx
				out.Pix[o+c] = uint8(top*(1-fy) + bot*fy + 0.5)
			}
		}
	}
	return out
}

func splitCoord(v float64, lo, hi int) (idx int, frac float64) {
	if v < float64(lo) {
		return lo, 0
	}
	i := int(v)
	if i >= hi {
		return hi, 0
	}
	return i, v - float64(i)
}
