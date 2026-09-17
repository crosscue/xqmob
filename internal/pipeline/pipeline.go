package pipeline

import (
	"bufio"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/eventizer"
	"github.com/crosscue/xqmob/internal/input"
	"github.com/crosscue/xqmob/internal/metrics"
	"github.com/crosscue/xqmob/internal/model"
	"github.com/crosscue/xqmob/internal/output"
	validatepkg "github.com/crosscue/xqmob/internal/validate"
)

type Diagnostics struct {
	RejectReasons map[string]int64           `json:"reject_reasons"`
	Eventizer     model.EventizerDiagnostics `json:"eventizer"`
}

type AccuracyStats struct {
	InputHAPresentRows      int64   `json:"input_ha_present_rows"`
	InputHAMissingRows      int64   `json:"input_ha_missing_rows"`
	AcceptedWithHA          int64   `json:"accepted_with_ha"`
	AcceptedWithoutHA       int64   `json:"accepted_without_ha"`
	NormalizedWithHA        int64   `json:"normalized_with_ha"`
	NormalizedWithoutHA     int64   `json:"normalized_without_ha"`
	NormalizedHACoveragePct float64 `json:"normalized_ha_coverage_pct"`
	EntitiesAllKnown        int64   `json:"entities_all_known"`
	EntitiesMixed           int64   `json:"entities_mixed"`
	EntitiesAllMissing      int64   `json:"entities_all_missing"`
	SegmentsAllKnown        int64   `json:"segments_all_known"`
	SegmentsMixed           int64   `json:"segments_mixed"`
	SegmentsAllMissing      int64   `json:"segments_all_missing"`
	PresenceAllKnown        int64   `json:"presence_all_known"`
	PresenceMixed           int64   `json:"presence_mixed"`
	PresenceAllMissing      int64   `json:"presence_all_missing"`
	TransitionsAllKnown     int64   `json:"transitions_all_known"`
	TransitionsMixed        int64   `json:"transitions_mixed"`
	TransitionsAllMissing   int64   `json:"transitions_all_missing"`
}

type StepDistributions struct {
	RawDistanceM       map[string]metrics.Distribution `json:"raw_distance_m"`
	EffectiveDistanceM map[string]metrics.Distribution `json:"effective_distance_m"`
	ImpliedSpeedMPS    map[string]metrics.Distribution `json:"implied_speed_mps"`
	StepDTS            map[string]metrics.Distribution `json:"step_dt_s"`
}

type TemporalDiagnostics struct {
	EdgeDTBuckets                 map[string]map[string]int64            `json:"edge_dt_buckets"`
	DiscontinuityDTBuckets        map[string]map[string]int64            `json:"discontinuity_dt_buckets"`
	DiscontinuityReasonAccuracyDT map[string]map[string]map[string]int64 `json:"discontinuity_reason_accuracy_dt"`
	SpeedOnlyEffectiveDistanceM   map[string]metrics.Distribution        `json:"speed_only_effective_distance_m"`
	SpeedOnlyDTDistance           map[string]map[string]int64            `json:"speed_only_dt_distance"`
	SpeedOnlyAccuracyDTDistance   map[string]map[string]map[string]int64 `json:"speed_only_accuracy_dt_distance"`
	SpeedOnlyCandidateClasses     map[string]int64                       `json:"speed_only_candidate_classes"`
}

type PathContext struct {
	PathClass      string `json:"path_class"`
	MountPoint     string `json:"mount_point,omitempty"`
	FilesystemType string `json:"filesystem_type,omitempty"`
}

type ExecutionContext struct {
	GOOS      string      `json:"goos"`
	GOARCH    string      `json:"goarch"`
	WSL       bool        `json:"wsl"`
	Input     PathContext `json:"input"`
	Output    PathContext `json:"output"`
	Temporary PathContext `json:"temporary"`
}

type PhaseTimings struct {
	PartitionSeconds   float64 `json:"partition_seconds"`
	ReadSeconds        float64 `json:"partition_read_seconds"`
	SortSeconds        float64 `json:"sort_seconds"`
	EventizeSeconds    float64 `json:"eventize_seconds"`
	ValidateSeconds    float64 `json:"validate_seconds"`
	WriteSeconds       float64 `json:"write_seconds"`
	CloseSeconds       float64 `json:"writer_close_seconds"`
	StorageSeconds     float64 `json:"storage_accounting_seconds"`
	HeapProfileSeconds float64 `json:"heap_profile_seconds,omitempty"`
}

type PerformanceStats struct {
	ElapsedSeconds          float64           `json:"elapsed_seconds"`
	InputRowsPerSecond      float64           `json:"input_rows_per_second"`
	NormalizedRowsPerSecond float64           `json:"normalized_rows_per_second"`
	EventsPerSecond         float64           `json:"events_per_second"`
	PeakRSSBytes            int64             `json:"peak_rss_bytes,omitempty"`
	PeakRSSAvailable        bool              `json:"peak_rss_available"`
	TemporaryPeakBytes      int64             `json:"temporary_peak_bytes"`
	Phase                   PhaseTimings      `json:"phase"`
	Execution               ExecutionContext  `json:"execution_context"`
	Memory                  MemoryDiagnostics `json:"memory"`
}

type distributionCollectors struct {
	raw                           map[string]*metrics.DistributionEstimator
	effective                     map[string]*metrics.DistributionEstimator
	speed                         map[string]*metrics.DistributionEstimator
	dt                            map[string]*metrics.DistributionEstimator
	speedOnlyDistance             map[string]*metrics.DistributionEstimator
	edgeDTBuckets                 map[string]map[string]int64
	discontinuityDTBuckets        map[string]map[string]int64
	discontinuityReasonAccuracyDT map[string]map[string]map[string]int64
	speedOnlyDTDistance           map[string]map[string]int64
	speedOnlyAccuracyDTDistance   map[string]map[string]map[string]int64
	speedOnlyCandidateClasses     map[string]int64
}

func newDistributionCollectors() *distributionCollectors {
	return &distributionCollectors{
		raw:                           map[string]*metrics.DistributionEstimator{},
		effective:                     map[string]*metrics.DistributionEstimator{},
		speed:                         map[string]*metrics.DistributionEstimator{},
		dt:                            map[string]*metrics.DistributionEstimator{},
		speedOnlyDistance:             map[string]*metrics.DistributionEstimator{},
		edgeDTBuckets:                 map[string]map[string]int64{},
		discontinuityDTBuckets:        map[string]map[string]int64{},
		discontinuityReasonAccuracyDT: map[string]map[string]map[string]int64{},
		speedOnlyDTDistance:           map[string]map[string]int64{},
		speedOnlyAccuracyDTDistance:   map[string]map[string]map[string]int64{},
		speedOnlyCandidateClasses:     map[string]int64{},
	}
}

func distributionFor(m map[string]*metrics.DistributionEstimator, state string) *metrics.DistributionEstimator {
	d := m[state]
	if d == nil {
		d = metrics.NewDistributionEstimator()
		m[state] = d
	}
	return d
}

func (d *distributionCollectors) AddObservations(obs []model.Observation, cfg config.Config) {
	for i := 1; i < len(obs); i++ {
		o := obs[i]
		state := o.StepAccuracyState
		if state == "" || state == "initial" {
			continue
		}
		distributionFor(d.raw, state).Add(o.StepDistanceM)
		distributionFor(d.effective, state).Add(o.StepEffectiveDistanceM)
		if o.StepDTS > 0 {
			distributionFor(d.speed, state).Add(o.ImpliedSpeedMPS)
			distributionFor(d.dt, state).Add(o.StepDTS)
		}
		bucket := dtBucket(o.StepDTS)
		addNestedCount(d.edgeDTBuckets, state, bucket)
		if o.IsDiscontinuity {
			addNestedCount(d.discontinuityDTBuckets, state, bucket)
			reason := discontinuityReason(o, cfg)
			addTripleCount(d.discontinuityReasonAccuracyDT, reason, state, bucket)
			if reason == "speed_only" {
				distributionFor(d.speedOnlyDistance, state).Add(o.StepEffectiveDistanceM)
				distanceBucket := effectiveDistanceBucket(o.StepEffectiveDistanceM)
				addNestedCount(d.speedOnlyDTDistance, bucket, distanceBucket)
				addTripleCount(d.speedOnlyAccuracyDTDistance, state, bucket, distanceBucket)
				if bucket == "lt_1s" && distanceBucket == "lt_5m" {
					d.speedOnlyCandidateClasses["micro_displacement"]++
				} else {
					d.speedOnlyCandidateClasses["other_speed_only"]++
				}
			}
		}
	}
}

func dtBucket(dt float64) string {
	switch {
	case dt <= 0:
		return "non_positive"
	case dt < 1:
		return "lt_1s"
	case dt < 5:
		return "1_5s"
	case dt < 30:
		return "5_30s"
	case dt < 60:
		return "30_60s"
	case dt < 300:
		return "1_5m"
	case dt < 1800:
		return "5_30m"
	case dt < 7200:
		return "30m_2h"
	default:
		return "ge_2h"
	}
}

func effectiveDistanceBucket(distanceM float64) string {
	switch {
	case distanceM < 5:
		return "lt_5m"
	case distanceM < 50:
		return "5_50m"
	case distanceM < 500:
		return "50_500m"
	case distanceM < 5000:
		return "500m_5km"
	default:
		return "ge_5km"
	}
}

func discontinuityReason(o model.Observation, cfg config.Config) string {
	jump := o.StepEffectiveDistanceM > cfg.MaxJumpM
	speed := o.StepDTS > 0 && o.ImpliedSpeedMPS > cfg.MaxSpeedMPS
	switch {
	case jump && speed:
		return "jump_and_speed"
	case jump:
		return "jump_only"
	case speed:
		return "speed_only"
	default:
		return "unknown"
	}
}

func addNestedCount(m map[string]map[string]int64, a, b string) {
	if m[a] == nil {
		m[a] = map[string]int64{}
	}
	m[a][b]++
}

func addTripleCount(m map[string]map[string]map[string]int64, a, b, c string) {
	if m[a] == nil {
		m[a] = map[string]map[string]int64{}
	}
	if m[a][b] == nil {
		m[a][b] = map[string]int64{}
	}
	m[a][b][c]++
}

func summarizeDistributions(in map[string]*metrics.DistributionEstimator) map[string]metrics.Distribution {
	out := map[string]metrics.Distribution{}
	for state, d := range in {
		out[state] = d.Summary()
	}
	return out
}

func (d *distributionCollectors) Summary() StepDistributions {
	return StepDistributions{
		RawDistanceM:       summarizeDistributions(d.raw),
		EffectiveDistanceM: summarizeDistributions(d.effective),
		ImpliedSpeedMPS:    summarizeDistributions(d.speed),
		StepDTS:            summarizeDistributions(d.dt),
	}
}

func (d *distributionCollectors) TemporalSummary() TemporalDiagnostics {
	return TemporalDiagnostics{
		EdgeDTBuckets:                 d.edgeDTBuckets,
		DiscontinuityDTBuckets:        d.discontinuityDTBuckets,
		DiscontinuityReasonAccuracyDT: d.discontinuityReasonAccuracyDT,
		SpeedOnlyEffectiveDistanceM:   summarizeDistributions(d.speedOnlyDistance),
		SpeedOnlyDTDistance:           d.speedOnlyDTDistance,
		SpeedOnlyAccuracyDTDistance:   d.speedOnlyAccuracyDTDistance,
		SpeedOnlyCandidateClasses:     d.speedOnlyCandidateClasses,
	}
}

type Stats struct {
	InputRows          int64                              `json:"input_rows"`
	ValidInputRows     int64                              `json:"valid_input_rows"`
	RejectedInputRows  int64                              `json:"rejected_input_rows"`
	NormalizedRows     int64                              `json:"normalized_observations"`
	EntityCount        int64                              `json:"entities"`
	TrackCount         int64                              `json:"tracks"`
	SegmentCount       int64                              `json:"segments"`
	PresenceCount      int64                              `json:"presence_intervals"`
	TransitionCount    int64                              `json:"transitions"`
	EventCount         int64                              `json:"events"`
	GapCount           int64                              `json:"gaps"`
	DiscontinuityCount int64                              `json:"discontinuities"`
	Diagnostics        Diagnostics                        `json:"diagnostics"`
	Accuracy           AccuracyStats                      `json:"accuracy"`
	StepDistributions  StepDistributions                  `json:"step_distributions"`
	Temporal           TemporalDiagnostics                `json:"temporal_diagnostics"`
	Spatial            SpatialDiagnostics                 `json:"spatial_diagnostics"`
	Performance        PerformanceStats                   `json:"performance"`
	Parquet            output.ParquetMaterializationStats `json:"parquet"`
	TraceFiles         []string                           `json:"trace_files,omitempty"`
	Storage            StorageStats                       `json:"-"`
}

type FileStorage struct {
	Path     string `json:"path"`
	Category string `json:"category"`
	Bytes    int64  `json:"bytes"`
}

type StorageStats struct {
	InputBytes                  int64         `json:"input_bytes"`
	CanonicalBytes              int64         `json:"canonical_bytes"`
	AnalysisBytes               int64         `json:"analysis_bytes"`
	RejectsBytes                int64         `json:"rejects_bytes"`
	DiagnosticBytes             int64         `json:"diagnostic_bytes"`
	OutputArtifactBytes         int64         `json:"output_artifact_bytes_excluding_manifest"`
	CanonicalExpansionRatio     float64       `json:"canonical_expansion_ratio"`
	AnalysisExpansionRatio      float64       `json:"analysis_expansion_ratio"`
	TotalArtifactExpansionRatio float64       `json:"total_artifact_expansion_ratio"`
	AnalysisBytesPerObservation float64       `json:"analysis_bytes_per_normalized_observation"`
	Files                       []FileStorage `json:"files"`
}

type Manifest struct {
	Tool             string        `json:"tool"`
	ToolVersion      string        `json:"tool_version"`
	Eventizer        string        `json:"eventizer"`
	EventModelWire   string        `json:"event_model_wire_version"`
	MobilityProfile  string        `json:"mobility_profile"`
	AnalysisContract string        `json:"analysis_contract"`
	CreatedUTC       string        `json:"created_utc"`
	Input            string        `json:"input"`
	Source           string        `json:"source"`
	Config           config.Config `json:"config"`
	Stats            Stats         `json:"stats"`
	Storage          StorageStats  `json:"storage"`
	Canonical        []string      `json:"canonical"`
	Analysis         []string      `json:"analysis"`
	Rejects          string        `json:"rejects"`
	Traces           []string      `json:"traces,omitempty"`
	Notes            []string      `json:"notes"`
}

func Run(inputPath, outDir string, cfg config.Config) (Stats, error) {
	totalStart := time.Now()
	var stats Stats
	distributions := newDistributionCollectors()
	spatial := newTargetPopulationCollector()
	memory := newMemoryTracker(1024)
	var workingSet WorkingSetStats
	workingSet.PartitionWriterBufferBytes = int64(cfg.Partitions) * int64(input.PartitionWriterBufferBytes)
	if cfg.HAPolicy == "allow-missing" {
		cfg.HAPolicy = "preserve"
	}
	if cfg.Source == "" {
		return stats, errors.New("--source is required")
	}
	if err := validateConfig(cfg); err != nil {
		return stats, err
	}
	if err := checkOutputDirectory(outDir, cfg.ReplaceOutput); err != nil {
		return stats, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return stats, err
	}
	profiler, err := newHeapProfiler(cfg.MemoryProfileDir)
	if err != nil {
		return stats, err
	}
	captureHeap := func(label string) error {
		if err := profiler.Capture(label); err != nil {
			return err
		}
		return nil
	}
	tempParent := outDir
	if cfg.TempDir != "" {
		resolvedTempParent, err := filepath.Abs(cfg.TempDir)
		if err != nil {
			return stats, fmt.Errorf("resolve --temp-dir: %w", err)
		}
		tempParent = resolvedTempParent
		if cfg.ReplaceOutput {
			diagnostics, err := filepath.Abs(filepath.Join(outDir, "diagnostics"))
			if err != nil {
				return stats, err
			}
			rel, err := filepath.Rel(diagnostics, tempParent)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
				return stats, errors.New("--temp-dir must be outside the output diagnostics directory when using --replace")
			}
		}
		if err := os.MkdirAll(tempParent, 0o755); err != nil {
			return stats, fmt.Errorf("create --temp-dir: %w", err)
		}
	}
	tempDir, err := os.MkdirTemp(tempParent, ".xqmob-tmp-")
	if err != nil {
		return stats, err
	}
	if !cfg.KeepTemporary {
		defer os.RemoveAll(tempDir)
	}
	stats.Performance.Execution = executionContext(inputPath, outDir, tempDir)
	memory.Sample("startup")
	if err := captureHeap("01-startup"); err != nil {
		return stats, err
	}

	// Stage rejects with the partitions so failed ingestion cannot damage a
	// previous dataset, even when replacement was explicitly requested.
	rejectsPath := filepath.Join(tempDir, "rejects.csv")
	phaseStart := time.Now()
	parts, err := input.PartitionCSV(inputPath, tempDir, rejectsPath, cfg)
	stats.Performance.Phase.PartitionSeconds = time.Since(phaseStart).Seconds()
	memory.Sample("partition")
	if err != nil {
		return stats, err
	}
	if err := captureHeap("02-post-partition"); err != nil {
		return stats, err
	}
	if n, err := directorySize(tempDir); err == nil {
		stats.Performance.TemporaryPeakBytes = n
	}
	stats.InputRows = parts.InputRows
	stats.ValidInputRows = parts.ValidRows
	stats.RejectedInputRows = parts.RejectedRows
	stats.Diagnostics.RejectReasons = parts.RejectReasons
	stats.Accuracy.InputHAPresentRows = parts.InputHAPresentRows
	stats.Accuracy.InputHAMissingRows = parts.InputHAMissingRows
	stats.Accuracy.AcceptedWithHA = parts.AcceptedWithHA
	stats.Accuracy.AcceptedWithoutHA = parts.AcceptedWithoutHA

	if cfg.ReplaceOutput {
		if err := removeKnownArtifacts(outDir); err != nil {
			return stats, err
		}
	}
	if err := copyRejects(rejectsPath, filepath.Join(outDir, "rejects.csv")); err != nil {
		return stats, err
	}
	writers, err := output.New(outDir, cfg)
	if err != nil {
		return stats, err
	}
	memory.Sample("writers_open")
	if err := captureHeap("03-writers-open"); err != nil {
		return stats, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = writers.Close()
		}
	}()

	afterPartition := func(partIdx int) error {
		memory.Sample("partition_complete")
		completed := partIdx + 1
		totalParts := len(parts.Files)
		for _, milestone := range []struct {
			num   int
			label string
		}{
			{1, "04-after-25pct-partitions"},
			{2, "05-after-50pct-partitions"},
			{3, "06-after-75pct-partitions"},
		} {
			target := (totalParts*milestone.num + 3) / 4
			if completed == target {
				if err := captureHeap(milestone.label); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for partIdx, partPath := range parts.Files {
		phaseStart = time.Now()
		rows, err := readPartition(partPath)
		stats.Performance.Phase.ReadSeconds += time.Since(phaseStart).Seconds()
		if err != nil {
			return stats, fmt.Errorf("read partition %s: %w", filepath.Base(partPath), err)
		}
		if len(rows) == 0 {
			if err := afterPartition(partIdx); err != nil {
				return stats, err
			}
			continue
		}
		if int64(len(rows)) > workingSet.MaxPartitionRows {
			workingSet.MaxPartitionRows = int64(len(rows))
			workingSet.MaxPartitionObservationShallowBytes = partitionObservationShallowBytes(len(rows))
		}
		memory.Sample("read")
		phaseStart = time.Now()
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].EntityID != rows[j].EntityID {
				return rows[i].EntityID < rows[j].EntityID
			}
			if !rows[i].TS.Equal(rows[j].TS) {
				return rows[i].TS.Before(rows[j].TS)
			}
			if rows[i].HasHA != rows[j].HasHA {
				return rows[i].HasHA
			}
			if rows[i].HasHA && rows[i].HA != rows[j].HA {
				return rows[i].HA < rows[j].HA
			}
			if rows[i].Lat != rows[j].Lat {
				return rows[i].Lat < rows[j].Lat
			}
			if rows[i].Lon != rows[j].Lon {
				return rows[i].Lon < rows[j].Lon
			}
			return rows[i].SourceRow < rows[j].SourceRow
		})
		stats.Performance.Phase.SortSeconds += time.Since(phaseStart).Seconds()
		memory.Sample("sort")
		for start := 0; start < len(rows); {
			end := start + 1
			for end < len(rows) && rows[end].EntityID == rows[start].EntityID {
				end++
			}
			phaseStart = time.Now()
			result, err := eventizer.Eventize(rows[start:end], cfg)
			stats.Performance.Phase.EventizeSeconds += time.Since(phaseStart).Seconds()
			if err != nil {
				return stats, err
			}
			newWorkingMax := updateWorkingSet(&workingSet, result)
			sampleEntity := stats.EntityCount%memory.stride == 0 || newWorkingMax
			if sampleEntity {
				memory.Sample("eventize")
			}
			phaseStart = time.Now()
			for _, ev := range result.Events {
				if problems := validatepkg.Event(ev); len(problems) > 0 {
					return stats, fmt.Errorf("generated event %s failed internal conformance: %v", ev.ID, problems)
				}
			}
			stats.Performance.Phase.ValidateSeconds += time.Since(phaseStart).Seconds()
			if sampleEntity {
				memory.Sample("validate")
			}
			phaseStart = time.Now()
			if err := writers.WriteResult(result, cfg); err != nil {
				return stats, err
			}
			if len(result.Trace) > 0 {
				tracePath, err := output.WriteTrace(outDir, result.Trace)
				if err != nil {
					return stats, err
				}
				if tracePath != "" {
					stats.TraceFiles = append(stats.TraceFiles, tracePath)
				}
			}
			stats.Performance.Phase.WriteSeconds += time.Since(phaseStart).Seconds()
			if sampleEntity {
				memory.Sample("write")
			}
			distributions.AddObservations(result.Observations, cfg)
			spatial.AddObservations(result.Observations, cfg)
			stats.EntityCount++
			stats.TrackCount++
			stats.NormalizedRows += int64(len(result.Observations))
			stats.Accuracy.NormalizedWithHA += int64(result.Entity.ObservationsWithHA)
			stats.Accuracy.NormalizedWithoutHA += int64(result.Entity.ObservationsWithoutHA)
			addSupportCount(result.Entity.HAClass, &stats.Accuracy.EntitiesAllKnown, &stats.Accuracy.EntitiesMixed, &stats.Accuracy.EntitiesAllMissing)
			for _, s := range result.Segments {
				addSupportCount(s.AccuracySupport, &stats.Accuracy.SegmentsAllKnown, &stats.Accuracy.SegmentsMixed, &stats.Accuracy.SegmentsAllMissing)
			}
			for _, p := range result.Presence {
				addSupportCount(p.AccuracySupport, &stats.Accuracy.PresenceAllKnown, &stats.Accuracy.PresenceMixed, &stats.Accuracy.PresenceAllMissing)
			}
			for _, tr := range result.Transitions {
				addSupportCount(tr.AccuracySupport, &stats.Accuracy.TransitionsAllKnown, &stats.Accuracy.TransitionsMixed, &stats.Accuracy.TransitionsAllMissing)
			}
			stats.SegmentCount += int64(len(result.Segments))
			stats.PresenceCount += int64(len(result.Presence))
			stats.TransitionCount += int64(len(result.Transitions))
			stats.EventCount += int64(len(result.Events))
			stats.GapCount += int64(result.Track.GapCount)
			stats.DiscontinuityCount += int64(result.Track.DiscontinuityCount)
			stats.Diagnostics.Eventizer.Add(result.Diagnostics)
			start = end
		}
		if err := afterPartition(partIdx); err != nil {
			return stats, err
		}
	}
	if err := captureHeap("07-pre-close"); err != nil {
		return stats, err
	}
	phaseStart = time.Now()
	if err := writers.Close(); err != nil {
		return stats, err
	}
	stats.Parquet = writers.ParquetStats(cfg)
	stats.Performance.Phase.CloseSeconds = time.Since(phaseStart).Seconds()
	memory.Sample("close")
	if err := captureHeap("08-post-close"); err != nil {
		return stats, err
	}
	closed = true
	stats.StepDistributions = distributions.Summary()
	stats.Temporal = distributions.TemporalSummary()
	stats.Spatial = spatial.Summary()
	if stats.NormalizedRows > 0 {
		stats.Accuracy.NormalizedHACoveragePct = 100 * float64(stats.Accuracy.NormalizedWithHA) / float64(stats.NormalizedRows)
	}
	phaseStart = time.Now()
	storage, err := collectStorage(outDir, inputPath, cfg, stats)
	stats.Performance.Phase.StorageSeconds = time.Since(phaseStart).Seconds()
	if err != nil {
		return stats, err
	}
	stats.Performance.ElapsedSeconds = time.Since(totalStart).Seconds()
	if stats.Performance.ElapsedSeconds > 0 {
		stats.Performance.InputRowsPerSecond = float64(stats.InputRows) / stats.Performance.ElapsedSeconds
		stats.Performance.NormalizedRowsPerSecond = float64(stats.NormalizedRows) / stats.Performance.ElapsedSeconds
		stats.Performance.EventsPerSecond = float64(stats.EventCount) / stats.Performance.ElapsedSeconds
	}
	stats.Performance.PeakRSSBytes, stats.Performance.PeakRSSAvailable = peakRSSBytes()
	stats.Performance.Memory = memory.Summary(workingSet)
	stats.Performance.Memory.ProfilingEnabled = profiler.enabled
	stats.Performance.Memory.ProfilingForcesGC = profiler.enabled
	stats.Performance.Memory.HeapProfiles = append([]HeapProfileInfo(nil), profiler.profiles...)
	stats.Performance.Phase.HeapProfileSeconds = profiler.seconds
	if err := writeManifest(outDir, inputPath, cfg, stats, storage); err != nil {
		return stats, err
	}
	stats.Storage = storage
	return stats, nil
}

func addSupportCount(class string, allKnown, mixed, allMissing *int64) {
	switch class {
	case "all_known":
		*allKnown = *allKnown + 1
	case "mixed":
		*mixed = *mixed + 1
	case "all_missing":
		*allMissing = *allMissing + 1
	}
}

func readPartition(path string) ([]model.Observation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := gob.NewDecoder(bufio.NewReaderSize(f, 1<<20))
	rows := make([]model.Observation, 0)
	for {
		var o model.Observation
		if err := dec.Decode(&o); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		rows = append(rows, o)
	}
	return rows, nil
}

func validateConfig(c config.Config) error {
	if c.GeohashPrecision < 1 || c.GeohashPrecision > 12 {
		return errors.New("geohash-precision must be between 1 and 12")
	}
	if c.H3Resolution < 0 || c.H3Resolution > 15 {
		return errors.New("h3-resolution must be between 0 and 15")
	}
	if c.MoveRadiusM < 0 || c.DwellThresholdS < 0 || c.GapThresholdS < 0 || c.MaxSpeedMPS < 0 || c.MaxJumpM < 0 || c.ConfirmWindowS < 0 || c.WalkMaxSpeedMPS < 0 || c.WalkMaxJumpM < 0 {
		return errors.New("thresholds must be non-negative")
	}
	if c.Partitions < 1 || c.Partitions > 512 {
		return errors.New("partitions must be between 1 and 512")
	}
	if c.AccuracyPolicy != "subtract_radii" {
		return fmt.Errorf("unsupported accuracy-policy %q", c.AccuracyPolicy)
	}
	if c.HAPolicy != "preserve" && c.HAPolicy != "require" && c.HAPolicy != "allow-missing" {
		return fmt.Errorf("ha-policy must be preserve or require (allow-missing is a compatibility alias), got %q", c.HAPolicy)
	}
	if c.OutputProfile != "standard" && c.OutputProfile != "full" {
		return fmt.Errorf("output-profile must be standard or full, got %q", c.OutputProfile)
	}
	if c.ParquetCompression != "zstd" && c.ParquetCompression != "snappy" && c.ParquetCompression != "none" {
		return fmt.Errorf("parquet-compression must be zstd, snappy, or none, got %q", c.ParquetCompression)
	}
	if c.ParquetRowGroupRows <= 0 {
		return fmt.Errorf("parquet-row-group-rows must be > 0, got %d", c.ParquetRowGroupRows)
	}
	if c.ParquetDictionaryMaxBytes < 0 {
		return fmt.Errorf("parquet dictionary max bytes must be >= 0, got %d", c.ParquetDictionaryMaxBytes)
	}
	return nil
}

func checkOutputDirectory(outDir string, replace bool) error {
	entries, err := os.ReadDir(outDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 && !replace {
		return fmt.Errorf("output directory %q is not empty; choose a new directory or use --replace to replace the existing dataset", outDir)
	}
	return nil
}

// Copy rather than rename because --temp-dir may be on another filesystem.
func copyRejects(source, destination string) (err error) {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open staged rejects: %w", err)
	}
	defer in.Close()
	out, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create rejects: %w", err)
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy rejects: %w", err)
	}
	return nil
}

func removeKnownArtifacts(outDir string) error {
	if err := os.RemoveAll(filepath.Join(outDir, "diagnostics")); err != nil {
		return fmt.Errorf("remove stale diagnostics: %w", err)
	}
	paths := []string{
		"manifest.json",
		"rejects.csv",
		"canonical/events.jsonl",
		"analysis/entities.parquet",
		"analysis/observations.parquet",
		"analysis/events.parquet",
		"analysis/tracks.parquet",
		"analysis/segments.parquet",
		"analysis/presence_intervals.parquet",
		"analysis/transitions.parquet",
	}
	for _, p := range paths {
		err := os.Remove(filepath.Join(outDir, filepath.FromSlash(p)))
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale output %s: %w", p, err)
		}
	}
	return nil
}

func analysisPaths(cfg config.Config) []string {
	paths := []string{
		"analysis/entities.parquet",
		"analysis/observations.parquet",
		"analysis/events.parquet",
		"analysis/segments.parquet",
		"analysis/presence_intervals.parquet",
		"analysis/transitions.parquet",
	}
	if cfg.OutputProfile == "full" {
		paths = append(paths, "analysis/tracks.parquet")
	}
	sort.Strings(paths)
	return paths
}

func collectStorage(outDir, inputPath string, cfg config.Config, stats Stats) (StorageStats, error) {
	var st StorageStats
	info, err := os.Stat(inputPath)
	if err != nil {
		return st, err
	}
	st.InputBytes = info.Size()
	artifacts := []struct{ path, category string }{{"canonical/events.jsonl", "canonical"}, {"rejects.csv", "rejects"}}
	for _, p := range analysisPaths(cfg) {
		artifacts = append(artifacts, struct{ path, category string }{p, "analysis"})
	}
	for _, p := range stats.TraceFiles {
		artifacts = append(artifacts, struct{ path, category string }{p, "diagnostic"})
	}
	for _, a := range artifacts {
		info, err := os.Stat(filepath.Join(outDir, filepath.FromSlash(a.path)))
		if err != nil {
			return st, err
		}
		f := FileStorage{Path: a.path, Category: a.category, Bytes: info.Size()}
		st.Files = append(st.Files, f)
		switch a.category {
		case "canonical":
			st.CanonicalBytes += f.Bytes
		case "analysis":
			st.AnalysisBytes += f.Bytes
		case "rejects":
			st.RejectsBytes += f.Bytes
		case "diagnostic":
			st.DiagnosticBytes += f.Bytes
		}
	}
	st.OutputArtifactBytes = st.CanonicalBytes + st.AnalysisBytes + st.RejectsBytes + st.DiagnosticBytes
	if st.InputBytes > 0 {
		st.CanonicalExpansionRatio = float64(st.CanonicalBytes) / float64(st.InputBytes)
		st.AnalysisExpansionRatio = float64(st.AnalysisBytes) / float64(st.InputBytes)
		st.TotalArtifactExpansionRatio = float64(st.OutputArtifactBytes) / float64(st.InputBytes)
	}
	if stats.NormalizedRows > 0 {
		st.AnalysisBytesPerObservation = float64(st.AnalysisBytes) / float64(stats.NormalizedRows)
	}
	sort.Slice(st.Files, func(i, j int) bool {
		if st.Files[i].Bytes != st.Files[j].Bytes {
			return st.Files[i].Bytes > st.Files[j].Bytes
		}
		return st.Files[i].Path < st.Files[j].Path
	})
	return st, nil
}

func directorySize(root string) (int64, error) {
	var total int64
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func peakRSSBytes() (int64, bool) {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "VmHWM:") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}

func executionContext(inputPath, outDir, tempDir string) ExecutionContext {
	return ExecutionContext{
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		WSL:       isWSL(),
		Input:     pathContext(inputPath),
		Output:    pathContext(outDir),
		Temporary: pathContext(tempDir),
	}
}

func isWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	b, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	v := strings.ToLower(string(b))
	return strings.Contains(v, "microsoft") || strings.Contains(v, "wsl")
}

func pathContext(path string) PathContext {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	pc := PathContext{PathClass: classifyPath(abs)}
	if mp, fsType, ok := mountForPath(abs); ok {
		pc.MountPoint = mp
		pc.FilesystemType = fsType
	}
	return pc
}

func classifyPath(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "linux" {
		parts := strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator))
		if len(parts) >= 2 && parts[0] == "mnt" && len(parts[1]) == 1 {
			c := parts[1][0]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				return "windows_mounted_path"
			}
		}
	}
	return "native_or_other"
}

func mountForPath(path string) (string, string, bool) {
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return "", "", false
	}
	bestMount, bestFS := "", ""
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, " - ")
		if len(parts) != 2 {
			continue
		}
		left := strings.Fields(parts[0])
		right := strings.Fields(parts[1])
		if len(left) < 5 || len(right) < 1 {
			continue
		}
		mp := unescapeMountField(left[4])
		if path == mp || strings.HasPrefix(path, strings.TrimRight(mp, "/")+"/") {
			if len(mp) > len(bestMount) {
				bestMount, bestFS = mp, right[0]
			}
		}
	}
	if bestMount == "" {
		return "", "", false
	}
	return bestMount, bestFS, true
}

func unescapeMountField(s string) string {
	r := strings.NewReplacer("\\040", " ", "\\011", "	", "\\012", "\n", "\\134", "\\")
	return r.Replace(s)
}

func writeManifest(outDir, inputPath string, cfg config.Config, stats Stats, storage StorageStats) error {
	m := Manifest{
		Tool: "xqmob", ToolVersion: config.Version, Eventizer: config.EventizerVersion,
		EventModelWire: config.WireVersion, MobilityProfile: config.ProfileID, AnalysisContract: config.AnalysisContractVersion,
		CreatedUTC: time.Now().UTC().Format(time.RFC3339), Input: filepath.Base(inputPath), Source: cfg.Source, Config: cfg, Stats: stats, Storage: storage,
		Canonical: []string{"canonical/events.jsonl"},
		Analysis:  analysisPaths(cfg),
		Rejects:   "rejects.csv",
		Traces:    append([]string(nil), stats.TraceFiles...),
		Notes: []string{
			"Canonical Core Events are in canonical/events.jsonl; Parquet files are analytical projections.",
			"Spatial Parquet projections use GeoParquet 1.1.0 metadata, WKB geometry, and default OGC:CRS84 longitude/latitude coordinates.",
			"Analytical spatial projections carry both human-readable geohash indexing and an H3 cell index at config.h3_resolution; H3 never drives eventization or changes canonical Core Events.",
			"H3 cells are materialized using pure-Go github.com/dimchansky/h3-go v0.4.0, behaviorally compatible with H3 Core 4.5.0; coarser H3 parents should be derived downstream rather than redundantly stored.",
			"Gap-only edges are excluded from observed path-distance metrics; a GAP does not itself create a segment break.",
			"Parquet projections use explicit page compression; the selected codec is recorded in config.parquet_compression and file metadata.",
			"Parquet row groups and per-column dictionaries are explicitly bounded physical-output controls; limits are recorded in config, stats.parquet, and Parquet key/value metadata and do not alter event or analytical semantics.",
			"stats.parquet.files records output row counts and the actual row-group count reported by parquet-go after writer close.",
			"Storage accounting excludes manifest.json itself and temporary partition files.",
			"Canonical events.jsonl remains uncompressed for direct observability in this release.",
			"Eventizer diagnostics explain decisions; targeted traces are written only for explicit --trace-id values.",
			"Discontinuity reason attribution, time-delta diagnostics, and step-distribution quantiles are descriptive diagnostics and do not alter eventization thresholds or Event Model confidence.",
			"Time-delta buckets are fixed diagnostic bands (<1s, 1-5s, 5-30s, 30-60s, 1-5m, 5-30m, 30m-2h, >=2h) and are not eventization thresholds.",
			"Execution context records WSL detection and input/output/temp mount/filesystem context so benchmark results can distinguish native filesystems from Windows-mounted WSL paths; no performance correction is applied.",
			"--temp-dir selects the parent filesystem for deterministic partition/sort scratch space; the actual generated temporary workspace is described independently in performance.execution_context.temporary.",
			"Step p50/p90/p99 values use constant-memory streaming P2 estimators and are approximate; query observations.parquet for exact distribution analysis.",
			"Performance elapsed/throughput/phase metrics describe this execution; Linux peak RSS uses /proc/self/status VmHWM when available, and temporary_peak_bytes measures the deterministic partition workspace.",
			"Phase memory values are sampled observability metrics; shallow working-set bytes exclude referenced heap data and parquet-go internal buffers and must not be interpreted as total RSS.",
			"--profile-memory DIR writes native Go heap snapshots at fixed run milestones. Profiling forces a GC before each snapshot so those runs are diagnostic and their wall-clock timings are not benchmark-comparable to normal runs.",
			"Precision-transition and A-B-A reversal diagnostics are source-characterization evidence only; their fixed 5m return, 22.5-degree opposite-bearing, and 5m symmetric-distance tests never alter event or segment semantics.",
			"Spatial source-characterization diagnostics describe the both-missing-HA speed-only 0<dt<5s, 50-500m population using fixed bins/modes and never alter DISCONTINUITY or segment semantics.",
			"mobility-reference-v1.2 preserves missing HA under the default ha_policy=preserve; require/max-ha-m are explicit analyst-selected filters; uncertainty subtraction uses only known radii.",
			"xqmob-analysis-v0.2 treats horizontal accuracy as probabilistic evidence and exposes support/coverage metadata without assigning analytical quality labels or enforcing a downstream filtering workflow.",
			"ENTER remains suppressed unless stable destination presence follows a confirmed departure/pending transition.",
		},
	}
	f, err := os.Create(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}
