package tiles

import (
	"context"
	"fmt"
	"sort"

	"github.com/osint-builders/geoimg/internal/geo"
)

// ZoomOptions controls adaptive zoom selection.
type ZoomOptions struct {
	MaxZoom     int     // highest zoom to try
	MinZoom     int     // lowest acceptable zoom
	MaxTiles    int     // tile budget for the whole area
	MinCoverage float64 // fraction of probe samples that must hold real imagery
	DetailRatio float64 // min finest/next octave energy ratio for native imagery (0 disables)
	Workers     int
	// Batch is how many zoom levels are probed concurrently per round trip.
	Batch int
}

// ZoomAttempt records how one zoom level was judged.
type ZoomAttempt struct {
	Zoom        int    `json:"zoom"`
	Tiles       int    `json:"tiles"`
	Sampled     int    `json:"sampled,omitempty"`
	OK          int    `json:"ok,omitempty"`
	Missing     int    `json:"missing,omitempty"`
	Placeholder int    `json:"placeholder,omitempty"`
	Failed      int    `json:"failed,omitempty"`
	Sharp       int    `json:"sharp,omitempty"`
	Upsampled   int    `json:"upsampled,omitempty"`
	Accepted    bool   `json:"accepted"`
	Reason      string `json:"reason"`
}

// SelectZoom finds the highest zoom level at which imagery genuinely exists
// for the area and fits the tile budget.
//
// Algorithm:
//  1. Budget cap: drop zoom levels whose full tile grid exceeds MaxTiles (no network).
//  2. Probe a 3×3 lattice of sample tiles (corners, edge midpoints, center) at
//     several zoom levels in parallel, one round trip per batch of levels.
//  3. A level is accepted when enough samples hold real imagery (not 404,
//     not a grey placeholder) and, if enabled, most judgeable samples carry
//     genuine pixel-scale detail rather than being a coarser level the
//     server stretched (see Measure).
func SelectZoom(ctx context.Context, src *Source, box geo.BBox, o ZoomOptions) (int, []ZoomAttempt, error) {
	var attempts []ZoomAttempt
	top := -1
	for z := o.MaxZoom; z >= o.MinZoom; z-- {
		n := box.PixelWindow(z).Tiles().Count()
		if n <= o.MaxTiles {
			top = z
			break
		}
		attempts = append(attempts, ZoomAttempt{Zoom: z, Tiles: n, Reason: fmt.Sprintf("exceeds tile budget (%d > %d)", n, o.MaxTiles)})
	}
	if top < 0 {
		n := box.PixelWindow(o.MinZoom).Tiles().Count()
		return 0, attempts, fmt.Errorf("area too large: needs %d tiles even at zoom %d (budget %d); shrink the area or raise -max-tiles", n, o.MinZoom, o.MaxTiles)
	}
	batch := max(o.Batch, 1)
	for hi := top; hi >= o.MinZoom; hi -= batch {
		lo := max(hi-batch+1, o.MinZoom)
		results := probeLevels(ctx, src, box, hi, lo, o)
		if err := ctx.Err(); err != nil {
			return 0, attempts, err
		}
		if err := allFailed(results); err != nil {
			return 0, attempts, fmt.Errorf("tile server unreachable: %w", err)
		}
		for z := hi; z >= lo; z-- {
			a := results[z]
			if a.err != nil {
				return 0, attempts, a.err
			}
			attempts = append(attempts, a.ZoomAttempt)
			if a.Accepted {
				return z, attempts, nil
			}
		}
	}
	return 0, attempts, fmt.Errorf("no imagery found between zoom %d and %d for this area", top, o.MinZoom)
}

type probeResult struct {
	ZoomAttempt
	err     error
	lastErr error // most recent non-fatal fetch error
}

// allFailed returns a fetch error when every sample in every level of the
// batch failed at the network level: stepping down zoom would not help.
func allFailed(rs map[int]probeResult) error {
	var last error
	for _, r := range rs {
		if r.err != nil || r.Failed < r.Sampled || r.Sampled == 0 {
			return nil
		}
		last = r.lastErr
	}
	return last
}

func probeLevels(ctx context.Context, src *Source, box geo.BBox, hi, lo int, o ZoomOptions) map[int]probeResult {
	var jobs []geo.Tile
	samples := map[int][]geo.Tile{}
	for z := hi; z >= lo; z-- {
		s := SampleTiles(box.PixelWindow(z).Tiles())
		samples[z] = s
		jobs = append(jobs, s...)
	}
	_ = ForEach(ctx, o.Workers, jobs, func(ctx context.Context, t geo.Tile) error {
		src.Probe(ctx, t) // memoized; results read below
		return nil
	})

	out := map[int]probeResult{}
	for z := hi; z >= lo; z-- {
		a := ZoomAttempt{Zoom: z, Tiles: box.PixelWindow(z).Tiles().Count(), Sampled: len(samples[z])}
		var fatal, last error
		for _, t := range samples[z] {
			img, st, err := src.Probe(ctx, t)
			switch {
			case err != nil:
				if isFatal(ctx, err) && fatal == nil {
					fatal = err
				}
				last = err
				a.Failed++
			case st == Missing:
				a.Missing++
			case st == Placeholder:
				a.Placeholder++
			default:
				a.OK++
				if o.DetailRatio > 0 {
					switch Measure(img).Verdict(o.DetailRatio) {
					case NativeRes:
						a.Sharp++
					case Upsampled:
						a.Upsampled++
					}
				}
			}
		}
		if fatal != nil {
			out[z] = probeResult{err: fatal}
			continue
		}
		cov := float64(a.OK) / float64(max(a.Sampled, 1))
		switch {
		case cov < o.MinCoverage:
			a.Reason = fmt.Sprintf("coverage %.0f%% < %.0f%%", cov*100, o.MinCoverage*100)
		case a.Upsampled > a.Sharp:
			a.Reason = fmt.Sprintf("upsampled: %d of %d judged samples lack native detail", a.Upsampled, a.Upsampled+a.Sharp)
		default:
			a.Accepted, a.Reason = true, "imagery available"
		}
		out[z] = probeResult{ZoomAttempt: a, lastErr: last}
	}
	return out
}

// SampleTiles picks up to nine representative tiles from a range: the
// corners, edge midpoints and center.
func SampleTiles(r geo.TileRange) []geo.Tile {
	xs := uniq(r.X0, (r.X0+r.X1)/2, r.X1)
	ys := uniq(r.Y0, (r.Y0+r.Y1)/2, r.Y1)
	out := make([]geo.Tile, 0, len(xs)*len(ys))
	for _, y := range ys {
		for _, x := range xs {
			out = append(out, geo.Tile{Z: r.Z, X: x, Y: y})
		}
	}
	return out
}

func uniq(v ...int) []int {
	seen := map[int]bool{}
	var out []int
	for _, x := range v {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[len(s)/2]
}
