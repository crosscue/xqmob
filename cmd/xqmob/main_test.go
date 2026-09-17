package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEventizeRequiresReplaceFlag(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	input := filepath.Join("..", "..", "testdata", "sample.csv")
	args := []string{input, "--source", "cli-test", "--out", out, "--partitions", "2", "--full", "--trace-id", "a"}
	if err := runEventize(args); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(out, "manifest.json")
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	args = []string{"--source", "cli-test", "--out", out, "--partitions", "2", input}
	if err := runEventize(args); err == nil || !strings.Contains(err.Error(), "--replace") {
		t.Fatalf("expected replacement error: %v", err)
	}
	after, err := os.ReadFile(manifest)
	if err != nil || string(after) != string(before) {
		t.Fatal("refused rerun modified the dataset")
	}
	if err := os.WriteFile(filepath.Join(out, "notes.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runEventize(append([]string{"--replace"}, args...)); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"analysis/tracks.parquet", "diagnostics"} {
		if _, err := os.Stat(filepath.Join(out, p)); !os.IsNotExist(err) {
			t.Fatalf("stale artifact %s survived replacement: %v", p, err)
		}
	}
	if b, err := os.ReadFile(filepath.Join(out, "notes.txt")); err != nil || string(b) != "keep" {
		t.Fatal("replacement removed an unrelated file")
	}
}
