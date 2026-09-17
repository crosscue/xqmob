package geo

import (
	"fmt"

	h3 "github.com/dimchansky/h3-go"
)

// H3Cell returns the standard hexadecimal H3 cell index for a WGS84
// latitude/longitude at the requested resolution. H3 is an analytical
// spatial index only; eventization remains metric/geodesic.
func H3Cell(lat, lon float64, resolution int) (string, error) {
	if resolution < 0 || resolution > 15 {
		return "", fmt.Errorf("H3 resolution must be between 0 and 15, got %d", resolution)
	}
	cell, err := h3.LatLngToCell(h3.LatLngDegs(lat, lon), resolution)
	if err != nil {
		return "", err
	}
	return cell.String(), nil
}
