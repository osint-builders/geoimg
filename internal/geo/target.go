package geo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Target kinds.
const (
	KindPoint   = "point"
	KindBBox    = "bbox"
	KindGeoJSON = "geojson"
)

// LonLat is a GeoJSON position (longitude first, per RFC 7946).
type LonLat [2]float64

// Polygon is a list of linear rings: the first is the outer boundary, the rest are holes.
type Polygon [][]LonLat

// Target is the resolved area of interest.
type Target struct {
	Kind     string    `json:"kind"`
	Input    string    `json:"input"`
	BBox     BBox      `json:"bbox"`
	Polygons []Polygon `json:"-"`
}

// ParseTarget interprets the single positional argument of the CLI:
//
//	"lat,lon"                  a point, expanded by radius meters
//	"lat1,lon1,lat2,lon2"      a bounding box given by two opposite corners
//	"area.geojson" or "-"      a GeoJSON file (or stdin)
func ParseTarget(arg string, radius float64, stdin io.Reader) (Target, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return Target{}, errors.New("no target given")
	}
	if arg == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return Target{}, fmt.Errorf("reading stdin: %w", err)
		}
		return parseGeoJSON(data, "stdin", radius)
	}
	if nums, ok := parseNumbers(arg); ok {
		return coordTarget(arg, nums, radius)
	}
	data, err := os.ReadFile(arg)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Target{}, fmt.Errorf("%q is neither coordinates (lat,lon or lat1,lon1,lat2,lon2) nor an existing GeoJSON file", arg)
		}
		return Target{}, err
	}
	return parseGeoJSON(data, arg, radius)
}

func parseNumbers(s string) ([]float64, bool) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	if len(fields) == 0 {
		return nil, false
	}
	out := make([]float64, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil, false
		}
		out = append(out, v)
	}
	return out, true
}

func coordTarget(arg string, n []float64, radius float64) (Target, error) {
	switch len(n) {
	case 2:
		if err := checkLatLon(n[0], n[1]); err != nil {
			return Target{}, err
		}
		if radius <= 0 {
			return Target{}, errors.New("radius must be > 0 for a point target")
		}
		b := PointBox(n[0], n[1], radius)
		return Target{Kind: KindPoint, Input: arg, BBox: b}, b.Validate()
	case 4:
		if err := checkLatLon(n[0], n[1]); err != nil {
			return Target{}, err
		}
		if err := checkLatLon(n[2], n[3]); err != nil {
			return Target{}, err
		}
		b := EmptyBBox()
		b.Extend(n[0], n[1])
		b.Extend(n[2], n[3])
		return Target{Kind: KindBBox, Input: arg, BBox: b}, b.Validate()
	default:
		return Target{}, fmt.Errorf("expected 2 numbers (lat,lon) or 4 numbers (lat1,lon1,lat2,lon2), got %d", len(n))
	}
}

func checkLatLon(lat, lon float64) error {
	if lat < -90 || lat > 90 {
		return fmt.Errorf("latitude %g out of range; coordinates are lat,lon (latitude first)", lat)
	}
	if lon < -180 || lon > 180 {
		return fmt.Errorf("longitude %g out of range [-180, 180]", lon)
	}
	return nil
}

type gjObject struct {
	Type        string          `json:"type"`
	Features    []gjObject      `json:"features"`
	Geometry    *gjObject       `json:"geometry"`
	Geometries  []gjObject      `json:"geometries"`
	Coordinates json.RawMessage `json:"coordinates"`
}

type gjCollector struct {
	box        BBox
	polys      []Polygon
	count      int
	onlyPoints bool
}

func parseGeoJSON(data []byte, name string, radius float64) (Target, error) {
	var root gjObject
	if err := json.Unmarshal(data, &root); err != nil {
		return Target{}, fmt.Errorf("%s: invalid GeoJSON: %w", name, err)
	}
	c := &gjCollector{box: EmptyBBox(), onlyPoints: true}
	if err := c.walk(root); err != nil {
		return Target{}, fmt.Errorf("%s: %w", name, err)
	}
	if c.count == 0 {
		return Target{}, fmt.Errorf("%s: GeoJSON contains no coordinates", name)
	}
	b := c.box
	if c.onlyPoints || b.MinLat == b.MaxLat || b.MinLon == b.MaxLon {
		if radius <= 0 {
			return Target{}, errors.New("radius must be > 0 for point geometries")
		}
		b = b.Pad(radius)
	}
	return Target{Kind: KindGeoJSON, Input: name, BBox: b, Polygons: c.polys}, b.Validate()
}

func (c *gjCollector) walk(o gjObject) error {
	switch o.Type {
	case "FeatureCollection":
		for _, f := range o.Features {
			if err := c.walk(f); err != nil {
				return err
			}
		}
	case "Feature":
		if o.Geometry != nil {
			return c.walk(*o.Geometry)
		}
	case "GeometryCollection":
		for _, g := range o.Geometries {
			if err := c.walk(g); err != nil {
				return err
			}
		}
	case "Point":
		var p []float64
		return c.decode(o, &p, func() error { return c.add(p) })
	case "MultiPoint":
		var ps [][]float64
		return c.decode(o, &ps, func() error { return c.addAll(ps) })
	case "LineString":
		c.onlyPoints = false
		var ps [][]float64
		return c.decode(o, &ps, func() error { return c.addAll(ps) })
	case "MultiLineString":
		c.onlyPoints = false
		var ls [][][]float64
		return c.decode(o, &ls, func() error {
			for _, l := range ls {
				if err := c.addAll(l); err != nil {
					return err
				}
			}
			return nil
		})
	case "Polygon":
		c.onlyPoints = false
		var rings [][][]float64
		return c.decode(o, &rings, func() error { return c.addPolygon(rings) })
	case "MultiPolygon":
		c.onlyPoints = false
		var polys [][][][]float64
		return c.decode(o, &polys, func() error {
			for _, p := range polys {
				if err := c.addPolygon(p); err != nil {
					return err
				}
			}
			return nil
		})
	case "":
		return errors.New("GeoJSON object without a \"type\"")
	default:
		return fmt.Errorf("unsupported GeoJSON type %q", o.Type)
	}
	return nil
}

func (c *gjCollector) decode(o gjObject, dst any, then func() error) error {
	if err := json.Unmarshal(o.Coordinates, dst); err != nil {
		return fmt.Errorf("%s: bad coordinates: %w", o.Type, err)
	}
	return then()
}

func (c *gjCollector) add(p []float64) error {
	if len(p) < 2 {
		return errors.New("position needs at least [lon, lat]")
	}
	lon, lat := p[0], p[1]
	if err := checkLatLon(lat, lon); err != nil {
		return fmt.Errorf("GeoJSON position [%g, %g] (GeoJSON order is [lon, lat]): %w", lon, lat, err)
	}
	c.box.Extend(lat, lon)
	c.count++
	return nil
}

func (c *gjCollector) addAll(ps [][]float64) error {
	for _, p := range ps {
		if err := c.add(p); err != nil {
			return err
		}
	}
	return nil
}

func (c *gjCollector) addPolygon(rings [][][]float64) error {
	poly := make(Polygon, 0, len(rings))
	for _, r := range rings {
		if err := c.addAll(r); err != nil {
			return err
		}
		ring := make([]LonLat, len(r))
		for i, p := range r {
			ring[i] = LonLat{p[0], p[1]}
		}
		poly = append(poly, ring)
	}
	if len(poly) > 0 {
		c.polys = append(c.polys, poly)
	}
	return nil
}
