package analyst

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/pipeline"
)

func TestCursorRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 100, 999999} {
		c := encodeCursor(n)
		got, err := decodeCursor(c)
		if err != nil || got != n {
			t.Fatalf("cursor %d -> %q -> %d, %v", n, c, got, err)
		}
	}
}

func TestCursorRejectsInvalid(t *testing.T) {
	if _, err := decodeCursor("not-base64!"); err == nil {
		t.Fatal("expected invalid cursor error")
	}
}

func TestLimitBounds(t *testing.T) {
	if got := normalizeLimit(0); got != DefaultLimit {
		t.Fatalf("default limit=%d", got)
	}
	if got := normalizeLimit(MaxLimit + 1); got != MaxLimit {
		t.Fatalf("max limit=%d", got)
	}
	if got := normalizeLimit(7); got != 7 {
		t.Fatalf("explicit limit=%d", got)
	}
}

func TestParseBounds(t *testing.T) {
	if _, _, err := parseBounds("2026-01-02T03:04:05Z", "2026-01-02T04:04:05Z"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseBounds("2026-01-02T05:04:05Z", "2026-01-02T04:04:05Z"); err == nil {
		t.Fatal("expected reversed bounds error")
	}
	if _, _, err := parseBounds("not-a-time", ""); err == nil {
		t.Fatal("expected timestamp error")
	}
}

func TestCompatibilitySelectAddsOnlyH3Columns(t *testing.T) {
	cases := map[string][]string{
		"entities":           {"unique_h3_cells", "start_h3_cell", "end_h3_cell"},
		"observations":       {"h3_cell"},
		"events":             {"h3_cell"},
		"segments":           {"start_h3_cell", "end_h3_cell"},
		"presence_intervals": {"h3_cell"},
		"transitions":        {"origin_h3_cell", "destination_h3_cell", "same_h3_cell"},
	}
	for view, want := range cases {
		q := compatibilitySelect(view)
		for _, col := range want {
			if !strings.Contains(q, col) {
				t.Fatalf("%s missing %s: %s", view, col, q)
			}
		}
	}
}

func TestDuckDBLockdown(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "lockdown-test"
	cfg.Partitions = 2
	out := filepath.Join(t.TempDir(), "out")
	if _, err := pipeline.Run(filepath.Join("..", "..", "testdata", "sample.csv"), out, cfg); err != nil {
		t.Fatal(err)
	}
	ds, err := Open(out, Options{Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer ds.Close()

	page, err := ds.SearchEntities(context.Background(), SearchEntitiesInput{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Returned == 0 {
		t.Fatal("expected analyst query over dataset parquet to succeed after lockdown")
	}

	stolen := filepath.Join(t.TempDir(), "stolen.parquet")
	copySQL := fmt.Sprintf("COPY (SELECT * FROM entities) TO '%s' (FORMAT PARQUET)", sqlQuote(filepath.ToSlash(stolen)))
	if _, err := ds.db.ExecContext(context.Background(), copySQL); err == nil {
		t.Fatal("expected COPY TO to fail after DuckDB lockdown")
	}
	if _, err := os.Stat(stolen); err == nil {
		t.Fatal("COPY TO wrote a file despite lockdown")
	}

	outside := filepath.Join(t.TempDir(), "outside.csv")
	if err := os.WriteFile(outside, []byte("a,b\n1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readSQL := fmt.Sprintf("SELECT * FROM read_csv_auto('%s')", sqlQuote(filepath.ToSlash(outside)))
	if _, err := ds.db.ExecContext(context.Background(), readSQL); err == nil {
		t.Fatal("expected read of a path outside the dataset to fail")
	}

	if _, err := ds.db.ExecContext(context.Background(), "SET enable_external_access=true"); err == nil {
		t.Fatal("expected locked DuckDB configuration to reject SET")
	}
}
