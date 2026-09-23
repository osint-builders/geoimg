package cli

import (
	"encoding/json"
	"os"

	"github.com/osint-builders/geoimg/internal/geo"
	"github.com/osint-builders/geoimg/internal/render"
	"github.com/osint-builders/geoimg/internal/tiles"
)

// Metadata is the JSON sidecar written next to every output.
type Metadata struct {
	Tool       string            `json:"tool"`
	Version    string            `json:"version"`
	CreatedUTC string            `json:"created_utc"`
	Source     MetaSource        `json:"source"`
	Target     MetaTarget        `json:"target"`
	Zoom       MetaZoom          `json:"zoom"`
	Resolution MetaResolution    `json:"resolution"`
	Grid       MetaGrid          `json:"grid"`
	Bounds     MetaBounds        `json:"bounds"`
	Output     MetaOutput        `json:"output"`
	Tiles      render.Stats      `json:"tiles"`
	GapTiles   []render.TileNote `json:"gap_tiles,omitempty"`
	Network    MetaNetwork       `json:"network"`
	ElapsedSec float64           `json:"elapsed_seconds"`
	Complete   bool              `json:"complete"`
}

type MetaSource struct {
	Provider    string `json:"provider"`
	URLTemplate string `json:"url_template"`
	Attribution string `json:"attribution"`
}

type MetaTarget struct {
	Kind     string   `json:"kind"`
	Input    string   `json:"input"`
	RadiusM  float64  `json:"radius_m,omitempty"`
	BBox     geo.BBox `json:"requested_bbox"`
	Polygons int      `json:"polygons,omitempty"`
	Clip     bool     `json:"clip"`
}

type MetaZoom struct {
	Selected int                 `json:"selected"`
	MaxTried int                 `json:"max_requested"`
	Min      int                 `json:"min_allowed"`
	Attempts []tiles.ZoomAttempt `json:"attempts"`
}

type MetaResolution struct {
	GroundMetersPerPixel   float64 `json:"ground_m_per_px"`
	MercatorMetersPerPixel float64 `json:"epsg3857_m_per_px"`
}

type MetaGrid struct {
	Tiles        geo.TileRange `json:"tile_range"`
	TileCount    int           `json:"tile_count"`
	PixelWidth   int           `json:"pixel_width"`
	PixelHeight  int           `json:"pixel_height"`
	ChunkRows    int           `json:"chunk_rows"`
	ChunkCols    int           `json:"chunk_cols"`
	ChunkMaxSide int           `json:"chunk_max_side"`
}

type MetaBounds struct {
	WGS84    geo.BBox   `json:"wgs84"`
	EPSG3857 [4]float64 `json:"epsg3857"` // minx, miny, maxx, maxy
}

type MetaOutput struct {
	Format   string        `json:"format"`
	Quality  int           `json:"quality"`
	Files    []render.File `json:"files"`
	Overview *render.File  `json:"overview,omitempty"`
}

type MetaNetwork struct {
	Requests int64 `json:"requests"`
	Bytes    int64 `json:"bytes"`
	Workers  int   `json:"workers"`
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
