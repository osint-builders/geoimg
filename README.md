# geoimg

Download and stitch the **sharpest available satellite imagery** for any point, bounding box, or GeoJSON area with one command.

```sh
geoimg 38.8977,-77.0365
```

`geoimg` works out which Esri World Imagery zoom level really holds imagery for your area, down to sub-meter (and in some cities sub-decimeter) resolution. It downloads the tiles in parallel and stitches them in memory. The result is a WebP (or JPEG/PNG), plus a JSON sidecar that records exactly what you got. Areas too large for a single image are split into a grid of georeferenced images and a stitched overview.

It ships as one static binary for Windows, macOS and Linux, with no runtime dependencies.

---

## Install

### macOS / Linux

```sh
curl -fsSL https://github.com/osint-builders/geoimg/releases/latest/download/install.sh | sh
```

The script picks the right build for your OS and CPU and verifies its SHA-256 checksum. It installs to `/usr/local/bin`, or to `~/.local/bin` if that isn't writable. To pin a version, set `GEOIMG_VERSION=v0.1.0`; to choose the target folder, set `GEOIMG_INSTALL_DIR`.

### Windows (PowerShell)

```powershell
irm https://github.com/osint-builders/geoimg/releases/latest/download/install.ps1 | iex
```

This installs to `%LOCALAPPDATA%\geoimg` and adds that folder to your user `PATH`.

### Manual download

Grab the archive for your platform from the [Releases page](https://github.com/osint-builders/geoimg/releases/latest), extract it and put `geoimg` (or `geoimg.exe`) somewhere on your `PATH`.

| Platform | Archive |
|---|---|
| Windows x64 | [`geoimg_windows_amd64.zip`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_windows_amd64.zip) |
| Windows ARM | [`geoimg_windows_arm64.zip`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_windows_arm64.zip) |
| macOS Apple Silicon | [`geoimg_darwin_arm64.tar.gz`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_darwin_arm64.tar.gz) |
| macOS Intel | [`geoimg_darwin_amd64.tar.gz`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_darwin_amd64.tar.gz) |
| Linux x64 | [`geoimg_linux_amd64.tar.gz`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_linux_amd64.tar.gz) |
| Linux ARM64 | [`geoimg_linux_arm64.tar.gz`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_linux_arm64.tar.gz) |

Checksums are published in `SHA256SUMS` alongside the archives.

> **macOS note:** if you downloaded the archive with a browser, Gatekeeper may block the unsigned binary. Clear the quarantine flag with `xattr -d com.apple.quarantine ./geoimg`. The install script doesn't need this step.

### From source

Requires Go 1.24 or newer.

```sh
go install -tags nodynamic github.com/osint-builders/geoimg/cmd/geoimg@latest
```

---

## Usage

```
geoimg [flags] <target>
```

The target is a single argument. Flags can go before or after it.

| Target | Meaning |
|---|---|
| `38.8977,-77.0365` | **Point**: `lat,lon`. The area is a square extending `-r` meters (default 200) from it on each side. |
| `40.748,-74.00,40.750,-73.98` | **Box**: any two opposite corners, `lat,lon,lat,lon`. |
| `area.geojson` | **GeoJSON**: Polygon, MultiPolygon, Point, LineString, Feature, FeatureCollection… |
| `-` | GeoJSON read from stdin. |

Coordinates are always **latitude first**, the same order Google Maps copies to your clipboard. GeoJSON files keep the standard `[lon, lat]` order.

### Examples

```sh
# The White House, sharpest imagery available, saved as WebP
geoimg 38.8977,-77.0365 -o whitehouse.webp

# 500 m around a point, as JPEG
geoimg 48.8584,2.2945 -r 500 -o eiffel.jpg

# A bounding box in Manhattan
geoimg 40.748,-74.00,40.750,-73.98 -o midtown.webp

# A GeoJSON polygon, masked to its outline (transparent outside), plus GIS sidecars
geoimg site.geojson -clip -world -o site.png

# See the zoom levels, tile counts and image sizes first; nothing is downloaded
geoimg 40.748,-74.00,40.750,-73.98 -n

# Pipe GeoJSON in
cat parcels.geojson | geoimg - -o parcels.webp
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `-o FILE` | `geoimg-<time>.webp` | Output image. The extension picks the format: `.webp`, `.jpg`, `.png`. |
| `-r METERS` | `200` | Distance from a point target to each edge of the square. |
| `-z ZOOM` | `23` | Highest zoom to try. geoimg steps down only when it has to. |
| `-q QUALITY` | `90` | Quality from 1 to 100. `.webp` at 100 is lossless; PNG is always lossless. |
| `-clip` | off | Mask the output to the GeoJSON polygon(s). Tiles fully outside are never downloaded. |
| `-world` | off | Write a world file (`.jgw`/`.pgw`/`.wld`) and `.prj` (EPSG:3857) so QGIS, ArcGIS and GDAL place the image exactly. |
| `-n` | off | Dry run: print per-zoom resolution, tile counts and image sizes, then exit. |
| `-j N` | `16` | Number of concurrent downloads. |
| `-quiet` | off | Suppress progress output. |
| `-version` | | Print the version. |

<details>
<summary>Advanced flags</summary>

| Flag | Default | Description |
|---|---|---|
| `-min-zoom Z` | `1` | Lowest zoom geoimg will accept. |
| `-max-tiles N` | `20000` | Safety ceiling on tiles per run. Zoom is lowered automatically to fit it. If the area doesn't fit even at `-min-zoom`, geoimg refuses before downloading anything. |
| `-chunk PX` | `8192` | Largest single image edge. Bigger areas are split into a grid of images (see below). Max 16383 for WebP. |
| `-coverage F` | `0.5` | Fraction of probe tiles that must hold real imagery for a zoom to be accepted. Holes are gap-filled. |
| `-detail F` | `0.2` | Upsampling-detector threshold (see below). `0` disables the detector. |
| `-url TEMPLATE` | Esri World Imagery | Any XYZ tile server, using `{z}`, `{x}` and `{y}` placeholders. |
| `-token KEY` | `$ARCGIS_API_KEY` | ArcGIS API key or token, appended as `?token=`. |

</details>

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Success. |
| `1` | Error: network failure, server refusal, or an area too large for the budget. |
| `2` | Usage error: bad target or flag. |
| `3` | Output was written, but some tiles had no imagery at any zoom. They are listed in the JSON. |

---

## What you get

A small area produces one image and its metadata:

```
whitehouse.webp      the stitched image, cropped to exactly your area
whitehouse.json      metadata (see below)
```

A large area is split into a grid of images, plus a downscaled overview of the whole area:

```
county_r00_c00.webp  county_r00_c01.webp  …
county_r01_c00.webp  …
county_overview.webp   whole area, longest edge ≤ 4096 px
county.json
```

`-world` adds a world file and `.prj` next to every image. The chunks then drop into any GIS as a seamless, georeferenced mosaic.

### The metadata sidecar (`<out>.json`)

Every run records:

- **Zoom decision**: the selected zoom and every level tried, each with its reason. The reason is one of "exceeds tile budget", "coverage 22% < 50%", "upsampled", or "imagery available".
- **Resolution**: ground meters per pixel at the area's center, and the EPSG:3857 pixel size.
- **Bounds**: the exact extent of the output in WGS84 and EPSG:3857, down to the pixel edge.
- **Files**: path, pixel size, byte size, grid position, bounds and **SHA-256** for each image, so you can show later that a file hasn't been altered.
- **Tile provenance**: counts of native, gap-filled, placeholder and empty tiles, with each non-native tile listed along with its source zoom.
- **Source and attribution**: the URL template and Esri's required attribution string.
- **Network**: request count, bytes downloaded, elapsed time.

Excerpt:

```json
{
  "zoom": {
    "selected": 19,
    "attempts": [
      { "zoom": 23, "tiles": 11881, "sampled": 9, "missing": 9, "accepted": false, "reason": "coverage 0% < 50%" },
      { "zoom": 22, "tiles": 3025,  "sampled": 9, "placeholder": 9, "accepted": false, "reason": "coverage 0% < 50%" },
      { "zoom": 21, "tiles": 784,   "sampled": 9, "ok": 9, "upsampled": 9, "accepted": false, "reason": "upsampled: 9 of 9 judged samples lack native detail" },
      { "zoom": 20, "tiles": 196,   "sampled": 9, "ok": 9, "upsampled": 7, "sharp": 2, "accepted": false, "reason": "upsampled: 7 of 9 judged samples lack native detail" },
      { "zoom": 19, "tiles": 49,    "sampled": 9, "ok": 9, "sharp": 9, "accepted": true, "reason": "imagery available" }
    ]
  },
  "resolution": { "ground_m_per_px": 0.2323, "epsg3857_m_per_px": 0.298582 },
  "output": { "format": "webp", "files": [ { "path": "whitehouse.webp", "width": 1722, "height": 1722, "sha256": "…" } ] },
  "tiles": { "native": 49, "gap_filled": 0, "placeholder": 0, "empty": 0 },
  "complete": true
}
```

The attempts above illustrate the kinds of decisions geoimg records. They aren't captured from a live run.

---

## How it works

```
target ─► geometry ─► zoom selection ─► chunk plan ─► parallel fetch ─► in-memory stitch ─► encode ─► files + JSON
```

### 1. Geometry

The target is converted to a WGS84 bounding box. A point grows by `-r` meters of true ground distance, with no degree distortion. The box is then projected to Web Mercator (EPSG:3857) at each zoom level. geoimg works with the exact **pixel window** of your area, not just whole tiles, so the output is cropped to the pixel.

### 2. Zoom selection: sharpest by default

The default is the highest zoom that holds real imagery for your area. geoimg finds it without downloading everything:

1. **Budget cap, with no network.** Starting from `-z` (23), skip any zoom whose tile count exceeds `-max-tiles`.
2. **Parallel probing.** Take a 3×3 lattice of sample tiles (corners, edge midpoints, center) at three zoom levels at once. That is one round trip per three levels.
3. **Classify every sample:**
   - *missing*: HTTP 404/204 or an empty body.
   - *placeholder*: Esri's grey "Map data not yet available" tile, recognised by being almost entirely unsaturated and dominated by one mid-grey.
   - *upsampled*: a coarser level the server has stretched. This is the case that silently wastes bandwidth elsewhere. geoimg measures the energy in the tile's finest spatial-frequency octave against the octave below it. Real imagery at native resolution has comparable energy in both, roughly a 1/f spectrum. A 2× stretched tile has an almost empty top octave, because stretching can't invent detail. Flat tiles such as water are treated as inconclusive and never count against a zoom.
4. **Accept** the highest zoom where at least `-coverage` of the samples hold real imagery and genuine detail outnumbers upsampling.

Probe tiles are cached and reused in the final image, so probing costs almost nothing extra.

### 3. Large areas: automatic image tiling

Once the zoom is fixed, geoimg knows the exact output size in pixels. If either edge exceeds `-chunk` (8192 px by default), the area is split into a grid of chunk images. Chunk edges are **aligned to tile boundaries**, so every source tile is downloaded exactly once and the chunks butt together with no seams or overlap. A multi-chunk run also writes a box-filtered overview of the whole area, at most 4096 px on its longest edge.

Chunking also keeps memory bounded, however large the area. At most two chunk canvases exist at once: one downloading while the previous one encodes.

### 4. Parallel fetch and gap filling

A fixed-size worker pool (`-j`, default 16) downloads tiles over keep-alive HTTP/2 connections. Transient failures (429 and 5xx responses, timeouts) are retried with jittered exponential backoff, and `Retry-After` is honoured. A server that never answers, or that refuses outright (401/403), stops the run immediately rather than burning retries.

If a tile at the chosen zoom is missing, a placeholder, or keeps failing, geoimg **gap-fills** it. It takes the nearest ancestor tile (up to 4 levels up) that has real imagery, crops the matching region and bilinearly upscales it. The image stays complete and as sharp as the source allows, and each fill is recorded in the JSON.

### 5. Stitch and encode

Workers decode tiles and draw them straight into disjoint regions of the chunk canvas. No locks are needed and no temporary files are written. Each chunk is encoded while the next one downloads. Files are written to a temp file and then renamed, so an interrupted run never leaves a half-written image behind. The SHA-256 is computed during the write.

### Output formats

| Format | When to use |
|---|---|
| **WebP** (default) | Best size for quality. Lossy WebP is typically 25–35% smaller than JPEG at equal visual quality, supports transparency (`-clip`) and opens in every modern browser. `-q 100` gives lossless WebP. Edges are limited to 16383 px, which chunking handles. |
| **JPEG** | Maximum compatibility and the fastest encode. No transparency: pixels outside a clip mask are black. |
| **PNG** | Lossless, with transparency. Files are large. |

The WebP encoder is libwebp compiled to pure Go. Release binaries are fully static, need no C toolchain to build, and run even on musl/Alpine.

### Performance tips

- Keep `-j` at 16–32. Much higher gains little and is unkind to the server.
- Peak memory is about `2 × chunk² × 4` bytes, roughly 512 MB at the default `-chunk 8192`. On small machines, use `-chunk 4096`, which needs about 128 MB.
- Use `.jpg` if encode speed matters more than file size.
- Run `-n` first on big areas; it shows how many tiles each zoom needs.

---

## Imagery source, terms and attribution

By default geoimg reads Esri's public **World Imagery** tile service (`server.arcgisonline.com`). A few points you need to know:

- **Attribution is required.** Anywhere you display the imagery, credit *"Esri, Maxar, Earthstar Geographics, and the GIS User Community"*. geoimg writes this string into every JSON sidecar.
- **Esri's terms govern your use**, not this tool. The service is publicly reachable, but that isn't a blanket licence for bulk or commercial harvesting. For production or commercial work, use an ArcGIS account and API key (`-token` or `ARCGIS_API_KEY`), and review Esri's terms of use for basemaps.
- **Be a good citizen.** geoimg identifies itself honestly (`User-Agent: geoimg/<version>`). It doesn't disguise itself as a browser, it keeps concurrency bounded and it backs off when throttled. Heavy abuse can still get an IP rate-limited, so don't raise `-j` or `-max-tiles` further than you need.

`-url` accepts any XYZ tile server; use only sources whose terms allow it.

---

## Development

```sh
make build     # ./geoimg for this machine
make test      # go test -race ./...
make lint      # gofmt + go vet
make dist      # cross-compile all release archives into dist/
```

The test suite needs no network. It runs against a synthetic tile server (`internal/tiles/tilestest`) that reproduces Esri's behaviours: a native maximum zoom, 404s, grey placeholders, server-side upsampling, holes and rate limiting.

### Layout

| Package | Responsibility |
|---|---|
| `cmd/geoimg` | Entry point; Ctrl-C cancels cleanly. |
| `internal/cli` | Argument parsing, orchestration, dry run, JSON metadata. |
| `internal/geo` | WGS84 ↔ Web Mercator math, pixel windows, tile ranges, target and GeoJSON parsing. |
| `internal/tiles` | HTTP fetching and retries, placeholder and upsampling detection, adaptive zoom selection, gap filling, worker pool. |
| `internal/render` | Chunk planning, stitching, polygon masks, encoding, world files, overview. |

### Releasing

Releases are built by GitHub Actions (`.github/workflows/release.yml`). Push a version tag:

```sh
git tag v0.2.0
git push origin v0.2.0
```

The workflow runs the tests, cross-compiles six targets (Windows, macOS and Linux, each on amd64 and arm64), and packages the archives, `SHA256SUMS` and both install scripts. It then publishes them as a GitHub Release. To rebuild the assets for an existing tag, run the workflow manually from the Actions tab and enter the tag.
