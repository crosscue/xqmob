//go:build h3integration

package geo

import "testing"

func TestH3CellMatchesReferenceExample(t *testing.T) {
	got, err := H3Cell(37.775938728915946, -122.41795063018799, 9)
	if err != nil {
		t.Fatal(err)
	}
	if got != "8928308280fffff" {
		t.Fatalf("H3 cell = %q, want %q", got, "8928308280fffff")
	}
}
