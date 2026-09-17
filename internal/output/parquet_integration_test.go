//go:build parquetintegration

package output

import (
	"path/filepath"
	"testing"

	"github.com/crosscue/xqmob/internal/config"
	parquet "github.com/parquet-go/parquet-go"
)

// This test is intentionally build-tagged so CI can exercise nullable Parquet
// behavior against the real pinned parquet-go dependency. The local development
// harness may use an API-compatible writer shim when module downloads are unavailable.
func TestOptionalHorizontalAccuracyRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observations.parquet")
	cfg := config.Default()
	cfg.ParquetRowGroupRows = 1
	cfg.ParquetDictionaryMaxBytes = 1 << 20
	sink, err := newParquetSink[ObservationRow](path, nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	five := 5.0
	if err := sink.Write(ObservationRow{ObservationID: "known", HA: &five, HAState: "known", StepAccuracyState: "initial", H3Cell: "8b283082800bfff"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Write(ObservationRow{ObservationID: "missing", HA: nil, HAState: "missing", StepAccuracyState: "current_missing", H3Cell: "8b2830828017fff"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	rows, err := parquet.ReadFile[ObservationRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2", len(rows))
	}
	if rows[0].HA == nil || *rows[0].HA != 5 {
		t.Fatalf("known HA did not round trip: %+v", rows[0])
	}
	if rows[1].HA != nil {
		t.Fatalf("missing HA was not preserved as NULL: %+v", rows[1])
	}
	if rows[0].H3Cell != "8b283082800bfff" || rows[1].H3Cell != "8b2830828017fff" {
		t.Fatalf("H3 cell strings did not round trip: %+v", rows)
	}
	if sink.rowsWritten != 2 || sink.rowGroupsWritten != 2 {
		t.Fatalf("bounded row groups not applied: rows=%d row_groups=%d", sink.rowsWritten, sink.rowGroupsWritten)
	}
}
