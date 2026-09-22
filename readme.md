# geoimg

A high-performance, cross-platform Command Line Interface (CLI) tool written in Go to harvest and stitch ultra-high-resolution satellite imagery using public Esri World Imagery (ArcGIS) servers.

`geoimg` bypasses restrictive web interfaces by calculating map tile matrices directly, downloading them concurrently via a managed worker pool, and stitching them into a single, high-resolution JPEG completely in-memory.

---

## Key Features

* **Adaptive Zoom Engine**: Automatically attempts to fetch imagery at the absolute closest ground resolution (Zoom 19, ~30cm per pixel via Maxar). If sub-meter resolution is unavailable for a remote region, it seamlessly steps down zoom layers until a valid matrix is found.
* **Blazing Fast Concurrency**: Leverages Go routines and an isolated worker pool to download dozens of map tiles simultaneously.
* **Zero Disk Bloat**: Decodes and stitches image grids completely in system memory before writing a single unified output file to your drive.
* **Cross-Platform Delivery**: Compiles natively into standalone, independent binaries tailored specifically to Windows, Linux, and macOS without requiring external runtimes.
* **Safe Harvesting Guards**: Built-in memory ceilings prevent accidental massive bounding box requests from crashing your system.

---

## Architectural Workflow

[CLI Coordinates Input] 
         │
         ▼
[Geometry Engine] ──► Translates Lat/Lon bounds to Web Mercator X/Y/Z Tiles
         │
         ▼
[Adaptive Network] ──► Probes Zoom 19 ──► (Fallback to Z18 if 404) ──► Concurrent Fetch
         │
         ▼
[Canvas Stitcher] ──► Maps image buffers to relative pixel grids in-memory
         │
         ▼
[Output Renderer] ──► Compresses and writes final target JPEG to disk

---

## Component Specifications

### 1. Geometry Module (`geo`)
Responsible for all geospatial coordinate transformations. Translates standard EPSG:4326 (WGS84 decimal degrees) into EPSG:3857 (Web Mercator tile indexes). It computes the bounding boundaries of your targets and outputs exact tile dimensions back to the application pipeline.

### 2. Network Module (`downloader`)
Manages upstream requests to Esri's REST tile server endpoints. Implements a thread-safe worker pool channel system to optimize your internet bandwidth safely, attaching realistic browser user-agents to circumvent basic automated traffic hurdles.

### 3. Processing Module (`stitcher`)
Utilizes Go’s native image drawing library to establish an empty system canvas calculated precisely against the tile matrix dimension grid (Tiles × 256 pixels). Slices downloaded raster buffers into exact spatial pixel offsets.



---

## Interface Execution Guide

### Mode A: Center Point with Padding Boundary
Provide a central coordinate point and specify a decimal degree padding offset radius to harvest the surrounding grid structure.

```geoimg -lat 38.8977 -lon -77.0365 -pad 0.001 -out target.jpg```

### Mode B: Direct Bounding Box Constraints
Input strict geographical perimeters (Minimum/Maximum Latitudes and Longitudes) to cover a broader area.

```geoimg -minlat 40.748 -maxlat 40.750 -minlon -74.00 -maxlon -73.98 -out urban_grid.jpg```

