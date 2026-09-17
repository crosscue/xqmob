package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/crosscue/xqmob/internal/config"
	inspectpkg "github.com/crosscue/xqmob/internal/inspect"
	"github.com/crosscue/xqmob/internal/mcpserver"
	"github.com/crosscue/xqmob/internal/model"
	"github.com/crosscue/xqmob/internal/output"
	"github.com/crosscue/xqmob/internal/pipeline"
	validatepkg "github.com/crosscue/xqmob/internal/validate"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "eventize":
		err = runEventize(os.Args[2:])
	case "validate":
		err = runValidate(os.Args[2:])
	case "inspect":
		err = runInspect(os.Args[2:])
	case "mcp":
		err = runMCP(os.Args[2:])
	case "version", "--version", "-version":
		fmt.Printf("xqmob %s\neventizer %s\nanalysis %s\nmcp %s\nCrosscue Event Model wire %s / %s\n", config.Version, config.EventizerVersion, config.AnalysisContractVersion, config.MCPContractVersion, config.WireVersion, config.ProfileID)
		return
	case "help", "--help", "-h":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "xqmob:", err)
		os.Exit(1)
	}
}

func runEventize(args []string) error {
	cfg := config.Default()
	fs := flag.NewFlagSet("eventize", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: xqmob eventize [flags] <input.csv>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Input CSV headers: id, ts, lat, lon, ha")
		fmt.Fprintln(os.Stderr, "")
		fs.PrintDefaults()
	}
	out := fs.String("out", "xqmob-output", "output directory")
	fs.BoolVar(&cfg.ReplaceOutput, "replace", false, "replace known dataset artifacts and diagnostics in a non-empty output directory")
	full := fs.Bool("full", false, "shorthand for --output-profile full (includes tracks.parquet with trajectory geometry)")
	verbose := fs.Bool("verbose", false, "print full diagnostic, source-characterization, performance, and storage detail")
	var traceIDs stringListFlag
	dictionaryMaxMiB := cfg.ParquetDictionaryMaxBytes / (1 << 20)
	fs.Var(&traceIDs, "trace-id", "write a per-observation decision trace for this entity id; may be repeated")
	fs.StringVar(&cfg.OutputProfile, "output-profile", cfg.OutputProfile, "output profile: standard or full")
	fs.StringVar(&cfg.ParquetCompression, "parquet-compression", cfg.ParquetCompression, "Parquet page compression: zstd, snappy, or none")
	fs.Int64Var(&cfg.ParquetRowGroupRows, "parquet-row-group-rows", cfg.ParquetRowGroupRows, "maximum rows per Parquet row group")
	fs.Int64Var(&dictionaryMaxMiB, "parquet-dictionary-max-mib", dictionaryMaxMiB, "maximum dictionary bytes per Parquet column per row group in MiB; 0 means unlimited")
	fs.StringVar(&cfg.Source, "source", "", "source identifier written to Core Events (required)")
	fs.StringVar(&cfg.SubjectPrefix, "subject-prefix", cfg.SubjectPrefix, "prefix applied to input id for Core subject")
	fs.IntVar(&cfg.GeohashPrecision, "geohash-precision", cfg.GeohashPrecision, "geohash precision")
	fs.IntVar(&cfg.H3Resolution, "h3-resolution", cfg.H3Resolution, "H3 analytical cell resolution (0-15; default 11)")
	fs.Float64Var(&cfg.MoveRadiusM, "move-radius-m", cfg.MoveRadiusM, "movement deadband in metres")
	fs.Float64Var(&cfg.DwellThresholdS, "dwell-threshold-s", cfg.DwellThresholdS, "DWELL threshold in seconds")
	fs.Float64Var(&cfg.GapThresholdS, "gap-threshold-s", cfg.GapThresholdS, "GAP threshold in seconds")
	fs.Float64Var(&cfg.MaxSpeedMPS, "max-speed-mps", cfg.MaxSpeedMPS, "maximum continuous implied speed in m/s")
	fs.Float64Var(&cfg.MaxJumpM, "max-jump-m", cfg.MaxJumpM, "maximum continuous effective jump in metres")
	fs.BoolVar(&cfg.ConfirmMoves, "confirm-moves", cfg.ConfirmMoves, "require two outside fixes to confirm departure")
	fs.Float64Var(&cfg.ConfirmWindowS, "confirm-window-s", cfg.ConfirmWindowS, "maximum seconds between departure candidate fixes")
	fs.Float64Var(&cfg.WalkMaxSpeedMPS, "walk-max-speed-mps", cfg.WalkMaxSpeedMPS, "walk-compatible speed ceiling")
	fs.Float64Var(&cfg.WalkMaxJumpM, "walk-max-jump-m", cfg.WalkMaxJumpM, "walk-compatible jump ceiling")
	fs.StringVar(&cfg.AccuracyPolicy, "accuracy-policy", cfg.AccuracyPolicy, "accuracy policy (v1: subtract_radii)")
	fs.StringVar(&cfg.HAPolicy, "ha-policy", cfg.HAPolicy, "horizontal-accuracy handling: preserve (default) or require; allow-missing is an alias for preserve")
	fs.IntVar(&cfg.Partitions, "partitions", cfg.Partitions, "deterministic spill partitions (1-512)")
	fs.Float64Var(&cfg.MaxHAM, "max-ha-m", 0, "optional analyst-selected HA filter: reject known HA above this value; 0 preserves all")
	fs.BoolVar(&cfg.Strict, "strict", false, "abort on first invalid input row")
	fs.BoolVar(&cfg.KeepTemporary, "keep-temp", false, "retain temporary partition files")
	fs.StringVar(&cfg.TempDir, "temp-dir", "", "parent directory for temporary partition/sort workspace (default: output directory)")
	fs.StringVar(&cfg.MemoryProfileDir, "profile-memory", "", "optional directory for native Go heap snapshots; forces GC and perturbs benchmark timings")

	inputPath, flagArgs := peelInput(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if inputPath == "" && fs.NArg() == 1 {
		inputPath = fs.Arg(0)
	}
	if inputPath == "" || fs.NArg() > 1 {
		return fmt.Errorf("usage: xqmob eventize [flags] <input.csv>")
	}
	if cfg.Source == "" {
		return fmt.Errorf("--source is required")
	}
	if cfg.HAPolicy == "allow-missing" {
		cfg.HAPolicy = "preserve"
	}
	if dictionaryMaxMiB < 0 {
		return fmt.Errorf("--parquet-dictionary-max-mib must be >= 0")
	}
	cfg.ParquetDictionaryMaxBytes = dictionaryMaxMiB * (1 << 20)
	if *full {
		cfg.OutputProfile = "full"
	}
	cfg.TraceIDs = append(cfg.TraceIDs, traceIDs...)
	absOut, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	stats, err := pipeline.Run(inputPath, absOut, cfg)
	if err != nil {
		return err
	}
	printEventizeSummary(absOut, cfg, stats, *verbose)
	return nil
}

func printEventizeSummary(absOut string, cfg config.Config, stats pipeline.Stats, verbose bool) {
	fmt.Printf("xqmob: wrote %s\n", absOut)
	fmt.Printf("  input=%d normalized=%d entities=%d events=%d segments=%d presence=%d transitions=%d\n",
		stats.InputRows, stats.NormalizedRows, stats.EntityCount, stats.EventCount, stats.SegmentCount, stats.PresenceCount, stats.TransitionCount)
	if stats.RejectedInputRows > 0 {
		fmt.Printf("  rejected=%d\n", stats.RejectedInputRows)
	}
	fmt.Printf("  accuracy_coverage=%.1f%%\n", stats.Accuracy.NormalizedHACoveragePct)
	if stats.Performance.ElapsedSeconds > 0 {
		fmt.Printf("  elapsed=%.2fs throughput=%.0f input_rows/s", stats.Performance.ElapsedSeconds, stats.Performance.InputRowsPerSecond)
		if stats.Performance.PeakRSSAvailable {
			fmt.Printf(" peak_rss=%s", humanBytes(stats.Performance.PeakRSSBytes))
		}
		fmt.Println()
	}
	fmt.Printf("  analysis=%s (%.2fx input) canonical=%s total=%s\n",
		humanBytes(stats.Storage.AnalysisBytes), stats.Storage.AnalysisExpansionRatio,
		humanBytes(stats.Storage.CanonicalBytes), humanBytes(stats.Storage.OutputArtifactBytes))
	if len(stats.TraceFiles) > 0 {
		fmt.Printf("  traces=%d\n", len(stats.TraceFiles))
	}
	if cfg.MemoryProfileDir != "" {
		fmt.Printf("  heap_profiles=%d (profiling forces GC; timings are diagnostic)\n", len(stats.Performance.Memory.HeapProfiles))
	}
	if !verbose {
		fmt.Println("  details: xqmob inspect --diagnostics " + absOut)
		return
	}

	fmt.Printf("  profile=%s parquet_compression=%s ha_policy=%s geohash_precision=%d h3_resolution=%d\n", cfg.OutputProfile, cfg.ParquetCompression, cfg.HAPolicy, cfg.GeohashPrecision, cfg.H3Resolution)
	fmt.Printf("  parquet: row_group_max_rows=%d dictionary_max=%s\n", cfg.ParquetRowGroupRows, humanBytes(cfg.ParquetDictionaryMaxBytes))
	fmt.Printf("  valid=%d rejected=%d normalized=%d same_ts_collapsed=%d\n", stats.ValidInputRows, stats.RejectedInputRows, stats.NormalizedRows, stats.Diagnostics.Eventizer.SameTimestampCollapsed)
	printDiagnostics(stats)
	fmt.Printf("  accuracy: input_present=%d input_missing=%d accepted_known=%d accepted_missing=%d normalized_known=%d normalized_missing=%d coverage=%.1f%%\n",
		stats.Accuracy.InputHAPresentRows, stats.Accuracy.InputHAMissingRows, stats.Accuracy.AcceptedWithHA, stats.Accuracy.AcceptedWithoutHA,
		stats.Accuracy.NormalizedWithHA, stats.Accuracy.NormalizedWithoutHA, stats.Accuracy.NormalizedHACoveragePct)
	fmt.Printf("    support: entities(all_known=%d mixed=%d all_missing=%d) segments(all_known=%d mixed=%d all_missing=%d) presence(all_known=%d mixed=%d all_missing=%d) transitions(all_known=%d mixed=%d all_missing=%d)\n",
		stats.Accuracy.EntitiesAllKnown, stats.Accuracy.EntitiesMixed, stats.Accuracy.EntitiesAllMissing,
		stats.Accuracy.SegmentsAllKnown, stats.Accuracy.SegmentsMixed, stats.Accuracy.SegmentsAllMissing,
		stats.Accuracy.PresenceAllKnown, stats.Accuracy.PresenceMixed, stats.Accuracy.PresenceAllMissing,
		stats.Accuracy.TransitionsAllKnown, stats.Accuracy.TransitionsMixed, stats.Accuracy.TransitionsAllMissing)
	printAccuracyAttribution(stats.Diagnostics.Eventizer)
	printDiscontinuityAttribution(stats.Diagnostics.Eventizer)
	printStepDistributions(stats.StepDistributions)
	printTemporalDiagnostics(stats.Temporal)
	printSpatialDiagnostics(stats.Spatial)
	printPerformance(stats.Performance)
	printParquetStats(stats.Parquet)
	fmt.Printf("  storage: input=%s canonical=%s (%.2fx) analysis=%s (%.2fx) total=%s (%.2fx)\n",
		humanBytes(stats.Storage.InputBytes), humanBytes(stats.Storage.CanonicalBytes), stats.Storage.CanonicalExpansionRatio,
		humanBytes(stats.Storage.AnalysisBytes), stats.Storage.AnalysisExpansionRatio,
		humanBytes(stats.Storage.OutputArtifactBytes), stats.Storage.TotalArtifactExpansionRatio)
	fmt.Printf("  analysis bytes/normalized observation: %.1f\n", stats.Storage.AnalysisBytesPerObservation)
	for _, f := range stats.Storage.Files {
		fmt.Printf("    %-42s %10s\n", f.Path, humanBytes(f.Bytes))
	}
	for _, trace := range stats.TraceFiles {
		fmt.Printf("  trace: %s\n", trace)
	}
}

// peelInput permits both `xqmob eventize input.csv --source x` and the conventional
// `xqmob eventize --source x input.csv` form without introducing a CLI framework.
func peelInput(args []string) (string, []string) {
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		return args[0], args[1:]
	}
	return "", args
}

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: xqmob validate <events.jsonl>")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: xqmob validate <events.jsonl>")
	}
	report, err := validatepkg.EventsFile(fs.Arg(0))
	if err != nil {
		for _, msg := range report.Errors {
			fmt.Fprintln(os.Stderr, msg)
		}
		return fmt.Errorf("%d event(s), %d validation error(s)", report.Events, len(report.Errors))
	}
	fmt.Printf("valid: %d Mobility Profile 0.1 event(s)\n", report.Events)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `xqmob - Crosscue mobility eventizer

Usage:
  xqmob eventize [flags] <input.csv>
  xqmob eventize <input.csv> [flags]
  xqmob validate <events.jsonl>
  xqmob inspect [--entity ID] [--diagnostics] [--json] <output-dir>
  xqmob mcp [--duckdb-threads N] <output-dir>
  xqmob version

Input CSV must contain headers: id, ts, lat, lon, ha

The canonical output is canonical/events.jsonl. The default standard profile writes
GeoParquet/Parquet layers for observations, events, segments, presence intervals,
transitions and entity summaries. Spatial analytical rows carry both geohash and H3
cell indexing; H3 defaults to resolution 11 and does not drive eventization. Use --full (or --output-profile full) to also
materialize tracks.parquet with complete track trajectory geometry. Default eventize output is a concise operational summary; add --verbose for the full diagnostic report retained in manifest.json. Parquet row groups default to 131072 rows and per-column dictionaries to 8 MiB; use --parquet-row-group-rows and --parquet-dictionary-max-mib for physical-output benchmarking. By default --ha-policy preserve
retains missing horizontal accuracy as null. --ha-policy require is an explicit analyst-selected
filter. Missing HA is never imputed; distance calculations subtract only known accuracy radii. Use --temp-dir DIR to place spill partitions on a different filesystem (for example native WSL storage while output remains under /mnt/d). Use --profile-memory DIR to capture native Go heap profiles at fixed milestones; profiling forces GC and is not benchmark-comparable. Use --trace-id ID to write a targeted per-observation eventizer decision trace; repeat the flag for
multiple entities. Use xqmob inspect OUTPUT for a compact dataset summary and
xqmob inspect --entity ID OUTPUT for a single entity summary without separate Parquet tooling.

xqmob mcp OUTPUT starts the read-only domain-specific MCP server over stdio. MCP queries the analytical Parquet layer through embedded DuckDB; stdout is reserved exclusively for protocol messages and diagnostics go to stderr.`)
}

func runMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: xqmob mcp [--duckdb-threads N] <output-dir>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Starts a read-only xqmob MCP server over stdio for an existing output directory.")
		fmt.Fprintln(os.Stderr, "stdout is reserved exclusively for MCP JSON-RPC; server diagnostics go to stderr.")
		fs.PrintDefaults()
	}
	threads := fs.Int("duckdb-threads", 4, "DuckDB worker threads used by MCP analytical queries")
	dataset, flagArgs := peelInput(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if dataset == "" && fs.NArg() == 1 {
		dataset = fs.Arg(0)
	}
	if dataset == "" || fs.NArg() > 1 {
		return fmt.Errorf("usage: xqmob mcp [--duckdb-threads N] <output-dir>")
	}
	if *threads < 1 || *threads > 128 {
		return fmt.Errorf("--duckdb-threads must be between 1 and 128")
	}
	return mcpserver.RunStdio(context.Background(), dataset, mcpserver.Options{DuckDBThreads: *threads})
}

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: xqmob inspect [--entity ID] [--diagnostics] [--json] <output-dir>")
		fs.PrintDefaults()
	}
	entityID := fs.String("entity", "", "inspect a single raw entity id from analysis/entities.parquet")
	diagnostics := fs.Bool("diagnostics", false, "print full stored diagnostic, source-characterization, performance, and Parquet detail")
	asJSON := fs.Bool("json", false, "emit the complete inspection report as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: xqmob inspect [--entity ID] [--diagnostics] [--json] <output-dir>")
	}
	report, err := inspectpkg.Load(fs.Arg(0), *entityID)
	if err != nil {
		return err
	}
	if *asJSON {
		b, err := inspectpkg.JSON(report)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	m := report.Manifest
	s := m.Stats
	fmt.Printf("xqmob inspect: %s\n", report.Root)
	fmt.Printf("  tool=%s eventizer=%s analysis_contract=%s profile=%s\n", m.ToolVersion, m.Eventizer, m.AnalysisContract, m.MobilityProfile)
	fmt.Printf("  spatial_index: geohash_precision=%d h3_resolution=%d\n", m.Config.GeohashPrecision, m.Config.H3Resolution)
	fmt.Printf("  input=%s source=%s created=%s\n", m.Input, m.Source, m.CreatedUTC)
	fmt.Printf("  rows: input=%d valid=%d rejected=%d normalized=%d entities=%d events=%d segments=%d presence=%d transitions=%d\n",
		s.InputRows, s.ValidInputRows, s.RejectedInputRows, s.NormalizedRows, s.EntityCount, s.EventCount, s.SegmentCount, s.PresenceCount, s.TransitionCount)
	fmt.Printf("  accuracy: coverage=%.1f%% entities(all_known=%d mixed=%d all_missing=%d) transitions(all_known=%d mixed=%d all_missing=%d)\n",
		s.Accuracy.NormalizedHACoveragePct, s.Accuracy.EntitiesAllKnown, s.Accuracy.EntitiesMixed, s.Accuracy.EntitiesAllMissing, s.Accuracy.TransitionsAllKnown, s.Accuracy.TransitionsMixed, s.Accuracy.TransitionsAllMissing)
	fmt.Printf("  storage: canonical=%s analysis=%s analysis/input=%.2fx total=%s\n",
		humanBytes(m.Storage.CanonicalBytes), humanBytes(m.Storage.AnalysisBytes), m.Storage.AnalysisExpansionRatio, humanBytes(m.Storage.OutputArtifactBytes))
	if *diagnostics {
		printDiagnostics(s)
		printAccuracyAttribution(s.Diagnostics.Eventizer)
		printDiscontinuityAttribution(s.Diagnostics.Eventizer)
		printStepDistributions(s.StepDistributions)
		printTemporalDiagnostics(s.Temporal)
		printSpatialDiagnostics(s.Spatial)
		printPerformance(s.Performance)
		printParquetStats(s.Parquet)
	}
	if report.Entity != nil {
		e := report.Entity
		fmt.Printf("  entity=%s subject=%s\n", e.EntityID, e.Subject)
		fmt.Printf("    time: first=%s last=%s coverage=%.0fs\n", millisTime(e.FirstSeen), millisTime(e.LastSeen), e.CoverageS)
		fmt.Printf("    structure: observations=%d segments=%d presence=%d transitions=%d gaps=%d discontinuities=%d\n",
			e.ObservationCount, e.SegmentCount, e.PresenceCount, e.TransitionCount, e.GapCount, e.DiscontinuityCount)
		fmt.Printf("    presence: stays=%d dwells=%d total_presence=%.0fs total_dwell=%.0fs\n", e.StayCount, e.DwellCount, e.TotalPresenceS, e.TotalDwellS)
		fmt.Printf("    accuracy: class=%s known=%d missing=%d coverage=%.1f%%\n", e.HAClass, e.ObservationsWithHA, e.ObservationsWithoutHA, e.HACoveragePct)
		fmt.Printf("    spatial: unique_geohash=%d unique_h3=%d start_h3=%s end_h3=%s\n", e.UniqueGeohashCells, e.UniqueH3Cells, e.StartH3Cell, e.EndH3Cell)
		fmt.Printf("    decisions: departures=%d transitions=%d cancelled_disc=%d unresolved_eof=%d\n",
			e.DeparturesConfirmed, e.TransitionsConfirmed, e.TransitionsCancelledDiscontinuity, e.TransitionsUnresolvedEOF)
	}
	return nil
}

func printParquetStats(p output.ParquetMaterializationStats) {
	if p.RowGroupMaxRows == 0 && p.DictionaryMaxBytes == 0 && len(p.Files) == 0 {
		return
	}
	fmt.Printf("  parquet_materialization: row_group_max_rows=%d dictionary_max=%s\n", p.RowGroupMaxRows, humanBytes(p.DictionaryMaxBytes))
	paths := make([]string, 0, len(p.Files))
	for path := range p.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		st := p.Files[path]
		fmt.Printf("    %-42s rows=%d row_groups=%d\n", path, st.Rows, st.RowGroups)
	}
}

func millisTime(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func printAccuracyAttribution(d model.EventizerDiagnostics) {
	if len(d.AccuracyAttribution) == 0 {
		return
	}
	order := []string{"continuous_edges", "gap_edges", "discontinuity_edges", "departure_candidates", "departures_confirmed", "transitions_confirmed"}
	edgeStates := []string{"both_known", "previous_missing", "current_missing", "both_missing"}
	supportStates := []string{"all_known", "mixed", "all_missing"}
	fmt.Printf("    accuracy_attribution:\n")
	for _, kind := range order {
		states, ok := d.AccuracyAttribution[kind]
		if !ok {
			continue
		}
		fmt.Printf("      %s:", kind)
		keys := edgeStates
		if kind == "transitions_confirmed" {
			keys = supportStates
		}
		for _, state := range keys {
			if n := states[state]; n > 0 {
				fmt.Printf(" %s=%d", state, n)
			}
		}
		fmt.Println()
	}
}

func printDiscontinuityAttribution(d model.EventizerDiagnostics) {
	if len(d.DiscontinuityReasons) == 0 {
		return
	}
	reasons := []string{"jump_only", "speed_only", "jump_and_speed", "unknown"}
	fmt.Printf("    discontinuity_reasons:")
	for _, reason := range reasons {
		if n := d.DiscontinuityReasons[reason]; n > 0 {
			fmt.Printf(" %s=%d", reason, n)
		}
	}
	fmt.Println()
	if len(d.DiscontinuityReasonAccuracy) == 0 {
		return
	}
	states := []string{"both_known", "previous_missing", "current_missing", "both_missing"}
	fmt.Printf("    discontinuity_reason_by_accuracy:\n")
	for _, reason := range reasons {
		m := d.DiscontinuityReasonAccuracy[reason]
		if len(m) == 0 {
			continue
		}
		fmt.Printf("      %s:", reason)
		for _, state := range states {
			if n := m[state]; n > 0 {
				fmt.Printf(" %s=%d", state, n)
			}
		}
		fmt.Println()
	}
}

func printStepDistributions(d pipeline.StepDistributions) {
	states := []string{"both_known", "previous_missing", "current_missing", "both_missing"}
	if len(d.EffectiveDistanceM) == 0 && len(d.ImpliedSpeedMPS) == 0 {
		return
	}
	fmt.Printf("  step_distributions (approx):\n")
	fmt.Printf("    effective_distance_m:\n")
	for _, state := range states {
		q, ok := d.EffectiveDistanceM[state]
		if !ok || q.Count == 0 {
			continue
		}
		fmt.Printf("      %-16s n=%d p50=%.1f p90=%.1f p99=%.1f\n", state, q.Count, q.P50, q.P90, q.P99)
	}
	fmt.Printf("    implied_speed_mps:\n")
	for _, state := range states {
		q, ok := d.ImpliedSpeedMPS[state]
		if !ok || q.Count == 0 {
			continue
		}
		fmt.Printf("      %-16s n=%d p50=%.3f p90=%.3f p99=%.3f\n", state, q.Count, q.P50, q.P90, q.P99)
	}
	if len(d.StepDTS) > 0 {
		fmt.Printf("    step_dt_s:\n")
		for _, state := range states {
			q, ok := d.StepDTS[state]
			if !ok || q.Count == 0 {
				continue
			}
			fmt.Printf("      %-16s n=%d p50=%.3f p90=%.3f p99=%.3f\n", state, q.Count, q.P50, q.P90, q.P99)
		}
	}
}

func printTemporalDiagnostics(t pipeline.TemporalDiagnostics) {
	states := []string{"both_known", "previous_missing", "current_missing", "both_missing"}
	buckets := []string{"non_positive", "lt_1s", "1_5s", "5_30s", "30_60s", "1_5m", "5_30m", "30m_2h", "ge_2h"}
	distanceBuckets := []string{"lt_5m", "5_50m", "50_500m", "500m_5km", "ge_5km"}
	reasons := []string{"jump_only", "speed_only", "jump_and_speed", "unknown"}
	if len(t.EdgeDTBuckets) == 0 && len(t.DiscontinuityDTBuckets) == 0 && len(t.SpeedOnlyEffectiveDistanceM) == 0 && len(t.SpeedOnlyDTDistance) == 0 {
		return
	}
	fmt.Printf("  temporal_diagnostics:\n")
	if len(t.EdgeDTBuckets) > 0 {
		fmt.Printf("    edge_dt_buckets:")
		for _, b := range buckets {
			var total int64
			for _, st := range states {
				total += t.EdgeDTBuckets[st][b]
			}
			if total > 0 {
				fmt.Printf(" %s=%d", b, total)
			}
		}
		fmt.Println()
	}
	if len(t.DiscontinuityDTBuckets) > 0 {
		fmt.Printf("    discontinuity_dt_by_accuracy:\n")
		for _, st := range states {
			m := t.DiscontinuityDTBuckets[st]
			if len(m) == 0 {
				continue
			}
			fmt.Printf("      %s:", st)
			for _, b := range buckets {
				if n := m[b]; n > 0 {
					fmt.Printf(" %s=%d", b, n)
				}
			}
			fmt.Println()
		}
	}
	if len(t.DiscontinuityReasonAccuracyDT) > 0 {
		fmt.Printf("    discontinuity_reason_accuracy_dt:\n")
		for _, reason := range reasons {
			byState := t.DiscontinuityReasonAccuracyDT[reason]
			if len(byState) == 0 {
				continue
			}
			for _, st := range states {
				m := byState[st]
				if len(m) == 0 {
					continue
				}
				fmt.Printf("      %s/%s:", reason, st)
				for _, b := range buckets {
					if n := m[b]; n > 0 {
						fmt.Printf(" %s=%d", b, n)
					}
				}
				fmt.Println()
			}
		}
	}
	if len(t.SpeedOnlyEffectiveDistanceM) > 0 {
		fmt.Printf("    speed_only_effective_distance_m (approx):\n")
		for _, st := range states {
			q, ok := t.SpeedOnlyEffectiveDistanceM[st]
			if !ok || q.Count == 0 {
				continue
			}
			fmt.Printf("      %-16s n=%d p50=%.1f p90=%.1f p99=%.1f\n", st, q.Count, q.P50, q.P90, q.P99)
		}
	}
	if len(t.SpeedOnlyCandidateClasses) > 0 {
		fmt.Printf("    speed_only_candidate_classes:")
		for _, class := range []string{"micro_displacement", "other_speed_only"} {
			if n := t.SpeedOnlyCandidateClasses[class]; n > 0 {
				fmt.Printf(" %s=%d", class, n)
			}
		}
		fmt.Println()
	}
	if len(t.SpeedOnlyDTDistance) > 0 {
		fmt.Printf("    speed_only_dt_distance:\n")
		for _, dt := range buckets {
			m := t.SpeedOnlyDTDistance[dt]
			if len(m) == 0 {
				continue
			}
			fmt.Printf("      %s:", dt)
			for _, db := range distanceBuckets {
				if n := m[db]; n > 0 {
					fmt.Printf(" %s=%d", db, n)
				}
			}
			fmt.Println()
		}
	}
	if len(t.SpeedOnlyAccuracyDTDistance) > 0 {
		fmt.Printf("    speed_only_accuracy_dt_distance:\n")
		for _, st := range states {
			byDT := t.SpeedOnlyAccuracyDTDistance[st]
			if len(byDT) == 0 {
				continue
			}
			for _, dt := range buckets {
				m := byDT[dt]
				if len(m) == 0 {
					continue
				}
				fmt.Printf("      %s/%s:", st, dt)
				for _, db := range distanceBuckets {
					if n := m[db]; n > 0 {
						fmt.Printf(" %s=%d", db, n)
					}
				}
				fmt.Println()
			}
		}
	}
}

func printPerformance(p pipeline.PerformanceStats) {
	if p.ElapsedSeconds <= 0 {
		return
	}
	fmt.Printf("  performance: elapsed=%.2fs input_rows/s=%.0f normalized_rows/s=%.0f events/s=%.0f", p.ElapsedSeconds, p.InputRowsPerSecond, p.NormalizedRowsPerSecond, p.EventsPerSecond)
	if p.PeakRSSAvailable {
		fmt.Printf(" peak_rss=%s", humanBytes(p.PeakRSSBytes))
	}
	fmt.Printf(" temp_peak=%s\n", humanBytes(p.TemporaryPeakBytes))
	ph := p.Phase
	fmt.Printf("    phases: partition=%.2fs read=%.2fs sort=%.2fs eventize=%.2fs validate=%.2fs write=%.2fs close=%.2fs storage=%.2fs",
		ph.PartitionSeconds, ph.ReadSeconds, ph.SortSeconds, ph.EventizeSeconds, ph.ValidateSeconds, ph.WriteSeconds, ph.CloseSeconds, ph.StorageSeconds)
	if ph.HeapProfileSeconds > 0 {
		fmt.Printf(" heap_profiles=%.2fs", ph.HeapProfileSeconds)
	}
	fmt.Println()
	e := p.Execution
	if e.GOOS != "" {
		fmt.Printf("    execution: goos=%s goarch=%s wsl=%t input=%s", e.GOOS, e.GOARCH, e.WSL, e.Input.PathClass)
		if e.Input.FilesystemType != "" {
			fmt.Printf("(%s)", e.Input.FilesystemType)
		}
		fmt.Printf(" output=%s", e.Output.PathClass)
		if e.Output.FilesystemType != "" {
			fmt.Printf("(%s)", e.Output.FilesystemType)
		}
		fmt.Printf(" temp=%s", e.Temporary.PathClass)
		if e.Temporary.FilesystemType != "" {
			fmt.Printf("(%s)", e.Temporary.FilesystemType)
		}
		fmt.Println()
	}
	printMemoryDiagnostics(p.Memory)
}

func printDiagnostics(stats pipeline.Stats) {
	d := stats.Diagnostics.Eventizer
	fmt.Printf("  diagnostics: same_ts_collapsed=%d continuous_edges=%d gaps=%d discontinuities=%d gap+discontinuity=%d\n",
		d.SameTimestampCollapsed, d.ContinuousEdges, d.GapEdges, d.DiscontinuityEdges, d.GapAndDiscontinuityEdges)
	fmt.Printf("    presence: confirmed=%d starts(dataset=%d reset=%d candidate=%d arrival=%d)\n",
		d.PresenceConfirmations, d.PresenceStartDataset, d.PresenceStartContinuityReset, d.PresenceStartCandidate, d.PresenceStartMovementArrival)
	fmt.Printf("    presence_end: movement=%d gap=%d discontinuity=%d dataset_end=%d enter_suppressed_no_pending=%d\n",
		d.PresenceEndConfirmedMovement, d.PresenceEndGap, d.PresenceEndDiscontinuity, d.PresenceEndDataset, d.EnterSuppressedNoPendingTransition)
	candidateAccounted := d.DepartureCandidateOutcomes()
	candidateUnaccounted := d.DepartureCandidateUnaccounted
	fmt.Printf("    departure: candidate_sequences=%d accounted=%d unaccounted=%d outside_fixes=%d confirmed=%d returned_inside=%d window_expired=%d cancelled_gap=%d cancelled_discontinuity=%d cancelled_eof=%d\n",
		d.DepartureCandidateSequences, candidateAccounted, candidateUnaccounted, d.DepartureCandidateOutsideFixes, d.DeparturesConfirmed, d.DepartureCandidatesReturnedInside, d.DepartureCandidateWindowExpired,
		d.DepartureCandidatesCancelledGap, d.DepartureCandidatesCancelledDiscontinuity, d.DepartureCandidatesCancelledEOF)
	completion := 0.0
	if d.PendingTransitionsStarted > 0 {
		completion = 100 * float64(d.TransitionsConfirmed) / float64(d.PendingTransitionsStarted)
	}
	fmt.Printf("    transitions: pending=%d confirmed=%d (%.1f%%) cancelled_gap=%d cancelled_discontinuity=%d cancelled_segment=%d unresolved_eof=%d unaccounted=%d\n",
		d.PendingTransitionsStarted, d.TransitionsConfirmed, completion, d.PendingTransitionsCancelledGap,
		d.PendingTransitionsCancelledDiscontinuity, d.PendingTransitionsCancelledSegmentMismatch, d.PendingTransitionsUnresolvedEOF, d.PendingTransitionUnaccounted)
	if len(stats.Diagnostics.RejectReasons) > 0 {
		type pair struct {
			key string
			n   int64
		}
		pairs := make([]pair, 0, len(stats.Diagnostics.RejectReasons))
		for k, n := range stats.Diagnostics.RejectReasons {
			pairs = append(pairs, pair{k, n})
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].n != pairs[j].n {
				return pairs[i].n > pairs[j].n
			}
			return pairs[i].key < pairs[j].key
		})
		fmt.Printf("    rejects:")
		for _, p := range pairs {
			fmt.Printf(" %s=%d", p.key, p.n)
		}
		fmt.Println()
	}
}

type stringListFlag []string

func (s *stringListFlag) String() string {
	return fmt.Sprint([]string(*s))
}

func (s *stringListFlag) Set(v string) error {
	if v == "" {
		return fmt.Errorf("trace-id cannot be empty")
	}
	*s = append(*s, v)
	return nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 5; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func printSpatialDiagnostics(s pipeline.SpatialDiagnostics) {
	t := s.BothMissingSpeedOnlyShortDT50To500M
	if t.Count == 0 {
		return
	}
	fmt.Printf("  spatial_characterization:\n")
	fmt.Printf("    target_population: both_missing + speed_only + dt<5s + 50-500m count=%d\n", t.Count)
	fmt.Printf("      raw_distance_m: p50=%.1f p90=%.1f p99=%.1f\n", t.RawDistanceM.P50, t.RawDistanceM.P90, t.RawDistanceM.P99)
	fmt.Printf("      step_dt_s: p50=%.3f p90=%.3f p99=%.3f\n", t.StepDTS.P50, t.StepDTS.P90, t.StepDTS.P99)
	fmt.Printf("      abs_delta_deg: lat(p50=%.6f p90=%.6f p99=%.6f) lon(p50=%.6f p90=%.6f p99=%.6f)\n",
		t.AbsDeltaLatDeg.P50, t.AbsDeltaLatDeg.P90, t.AbsDeltaLatDeg.P99,
		t.AbsDeltaLonDeg.P50, t.AbsDeltaLonDeg.P90, t.AbsDeltaLonDeg.P99)
	if len(t.Bearing16) > 0 {
		fmt.Printf("      bearing_16:")
		for _, k := range []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"} {
			if n := t.Bearing16[k]; n > 0 {
				fmt.Printf(" %s=%d", k, n)
			}
		}
		fmt.Println()
	}
	printTopCounts("      top_distance_1m:", t.DistanceRounded1M, 10, "m")
	printTopCounts("      top_vector_10m:", t.VectorRounded10M, 10, "")
	printTopCounts("      previous_coordinate_precision:", t.PreviousCoordinateDecimalPlaces, 10, "")
	printTopCounts("      current_coordinate_precision:", t.CurrentCoordinateDecimalPlaces, 10, "")
	printTopCounts("      precision_transitions:", t.PrecisionTransitions, 10, "")
	r := t.Reversals
	if r.ConsecutiveTargetEdgePairs > 0 {
		fmt.Printf("      reversals: consecutive_pairs=%d exact_A_B_A=%d return_lt_5m=%d near_opposite=%d symmetric_distance_5m=%d A_B_A_like=%d precision_A_B_A=%d\n",
			r.ConsecutiveTargetEdgePairs, r.ExactABAReturns, r.ReturnWithin5M, r.NearOppositeBearing, r.NearSymmetricDistance5M, r.ABALikePairs, r.PrecisionABAReturns)
		printTopCounts("      precision_triplets:", r.PrecisionTriplets, 10, "")
	}
}

func printTopCounts(prefix string, counts map[string]int64, limit int, suffix string) {
	if len(counts) == 0 {
		return
	}
	type pair struct {
		key string
		n   int64
	}
	pairs := make([]pair, 0, len(counts))
	for k, n := range counts {
		pairs = append(pairs, pair{k, n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n != pairs[j].n {
			return pairs[i].n > pairs[j].n
		}
		return pairs[i].key < pairs[j].key
	})
	if len(pairs) > limit {
		pairs = pairs[:limit]
	}
	fmt.Print(prefix)
	for _, p := range pairs {
		fmt.Printf(" %s%s=%d", p.key, suffix, p.n)
	}
	fmt.Println()
}

func printMemoryDiagnostics(m pipeline.MemoryDiagnostics) {
	if len(m.ByPhase) == 0 {
		return
	}
	fmt.Printf("    memory_by_phase (sampled every %d entities):\n", m.SampleStrideEntities)
	for _, phase := range []string{"startup", "partition", "writers_open", "read", "sort", "eventize", "validate", "write", "partition_complete", "close"} {
		s, ok := m.ByPhase[phase]
		if !ok || s.Samples == 0 {
			continue
		}
		fmt.Printf("      %-18s samples=%d rss_max=%s heap_alloc_max=%s heap_inuse_max=%s go_sys_max=%s\n",
			phase, s.Samples, humanBytes(s.MaxCurrentRSSBytes), humanBytes(s.MaxGoHeapAllocBytes), humanBytes(s.MaxGoHeapInuseBytes), humanBytes(s.MaxGoSysBytes))
	}
	w := m.WorkingSet
	fmt.Printf("    working_set: max_partition_rows=%d partition_shallow=%s partition_writer_buffers=%s canonical_json_buffer=%s max_entity(obs=%d events=%d segments=%d presence=%d transitions=%d) entity_result_shallow=%s parquet_batch_rows=%d\n",
		w.MaxPartitionRows, humanBytes(w.MaxPartitionObservationShallowBytes), humanBytes(w.PartitionWriterBufferBytes), humanBytes(w.CanonicalJSONBufferBytes),
		w.MaxEntityObservations, w.MaxEntityEvents, w.MaxEntitySegments, w.MaxEntityPresenceIntervals, w.MaxEntityTransitions,
		humanBytes(w.MaxEntityResultShallowBytes), w.ParquetApplicationBatchRows)
	if m.ProfilingEnabled {
		fmt.Printf("    heap_profiles: enabled=true forces_gc=%t snapshots=%d\n", m.ProfilingForcesGC, len(m.HeapProfiles))
		for _, hp := range m.HeapProfiles {
			fmt.Printf("      %s file=%s size=%s heap_alloc=%s rss=%s\n", hp.Label, hp.Path, humanBytes(hp.Bytes), humanBytes(hp.GoHeapAllocBytes), humanBytes(hp.CurrentRSSBytes))
		}
	}
}
