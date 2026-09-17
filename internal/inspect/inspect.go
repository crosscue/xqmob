package inspect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/crosscue/xqmob/internal/output"
	"github.com/crosscue/xqmob/internal/pipeline"
	parquet "github.com/parquet-go/parquet-go"
)

// Report is the compact inspection surface for an xqmob output directory.
// Entity is populated only when the caller requests an entity id.
type Report struct {
	Root     string            `json:"root"`
	Manifest pipeline.Manifest `json:"manifest"`
	Entity   *output.EntityRow `json:"entity,omitempty"`
}

// Load reads manifest.json and, when entityID is non-empty, resolves that entity
// from the analysis/entities.parquet analytical index.
func Load(path, entityID string) (Report, error) {
	root, manifestPath, err := resolveRoot(path)
	if err != nil {
		return Report{}, err
	}
	f, err := os.Open(manifestPath)
	if err != nil {
		return Report{}, fmt.Errorf("open manifest: %w", err)
	}
	defer f.Close()
	var m pipeline.Manifest
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return Report{}, fmt.Errorf("decode manifest: %w", err)
	}
	report := Report{Root: root, Manifest: m}
	if entityID == "" {
		return report, nil
	}
	entityPath := filepath.Join(root, "analysis", "entities.parquet")
	ef, err := os.Open(entityPath)
	if err != nil {
		return Report{}, fmt.Errorf("open entities.parquet: %w", err)
	}
	defer ef.Close()
	reader := parquet.NewGenericReader[output.EntityRow](ef)
	defer reader.Close()
	buf := make([]output.EntityRow, 1024)
	for {
		n, readErr := reader.Read(buf)
		for i := 0; i < n; i++ {
			if buf[i].EntityID == entityID {
				row := buf[i]
				report.Entity = &row
				return report, nil
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return Report{}, fmt.Errorf("read entities.parquet: %w", readErr)
		}
	}
	return Report{}, fmt.Errorf("entity %q not found", entityID)
}

func resolveRoot(path string) (root, manifest string, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", err
	}
	if info.IsDir() {
		return abs, filepath.Join(abs, "manifest.json"), nil
	}
	if filepath.Base(abs) != "manifest.json" {
		return "", "", fmt.Errorf("inspect expects an xqmob output directory or manifest.json")
	}
	return filepath.Dir(abs), abs, nil
}

// JSON returns a stable indented JSON representation suitable for scripting.
func JSON(report Report) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}
