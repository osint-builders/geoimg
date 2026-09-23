package render

import (
	"fmt"
	"os"
	"strings"

	"github.com/osint-builders/geoimg/internal/geo"
)

// wkt3857 is the ESRI-flavoured WKT for EPSG:3857 understood by GDAL, QGIS and ArcGIS.
const wkt3857 = `PROJCS["WGS_1984_Web_Mercator_Auxiliary_Sphere",GEOGCS["GCS_WGS_1984",DATUM["D_WGS_1984",SPHEROID["WGS_1984",6378137.0,298.257223563]],PRIMEM["Greenwich",0.0],UNIT["Degree",0.0174532925199433]],PROJECTION["Mercator_Auxiliary_Sphere"],PARAMETER["False_Easting",0.0],PARAMETER["False_Northing",0.0],PARAMETER["Central_Meridian",0.0],PARAMETER["Standard_Parallel_1",0.0],PARAMETER["Auxiliary_Sphere_Type",0.0],UNIT["Meter",1.0]]`

// WriteWorldFile writes an ESRI world file and .prj (EPSG:3857) next to
// imagePath so GIS tools place the image correctly. Returns the world file path.
func WriteWorldFile(imagePath string, f Format, w geo.Window) (string, error) {
	base := strings.TrimSuffix(imagePath, extOf(imagePath))
	res := geo.MetersPerPixel(w.Z)
	// Coordinates of the center of the top-left pixel.
	cx, cy := geo.PixelToMercator(float64(w.X0)+0.5, float64(w.Y0)+0.5, w.Z)
	body := fmt.Sprintf("%.10f\n0.0\n0.0\n%.10f\n%.6f\n%.6f\n", res, -res, cx, cy)
	wf := base + f.WorldFileExt()
	if err := os.WriteFile(wf, []byte(body), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(base+".prj", []byte(wkt3857), 0o644); err != nil {
		return "", err
	}
	return wf, nil
}

func extOf(p string) string {
	i := strings.LastIndexByte(p, '.')
	if i < 0 || strings.ContainsAny(p[i:], `/\`) {
		return ""
	}
	return p[i:]
}
