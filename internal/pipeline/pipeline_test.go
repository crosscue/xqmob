package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/model"
)

func TestExistingOutputRequiresReplacement(t *testing.T) {
	for _, name := range []string{"manifest.json", "notes.txt"} {
		t.Run(name, func(t *testing.T) {
			out := t.TempDir()
			path := filepath.Join(out, name)
			if err := os.WriteFile(path, []byte("preserve me"), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg := config.Default()
			cfg.Source = "test"
			_, err := Run(filepath.Join("..", "..", "testdata", "sample.csv"), out, cfg)
			if err == nil || !strings.Contains(err.Error(), "--replace") {
				t.Fatalf("expected explicit replacement error, got %v", err)
			}
			b, err := os.ReadFile(path)
			if err != nil || string(b) != "preserve me" {
				t.Fatalf("existing output changed: %q, %v", b, err)
			}
			entries, err := os.ReadDir(out)
			if err != nil || len(entries) != 1 {
				t.Fatalf("refused run created artifacts: %v, %v", entries, err)
			}
		})
	}
}

func TestReplacementPreservesDatasetOnIngestionFailure(t *testing.T) {
	for _, content := range []string{"missing", "bad,header\n", "id,ts,lat,lon,ha\na,invalid,51,0,5\n"} {
		t.Run(content, func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, "out")
			paths := []string{"manifest.json", "rejects.csv", "canonical/events.jsonl", "analysis/tracks.parquet", "diagnostics/traces/old.csv"}
			for _, p := range paths {
				path := filepath.Join(out, p)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("previous dataset"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			input := filepath.Join(root, "input.csv")
			if content != "missing" {
				if err := os.WriteFile(input, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg := config.Default()
			cfg.Source, cfg.Partitions = "test", 2
			cfg.ReplaceOutput, cfg.Strict = true, true
			if _, err := Run(input, out, cfg); err == nil {
				t.Fatal("expected ingestion failure")
			}
			for _, p := range paths {
				b, err := os.ReadFile(filepath.Join(out, p))
				if err != nil || string(b) != "previous dataset" {
					t.Fatalf("%s changed on failed ingestion: %q, %v", p, b, err)
				}
			}
		})
	}
}

func TestNanosecondEpochsRemainDistinct(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input.csv")
	data := "id,ts,lat,lon,ha\na,1788266096000000001,51,0,5\na,1788266096000000002,51,0,5\na,NaN,51,0,5\n"
	if err := os.WriteFile(input, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Source, cfg.Partitions = "test", 2
	stats, err := Run(input, filepath.Join(root, "out"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if stats.NormalizedRows != 2 || stats.Diagnostics.RejectReasons["invalid_ts"] != 1 {
		t.Fatalf("lost distinct timestamps or accepted NaN: normalized=%d rejects=%v", stats.NormalizedRows, stats.Diagnostics.RejectReasons)
	}
}

func TestReplacementRejectsTemporaryWorkspaceInsideDiagnostics(t *testing.T) {
	out := t.TempDir()
	path := filepath.Join(out, "manifest.json")
	if err := os.WriteFile(path, []byte("previous dataset"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Source, cfg.ReplaceOutput = "test", true
	cfg.TempDir = filepath.Join(out, "diagnostics", "scratch")
	_, err := Run(filepath.Join("..", "..", "testdata", "sample.csv"), out, cfg)
	if err == nil || !strings.Contains(err.Error(), "--temp-dir") {
		t.Fatalf("expected unsafe workspace error, got %v", err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "previous dataset" {
		t.Fatal("unsafe workspace removed existing output")
	}
}

func TestStandardProfileOmitsTracksAndAccountsStorage(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	cfg.Partitions = 2

	out := filepath.Join(t.TempDir(), "out")
	stats, err := Run(filepath.Join("..", "..", "testdata", "sample.csv"), out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "analysis", "tracks.parquet")); !os.IsNotExist(err) {
		t.Fatalf("standard profile should omit tracks.parquet; stat err=%v", err)
	}
	if stats.Storage.InputBytes <= 0 || stats.Storage.CanonicalBytes <= 0 || stats.Storage.AnalysisBytes <= 0 {
		t.Fatalf("expected non-zero storage accounting: %+v", stats.Storage)
	}
	if stats.Storage.AnalysisExpansionRatio <= 0 || stats.Storage.AnalysisBytesPerObservation <= 0 {
		t.Fatalf("expected derived storage ratios: %+v", stats.Storage)
	}

	b, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Config.OutputProfile != "standard" || m.Config.ParquetCompression != "zstd" {
		t.Fatalf("unexpected manifest config: %+v", m.Config)
	}
	if m.Config.ParquetRowGroupRows != 131072 || m.Config.ParquetDictionaryMaxBytes != 8<<20 {
		t.Fatalf("unexpected parquet tuning defaults: %+v", m.Config)
	}
	if m.Stats.Parquet.RowGroupMaxRows != 131072 || m.Stats.Parquet.DictionaryMaxBytes != 8<<20 {
		t.Fatalf("unexpected parquet materialization stats: %+v", m.Stats.Parquet)
	}
	if got := m.Stats.Parquet.Files["analysis/observations.parquet"]; got.Rows == 0 || got.RowGroups == 0 {
		t.Fatalf("missing parquet file row-group stats: %+v", m.Stats.Parquet.Files)
	}
	for _, p := range m.Analysis {
		if p == "analysis/tracks.parquet" {
			t.Fatalf("standard manifest unexpectedly lists tracks.parquet")
		}
	}
}

func TestFullProfileIncludesTracks(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	cfg.Partitions = 2
	cfg.OutputProfile = "full"

	out := filepath.Join(t.TempDir(), "out")
	_, err := Run(filepath.Join("..", "..", "testdata", "sample.csv"), out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(out, "analysis", "tracks.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("tracks.parquet is empty")
	}
}

func TestOutputProfileDoesNotChangeCanonicalEvents(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "sample.csv")

	standardCfg := config.Default()
	standardCfg.Source = "test"
	standardCfg.Partitions = 2
	standardOut := filepath.Join(t.TempDir(), "standard")
	if _, err := Run(input, standardOut, standardCfg); err != nil {
		t.Fatal(err)
	}

	fullCfg := standardCfg
	fullCfg.OutputProfile = "full"
	fullOut := filepath.Join(t.TempDir(), "full")
	if _, err := Run(input, fullOut, fullCfg); err != nil {
		t.Fatal(err)
	}

	a, err := os.ReadFile(filepath.Join(standardOut, "canonical", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(fullOut, "canonical", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("output profile changed canonical event stream")
	}
}

func TestStandardProfileRemovesStaleFullTrackArtifact(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "sample.csv")
	out := filepath.Join(t.TempDir(), "out")

	full := config.Default()
	full.Source = "test"
	full.Partitions = 2
	full.OutputProfile = "full"
	if _, err := Run(input, out, full); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "analysis", "tracks.parquet")); err != nil {
		t.Fatal(err)
	}

	standard := full
	standard.OutputProfile = "standard"
	standard.ReplaceOutput = true
	if _, err := Run(input, out, standard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "analysis", "tracks.parquet")); !os.IsNotExist(err) {
		t.Fatalf("stale tracks.parquet survived standard rerun; stat err=%v", err)
	}
}

func TestManifestIncludesRejectAndEventizerDiagnostics(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	cfg.Partitions = 2
	cfg.HAPolicy = "require"

	input := filepath.Join(t.TempDir(), "input.csv")
	csv := "id,ts,lat,lon,ha\n" +
		"a,2026-09-01T10:00:00Z,51.5,-0.1,5\n" +
		"a,2026-09-01T10:05:00Z,51.5,-0.1,5\n" +
		",2026-09-01T10:10:00Z,51.5,-0.1,5\n" +
		"b,bad-ts,51.5,-0.1,5\n" +
		"c,2026-09-01T10:10:00Z,51.5,-0.1,\n" +
		"d,2026-09-01T10:10:00Z,51.5,-0.1,bad\n" +
		"e,2026-09-01T10:10:00Z,51.5,-0.1,NaN\n" +
		"f,2026-09-01T10:10:00Z,51.5,-0.1,-1\n"
	if err := os.WriteFile(input, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "out")
	stats, err := Run(input, out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	wantRejects := []string{"empty_id", "invalid_ts", "missing_ha", "non_numeric_ha", "non_finite_ha", "negative_ha"}
	for _, code := range wantRejects {
		if stats.Diagnostics.RejectReasons[code] != 1 {
			t.Fatalf("reject %s=%d want 1; all=%+v", code, stats.Diagnostics.RejectReasons[code], stats.Diagnostics.RejectReasons)
		}
	}
	if stats.Diagnostics.Eventizer.PresenceConfirmations == 0 {
		t.Fatalf("expected eventizer diagnostics: %+v", stats.Diagnostics.Eventizer)
	}

	b, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Stats.Diagnostics.RejectReasons["empty_id"] != 1 || m.Stats.Diagnostics.RejectReasons["missing_ha"] != 1 {
		t.Fatalf("manifest missing reject diagnostics: %+v", m.Stats.Diagnostics)
	}
}

func TestTargetedTraceWrittenAndListedInManifest(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	cfg.Partitions = 2
	cfg.TraceIDs = []string{"a"}

	out := filepath.Join(t.TempDir(), "out")
	stats, err := Run(filepath.Join("..", "..", "testdata", "sample.csv"), out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.TraceFiles) != 1 {
		t.Fatalf("trace files=%v want one", stats.TraceFiles)
	}
	tracePath := filepath.Join(out, filepath.FromSlash(stats.TraceFiles[0]))
	b, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 || !containsBytes(b, []byte("decision")) || !containsBytes(b, []byte("departure_confirmed")) {
		t.Fatalf("unexpected trace content: %s", string(b))
	}

	mb, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(mb, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Traces) != 1 || m.Traces[0] != stats.TraceFiles[0] {
		t.Fatalf("manifest traces=%v stats=%v", m.Traces, stats.TraceFiles)
	}
	if m.Storage.DiagnosticBytes <= 0 {
		t.Fatalf("diagnostic storage not accounted: %+v", m.Storage)
	}
}

func containsBytes(haystack, needle []byte) bool {
	return len(needle) == 0 || stringContains(string(haystack), string(needle))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestHAPolicyRequireVsAllowMissing(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.csv")
	csv := "id,ts,lat,lon,ha\n" +
		"a,2026-09-01T10:00:00Z,51.5000,-0.1000,5\n" +
		"a,2026-09-01T10:05:00Z,51.5000,-0.1000,\n" +
		"a,2026-09-01T10:10:00Z,51.5000,-0.1000,5\n"
	if err := os.WriteFile(input, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}

	require := config.Default()
	require.Source = "test"
	require.Partitions = 1
	require.HAPolicy = "require"
	requireOut := filepath.Join(t.TempDir(), "require")
	requireStats, err := Run(input, requireOut, require)
	if err != nil {
		t.Fatal(err)
	}
	if requireStats.ValidInputRows != 2 || requireStats.RejectedInputRows != 1 || requireStats.Diagnostics.RejectReasons["missing_ha"] != 1 {
		t.Fatalf("require stats unexpected: %+v", requireStats)
	}
	if requireStats.Accuracy.NormalizedWithoutHA != 0 || requireStats.Accuracy.NormalizedHACoveragePct != 100 {
		t.Fatalf("require accuracy unexpected: %+v", requireStats.Accuracy)
	}

	allow := require
	allow.HAPolicy = "preserve"
	allowOut := filepath.Join(t.TempDir(), "preserve")
	allowStats, err := Run(input, allowOut, allow)
	if err != nil {
		t.Fatal(err)
	}
	if allowStats.ValidInputRows != 3 || allowStats.RejectedInputRows != 0 {
		t.Fatalf("allow stats unexpected: %+v", allowStats)
	}
	if allowStats.Accuracy.NormalizedWithHA != 2 || allowStats.Accuracy.NormalizedWithoutHA != 1 {
		t.Fatalf("allow normalized accuracy unexpected: %+v", allowStats.Accuracy)
	}
	if allowStats.Accuracy.NormalizedHACoveragePct < 66.6 || allowStats.Accuracy.NormalizedHACoveragePct > 66.7 {
		t.Fatalf("allow coverage=%f", allowStats.Accuracy.NormalizedHACoveragePct)
	}

	b, err := os.ReadFile(filepath.Join(allowOut, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Config.HAPolicy != "preserve" || m.Stats.Accuracy.NormalizedWithoutHA != 1 {
		t.Fatalf("manifest missing HA policy/coverage: %+v", m)
	}
}

func TestPerformanceAndDistributionDiagnostics(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	out := t.TempDir()
	stats, err := Run(filepath.Join("..", "..", "testdata", "sample.csv"), out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Performance.ElapsedSeconds <= 0 || stats.Performance.InputRowsPerSecond <= 0 {
		t.Fatalf("missing performance metrics: %+v", stats.Performance)
	}
	if stats.Performance.TemporaryPeakBytes <= 0 {
		t.Fatalf("temporary peak bytes=%d", stats.Performance.TemporaryPeakBytes)
	}
	if len(stats.StepDistributions.EffectiveDistanceM) == 0 || len(stats.StepDistributions.ImpliedSpeedMPS) == 0 {
		t.Fatalf("missing step distributions: %+v", stats.StepDistributions)
	}
	d := stats.Diagnostics.Eventizer
	reasons := 0
	for _, n := range d.DiscontinuityReasons {
		reasons += n
	}
	if reasons != d.DiscontinuityEdges {
		t.Fatalf("discontinuity reasons=%d edges=%d", reasons, d.DiscontinuityEdges)
	}
	for reason, states := range d.DiscontinuityReasonAccuracy {
		n := 0
		for _, count := range states {
			n += count
		}
		if n != d.DiscontinuityReasons[reason] {
			t.Fatalf("reason %s accuracy total=%d reason total=%d", reason, n, d.DiscontinuityReasons[reason])
		}
	}
}

func TestDiscontinuityReasonIntegration(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	out := t.TempDir()
	stats, err := Run(filepath.Join("..", "..", "testdata", "discontinuity_diagnostics.csv"), out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	d := stats.Diagnostics.Eventizer
	for _, reason := range []string{"jump_only", "speed_only", "jump_and_speed"} {
		if d.DiscontinuityReasons[reason] != 1 {
			t.Fatalf("reason %s=%d, all=%+v", reason, d.DiscontinuityReasons[reason], d.DiscontinuityReasons)
		}
	}
	if d.DiscontinuityReasonAccuracy["speed_only"]["both_missing"] != 1 {
		t.Fatalf("expected both-missing speed-only cross-tab: %+v", d.DiscontinuityReasonAccuracy)
	}
}

func TestTimeDeltaBuckets(t *testing.T) {
	cases := []struct {
		dt   float64
		want string
	}{
		{0, "non_positive"}, {0.5, "lt_1s"}, {1, "1_5s"}, {4.999, "1_5s"},
		{5, "5_30s"}, {30, "30_60s"}, {60, "1_5m"}, {300, "5_30m"},
		{1800, "30m_2h"}, {7199.9, "30m_2h"}, {7200, "ge_2h"},
	}
	for _, tc := range cases {
		if got := dtBucket(tc.dt); got != tc.want {
			t.Fatalf("dtBucket(%v)=%q want %q", tc.dt, got, tc.want)
		}
	}
}

func TestEffectiveDistanceBuckets(t *testing.T) {
	cases := []struct {
		distance float64
		want     string
	}{
		{0, "lt_5m"},
		{4.999, "lt_5m"},
		{5, "5_50m"},
		{49.999, "5_50m"},
		{50, "50_500m"},
		{499.999, "50_500m"},
		{500, "500m_5km"},
		{4999.999, "500m_5km"},
		{5000, "ge_5km"},
	}
	for _, tc := range cases {
		if got := effectiveDistanceBucket(tc.distance); got != tc.want {
			t.Fatalf("effectiveDistanceBucket(%v)=%q want %q", tc.distance, got, tc.want)
		}
	}
}

func TestSpeedOnlyDTDistanceDiagnostics(t *testing.T) {
	cfg := config.Default()
	collector := newDistributionCollectors()
	obs := []model.Observation{
		{StepAccuracyState: "initial"},
		{
			StepAccuracyState:      "both_missing",
			StepDTS:                0.25,
			StepDistanceM:          2.2,
			StepEffectiveDistanceM: 2.2,
			ImpliedSpeedMPS:        80,
			IsDiscontinuity:        true,
		},
		{
			StepAccuracyState:      "both_known",
			StepDTS:                0.5,
			StepDistanceM:          100,
			StepEffectiveDistanceM: 100,
			ImpliedSpeedMPS:        200,
			IsDiscontinuity:        true,
		},
	}
	collector.AddObservations(obs, cfg)
	got := collector.TemporalSummary()
	if got.SpeedOnlyCandidateClasses["micro_displacement"] != 1 {
		t.Fatalf("micro_displacement=%d", got.SpeedOnlyCandidateClasses["micro_displacement"])
	}
	if got.SpeedOnlyCandidateClasses["other_speed_only"] != 1 {
		t.Fatalf("other_speed_only=%d", got.SpeedOnlyCandidateClasses["other_speed_only"])
	}
	if got.SpeedOnlyDTDistance["lt_1s"]["lt_5m"] != 1 || got.SpeedOnlyDTDistance["lt_1s"]["50_500m"] != 1 {
		t.Fatalf("unexpected dt/distance matrix: %+v", got.SpeedOnlyDTDistance)
	}
	if got.SpeedOnlyAccuracyDTDistance["both_missing"]["lt_1s"]["lt_5m"] != 1 {
		t.Fatalf("missing both-missing micro cell: %+v", got.SpeedOnlyAccuracyDTDistance)
	}
	if got.SpeedOnlyAccuracyDTDistance["both_known"]["lt_1s"]["50_500m"] != 1 {
		t.Fatalf("missing both-known material cell: %+v", got.SpeedOnlyAccuracyDTDistance)
	}
}

func TestTemporalDiagnosticsAccountingAndExecutionContext(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	cfg.Partitions = 2
	out := filepath.Join(t.TempDir(), "out")
	stats, err := Run(filepath.Join("..", "..", "testdata", "discontinuity_diagnostics.csv"), out, cfg)
	if err != nil {
		t.Fatal(err)
	}

	var edgeBuckets int64
	for _, buckets := range stats.Temporal.EdgeDTBuckets {
		for _, n := range buckets {
			edgeBuckets += n
		}
	}
	wantEdges := stats.NormalizedRows - stats.EntityCount
	if edgeBuckets != wantEdges {
		t.Fatalf("edge dt buckets=%d want adjacencies=%d", edgeBuckets, wantEdges)
	}

	var discBuckets int64
	for _, buckets := range stats.Temporal.DiscontinuityDTBuckets {
		for _, n := range buckets {
			discBuckets += n
		}
	}
	if discBuckets != int64(stats.Diagnostics.Eventizer.DiscontinuityEdges) {
		t.Fatalf("discontinuity dt buckets=%d edges=%d", discBuckets, stats.Diagnostics.Eventizer.DiscontinuityEdges)
	}

	var triple int64
	for _, byState := range stats.Temporal.DiscontinuityReasonAccuracyDT {
		for _, buckets := range byState {
			for _, n := range buckets {
				triple += n
			}
		}
	}
	if triple != int64(stats.Diagnostics.Eventizer.DiscontinuityEdges) {
		t.Fatalf("reason/accuracy/dt total=%d edges=%d", triple, stats.Diagnostics.Eventizer.DiscontinuityEdges)
	}

	var speedOnlyN int64
	for _, d := range stats.Temporal.SpeedOnlyEffectiveDistanceM {
		speedOnlyN += d.Count
	}
	if speedOnlyN != int64(stats.Diagnostics.Eventizer.DiscontinuityReasons["speed_only"]) {
		t.Fatalf("speed-only distance n=%d reason=%d", speedOnlyN, stats.Diagnostics.Eventizer.DiscontinuityReasons["speed_only"])
	}

	var matrixN int64
	for _, distances := range stats.Temporal.SpeedOnlyDTDistance {
		for _, n := range distances {
			matrixN += n
		}
	}
	if matrixN != speedOnlyN {
		t.Fatalf("speed-only dt/distance matrix=%d want %d", matrixN, speedOnlyN)
	}
	var accuracyMatrixN int64
	for _, byDT := range stats.Temporal.SpeedOnlyAccuracyDTDistance {
		for _, distances := range byDT {
			for _, n := range distances {
				accuracyMatrixN += n
			}
		}
	}
	if accuracyMatrixN != speedOnlyN {
		t.Fatalf("speed-only accuracy/dt/distance matrix=%d want %d", accuracyMatrixN, speedOnlyN)
	}
	var classN int64
	for _, n := range stats.Temporal.SpeedOnlyCandidateClasses {
		classN += n
	}
	if classN != speedOnlyN {
		t.Fatalf("speed-only candidate classes=%d want %d", classN, speedOnlyN)
	}

	if len(stats.StepDistributions.StepDTS) == 0 {
		t.Fatal("missing step dt quantiles")
	}
	if stats.Performance.Execution.GOOS == "" || stats.Performance.Execution.GOARCH == "" {
		t.Fatalf("missing execution context: %+v", stats.Performance.Execution)
	}
	if stats.Performance.Execution.Input.PathClass == "" || stats.Performance.Execution.Output.PathClass == "" || stats.Performance.Execution.Temporary.PathClass == "" {
		t.Fatalf("missing path context: %+v", stats.Performance.Execution)
	}
}

func TestClassifyWindowsMountedPath(t *testing.T) {
	if got := classifyPath("/mnt/d/crosscue/xqmob"); got != "windows_mounted_path" {
		t.Fatalf("classifyPath WSL mount=%q", got)
	}
}

func TestStructuredSourceCharacterization(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	out := t.TempDir()
	stats, err := Run(filepath.Join("..", "..", "testdata", "source_characterization.csv"), out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	target := stats.Spatial.BothMissingSpeedOnlyShortDT50To500M
	if target.Count != 1 {
		t.Fatalf("target count=%d want 1; %+v", target.Count, target)
	}
	if target.Bearing16["N"] != 1 {
		t.Fatalf("bearing distribution=%+v", target.Bearing16)
	}
	if target.PreviousCoordinateDecimalPlaces["lat_6_lon_6"] != 1 || target.CurrentCoordinateDecimalPlaces["lat_6_lon_6"] != 1 {
		t.Fatalf("coordinate precision prev=%+v current=%+v", target.PreviousCoordinateDecimalPlaces, target.CurrentCoordinateDecimalPlaces)
	}
	var distanceModes int64
	for _, n := range target.DistanceRounded1M {
		distanceModes += n
	}
	if distanceModes != 1 {
		t.Fatalf("distance modes total=%d", distanceModes)
	}
	var vectors int64
	for _, n := range target.VectorRounded10M {
		vectors += n
	}
	if vectors != 1 {
		t.Fatalf("vector bins total=%d", vectors)
	}
}

func TestCustomTempDirAndMemoryDiagnostics(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "out")
	tempParent := filepath.Join(root, "native-temp")
	cfg := config.Default()
	cfg.Source = "test"
	cfg.Partitions = 2
	cfg.TempDir = tempParent
	cfg.KeepTemporary = true
	stats, err := Run(filepath.Join("..", "..", "testdata", "sample.csv"), out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(tempParent, ".xqmob-tmp-*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("custom temp workspace matches=%v err=%v", matches, err)
	}
	if len(stats.Performance.Memory.ByPhase) == 0 {
		t.Fatal("missing per-phase memory diagnostics")
	}
	ws := stats.Performance.Memory.WorkingSet
	if ws.MaxPartitionRows == 0 || ws.MaxEntityObservations == 0 || ws.ParquetApplicationBatchRows != 1024 {
		t.Fatalf("unexpected working-set diagnostics: %+v", ws)
	}
	if ws.PartitionWriterBufferBytes != 2*(1<<20) || ws.CanonicalJSONBufferBytes != 1<<20 {
		t.Fatalf("unexpected application buffers: %+v", ws)
	}
	if stats.Performance.Memory.SampleStrideEntities != 1024 {
		t.Fatalf("memory sample stride=%d", stats.Performance.Memory.SampleStrideEntities)
	}
}

func TestSourceRegimePrecisionTransitionsAndReversals(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	cfg.Partitions = 1
	out := t.TempDir()
	stats, err := Run(filepath.Join("..", "..", "testdata", "source_regime_reversal.csv"), out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	target := stats.Spatial.BothMissingSpeedOnlyShortDT50To500M
	if target.Count != 2 {
		t.Fatalf("target count=%d want 2; %+v", target.Count, target)
	}
	if target.PrecisionTransitions["lat_4_lon_5->lat_4_lon_3"] != 1 || target.PrecisionTransitions["lat_4_lon_3->lat_4_lon_5"] != 1 {
		t.Fatalf("precision transitions=%+v", target.PrecisionTransitions)
	}
	var precisionTransitionTotal int64
	for _, n := range target.PrecisionTransitions {
		precisionTransitionTotal += n
	}
	if precisionTransitionTotal != target.Count {
		t.Fatalf("precision transition total=%d target=%d", precisionTransitionTotal, target.Count)
	}
	for transition, byBearing := range target.PrecisionTransitionBearing16 {
		var n int64
		for _, v := range byBearing {
			n += v
		}
		if n != target.PrecisionTransitions[transition] {
			t.Fatalf("bearing cross-tab %s=%d want %d", transition, n, target.PrecisionTransitions[transition])
		}
	}
	r := target.Reversals
	if r.ConsecutiveTargetEdgePairs != 1 || r.ExactABAReturns != 1 || r.ReturnWithin5M != 1 || r.ABALikePairs != 1 || r.PrecisionABAReturns != 1 {
		t.Fatalf("unexpected reversal diagnostics: %+v", r)
	}
	if r.PrecisionTriplets["lat_4_lon_5->lat_4_lon_3->lat_4_lon_5"] != 1 {
		t.Fatalf("precision triplets=%+v", r.PrecisionTriplets)
	}
}

func TestOptionalHeapProfiles(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.Source = "test"
	cfg.Partitions = 4
	cfg.MemoryProfileDir = filepath.Join(root, "heap")
	stats, err := Run(filepath.Join("..", "..", "testdata", "sample.csv"), filepath.Join(root, "out"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	m := stats.Performance.Memory
	if !m.ProfilingEnabled || !m.ProfilingForcesGC {
		t.Fatalf("heap profiling flags=%+v", m)
	}
	if len(m.HeapProfiles) != 8 {
		t.Fatalf("heap profiles=%d want 8: %+v", len(m.HeapProfiles), m.HeapProfiles)
	}
	if stats.Performance.Phase.HeapProfileSeconds <= 0 {
		t.Fatalf("heap profile time=%f", stats.Performance.Phase.HeapProfileSeconds)
	}
	for _, hp := range m.HeapProfiles {
		if hp.Path == "" || hp.Bytes <= 0 || hp.GoHeapAllocBytes <= 0 {
			t.Fatalf("invalid heap profile info: %+v", hp)
		}
		if info, err := os.Stat(hp.Path); err != nil || info.Size() <= 0 {
			t.Fatalf("heap profile %q stat=%v info=%v", hp.Path, err, info)
		}
	}
}

func TestParquetTuningDoesNotChangeCanonicalEvents(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "sample.csv")

	base := config.Default()
	base.Source = "test"
	base.Partitions = 2
	baseOut := filepath.Join(t.TempDir(), "base")
	if _, err := Run(input, baseOut, base); err != nil {
		t.Fatal(err)
	}

	tuned := base
	tuned.ParquetRowGroupRows = 2
	tuned.ParquetDictionaryMaxBytes = 1024
	tunedOut := filepath.Join(t.TempDir(), "tuned")
	if _, err := Run(input, tunedOut, tuned); err != nil {
		t.Fatal(err)
	}

	a, err := os.ReadFile(filepath.Join(baseOut, "canonical", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(tunedOut, "canonical", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("Parquet physical tuning changed canonical event stream")
	}
}

func TestRejectInvalidParquetTuning(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	cfg.ParquetRowGroupRows = 0
	if _, err := Run(filepath.Join("..", "..", "testdata", "sample.csv"), filepath.Join(t.TempDir(), "out"), cfg); err == nil {
		t.Fatal("expected zero parquet row-group limit to be rejected")
	}

	cfg = config.Default()
	cfg.Source = "test"
	cfg.ParquetDictionaryMaxBytes = -1
	if _, err := Run(filepath.Join("..", "..", "testdata", "sample.csv"), filepath.Join(t.TempDir(), "out2"), cfg); err == nil {
		t.Fatal("expected negative dictionary limit to be rejected")
	}
}

func TestH3ResolutionDoesNotChangeCanonicalEvents(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "sample.csv")

	r11 := config.Default()
	r11.Source = "h3-invariance"
	r11.Partitions = 2
	out11 := filepath.Join(t.TempDir(), "r11")
	if _, err := Run(input, out11, r11); err != nil {
		t.Fatal(err)
	}

	r9 := r11
	r9.H3Resolution = 9
	out9 := filepath.Join(t.TempDir(), "r9")
	if _, err := Run(input, out9, r9); err != nil {
		t.Fatal(err)
	}

	a, err := os.ReadFile(filepath.Join(out11, "canonical", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out9, "canonical", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("H3 resolution changed canonical event stream")
	}

	var m11, m9 Manifest
	mb, err := os.ReadFile(filepath.Join(out11, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mb, &m11); err != nil {
		t.Fatal(err)
	}
	mb, err = os.ReadFile(filepath.Join(out9, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mb, &m9); err != nil {
		t.Fatal(err)
	}
	if m11.Config.H3Resolution != 11 || m9.Config.H3Resolution != 9 {
		t.Fatalf("unexpected H3 resolutions: r11=%d r9=%d", m11.Config.H3Resolution, m9.Config.H3Resolution)
	}
}

func TestValidateConfigRejectsInvalidH3Resolution(t *testing.T) {
	cfg := config.Default()
	cfg.H3Resolution = 16
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected invalid H3 resolution to be rejected")
	}
}
