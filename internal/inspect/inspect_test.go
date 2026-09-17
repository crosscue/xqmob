package inspect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/crosscue/xqmob/internal/output"
	"github.com/crosscue/xqmob/internal/pipeline"
	parquet "github.com/parquet-go/parquet-go"
)

func TestLoadManifestAndEntity(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "analysis"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := pipeline.Manifest{Tool: "xqmob", ToolVersion: "test", AnalysisContract: "xqmob-analysis-v0.2", Stats: pipeline.Stats{EntityCount: 1}}
	mf, err := os.Create(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(mf).Encode(m); err != nil {
		t.Fatal(err)
	}
	if err := mf.Close(); err != nil {
		t.Fatal(err)
	}

	ef, err := os.Create(filepath.Join(root, "analysis", "entities.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	w := parquet.NewGenericWriter[output.EntityRow](ef)
	rows := []output.EntityRow{{EntityID: "abc", Subject: "device:abc", ObservationCount: 7, HAClass: "mixed", HACoveragePct: 80}}
	if _, err := w.Write(rows); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ef.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := Load(root, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if report.Manifest.AnalysisContract != "xqmob-analysis-v0.2" || report.Entity == nil || report.Entity.EntityID != "abc" {
		t.Fatalf("unexpected report: %+v", report)
	}
	b, err := JSON(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Entity map[string]json.RawMessage `json:"entity"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"entity_id":         `"abc"`,
		"subject":           `"device:abc"`,
		"observation_count": "7",
		"ha_class":          `"mixed"`,
		"ha_coverage_pct":   "80",
		"median_ha_m":       "null",
	} {
		if got := string(decoded.Entity[key]); got != want {
			t.Errorf("entity.%s = %s, want %s", key, got, want)
		}
	}
	for _, key := range []string{"EntityID", "ObservationCount", "HAClass", "MedianHAM"} {
		if _, ok := decoded.Entity[key]; ok {
			t.Errorf("entity JSON exposes Go field name %q", key)
		}
	}
}
