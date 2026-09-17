package geo

import "testing"

func TestDistanceAndEffectiveDistance(t *testing.T) {
	d := DistanceM(51.5, -0.1, 51.5, -0.099)
	if d < 60 || d > 80 {
		t.Fatalf("unexpected distance %.2f", d)
	}
	e := EffectiveDistanceM(100, 30, 20)
	if e != 50 {
		t.Fatalf("expected 50, got %.2f", e)
	}
}

func TestGeohash(t *testing.T) {
	g := Geohash(51.5074, -0.1278, 8)
	if len(g) != 8 {
		t.Fatalf("expected precision 8, got %q", g)
	}
}

func TestWKBGeometryHeaders(t *testing.T) {
	point := WKBPoint(-0.1, 51.5)
	if len(point) != 21 || point[0] != 1 || point[1] != 1 || point[2] != 0 || point[3] != 0 || point[4] != 0 {
		t.Fatalf("unexpected point WKB header/size: %v", point)
	}
	line := WKBLineString([][2]float64{{-0.1, 51.5}, {-0.11, 51.51}})
	if len(line) != 41 || line[0] != 1 || line[1] != 2 || line[2] != 0 || line[3] != 0 || line[4] != 0 {
		t.Fatalf("unexpected line WKB header/size: %v", line[:5])
	}
	multi := WKBPath([][][2]float64{
		{{-0.1, 51.5}, {-0.11, 51.51}},
		{{-0.2, 51.6}, {-0.21, 51.61}},
	})
	if len(multi) < 9 || multi[0] != 1 || multi[1] != 5 || multi[2] != 0 || multi[3] != 0 || multi[4] != 0 {
		t.Fatalf("unexpected multilinestring WKB header: %v", multi[:5])
	}
}
