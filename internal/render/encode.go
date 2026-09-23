// Package render turns downloaded tiles into output images: chunk planning,
// in-memory stitching, polygon clipping, gap filling, encoding and
// georeferencing sidecars.
package render

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gen2brain/webp"
)

// Format is an output image encoding.
type Format string

// Supported output formats.
const (
	WebP Format = "webp"
	JPEG Format = "jpeg"
	PNG  Format = "png"
)

// MaxWebPDim is the largest width/height the WebP bitstream can represent.
const MaxWebPDim = 16383

// MaxJPEGDim is the largest width/height Go's JPEG encoder accepts.
const MaxJPEGDim = 65535

// FormatFromPath picks the encoding from a file extension.
func FormatFromPath(path string) (Format, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".webp":
		return WebP, nil
	case ".jpg", ".jpeg":
		return JPEG, nil
	case ".png":
		return PNG, nil
	}
	return "", fmt.Errorf("unsupported output extension %q (use .webp, .jpg or .png)", filepath.Ext(path))
}

// MaxDim is the largest image edge the format supports.
func (f Format) MaxDim() int {
	if f == WebP {
		return MaxWebPDim
	}
	if f == JPEG {
		return MaxJPEGDim
	}
	return 1 << 20
}

// WorldFileExt returns the conventional world-file extension for the format.
func (f Format) WorldFileExt() string {
	switch f {
	case JPEG:
		return ".jgw"
	case PNG:
		return ".pgw"
	}
	return ".wld"
}

// Encode writes img to w in the given format. quality is 1-100; for WebP,
// 100 selects lossless mode. PNG is always lossless.
func Encode(w io.Writer, img image.Image, f Format, quality int) error {
	switch f {
	case WebP:
		if quality >= 100 {
			return webp.Encode(w, img, webp.Options{Lossless: true, Method: 2})
		}
		return webp.Encode(w, img, webp.Options{Quality: quality, Method: 2})
	case JPEG:
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	case PNG:
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		return enc.Encode(w, img)
	}
	return fmt.Errorf("unknown format %q", f)
}

// WriteFileAtomic encodes img to path via a temp file + rename and returns
// the SHA-256 of the bytes written and the file size. On any failure the temp
// file is removed and path is left untouched.
func WriteFileAtomic(path string, img image.Image, f Format, quality int) (sum string, size int64, err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".geoimg-*"+filepath.Ext(path))
	if err != nil {
		return "", 0, err
	}
	sum, size, werr := writeHashed(tmp, img, f, quality)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil {
		os.Remove(tmp.Name())
		return "", 0, fmt.Errorf("writing %s: %w", path, werr)
	}
	return sum, size, nil
}

// writeHashed encodes img to w, returning the SHA-256 and byte count written.
func writeHashed(w io.Writer, img image.Image, f Format, quality int) (sum string, size int64, err error) {
	h := sha256.New()
	cw := &countWriter{w: io.MultiWriter(w, h)}
	bw := bufio.NewWriterSize(cw, 1<<20)
	if err := Encode(bw, img, f, quality); err != nil {
		return "", 0, fmt.Errorf("encoding: %w", err)
	}
	if err := bw.Flush(); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), cw.n, nil
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
