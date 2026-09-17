//go:build h3projectionintegration

package pipeline

import (
	"path/filepath"
	"testing"

	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/output"
	parquet "github.com/parquet-go/parquet-go"
)

func TestH3ProjectionMaterialization(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "h3-projection-integration"
	cfg.Partitions = 2
	out := filepath.Join(t.TempDir(), "out")
	if _, err := Run(filepath.Join("..", "..", "testdata", "sample.csv"), out, cfg); err != nil {
		t.Fatal(err)
	}

	obs, err := parquet.ReadFile[output.ObservationRow](filepath.Join(out, "analysis", "observations.parquet"))
	if err != nil || len(obs) == 0 {
		t.Fatalf("observations: rows=%d err=%v", len(obs), err)
	}
	for _, r := range obs {
		if r.H3Cell == "" {
			t.Fatalf("observation missing H3 cell: %+v", r)
		}
	}

	events, err := parquet.ReadFile[output.EventRow](filepath.Join(out, "analysis", "events.parquet"))
	if err != nil || len(events) == 0 {
		t.Fatalf("events: rows=%d err=%v", len(events), err)
	}
	for _, r := range events {
		if len(r.Geometry) > 0 && r.H3Cell == "" {
			t.Fatalf("located event missing H3 cell: %+v", r)
		}
	}

	segments, err := parquet.ReadFile[output.SegmentRow](filepath.Join(out, "analysis", "segments.parquet"))
	if err != nil || len(segments) == 0 {
		t.Fatalf("segments: rows=%d err=%v", len(segments), err)
	}
	for _, r := range segments {
		if r.StartH3Cell == "" || r.EndH3Cell == "" {
			t.Fatalf("segment missing endpoint H3 cell: %+v", r)
		}
	}

	presence, err := parquet.ReadFile[output.PresenceRow](filepath.Join(out, "analysis", "presence_intervals.parquet"))
	if err != nil || len(presence) == 0 {
		t.Fatalf("presence: rows=%d err=%v", len(presence), err)
	}
	for _, r := range presence {
		if r.H3Cell == "" {
			t.Fatalf("presence missing H3 cell: %+v", r)
		}
	}

	transitions, err := parquet.ReadFile[output.TransitionRow](filepath.Join(out, "analysis", "transitions.parquet"))
	if err != nil || len(transitions) == 0 {
		t.Fatalf("transitions: rows=%d err=%v", len(transitions), err)
	}
	for _, r := range transitions {
		if r.OriginH3Cell == "" || r.DestinationH3Cell == "" {
			t.Fatalf("transition missing H3 cells: %+v", r)
		}
	}

	entities, err := parquet.ReadFile[output.EntityRow](filepath.Join(out, "analysis", "entities.parquet"))
	if err != nil || len(entities) == 0 {
		t.Fatalf("entities: rows=%d err=%v", len(entities), err)
	}
	for _, r := range entities {
		if r.StartH3Cell == "" || r.EndH3Cell == "" || r.UniqueH3Cells <= 0 {
			t.Fatalf("entity missing H3 summary: %+v", r)
		}
	}
}
