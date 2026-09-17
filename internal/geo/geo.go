package geo

import "math"

const earthRadiusM = 6371008.8

func DistanceM(lat1, lon1, lat2, lon2 float64) float64 {
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dphi := (lat2 - lat1) * math.Pi / 180
	dlambda := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dphi/2)*math.Sin(dphi/2) + math.Cos(phi1)*math.Cos(phi2)*math.Sin(dlambda/2)*math.Sin(dlambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}

func BearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dlambda := (lon2 - lon1) * math.Pi / 180
	y := math.Sin(dlambda) * math.Cos(phi2)
	x := math.Cos(phi1)*math.Sin(phi2) - math.Sin(phi1)*math.Cos(phi2)*math.Cos(dlambda)
	b := math.Atan2(y, x) * 180 / math.Pi
	if b < 0 {
		b += 360
	}
	return b
}

func Bearing8(b float64) string {
	dirs := [...]string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	return dirs[int(math.Floor((b+22.5)/45.0))%8]
}

func EffectiveDistanceM(raw, ha1, ha2 float64) float64 {
	v := raw - math.Max(0, ha1) - math.Max(0, ha2)
	if v < 0 {
		return 0
	}
	return v
}

func EffectiveDistanceKnownM(raw, ha1 float64, hasHA1 bool, ha2 float64, hasHA2 bool) float64 {
	v := raw
	if hasHA1 {
		v -= math.Max(0, ha1)
	}
	if hasHA2 {
		v -= math.Max(0, ha2)
	}
	if v < 0 {
		return 0
	}
	return v
}

type Centroid struct {
	Lat float64
	Lon float64
}

func MeanCentroid(points [][2]float64) Centroid {
	if len(points) == 0 {
		return Centroid{}
	}
	// ECEF mean avoids the most obvious longitude wrap problem at the antimeridian.
	var x, y, z float64
	for _, p := range points {
		lat := p[0] * math.Pi / 180
		lon := p[1] * math.Pi / 180
		x += math.Cos(lat) * math.Cos(lon)
		y += math.Cos(lat) * math.Sin(lon)
		z += math.Sin(lat)
	}
	x /= float64(len(points))
	y /= float64(len(points))
	z /= float64(len(points))
	lon := math.Atan2(y, x)
	hyp := math.Sqrt(x*x + y*y)
	lat := math.Atan2(z, hyp)
	return Centroid{Lat: lat * 180 / math.Pi, Lon: lon * 180 / math.Pi}
}

func MaxScatterM(points [][2]float64, c Centroid) float64 {
	var m float64
	for _, p := range points {
		d := DistanceM(c.Lat, c.Lon, p[0], p[1])
		if d > m {
			m = d
		}
	}
	return m
}

func Median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	v := append([]float64(nil), values...)
	for i := 1; i < len(v); i++ {
		x := v[i]
		j := i - 1
		for j >= 0 && v[j] > x {
			v[j+1] = v[j]
			j--
		}
		v[j+1] = x
	}
	mid := len(v) / 2
	if len(v)%2 == 1 {
		return v[mid]
	}
	return (v[mid-1] + v[mid]) / 2
}
