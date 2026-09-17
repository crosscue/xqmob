package geo

import (
	"bytes"
	"encoding/binary"
)

func WKBPoint(lon, lat float64) []byte {
	var b bytes.Buffer
	b.WriteByte(1) // little endian
	_ = binary.Write(&b, binary.LittleEndian, uint32(1))
	_ = binary.Write(&b, binary.LittleEndian, lon)
	_ = binary.Write(&b, binary.LittleEndian, lat)
	return b.Bytes()
}

func WKBLineString(coords [][2]float64) []byte {
	if len(coords) == 1 {
		return WKBPoint(coords[0][0], coords[0][1])
	}
	var b bytes.Buffer
	b.WriteByte(1)
	_ = binary.Write(&b, binary.LittleEndian, uint32(2))
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(coords)))
	for _, p := range coords {
		_ = binary.Write(&b, binary.LittleEndian, p[0])
		_ = binary.Write(&b, binary.LittleEndian, p[1])
	}
	return b.Bytes()
}

// WKBPath returns Point/LineString/MultiLineString WKB for visual path runs.
// A one-point run inside a MultiLineString is duplicated to form a zero-length
// two-point LineString, preserving the isolated observed location without
// drawing an unobserved connection to another run.
func WKBPath(runs [][][2]float64) []byte {
	filtered := make([][][2]float64, 0, len(runs))
	for _, run := range runs {
		if len(run) > 0 {
			filtered = append(filtered, run)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return WKBLineString(filtered[0])
	}
	var b bytes.Buffer
	b.WriteByte(1)
	_ = binary.Write(&b, binary.LittleEndian, uint32(5)) // MultiLineString
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(filtered)))
	for _, run := range filtered {
		if len(run) == 1 {
			run = [][2]float64{run[0], run[0]}
		}
		b.Write(WKBLineString(run))
	}
	return b.Bytes()
}
