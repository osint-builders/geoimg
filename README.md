# geoimg

Download and stitch the **sharpest available satellite imagery** for any point, bounding box, or GeoJSON area with one command.

```sh
geoimg 38.8977,-77.0365
```

`geoimg` works out which Esri World Imagery zoom level really holds imagery for your area, down to sub-meter (and in some cities sub-decimeter) resolution. It downloads the tiles in parallel and stitches them in memory. The result is a WebP (or JPEG/PNG), plus a JSON sidecar that records exactly what you got. Areas too large for a single image are split into a grid of georeferenced images and a stitched overview.

It ships as one static binary for Windows, macOS and Linux, with no runtime dependencies.

**Contents:** [Install](#install) · [Quick start](#quick-start) · [CLI reference](#cli-reference) · [Output files](#output-files) · [Metadata JSON reference](#metadata-json-reference) · [Recipes](#recipes) · [Do's and don'ts](#dos-and-donts) · [How it works](#how-it-works) · [Terms and attribution](#imagery-source-terms-and-attribution) · [Development](#development)

---

## Install

Every tagged version is published on the [GitHub Releases page](https://github.com/osint-builders/geoimg/releases). Each release contains:

| Asset | What it is |
|---|---|
| `geoimg_<os>_<arch>.tar.gz` / `.zip` | The binary for one platform, plus this README. |
| `SHA256SUMS` | SHA-256 checksums of every archive. |
| `install.sh` | One-line installer for macOS and Linux. |
| `install.ps1` | One-line installer for Windows PowerShell. |

Pick one of the options below. They all end with the same single `geoimg` binary on your `PATH`.

### Option A: one-line installer (recommended)

**macOS / Linux**

```sh
curl -fsSL https://github.com/osint-builders/geoimg/releases/latest/download/install.sh | sh
```

The script detects your OS and CPU, downloads the matching archive, **verifies its SHA-256 against `SHA256SUMS`**, and installs to `/usr/local/bin` (or `~/.local/bin` if that isn't writable). It prints a PATH hint if the folder isn't on your `PATH`.

| Variable | Effect |
|---|---|
| `GEOIMG_VERSION` | Install a specific tag instead of the latest, e.g. `GEOIMG_VERSION=v0.1.0`. |
| `GEOIMG_INSTALL_DIR` | Install into this folder instead. |

```sh
# Pin a version and install into ~/bin
curl -fsSL https://github.com/osint-builders/geoimg/releases/latest/download/install.sh \
  | GEOIMG_VERSION=v0.1.0 GEOIMG_INSTALL_DIR="$HOME/bin" sh
```

**Windows (PowerShell)**

```powershell
irm https://github.com/osint-builders/geoimg/releases/latest/download/install.ps1 | iex
```

This detects x64 vs ARM64, verifies the checksum, installs to `%LOCALAPPDATA%\geoimg` and adds that folder to your user `PATH`. Open a new terminal afterwards. Set `$env:GEOIMG_VERSION = 'v0.1.0'` first to pin a version.

### Option B: manual download from the Releases page

1. Open the [latest release](https://github.com/osint-builders/geoimg/releases/latest) (or pick an older one from the [releases list](https://github.com/osint-builders/geoimg/releases)).
2. Under **Assets**, download the archive for your platform and `SHA256SUMS`:

   | Platform | Archive |
   |---|---|
   | Windows x64 | [`geoimg_windows_amd64.zip`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_windows_amd64.zip) |
   | Windows ARM64 | [`geoimg_windows_arm64.zip`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_windows_arm64.zip) |
   | macOS Apple Silicon (M1–M4) | [`geoimg_darwin_arm64.tar.gz`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_darwin_arm64.tar.gz) |
   | macOS Intel | [`geoimg_darwin_amd64.tar.gz`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_darwin_amd64.tar.gz) |
   | Linux x64 | [`geoimg_linux_amd64.tar.gz`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_linux_amd64.tar.gz) |
   | Linux ARM64 (Raspberry Pi 4/5, Graviton) | [`geoimg_linux_arm64.tar.gz`](https://github.com/osint-builders/geoimg/releases/latest/download/geoimg_linux_arm64.tar.gz) |

   Not sure which one? Run `uname -sm` (macOS/Linux) or `$env:PROCESSOR_ARCHITECTURE` (PowerShell). `x86_64`/`AMD64` means amd64; `arm64`/`aarch64`/`ARM64` means arm64.

3. **Verify the checksum** (the hash printed must match the line for your archive in `SHA256SUMS`):

   ```sh
   # Linux
   sha256sum -c SHA256SUMS --ignore-missing
   # macOS
   shasum -a 256 -c SHA256SUMS --ignore-missing
   ```

   ```powershell
   # Windows: compare with SHA256SUMS (case-insensitive)
   (Get-FileHash .\geoimg_windows_amd64.zip -Algorithm SHA256).Hash
   ```

4. **Extract** and move the binary onto your `PATH`. Each archive contains a folder named after itself:

   ```sh
   # macOS / Linux
   tar -xzf geoimg_linux_amd64.tar.gz
   sudo install -m 0755 geoimg_linux_amd64/geoimg /usr/local/bin/geoimg
   # or, without sudo:
   mkdir -p ~/.local/bin && install -m 0755 geoimg_linux_amd64/geoimg ~/.local/bin/geoimg
   ```

   ```powershell
   # Windows
   Expand-Archive .\geoimg_windows_amd64.zip -DestinationPath .
   New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\geoimg" | Out-Null
   Copy-Item .\geoimg_windows_amd64\geoimg.exe "$env:LOCALAPPDATA\geoimg\"
   # then add %LOCALAPPDATA%\geoimg to your user PATH (Settings → Environment Variables)
   ```

5. **macOS only:** a browser-downloaded binary is quarantined by Gatekeeper, which blocks unsigned binaries. Clear the flag once:

   ```sh
   xattr -d com.apple.quarantine /usr/local/bin/geoimg
   ```

   (The install script downloads with `curl`, which doesn't set the flag, so it doesn't need this.)

6. **Check it works:**

   ```sh
   geoimg -version        # → geoimg v0.1.0
   ```

### Option C: from source

Requires Go 1.24 or newer. The `nodynamic` tag keeps the WebP encoder pure Go, so no C toolchain is needed.

```sh
go install -tags nodynamic github.com/osint-builders/geoimg/cmd/geoimg@latest
```

A binary built this way reports its version as `dev`.

### Upgrading and uninstalling

- **Upgrade:** re-run the installer, or download the newer archive and overwrite the binary. There is no config or cache to migrate.
- **Uninstall:** delete the binary (`/usr/local/bin/geoimg`, `~/.local/bin/geoimg`, or `%LOCALAPPDATA%\geoimg`) and, on Windows, remove that folder from your user `PATH`. geoimg writes nothing else outside the output paths you give it.

---

## Quick start

```sh
# 1. Preview what an area costs. Nothing is downloaded.
geoimg 38.8977,-77.0365 -n

# 2. Download the sharpest imagery for it.
geoimg 38.8977,-77.0365 -o whitehouse.webp
```

Typical output (zoom and sizes depend on the imagery available when you run it):

```
probing zoom levels 23→1 …                                   ← stderr
zoom 19 (0.23 m/px) · 1722×1722 px · 49 tiles · 1 image(s)   ← stderr
whitehouse.webp  1722×1722  …                                ← stdout
whitehouse.json  metadata                                    ← stdout
```

You now have `whitehouse.webp` and `whitehouse.json`, which records which zoom was chosen, the exact bounds, and a SHA-256 of the image.

---

## CLI reference

### Synopsis

```
geoimg [flags] <target>
geoimg -version
geoimg -h
```

- Exactly **one** target is required. Flags may go before or after it.
- Every flag has a single-dash form (`-o`). Go's flag parser also accepts `--o`, `-o=file` and `-o file`.
- Negative numbers are never mistaken for flags: `geoimg -33.8568,151.2153` works as-is.
- `--` ends flag parsing; everything after it is positional.

### Targets

| Form | Kind | Example | Area covered |
|---|---|---|---|
| `lat,lon` | point | `38.8977,-77.0365` | A square extending `-r` meters (default 200) from the point to each edge, so 400 m × 400 m by default. |
| `lat1,lon1,lat2,lon2` | bbox | `40.748,-74.00,40.750,-73.98` | The box between two **opposite corners**, in any order. |
| `path/to/file.geojson` | geojson | `site.geojson` | The bounding box of every coordinate in the file. |
| `-` | geojson | `cat a.geojson \| geoimg -` | Same, read from stdin. |

Rules:

- Command-line coordinates are **latitude first** (`lat,lon`), the order Google Maps copies to your clipboard. GeoJSON keeps the standard **`[lon, lat]`** order. Out-of-range values are rejected with a hint about the order.
- Separators can be commas, semicolons or spaces, but a target containing spaces must be quoted: `"38.8977, -77.0365"`.
- Supported GeoJSON types: `FeatureCollection`, `Feature`, `GeometryCollection`, `Point`, `MultiPoint`, `LineString`, `MultiLineString`, `Polygon`, `MultiPolygon`.
- GeoJSON made only of points, or with zero width or height (e.g. a vertical line), is padded by `-r` meters.
- A target cannot cross the antimeridian (±180° longitude). Split such areas into two runs.

### Flags

| Flag | Alias | Type | Default | Valid range | Description |
|---|---|---|---|---|---|
| `-o FILE` | `-out` | path | `geoimg-YYYYMMDD-HHMMSS.webp` | `.webp` `.jpg` `.jpeg` `.png` | Output image. The extension picks the format. Missing parent folders are created. |
| `-r METERS` | `-radius` | float | `200` | `> 0` | Distance from a point target to each edge of the square. Also pads point-only GeoJSON. |
| `-z ZOOM` | `-zoom` | int | `23` | `0`–`23` | Highest zoom to try. geoimg steps down from here only when it has to. |
| `-q QUALITY` | `-quality` | int | `90` | `1`–`100` | Encoder quality. WebP at `100` is lossless. Ignored for PNG (always lossless). |
| `-clip` | | bool | off | | Make everything outside the GeoJSON polygon(s) transparent (black for JPEG). Tiles fully outside are never downloaded. Requires a GeoJSON target containing `Polygon`/`MultiPolygon`. |
| `-world` | | bool | off | | Write a world file (`.jgw`/`.pgw`/`.wld`) and `.prj` (EPSG:3857) next to every image. |
| `-n` | `-dry-run` | bool | off | | Print per-zoom resolution, tile count, pixel size and image count, then exit. No network access. |
| `-j N` | `-workers` | int | `16` | `1`–`64` | Concurrent downloads. |
| `-quiet` | | bool | off | | Suppress progress output on stderr. Results and warnings still print. |
| `-version` | | bool | | | Print `geoimg <version>` and exit. |
| `-h` | `-help` | | | | Print usage and exit `0`. |

**Advanced flags**

| Flag | Type | Default | Valid range | Description |
|---|---|---|---|---|
| `-min-zoom Z` | int | `1` | `0`–`-z` | Lowest zoom geoimg will accept. The run fails rather than go below it. |
| `-max-tiles N` | int | `20000` | `≥ 1` | Safety ceiling on tiles per run. Zooms over budget are skipped without any network access. If nothing fits even at `-min-zoom`, geoimg refuses before downloading. |
| `-chunk PX` | int | `8192` | `256`–`16383` (WebP), `256`–`65535` (JPEG) | Largest single image edge. Bigger outputs become a grid of images plus an overview. |
| `-coverage F` | float | `0.5` | `(0, 1]` | Fraction of probe tiles that must hold real imagery for a zoom to be accepted. The holes are gap-filled. |
| `-detail F` | float | `0.2` | `≥ 0` | Upsampling-detector threshold (see [How it works](#2-zoom-selection-sharpest-by-default)). `0` disables it. |
| `-url TEMPLATE` | string | Esri World Imagery | must contain `{z}`, `{x}`, `{y}` | Any XYZ tile server. |
| `-token KEY` | string | `$ARCGIS_API_KEY` | | ArcGIS API key or token, appended to each request as `?token=` (or `&token=`). Redacted from error messages and never written to the JSON. |

### Environment variables

| Variable | Used by | Effect |
|---|---|---|
| `ARCGIS_API_KEY` | `geoimg` | Default for `-token`. |
| `GEOIMG_VERSION` | installers | Tag to install. |
| `GEOIMG_INSTALL_DIR` | `install.sh` | Install folder. |

### Streams

| Stream | Contents |
|---|---|
| **stdout** | Only results: one line per written image (`path  W×H  size`) or a grid summary plus the overview line, then `<meta>.json  metadata`. In dry-run mode, the zoom table. Safe to parse. |
| **stderr** | Progress (`probing…`, `zoom …`, the tile counter), warnings and errors. The live tile counter appears only when stderr is a terminal. `-quiet` hides progress, not errors or warnings. |

### Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Success. All tiles have imagery. | |
| `1` | Runtime error: network failure, server refusal (401/403), write failure, or the area doesn't fit `-max-tiles` at any zoom ≥ `-min-zoom`. Also Ctrl-C (prints `interrupted`). | Read the message on stderr. For budget errors, shrink the area or raise `-max-tiles`/lower `-min-zoom`. |
| `2` | Usage error: bad target, unknown flag, out-of-range value, unsupported extension, `-clip` without polygons, or `-chunk` over the format limit. Nothing was downloaded. | Fix the command. |
| `3` | Output was written, but some tiles had no imagery at any zoom (filled black/transparent). | Check `gap_tiles` in the JSON. Often open ocean or the poles. |

---

## Output files

Given `-o NAME.EXT`:

| File | When | Contents |
|---|---|---|
| `NAME.EXT` | output fits in one `-chunk` | The stitched image, cropped to exactly your area. |
| `NAME_rRR_cCC.EXT` | output larger than `-chunk` | One image per grid cell. Row/column are zero-padded to at least 2 digits (`_r00_c00`, `_r00_c01`, …). |
| `NAME_overview.EXT` | chunked output | The whole area, box-filtered to ≤ 4096 px on its longest edge. |
| `*.jgw` / `*.pgw` / `*.wld` + `*.prj` | `-world` | For each image: an ESRI world file (JPEG/PNG/WebP respectively) and an EPSG:3857 projection file. |
| `NAME.json` | always | The [metadata sidecar](#metadata-json-reference). |

Images and JSON are written to a temp file and renamed into place, so an interrupted run never leaves a half-written file. Existing files with the same names are **overwritten**.

### Formats

| Extension | Max edge | Transparency | Notes |
|---|---|---|---|
| `.webp` (default) | 16383 px | yes | Best size for quality; `-q 100` is lossless. |
| `.jpg` / `.jpeg` | 65535 px | no (clipped areas are black) | Most compatible, fastest to encode. |
| `.png` | unlimited (chunked at `-chunk`) | yes | Lossless, large. |

---

## Metadata JSON reference

Every run writes `<out>.json` (the output path with its extension replaced). It is the machine-readable record of the run: use it to georeference, audit, or prove a file hasn't changed.

| Field | Type | Description |
|---|---|---|
| `tool`, `version` | string | `"geoimg"` and the binary's version. |
| `created_utc` | string | RFC 3339 start time. |
| `source.provider` / `url_template` / `attribution` | string | Tile source and the attribution you must display. |
| `target.kind` | string | `point`, `bbox` or `geojson`. |
| `target.input` | string | The target argument as given (`stdin` for `-`). |
| `target.radius_m` | number | Point targets only. |
| `target.requested_bbox` | bbox | The area you asked for, in WGS84. |
| `target.polygons` / `target.clip` | int / bool | Polygons found in the GeoJSON; whether `-clip` was used. |
| `zoom.selected` | int | The zoom used. |
| `zoom.max_requested` / `min_allowed` | int | `-z` and `-min-zoom`. |
| `zoom.attempts[]` | array | Every zoom considered, highest first: `zoom`, `tiles`, `sampled`, `ok`, `missing`, `placeholder`, `failed`, `sharp`, `upsampled`, `accepted`, `reason`. Zero counts are omitted. |
| `resolution.ground_m_per_px` | number | True ground meters per pixel at the area's center latitude. |
| `resolution.epsg3857_m_per_px` | number | Pixel size in Web Mercator units (what the world file uses). |
| `grid.tile_range` | object | `{z, x_min, y_min, x_max, y_max}` of source tiles. |
| `grid.tile_count`, `pixel_width`, `pixel_height` | int | Size of the full output. |
| `grid.chunk_rows`, `chunk_cols`, `chunk_max_side` | int | Chunk grid layout. |
| `bounds.wgs84` | bbox | The **actual** image extent, snapped to pixel edges (slightly larger than requested). |
| `bounds.epsg3857` | `[minx, miny, maxx, maxy]` | The same extent in meters, EPSG:3857. |
| `output.format`, `output.quality` | string / int | Encoder settings. |
| `output.files[]` | array | Per image: `path`, `row`, `col`, `width`, `height`, `bytes`, `sha256`, `bounds`, `world_file` (with `-world`). |
| `output.overview` | object | Same shape as a file entry; present only for chunked output. |
| `tiles` | object | Counts: `native`, `gap_filled`, `placeholder`, `empty`, `skipped_outside_clip`. |
| `gap_tiles[]` | array | Every non-native tile: `tile {z,x,y}` and `source {kind, from_zoom}`, where `kind` is `gapfill`, `placeholder` or `empty`. Omitted when empty. |
| `network` | object | `requests`, `bytes` downloaded, `workers`. |
| `elapsed_seconds` | number | Wall time. |
| `complete` | bool | `false` when any tile had no imagery (exit code `3`). |

A `bbox` is `{"min_lat", "min_lon", "max_lat", "max_lon"}`.

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

## Recipes

```sh
# The White House, sharpest imagery available, saved as WebP
geoimg 38.8977,-77.0365 -o whitehouse.webp

# 500 m around a point, as JPEG
geoimg 48.8584,2.2945 -r 500 -o eiffel.jpg

# Southern/western hemispheres: negative coordinates just work
geoimg -33.8568,151.2153 -o opera-house.webp

# A bounding box in Manhattan (any two opposite corners)
geoimg 40.748,-74.00,40.750,-73.98 -o midtown.webp

# A parcel polygon, transparent outside its outline, ready for QGIS
geoimg site.geojson -clip -world -o site.png

# GeoJSON from another tool on stdin
ogr2ogr -f GeoJSON /vsistdout/ parcels.shp | geoimg - -o parcels.webp

# Cap resolution at ~1 m/px for a fast, small overview of a big area
geoimg 40.70,-74.02,40.80,-73.93 -z 17 -o manhattan.jpg

# Large area on a small machine: smaller chunks, lower memory
geoimg county.geojson -chunk 4096 -world -o out/county.webp

# Use your own ArcGIS key without leaving it in shell history
export ARCGIS_API_KEY=...   # or put it in your shell profile / secrets manager
geoimg 51.5007,-0.1246 -o bigben.webp

# Scripting: accept full or partial coverage, fail on anything else
geoimg 38.8977,-77.0365 -quiet -o wh.webp; rc=$?
case $rc in
  0) ;;
  3) echo "partial coverage, see gap_tiles in wh.json" >&2 ;;
  *) exit $rc ;;
esac
jq '.zoom.selected, .resolution.ground_m_per_px' wh.json

# Verify an image against its sidecar later
jq -r '.output.files[] | "\(.sha256)  \(.path)"' wh.json | sha256sum -c
```

For the checksum check to pass, run it from the folder you ran geoimg in, since `path` is recorded as you passed it to `-o`.

---

## Do's and don'ts

### Do

- **Do run `-n` first** on anything bigger than a few city blocks. It costs nothing and shows tile counts and pixel sizes per zoom.
- **Do give coordinates as `lat,lon`**, the way map apps copy them. Only GeoJSON uses `[lon, lat]`.
- **Do quote targets that contain spaces**: `"38.8977, -77.0365"`.
- **Do keep the `.json` sidecar with the image.** It holds the bounds, resolution, provenance and SHA-256; without it the image is just pixels.
- **Do use `-world`** when the output is headed for QGIS, ArcGIS or GDAL. Use `-clip` when you only care about the inside of a polygon: it skips downloading tiles outside it.
- **Do check the exit code in scripts.** Treat `3` as "usable, but inspect `gap_tiles`", not as success.
- **Do pass `-token` through `ARCGIS_API_KEY`** for production or commercial work, and read Esri's terms.
- **Do display the attribution** (`source.attribution` in the JSON) wherever the imagery is shown.
- **Do lower `-chunk` to 4096** on machines with ≤ 2 GB free RAM.
- **Do pin a version** (`GEOIMG_VERSION=v0.1.0`) in CI and scripts so results are reproducible.

### Don't

- **Don't swap latitude and longitude.** Swapped pairs are rejected only when the "latitude" exceeds ±90°. The White House as `-77.0365,38.8977` is a valid point in Antarctica and will happily download the wrong place. `-n` prints the resolved bbox; glance at it.
- **Don't put spaces in a target without quotes.** `geoimg 38.8977, -77.0365` is two arguments and fails with exit `2`.
- **Don't crank `-j` past 32 or raise `-max-tiles` casually.** You gain little speed and risk being rate-limited or blocked. The limit on `-j` is 64.
- **Don't expect `-z 23` to mean 1.5 cm/px imagery.** It's where probing *starts*. geoimg drops to the highest zoom with real, non-upsampled imagery, and the JSON tells you which.
- **Don't use `-clip` with a point or bbox target.** It needs GeoJSON polygons and exits `2` otherwise.
- **Don't request areas that cross ±180° longitude.** Split them into two runs.
- **Don't set `-chunk` above 16383 for WebP.** It's rejected; use JPEG or PNG for bigger single images, or keep chunking.
- **Don't put secrets in `-url` or on the command line in shared shells.** `-url` is written to the JSON verbatim, and shell history keeps `-token`. Use `ARCGIS_API_KEY`.
- **Don't reuse an output name you want to keep.** Existing files are overwritten without a prompt.
- **Don't treat the public Esri endpoint as a licence** for bulk or commercial harvesting. See [Terms and attribution](#imagery-source-terms-and-attribution).
- **Don't disable the detector (`-detail 0`)** unless you know the source never upsamples. Otherwise you may download 4–16× more tiles with no extra detail.

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
make build     # the single static ./geoimg binary for this machine
make test      # go test -race ./...
make fmt       # rewrite code with goimports + golines
make lint      # goimports, golines (120 cols), go vet, go-critic (all checks)
make dist      # cross-compile all release archives into dist/
```

The linters are pinned in the `Makefile` and run through `go run`, so there's nothing to install. CI runs `make lint` on every push, and a PR must be clean under all four checks.

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
