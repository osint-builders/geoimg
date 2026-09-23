package tiles

import (
	"image"
	"image/color"
)

// IsPlaceholder reports whether img looks like a filler tile such as Esri's
// grey "Map data not yet available" square: almost entirely unsaturated,
// dominated by a single mid-grey, with only a little darker grey text.
//
// Real imagery essentially never meets all three conditions; oceans are
// saturated, and snow/cloud is brighter than the grey band.
func IsPlaceholder(img image.Image) bool {
	var hist [256]int
	var grey, total int
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 4 {
		for x := b.Min.X; x < b.Max.X; x += 4 {
			r, g, bl := rgb8(img.At(x, y))
			total++
			if max3(r, g, bl)-min3(r, g, bl) <= 8 {
				grey++
				hist[(int(r)+int(g)+int(bl))/3]++
			}
		}
	}
	if total == 0 || float64(grey) < 0.97*float64(total) {
		return false
	}
	mode := 0
	for v := range hist {
		if hist[v] > hist[mode] {
			mode = v
		}
	}
	if mode < 150 || mode > 235 {
		return false
	}
	near := 0
	for v := max(0, mode-8); v <= min(255, mode+8); v++ {
		near += hist[v]
	}
	return float64(near) >= 0.75*float64(total)
}

func rgb8(c color.Color) (r8, g8, b8 uint8) {
	r, g, b, _ := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

func max3(a, b, c uint8) uint8 { return max(a, b, c) }
func min3(a, b, c uint8) uint8 { return min(a, b, c) }

// Sharpness summarises how much fine detail a tile carries.
type Sharpness struct {
	Top      float64 // energy in the finest octave (pixel scale)
	Next     float64 // energy in the octave below it
	Variance float64 // total luminance variance
}

// Verdicts returned by Sharpness.Verdict.
const (
	Inconclusive = iota // too flat to judge (water, haze, uniform roofs)
	NativeRes           // real detail down to pixel scale
	Upsampled           // stretched from a coarser level
)

// Measure computes the Sharpness of img.
//
// Real imagery at its native resolution has a roughly 1/f spectrum, so its
// finest octave holds energy comparable to the octave below (Top/Next ≈
// 0.4–1). A tile the server has upsampled by 2× has an almost empty finest
// octave (≈ 0.15 or less), and one upsampled 4× or more is smooth in both
// while still showing coarse structure. Stretching cannot invent detail.
func Measure(img image.Image) Sharpness {
	l0, w, h := luminance(img)
	l1, w1, h1 := half(l0, w, h)
	l2, w2, h2 := half(l1, w1, h1)
	var mean, sq float64
	for _, v := range l0 {
		mean += v
		sq += v * v
	}
	n := float64(len(l0))
	mean /= n
	return Sharpness{
		Top:      bandEnergy(l0, w, h, l1, w1, h1),
		Next:     bandEnergy(l1, w1, h1, l2, w2, h2),
		Variance: sq/n - mean*mean,
	}
}

// Verdict classifies the tile; threshold is the minimum Top/Next ratio
// accepted as native resolution.
func (s Sharpness) Verdict(threshold float64) int {
	switch {
	case s.Variance < 4:
		return Inconclusive
	case s.Next < 2:
		if s.Variance >= 16 {
			return Upsampled // coarse structure but no fine detail at all
		}
		return Inconclusive
	case s.Top/s.Next < threshold:
		return Upsampled
	}
	return NativeRes
}

func luminance(img image.Image) (lum []float64, w, h int) {
	b := img.Bounds()
	w, h = b.Dx(), b.Dy()
	out := make([]float64, w*h)
	if yc, ok := img.(*image.YCbCr); ok {
		for y := 0; y < h; y++ {
			row := yc.Y[yc.YOffset(b.Min.X, b.Min.Y+y):]
			for x := 0; x < w; x++ {
				out[y*w+x] = float64(row[x])
			}
		}
		return out, w, h
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl := rgb8(img.At(b.Min.X+x, b.Min.Y+y))
			out[y*w+x] = 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)
		}
	}
	return out, w, h
}

// half box-downsamples by two.
func half(l []float64, w, h int) (out []float64, w2, h2 int) {
	w2, h2 = w/2, h/2
	out = make([]float64, w2*h2)
	for y := 0; y < h2; y++ {
		for x := 0; x < w2; x++ {
			i := 2*y*w + 2*x
			out[y*w2+x] = (l[i] + l[i+1] + l[i+w] + l[i+w+1]) / 4
		}
	}
	return out, w2, h2
}

// bandEnergy is the mean squared difference between fine and the bilinear
// reconstruction of coarse (its 2× downsample), i.e. the detail coarse lost.
func bandEnergy(fine []float64, w, h int, coarse []float64, cw, ch int) float64 {
	var sum float64
	n := 0
	for y := 1; y < h-1; y++ {
		cy := (float64(y)+0.5)/2 - 0.5
		y0 := min(max(int(cy), 0), ch-2)
		fy := cy - float64(y0)
		for x := 1; x < w-1; x++ {
			cx := (float64(x)+0.5)/2 - 0.5
			x0 := min(max(int(cx), 0), cw-2)
			fx := cx - float64(x0)
			i := y0*cw + x0
			top := coarse[i]*(1-fx) + coarse[i+1]*fx
			bot := coarse[i+cw]*(1-fx) + coarse[i+cw+1]*fx
			d := fine[y*w+x] - (top*(1-fy) + bot*fy)
			sum += d * d
			n++
		}
	}
	return sum / float64(n)
}
