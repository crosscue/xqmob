//go:build mcpintegration

package analyst

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestAnalysisV01Compatibility(t *testing.T) {
	src := os.Getenv("XQMOB_TEST_DATASET")
	if src == "" {
		t.Skip("XQMOB_TEST_DATASET not set")
	}
	dst := t.TempDir()
	if err := makeV01Fixture(src, dst); err != nil {
		t.Fatal(err)
	}

	ds, err := Open(dst, Options{Threads: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer ds.Close()
	if ds.H3Available() {
		t.Fatal("v0.1 compatibility dataset unexpectedly reports H3")
	}
	if got := ds.Summary(); got.AnalysisContract != "xqmob-analysis-v0.1" || got.H3Available || got.H3Resolution != -1 {
		t.Fatalf("unexpected summary: %+v", got)
	}

	page, err := ds.SearchEntities(context.Background(), SearchEntitiesInput{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 1 {
		t.Fatalf("expected one entity row, got %d", len(page.Rows))
	}
	entityID := fmt.Sprint(page.Rows[0]["entity_id"])
	if entityID == "" || entityID == "<nil>" {
		t.Fatalf("missing entity_id in %+v", page.Rows[0])
	}

	obs, err := ds.GetObservations(context.Background(), ObservationInput{EntityID: entityID, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Rows) == 0 {
		t.Fatal("expected observations for compatibility dataset")
	}
	if _, ok := obs.Rows[0]["h3_cell"]; !ok {
		t.Fatal("compatibility view must expose nullable h3_cell")
	}
	if obs.Rows[0]["h3_cell"] != nil {
		t.Fatalf("v0.1 synthesized h3_cell should be null, got %#v", obs.Rows[0]["h3_cell"])
	}

	if _, err := ds.GetObservations(context.Background(), ObservationInput{H3Cell: "8928308280fffff"}); err == nil || !strings.Contains(err.Error(), "H3 is not materialized") {
		t.Fatalf("expected explicit H3 capability error, got %v", err)
	}
	if _, err := ds.DescribeH3Cell(context.Background(), H3CellInput{H3Cell: "8928308280fffff"}); err == nil || !strings.Contains(err.Error(), "H3 is not materialized") {
		t.Fatalf("expected explicit describe_h3_cell capability error, got %v", err)
	}
}

func makeV01Fixture(src, dst string) error {
	if err := os.MkdirAll(filepath.Join(dst, "analysis"), 0o755); err != nil {
		return err
	}
	manifestBytes, err := os.ReadFile(filepath.Join(src, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return err
	}
	manifest["analysis_contract"] = "xqmob-analysis-v0.1"
	if cfg, ok := manifest["config"].(map[string]any); ok {
		delete(cfg, "h3_resolution")
	}
	outManifest, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dst, "manifest.json"), outManifest, 0o644); err != nil {
		return err
	}

	db, err := sql.Open("duckdb", "")
	if err != nil {
		return err
	}
	defer db.Close()
	files := map[string]string{
		"entities.parquet":           "unique_h3_cells, start_h3_cell, end_h3_cell",
		"observations.parquet":       "h3_cell",
		"events.parquet":             "h3_cell",
		"segments.parquet":           "start_h3_cell, end_h3_cell",
		"presence_intervals.parquet": "h3_cell",
		"transitions.parquet":        "origin_h3_cell, destination_h3_cell, same_h3_cell",
		"tracks.parquet":             "unique_h3_cells, start_h3_cell, end_h3_cell",
	}
	for name, exclude := range files {
		in := filepath.Join(src, "analysis", name)
		if _, err := os.Stat(in); err != nil {
			if os.IsNotExist(err) && name == "tracks.parquet" {
				continue
			}
			return err
		}
		out := filepath.Join(dst, "analysis", name)
		q := fmt.Sprintf("COPY (SELECT * EXCLUDE (%s) FROM read_parquet('%s')) TO '%s' (FORMAT PARQUET, COMPRESSION ZSTD)", exclude, sqlPath(in), sqlPath(out))
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("rewrite %s: %w", name, err)
		}
	}
	return nil
}

func sqlPath(p string) string { return strings.ReplaceAll(filepath.ToSlash(p), "'", "''") }
