package render

import (
	"context"
	"image"
	"image/draw"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/osint-builders/geoimg/internal/geo"
	"github.com/osint-builders/geoimg/internal/tiles"
)

// OverviewMax is the longest edge of the overview image written for
// multi-chunk outputs.
const OverviewMax = 4096

// Options configures a render.
type Options struct {
	Out       string // output path; chunk suffixes are inserted before the extension
	Format    Format
	Quality   int
	WorldFile bool
	Mask      *Mask // nil = no clipping
	Workers   int
	// Progress receives tile completion updates; may be nil.
	Progress func(done, total int)
}

// File describes one written image.
type File struct {
	Path      string   `json:"path"`
	Row       int      `json:"row"`
	Col       int      `json:"col"`
	Width     int      `json:"width"`
	Height    int      `json:"height"`
	Bytes     int64    `json:"bytes"`
	SHA256    string   `json:"sha256"`
	Bounds    geo.BBox `json:"bounds"`
	WorldFile string   `json:"world_file,omitempty"`
}

// Stats counts tile outcomes.
type Stats struct {
	Native      int64 `json:"native"`
	GapFilled   int64 `json:"gap_filled"`
	Placeholder int64 `json:"placeholder"`
	Empty       int64 `json:"empty"`
	Skipped     int64 `json:"skipped_outside_clip"`
}

// Result is everything a render produced.
type Result struct {
	Files    []File     `json:"files"`
	Overview *File      `json:"overview,omitempty"`
	Stats    Stats      `json:"tiles"`
	GapTiles []TileNote `json:"gap_tiles,omitempty"`
}

// TileNote records a tile that did not come from native imagery.
type TileNote struct {
	Tile geo.Tile         `json:"tile"`
	From tiles.Provenance `json:"source"`
}

type encodeJob struct {
	chunk  Chunk
	canvas *image.RGBA
}

// Render downloads, stitches and encodes every chunk of the layout. Chunks are
// processed one after another to bound memory, but encoding chunk N overlaps
// with downloading chunk N+1.
func Render(ctx context.Context, src *tiles.Source, l *Layout, o Options) (*Result, error) {
	res := &Result{}
	var (
		stats         [5]atomic.Int64
		notesMu       sync.Mutex
		done          atomic.Int64
		total         = l.Window.Tiles().Count()
		ext           = extOf(o.Out)
		base          = strings.TrimSuffix(o.Out, ext)
		multi         = len(l.Chunks) > 1
		overview      *image.RGBA
		overviewScale float64
		encErr        error
		encDone       = make(chan struct{})
		jobs          = make(chan encodeJob, 1)
	)
	if multi {
		overviewScale = min(1, float64(OverviewMax)/float64(max(l.Window.Width(), l.Window.Height())))
		overview = image.NewRGBA(image.Rect(0, 0,
			max(1, int(float64(l.Window.Width())*overviewScale)),
			max(1, int(float64(l.Window.Height())*overviewScale))))
	}

	go func() {
		defer close(encDone)
		for j := range jobs {
			if encErr != nil {
				continue // drain
			}
			path := base + l.ChunkName(&j.chunk) + ext
			sum, size, err := WriteFileAtomic(path, j.canvas, o.Format, o.Quality)
			if err != nil {
				encErr = err
				continue
			}
			f := File{Path: path, Row: j.chunk.Row, Col: j.chunk.Col, Width: j.chunk.Window.Width(),
				Height: j.chunk.Window.Height(), Bytes: size, SHA256: sum, Bounds: j.chunk.Window.BBox()}
			if o.WorldFile {
				if f.WorldFile, err = WriteWorldFile(path, o.Format, j.chunk.Window); err != nil {
					encErr = err
					continue
				}
			}
			res.Files = append(res.Files, f)
			if overview != nil {
				downsampleInto(
					overview,
					j.canvas,
					j.chunk.Window.X0-l.Window.X0,
					j.chunk.Window.Y0-l.Window.Y0,
					overviewScale,
				)
			}
		}
	}()

	var runErr error
	for _, c := range l.Chunks {
		if runErr = ctx.Err(); runErr != nil {
			break
		}
		canvas := image.NewRGBA(image.Rect(0, 0, c.Window.Width(), c.Window.Height()))
		var mask *image.Alpha
		if o.Mask != nil {
			mask = o.Mask.Render(c.Window)
		}
		var list []geo.Tile
		for y := c.Tiles.Y0; y <= c.Tiles.Y1; y++ {
			for x := c.Tiles.X0; x <= c.Tiles.X1; x++ {
				t := geo.Tile{Z: c.Tiles.Z, X: x, Y: y}
				if mask != nil && !Any(mask, tileRect(t, c.Window)) {
					stats[4].Add(1)
					if o.Progress != nil {
						o.Progress(int(done.Add(1)), total)
					}
					continue
				}
				list = append(list, t)
			}
		}
		runErr = tiles.ForEach(ctx, o.Workers, list, func(ctx context.Context, t geo.Tile) error {
			img, prov, err := src.Take(ctx, t)
			if err != nil {
				return err
			}
			switch prov.Kind {
			case tiles.Native:
				stats[0].Add(1)
			case tiles.GapFill:
				stats[1].Add(1)
			case tiles.Filler:
				stats[2].Add(1)
			default:
				stats[3].Add(1)
			}
			if prov.Kind != tiles.Native {
				notesMu.Lock()
				res.GapTiles = append(res.GapTiles, TileNote{Tile: t, From: prov})
				notesMu.Unlock()
			}
			if img != nil {
				// Tiles occupy disjoint canvas regions, so concurrent draws are race-free.
				r := tileRect(t, c.Window)
				draw.Draw(canvas, r, img, img.Bounds().Min.Add(tileOffset(t, c.Window)), draw.Src)
			}
			if o.Progress != nil {
				o.Progress(int(done.Add(1)), total)
			}
			return nil
		})
		if runErr != nil {
			break
		}
		src.Forget(func(geo.Tile) bool { return false }) // drop gap-fill ancestors; bounds memory on huge areas
		if mask != nil {
			Apply(canvas, mask)
		}
		jobs <- encodeJob{chunk: c, canvas: canvas}
	}
	close(jobs)
	<-encDone
	if runErr == nil {
		runErr = encErr
	}
	if runErr == nil && overview != nil {
		path := base + "_overview" + ext
		sum, size, err := WriteFileAtomic(path, overview, o.Format, o.Quality)
		if err != nil {
			return res, err
		}
		b := overview.Bounds()
		res.Overview = &File{
			Path:   path,
			Width:  b.Dx(),
			Height: b.Dy(),
			Bytes:  size,
			SHA256: sum,
			Bounds: l.Window.BBox(),
		}
	}
	res.Stats = Stats{
		Native:      stats[0].Load(),
		GapFilled:   stats[1].Load(),
		Placeholder: stats[2].Load(),
		Empty:       stats[3].Load(),
		Skipped:     stats[4].Load(),
	}
	return res, runErr
}

// tileRect is the canvas rectangle (clipped) that tile t covers within chunk window w.
func tileRect(t geo.Tile, w geo.Window) image.Rectangle {
	x0, y0 := t.X*geo.TileSize-w.X0, t.Y*geo.TileSize-w.Y0
	return image.Rect(x0, y0, x0+geo.TileSize, y0+geo.TileSize).Intersect(image.Rect(0, 0, w.Width(), w.Height()))
}

// tileOffset is how far into the tile image the clipped canvas rectangle starts.
func tileOffset(t geo.Tile, w geo.Window) image.Point {
	x0, y0 := t.X*geo.TileSize-w.X0, t.Y*geo.TileSize-w.Y0
	return image.Pt(max(0, -x0), max(0, -y0))
}

// downsampleInto box-filters src into dst at the given scale, placing it at
// the (full-resolution) offset ox, oy.
func downsampleInto(dst, src *image.RGBA, ox, oy int, scale float64) {
	sb := src.Bounds()
	dx0, dy0 := int(float64(ox)*scale), int(float64(oy)*scale)
	dx1 := min(int(float64(ox+sb.Dx())*scale), dst.Rect.Dx())
	dy1 := min(int(float64(oy+sb.Dy())*scale), dst.Rect.Dy())
	for dy := dy0; dy < dy1; dy++ {
		sy0 := max(int(float64(dy)/scale)-oy, 0)
		sy1 := min(max(int(float64(dy+1)/scale)-oy, sy0+1), sb.Dy())
		for dx := dx0; dx < dx1; dx++ {
			sx0 := max(int(float64(dx)/scale)-ox, 0)
			sx1 := min(max(int(float64(dx+1)/scale)-ox, sx0+1), sb.Dx())
			if sx0 >= sx1 || sy0 >= sy1 {
				continue
			}
			var r, g, b, a, n int
			for sy := sy0; sy < sy1; sy++ {
				p := src.Pix[sy*src.Stride+sx0*4 : sy*src.Stride+sx1*4]
				for i := 0; i < len(p); i += 4 {
					r += int(p[i])
					g += int(p[i+1])
					b += int(p[i+2])
					a += int(p[i+3])
					n++
				}
			}
			if n == 0 {
				continue
			}
			o := dst.PixOffset(dx, dy)
			dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = uint8(r/n), uint8(g/n), uint8(b/n), uint8(a/n)
		}
	}
}
