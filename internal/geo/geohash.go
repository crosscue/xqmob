package geo

const geohashAlphabet = "0123456789bcdefghjkmnpqrstuvwxyz"

func Geohash(lat, lon float64, precision int) string {
	if precision <= 0 {
		return ""
	}
	latMin, latMax := -90.0, 90.0
	lonMin, lonMax := -180.0, 180.0
	out := make([]byte, 0, precision)
	bit, ch := 0, 0
	even := true
	for len(out) < precision {
		if even {
			mid := (lonMin + lonMax) / 2
			if lon >= mid {
				ch |= 1 << (4 - bit)
				lonMin = mid
			} else {
				lonMax = mid
			}
		} else {
			mid := (latMin + latMax) / 2
			if lat >= mid {
				ch |= 1 << (4 - bit)
				latMin = mid
			} else {
				latMax = mid
			}
		}
		even = !even
		if bit < 4 {
			bit++
		} else {
			out = append(out, geohashAlphabet[ch])
			bit, ch = 0, 0
		}
	}
	return string(out)
}
