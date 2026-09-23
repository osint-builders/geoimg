package render

import (
	"fmt"

	"github.com/osint-builders/geoimg/internal/geo"
)

// Chunk is one output image: a rectangle of the full pixel window that is
// aligned to tile boundaries, so every tile belongs to exactly one chunk.
type Chunk struct {
	Row, Col int
	Window   geo.Window    // global pixel window of this chunk (cropped to the area)
	Tiles    geo.TileRange // tiles covering Window
}

// Layout is the chunk grid for an area at one zoom level.
type Layout struct {
	Window geo.Window
	Rows   int
	Cols   int
	Chunks []Chunk
}

// PlanChunks splits the pixel window into chunks at most maxSide pixels on a
// side. If the whole area fits in one image, the layout has a single chunk.
// maxSide is rounded down to a whole number of tiles (minimum one tile).
func PlanChunks(w geo.Window, maxSide int) Layout {
	per := max(maxSide/geo.TileSize, 1) // tiles per chunk side
	tr := w.Tiles()
	l := Layout{Window: w}
	if w.Width() <= maxSide && w.Height() <= maxSide {
		l.Rows, l.Cols = 1, 1
		l.Chunks = []Chunk{{Window: w, Tiles: tr}}
		return l
	}
	l.Cols = (tr.Cols() + per - 1) / per
	l.Rows = (tr.Rows() + per - 1) / per
	for r := 0; r < l.Rows; r++ {
		for c := 0; c < l.Cols; c++ {
			t := geo.TileRange{
				Z:  w.Z,
				X0: tr.X0 + c*per,
				Y0: tr.Y0 + r*per,
				X1: min(tr.X0+(c+1)*per-1, tr.X1),
				Y1: min(tr.Y0+(r+1)*per-1, tr.Y1),
			}
			cw := geo.Window{
				Z:  w.Z,
				X0: max(w.X0, t.X0*geo.TileSize),
				Y0: max(w.Y0, t.Y0*geo.TileSize),
				X1: min(w.X1, (t.X1+1)*geo.TileSize),
				Y1: min(w.Y1, (t.Y1+1)*geo.TileSize),
			}
			l.Chunks = append(l.Chunks, Chunk{Row: r, Col: c, Window: cw, Tiles: t})
		}
	}
	return l
}

// ChunkName returns the file name suffix for a chunk in a multi-chunk layout.
func (l *Layout) ChunkName(c *Chunk) string {
	if len(l.Chunks) == 1 {
		return ""
	}
	return fmt.Sprintf("_r%0*d_c%0*d", digits(l.Rows-1), c.Row, digits(l.Cols-1), c.Col)
}

func digits(n int) int {
	d := 2
	for p := 100; n >= p; p *= 10 {
		d++
	}
	return d
}
