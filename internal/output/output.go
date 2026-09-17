package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/core"
	"github.com/crosscue/xqmob/internal/geo"
	"github.com/crosscue/xqmob/internal/idgen"
	"github.com/crosscue/xqmob/internal/model"
	parquet "github.com/parquet-go/parquet-go"
)

type ObservationRow struct {
	ObservationID      string   `parquet:"observation_id"`
	EntityID           string   `parquet:"entity_id,dict"`
	Subject            string   `parquet:"subject,dict"`
	TS                 int64    `parquet:"ts,timestamp(millisecond)"`
	Lat                float64  `parquet:"lat"`
	Lon                float64  `parquet:"lon"`
	HA                 *float64 `parquet:"ha_m,optional"`
	HAState            string   `parquet:"ha_state,dict"`
	StepAccuracyState  string   `parquet:"step_accuracy_state,dict"`
	Geohash            string   `parquet:"geohash,dict"`
	H3Cell             string   `parquet:"h3_cell,dict"`
	TrackID            string   `parquet:"track_id,dict"`
	SegmentID          string   `parquet:"segment_id,dict"`
	PresenceIntervalID string   `parquet:"presence_interval_id,dict"`
	SampleCount        int64    `parquet:"sample_count"`
	SameTSScatterM     float64  `parquet:"same_ts_scatter_m"`
	HasStep            bool     `parquet:"has_step"`
	StepDTS            float64  `parquet:"step_dt_s"`
	StepDistanceM      float64  `parquet:"step_distance_m"`
	EffectiveDistanceM float64  `parquet:"effective_distance_m"`
	ImpliedSpeedMPS    float64  `parquet:"implied_speed_mps"`
	IsGap              bool     `parquet:"is_gap"`
	IsDiscontinuity    bool     `parquet:"is_discontinuity"`
	MovementState      string   `parquet:"movement_state,dict"`
	SpeedClass         string   `parquet:"speed_class,dict"`
	SourceRow          int64    `parquet:"source_row"`
	EventizerVersion   string   `parquet:"eventizer_version,dict"`
	Geometry           []byte   `parquet:"geometry"`
}

type EventRow struct {
	EventID              string   `parquet:"event_id"`
	EntityID             string   `parquet:"entity_id,dict"`
	Subject              string   `parquet:"subject,dict"`
	EventType            string   `parquet:"event_type,dict"`
	EventTime            int64    `parquet:"event_time,timestamp(millisecond)"`
	HasEndTime           bool     `parquet:"has_end_time"`
	EndTime              int64    `parquet:"end_time,timestamp(millisecond)"`
	Source               string   `parquet:"source,dict"`
	Modality             string   `parquet:"modality,dict"`
	Class                string   `parquet:"class,dict"`
	Profile              string   `parquet:"profile,dict"`
	Feature              string   `parquet:"feature,dict"`
	Action               string   `parquet:"action,dict"`
	State                string   `parquet:"state,dict"`
	Polarity             int64    `parquet:"polarity"`
	HasMagnitude         bool     `parquet:"has_magnitude"`
	Magnitude            float64  `parquet:"magnitude"`
	Unit                 string   `parquet:"unit,dict"`
	Lat                  float64  `parquet:"lat"`
	Lon                  float64  `parquet:"lon"`
	AccuracyM            *float64 `parquet:"accuracy_m,optional"`
	AccuracyState        string   `parquet:"accuracy_state,dict"`
	AccuracySupport      string   `parquet:"accuracy_support,dict"`
	AccuracyKnownCount   int64    `parquet:"accuracy_known_count"`
	AccuracyMissingCount int64    `parquet:"accuracy_missing_count"`
	H3Cell               string   `parquet:"h3_cell,dict"`
	TrackID              string   `parquet:"track_id,dict"`
	SegmentID            string   `parquet:"segment_id,dict"`
	PresenceIntervalID   string   `parquet:"presence_interval_id,dict"`
	ContextJSON          string   `parquet:"context_json"`
	ParametersJSON       string   `parquet:"parameters_json"`
	EventizerVersion     string   `parquet:"eventizer_version,dict"`
	Geometry             []byte   `parquet:"geometry"`
}

type TrackRow struct {
	TrackID               string   `parquet:"track_id"`
	EntityID              string   `parquet:"entity_id,dict"`
	Subject               string   `parquet:"subject,dict"`
	StartTS               int64    `parquet:"start_ts,timestamp(millisecond)"`
	EndTS                 int64    `parquet:"end_ts,timestamp(millisecond)"`
	CoverageS             float64  `parquet:"coverage_s"`
	ObservationCount      int64    `parquet:"observation_count"`
	EventCount            int64    `parquet:"event_count"`
	SegmentCount          int64    `parquet:"segment_count"`
	PresenceIntervalCount int64    `parquet:"presence_interval_count"`
	TransitionCount       int64    `parquet:"transition_count"`
	GapCount              int64    `parquet:"gap_count"`
	DiscontinuityCount    int64    `parquet:"discontinuity_count"`
	ObservedPathM         float64  `parquet:"observed_path_m"`
	UniqueGeohashCells    int64    `parquet:"unique_geohash_cells"`
	UniqueH3Cells         int64    `parquet:"unique_h3_cells"`
	DwellCount            int64    `parquet:"dwell_count"`
	StayCount             int64    `parquet:"stay_count"`
	TotalDwellS           float64  `parquet:"total_dwell_s"`
	MedianHAM             *float64 `parquet:"median_ha_m,optional"`
	ObservationsWithHA    int64    `parquet:"observations_with_ha"`
	ObservationsWithoutHA int64    `parquet:"observations_without_ha"`
	HACoveragePct         float64  `parquet:"ha_coverage_pct"`
	HAClass               string   `parquet:"ha_class,dict"`
	StartLat              float64  `parquet:"start_lat"`
	StartLon              float64  `parquet:"start_lon"`
	EndLat                float64  `parquet:"end_lat"`
	EndLon                float64  `parquet:"end_lon"`
	StartH3Cell           string   `parquet:"start_h3_cell,dict"`
	EndH3Cell             string   `parquet:"end_h3_cell,dict"`
	PathComponentCount    int64    `parquet:"path_component_count"`
	EventizerVersion      string   `parquet:"eventizer_version,dict"`
	Geometry              []byte   `parquet:"geometry"`
}

type SegmentRow struct {
	SegmentID                string  `parquet:"segment_id"`
	TrackID                  string  `parquet:"track_id,dict"`
	EntityID                 string  `parquet:"entity_id,dict"`
	SegmentIndex             int64   `parquet:"segment_index"`
	StartTS                  int64   `parquet:"start_ts,timestamp(millisecond)"`
	EndTS                    int64   `parquet:"end_ts,timestamp(millisecond)"`
	ObservationCount         int64   `parquet:"observation_count"`
	GapCount                 int64   `parquet:"gap_count"`
	DurationS                float64 `parquet:"duration_s"`
	ObservedPathM            float64 `parquet:"observed_path_m"`
	DisplacementM            float64 `parquet:"displacement_m"`
	MeanObservedSpeedMPS     float64 `parquet:"mean_observed_speed_mps"`
	MaxImpliedSpeedMPS       float64 `parquet:"max_implied_speed_mps"`
	StartLat                 float64 `parquet:"start_lat"`
	StartLon                 float64 `parquet:"start_lon"`
	EndLat                   float64 `parquet:"end_lat"`
	EndLon                   float64 `parquet:"end_lon"`
	StartH3Cell              string  `parquet:"start_h3_cell,dict"`
	EndH3Cell                string  `parquet:"end_h3_cell,dict"`
	MinLat                   float64 `parquet:"min_lat"`
	MinLon                   float64 `parquet:"min_lon"`
	MaxLat                   float64 `parquet:"max_lat"`
	MaxLon                   float64 `parquet:"max_lon"`
	StartReason              string  `parquet:"start_reason,dict"`
	EndReason                string  `parquet:"end_reason,dict"`
	ObservationsWithHA       int64   `parquet:"observations_with_ha"`
	ObservationsWithoutHA    int64   `parquet:"observations_without_ha"`
	HACoveragePct            float64 `parquet:"ha_coverage_pct"`
	AccuracySupport          string  `parquet:"accuracy_support,dict"`
	StartHAState             string  `parquet:"start_ha_state,dict"`
	EndHAState               string  `parquet:"end_ha_state,dict"`
	EdgeBothKnownCount       int64   `parquet:"edge_both_known_count"`
	EdgePreviousMissingCount int64   `parquet:"edge_previous_missing_count"`
	EdgeCurrentMissingCount  int64   `parquet:"edge_current_missing_count"`
	EdgeBothMissingCount     int64   `parquet:"edge_both_missing_count"`
	PathComponentCount       int64   `parquet:"path_component_count"`
	EventizerVersion         string  `parquet:"eventizer_version,dict"`
	Geometry                 []byte  `parquet:"geometry"`
}

type PresenceRow struct {
	PresenceIntervalID    string   `parquet:"presence_interval_id"`
	TrackID               string   `parquet:"track_id,dict"`
	SegmentID             string   `parquet:"segment_id,dict"`
	EntityID              string   `parquet:"entity_id,dict"`
	StartTS               int64    `parquet:"start_ts,timestamp(millisecond)"`
	EndTS                 int64    `parquet:"end_ts,timestamp(millisecond)"`
	DurationS             float64  `parquet:"duration_s"`
	Classification        string   `parquet:"classification,dict"`
	CentroidLat           float64  `parquet:"centroid_lat"`
	CentroidLon           float64  `parquet:"centroid_lon"`
	H3Cell                string   `parquet:"h3_cell,dict"`
	ObservationCount      int64    `parquet:"observation_count"`
	ScatterM              float64  `parquet:"scatter_m"`
	MedianHAM             *float64 `parquet:"median_ha_m,optional"`
	MinHAM                *float64 `parquet:"min_ha_m,optional"`
	MaxHAM                *float64 `parquet:"max_ha_m,optional"`
	ObservationsWithHA    int64    `parquet:"observations_with_ha"`
	ObservationsWithoutHA int64    `parquet:"observations_without_ha"`
	HACoveragePct         float64  `parquet:"ha_coverage_pct"`
	AccuracySupport       string   `parquet:"accuracy_support,dict"`
	StartHAState          string   `parquet:"start_ha_state,dict"`
	EndHAState            string   `parquet:"end_ha_state,dict"`
	StartReason           string   `parquet:"start_reason,dict"`
	EndReason             string   `parquet:"end_reason,dict"`
	IntervalEventID       string   `parquet:"interval_event_id"`
	EnterEventID          string   `parquet:"enter_event_id"`
	LeaveEventID          string   `parquet:"leave_event_id"`
	EventizerVersion      string   `parquet:"eventizer_version,dict"`
	Geometry              []byte   `parquet:"geometry"`
}

type TransitionRow struct {
	TransitionID                     string  `parquet:"transition_id"`
	EntityID                         string  `parquet:"entity_id,dict"`
	TrackID                          string  `parquet:"track_id,dict"`
	SegmentID                        string  `parquet:"segment_id,dict"`
	OriginPresenceID                 string  `parquet:"origin_presence_id"`
	DestinationPresenceID            string  `parquet:"destination_presence_id"`
	DepartTS                         int64   `parquet:"depart_ts,timestamp(millisecond)"`
	ArriveTS                         int64   `parquet:"arrive_ts,timestamp(millisecond)"`
	DurationS                        float64 `parquet:"duration_s"`
	OriginLat                        float64 `parquet:"origin_lat"`
	OriginLon                        float64 `parquet:"origin_lon"`
	DestinationLat                   float64 `parquet:"destination_lat"`
	DestinationLon                   float64 `parquet:"destination_lon"`
	OriginH3Cell                     string  `parquet:"origin_h3_cell,dict"`
	DestinationH3Cell                string  `parquet:"destination_h3_cell,dict"`
	SameH3Cell                       bool    `parquet:"same_h3_cell"`
	StraightLineDistanceM            float64 `parquet:"straight_line_distance_m"`
	ObservedPathDistanceM            float64 `parquet:"observed_path_distance_m"`
	ObservationCount                 int64   `parquet:"observation_count"`
	ObservationsWithHA               int64   `parquet:"observations_with_ha"`
	ObservationsWithoutHA            int64   `parquet:"observations_without_ha"`
	HACoveragePct                    float64 `parquet:"ha_coverage_pct"`
	AccuracySupport                  string  `parquet:"accuracy_support,dict"`
	OriginObservationsWithHA         int64   `parquet:"origin_observations_with_ha"`
	OriginObservationsWithoutHA      int64   `parquet:"origin_observations_without_ha"`
	OriginHACoveragePct              float64 `parquet:"origin_ha_coverage_pct"`
	OriginAccuracySupport            string  `parquet:"origin_accuracy_support,dict"`
	MovementObservationsWithHA       int64   `parquet:"movement_observations_with_ha"`
	MovementObservationsWithoutHA    int64   `parquet:"movement_observations_without_ha"`
	MovementHACoveragePct            float64 `parquet:"movement_ha_coverage_pct"`
	MovementAccuracySupport          string  `parquet:"movement_accuracy_support,dict"`
	DestinationObservationsWithHA    int64   `parquet:"destination_observations_with_ha"`
	DestinationObservationsWithoutHA int64   `parquet:"destination_observations_without_ha"`
	DestinationHACoveragePct         float64 `parquet:"destination_ha_coverage_pct"`
	DestinationAccuracySupport       string  `parquet:"destination_accuracy_support,dict"`
	EventizerVersion                 string  `parquet:"eventizer_version,dict"`
	Geometry                         []byte  `parquet:"geometry"`
}

type EntityRow struct {
	EntityID                          string   `parquet:"entity_id"`
	Subject                           string   `parquet:"subject,dict"`
	FirstSeen                         int64    `parquet:"first_seen,timestamp(millisecond)"`
	LastSeen                          int64    `parquet:"last_seen,timestamp(millisecond)"`
	CoverageS                         float64  `parquet:"coverage_s"`
	ObservationCount                  int64    `parquet:"observation_count"`
	TrackCount                        int64    `parquet:"track_count"`
	SegmentCount                      int64    `parquet:"segment_count"`
	PresenceCount                     int64    `parquet:"presence_count"`
	TransitionCount                   int64    `parquet:"transition_count"`
	StayCount                         int64    `parquet:"stay_count"`
	DwellCount                        int64    `parquet:"dwell_count"`
	GapCount                          int64    `parquet:"gap_count"`
	DiscontinuityCount                int64    `parquet:"discontinuity_count"`
	TotalObservedPathM                float64  `parquet:"total_observed_path_m"`
	TotalPresenceS                    float64  `parquet:"total_presence_s"`
	TotalDwellS                       float64  `parquet:"total_dwell_s"`
	UniquePresenceLocations           int64    `parquet:"unique_presence_locations"`
	UniqueGeohashCells                int64    `parquet:"unique_geohash_cells"`
	UniqueH3Cells                     int64    `parquet:"unique_h3_cells"`
	MedianHAM                         *float64 `parquet:"median_ha_m,optional"`
	ObservationsWithHA                int64    `parquet:"observations_with_ha"`
	ObservationsWithoutHA             int64    `parquet:"observations_without_ha"`
	HACoveragePct                     float64  `parquet:"ha_coverage_pct"`
	HAClass                           string   `parquet:"ha_class,dict"`
	StartLat                          float64  `parquet:"start_lat"`
	StartLon                          float64  `parquet:"start_lon"`
	EndLat                            float64  `parquet:"end_lat"`
	EndLon                            float64  `parquet:"end_lon"`
	StartH3Cell                       string   `parquet:"start_h3_cell,dict"`
	EndH3Cell                         string   `parquet:"end_h3_cell,dict"`
	SameTimestampCollapsed            int64    `parquet:"same_timestamp_collapsed"`
	PresenceEndGap                    int64    `parquet:"presence_end_gap"`
	PresenceEndDiscontinuity          int64    `parquet:"presence_end_discontinuity"`
	PresenceEndMovement               int64    `parquet:"presence_end_confirmed_movement"`
	DepartureCandidateSequences       int64    `parquet:"departure_candidate_sequences"`
	DepartureCandidateWindowExpired   int64    `parquet:"departure_candidate_window_expired"`
	DepartureCandidatesCancelledGap   int64    `parquet:"departure_candidates_cancelled_gap"`
	DepartureCandidatesCancelledDisc  int64    `parquet:"departure_candidates_cancelled_discontinuity"`
	DepartureCandidatesCancelledEOF   int64    `parquet:"departure_candidates_cancelled_eof"`
	DepartureCandidateUnaccounted     int64    `parquet:"departure_candidate_unaccounted"`
	EnterSuppressedNoPending          int64    `parquet:"enter_suppressed_no_pending_transition"`
	DeparturesConfirmed               int64    `parquet:"departures_confirmed"`
	PendingTransitionsStarted         int64    `parquet:"pending_transitions_started"`
	TransitionsConfirmed              int64    `parquet:"transitions_confirmed"`
	TransitionsCancelledGap           int64    `parquet:"transitions_cancelled_gap"`
	TransitionsCancelledDiscontinuity int64    `parquet:"transitions_cancelled_discontinuity"`
	TransitionsUnresolvedEOF          int64    `parquet:"transitions_unresolved_eof"`
	PendingTransitionUnaccounted      int64    `parquet:"pending_transition_unaccounted"`
	EventizerVersion                  string   `parquet:"eventizer_version,dict"`
}

type ParquetFileStats struct {
	Rows      int64 `json:"rows"`
	RowGroups int64 `json:"row_groups"`
}

type ParquetMaterializationStats struct {
	RowGroupMaxRows    int64                       `json:"row_group_max_rows"`
	DictionaryMaxBytes int64                       `json:"dictionary_max_bytes"`
	Files              map[string]ParquetFileStats `json:"files"`
}

type parquetSink[T any] struct {
	file             *os.File
	writer           *parquet.GenericWriter[T]
	buf              []T
	rowsWritten      int64
	rowGroupsWritten int64
}

// ParquetBatchRows is the application-level row batch passed to parquet-go.
// parquet-go may maintain additional internal buffers; those are reflected in
// process/Go-heap memory diagnostics rather than this count.
const ParquetBatchRows = 1024

// CanonicalJSONBufferBytes is the bufio buffer used for canonical JSONL.
const CanonicalJSONBufferBytes = 1 << 20

func newParquetSink[T any](path string, geometryTypes []string, cfg config.Config) (*parquetSink[T], error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	options := []parquet.WriterOption{
		parquet.MaxRowsPerRowGroup(cfg.ParquetRowGroupRows),
		parquet.DictionaryMaxBytes(cfg.ParquetDictionaryMaxBytes),
	}
	switch cfg.ParquetCompression {
	case "zstd":
		options = append(options, parquet.Compression(&parquet.Zstd))
	case "snappy":
		options = append(options, parquet.Compression(&parquet.Snappy))
	case "none":
		options = append(options, parquet.Compression(&parquet.Uncompressed))
	default:
		_ = f.Close()
		return nil, fmt.Errorf("unsupported parquet compression %q", cfg.ParquetCompression)
	}
	w := parquet.NewGenericWriter[T](f, options...)
	w.SetKeyValueMetadata("xq:eventizer", config.EventizerVersion)
	w.SetKeyValueMetadata("xq:analysis_contract", config.AnalysisContractVersion)
	w.SetKeyValueMetadata("xq:compression", cfg.ParquetCompression)
	w.SetKeyValueMetadata("xq:parquet_row_group_max_rows", fmt.Sprintf("%d", cfg.ParquetRowGroupRows))
	w.SetKeyValueMetadata("xq:parquet_dictionary_max_bytes", fmt.Sprintf("%d", cfg.ParquetDictionaryMaxBytes))
	w.SetKeyValueMetadata("xq:h3_resolution", fmt.Sprintf("%d", cfg.H3Resolution))
	w.SetKeyValueMetadata("xq:h3_library", config.H3Library)
	w.SetKeyValueMetadata("xq:h3_library_version", config.H3LibraryVersion)
	w.SetKeyValueMetadata("xq:h3_core_version", config.H3CoreVersion)
	if len(geometryTypes) > 0 {
		meta := map[string]any{
			"version":        "1.1.0",
			"primary_column": "geometry",
			"columns": map[string]any{
				"geometry": map[string]any{
					"encoding":       "WKB",
					"geometry_types": geometryTypes,
				},
			},
		}
		b, _ := json.Marshal(meta)
		w.SetKeyValueMetadata("geo", string(b))
	}
	return &parquetSink[T]{file: f, writer: w, buf: make([]T, 0, ParquetBatchRows)}, nil
}

func (s *parquetSink[T]) Write(row T) error {
	s.buf = append(s.buf, row)
	if len(s.buf) >= cap(s.buf) {
		return s.Flush()
	}
	return nil
}

func (s *parquetSink[T]) Flush() error {
	if len(s.buf) == 0 {
		return nil
	}
	n, err := s.writer.Write(s.buf)
	if err != nil {
		return err
	}
	s.rowsWritten += int64(n)
	s.buf = s.buf[:0]
	return nil
}

func (s *parquetSink[T]) Close() error {
	if s == nil || (s.writer == nil && s.file == nil) {
		return nil
	}
	var first error
	if s.writer != nil {
		if err := s.Flush(); err != nil && first == nil {
			first = err
		}
		if err := s.writer.Close(); err != nil && first == nil {
			first = err
		}
		if view := s.writer.File(); view != nil {
			s.rowGroupsWritten = int64(len(view.RowGroups()))
		}
	}
	if s.file != nil {
		if err := s.file.Close(); err != nil && first == nil {
			first = err
		}
	}
	// Release potentially large parquet-go writer/dictionary structures as soon
	// as this sink is closed instead of retaining them until pipeline.Run returns.
	s.writer = nil
	s.file = nil
	s.buf = nil
	return first
}

type Writers struct {
	jsonFile *os.File
	jsonBuf  *bufio.Writer
	jsonEnc  *json.Encoder

	observations   *parquetSink[ObservationRow]
	events         *parquetSink[EventRow]
	tracks         *parquetSink[TrackRow]
	segments       *parquetSink[SegmentRow]
	presence       *parquetSink[PresenceRow]
	transitions    *parquetSink[TransitionRow]
	entities       *parquetSink[EntityRow]
	parquetStats   ParquetMaterializationStats
	statsFinalized bool
}

func New(root string, cfg config.Config) (*Writers, error) {
	canonical := filepath.Join(root, "canonical")
	analysis := filepath.Join(root, "analysis")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(analysis, 0o755); err != nil {
		return nil, err
	}
	jf, err := os.Create(filepath.Join(canonical, "events.jsonl"))
	if err != nil {
		return nil, err
	}
	w := &Writers{
		jsonFile: jf,
		jsonBuf:  bufio.NewWriterSize(jf, CanonicalJSONBufferBytes),
		parquetStats: ParquetMaterializationStats{
			RowGroupMaxRows:    cfg.ParquetRowGroupRows,
			DictionaryMaxBytes: cfg.ParquetDictionaryMaxBytes,
			Files:              map[string]ParquetFileStats{},
		},
	}
	w.jsonEnc = json.NewEncoder(w.jsonBuf)
	if w.observations, err = newParquetSink[ObservationRow](filepath.Join(analysis, "observations.parquet"), []string{"Point"}, cfg); err != nil {
		w.Close()
		return nil, err
	}
	if w.events, err = newParquetSink[EventRow](filepath.Join(analysis, "events.parquet"), []string{"Point"}, cfg); err != nil {
		w.Close()
		return nil, err
	}
	if cfg.OutputProfile == "full" {
		if w.tracks, err = newParquetSink[TrackRow](filepath.Join(analysis, "tracks.parquet"), []string{"MultiLineString", "LineString", "Point"}, cfg); err != nil {
			w.Close()
			return nil, err
		}
	}
	if w.segments, err = newParquetSink[SegmentRow](filepath.Join(analysis, "segments.parquet"), []string{"MultiLineString", "LineString", "Point"}, cfg); err != nil {
		w.Close()
		return nil, err
	}
	if w.presence, err = newParquetSink[PresenceRow](filepath.Join(analysis, "presence_intervals.parquet"), []string{"Point"}, cfg); err != nil {
		w.Close()
		return nil, err
	}
	if w.transitions, err = newParquetSink[TransitionRow](filepath.Join(analysis, "transitions.parquet"), []string{"LineString"}, cfg); err != nil {
		w.Close()
		return nil, err
	}
	if w.entities, err = newParquetSink[EntityRow](filepath.Join(analysis, "entities.parquet"), nil, cfg); err != nil {
		w.Close()
		return nil, err
	}
	return w, nil
}

func sinkStats[T any](s *parquetSink[T]) ParquetFileStats {
	if s == nil {
		return ParquetFileStats{}
	}
	return ParquetFileStats{Rows: s.rowsWritten + int64(len(s.buf)), RowGroups: s.rowGroupsWritten}
}

func (w *Writers) ParquetStats(cfg config.Config) ParquetMaterializationStats {
	if w != nil && w.statsFinalized {
		return w.parquetStats
	}
	files := map[string]ParquetFileStats{}
	add := func(path string, st ParquetFileStats) {
		if st.Rows > 0 || st.RowGroups > 0 {
			files[path] = st
		}
	}
	add("analysis/observations.parquet", sinkStats(w.observations))
	add("analysis/events.parquet", sinkStats(w.events))
	if w.tracks != nil {
		add("analysis/tracks.parquet", sinkStats(w.tracks))
	}
	add("analysis/segments.parquet", sinkStats(w.segments))
	add("analysis/presence_intervals.parquet", sinkStats(w.presence))
	add("analysis/transitions.parquet", sinkStats(w.transitions))
	add("analysis/entities.parquet", sinkStats(w.entities))
	return ParquetMaterializationStats{
		RowGroupMaxRows:    cfg.ParquetRowGroupRows,
		DictionaryMaxBytes: cfg.ParquetDictionaryMaxBytes,
		Files:              files,
	}
}

type h3Indexer struct {
	resolution int
	cache      map[[2]uint64]string
}

func newH3Indexer(resolution int, capacity int) *h3Indexer {
	return &h3Indexer{resolution: resolution, cache: make(map[[2]uint64]string, capacity)}
}

func (h *h3Indexer) Cell(lat, lon float64) (string, error) {
	key := [2]uint64{math.Float64bits(lat), math.Float64bits(lon)}
	if cell, ok := h.cache[key]; ok {
		return cell, nil
	}
	cell, err := geo.H3Cell(lat, lon, h.resolution)
	if err != nil {
		return "", err
	}
	h.cache[key] = cell
	return cell, nil
}

func (w *Writers) WriteResult(r model.EntityResult, cfg config.Config) error {
	h3Index := newH3Indexer(cfg.H3Resolution, len(r.Observations)+len(r.Presence)+8)
	h3Cells := make([]string, len(r.Observations))
	uniqueH3 := make(map[string]struct{})
	for i, o := range r.Observations {
		cell, err := h3Index.Cell(o.Lat, o.Lon)
		if err != nil {
			return fmt.Errorf("H3 observation cell for entity %q: %w", o.EntityID, err)
		}
		h3Cells[i] = cell
		uniqueH3[cell] = struct{}{}
		row := ObservationRow{
			ObservationID: idgen.Stable("obs", r.Track.ID, core.RFC3339(o.TS), fmt.Sprintf("%.9f", o.Lat), fmt.Sprintf("%.9f", o.Lon), observationHAIDPart(o)), EntityID: o.EntityID,
			Subject: r.Track.Subject, TS: millis(o.TS), Lat: o.Lat, Lon: o.Lon, HA: nullableHA(o), HAState: o.AccuracyState, StepAccuracyState: o.StepAccuracyState,
			Geohash: o.Geohash, H3Cell: h3Cells[i], TrackID: o.TrackID, SegmentID: o.SegmentID, PresenceIntervalID: o.PresenceIntervalID,
			SampleCount: int64(o.SampleCount), SameTSScatterM: o.SameTSScatterM, HasStep: i > 0,
			StepDTS: o.StepDTS, StepDistanceM: o.StepDistanceM, EffectiveDistanceM: o.StepEffectiveDistanceM,
			ImpliedSpeedMPS: o.ImpliedSpeedMPS, IsGap: o.IsGap, IsDiscontinuity: o.IsDiscontinuity,
			MovementState: o.MovementState, SpeedClass: o.SpeedClass, SourceRow: o.SourceRow,
			EventizerVersion: config.EventizerVersion, Geometry: geo.WKBPoint(o.Lon, o.Lat),
		}
		if err := w.observations.Write(row); err != nil {
			return err
		}
	}
	for _, e := range r.Events {
		if err := w.jsonEnc.Encode(e); err != nil {
			return err
		}
		row, err := eventRow(e, r.Entity.EntityID, h3Index)
		if err != nil {
			return err
		}
		if err := w.events.Write(row); err != nil {
			return err
		}
	}
	tr := r.Track
	startH3, err := h3Index.Cell(tr.StartLat, tr.StartLon)
	if err != nil {
		return fmt.Errorf("H3 track start cell for entity %q: %w", tr.EntityID, err)
	}
	endH3, err := h3Index.Cell(tr.EndLat, tr.EndLon)
	if err != nil {
		return fmt.Errorf("H3 track end cell for entity %q: %w", tr.EntityID, err)
	}
	if w.tracks != nil {
		if err := w.tracks.Write(TrackRow{
			TrackID: tr.ID, EntityID: tr.EntityID, Subject: tr.Subject, StartTS: millis(tr.Start), EndTS: millis(tr.End),
			CoverageS: tr.CoverageS, ObservationCount: int64(tr.ObservationCount), EventCount: int64(tr.EventCount), SegmentCount: int64(tr.SegmentCount),
			PresenceIntervalCount: int64(tr.PresenceIntervalCount), TransitionCount: int64(tr.TransitionCount), GapCount: int64(tr.GapCount),
			DiscontinuityCount: int64(tr.DiscontinuityCount), ObservedPathM: tr.ObservedPathM, UniqueGeohashCells: int64(tr.UniqueGeohashCells), UniqueH3Cells: int64(len(uniqueH3)),
			DwellCount: int64(tr.DwellCount), StayCount: int64(tr.StayCount), TotalDwellS: tr.TotalDwellS, MedianHAM: nullableMedianHA(tr.MedianHAM, tr.HasMedianHA),
			ObservationsWithHA: int64(tr.ObservationsWithHA), ObservationsWithoutHA: int64(tr.ObservationsWithoutHA), HACoveragePct: tr.HACoveragePct, HAClass: tr.HAClass,
			StartLat: tr.StartLat, StartLon: tr.StartLon, EndLat: tr.EndLat, EndLon: tr.EndLon, StartH3Cell: startH3, EndH3Cell: endH3,
			PathComponentCount: int64(len(tr.PathRuns)), EventizerVersion: config.EventizerVersion, Geometry: geo.WKBPath(tr.PathRuns),
		}); err != nil {
			return err
		}
	}
	for _, s := range r.Segments {
		segStartH3, err := h3Index.Cell(s.StartLat, s.StartLon)
		if err != nil {
			return fmt.Errorf("H3 segment start cell %s: %w", s.ID, err)
		}
		segEndH3, err := h3Index.Cell(s.EndLat, s.EndLon)
		if err != nil {
			return fmt.Errorf("H3 segment end cell %s: %w", s.ID, err)
		}
		if err := w.segments.Write(SegmentRow{
			SegmentID: s.ID, TrackID: s.TrackID, EntityID: s.EntityID, SegmentIndex: int64(s.Index), StartTS: millis(s.Start), EndTS: millis(s.End),
			ObservationCount: int64(s.ObservationCount), GapCount: int64(s.GapCount), DurationS: s.DurationS, ObservedPathM: s.ObservedPathM,
			DisplacementM: s.DisplacementM, MeanObservedSpeedMPS: s.MeanObservedSpeedMPS, MaxImpliedSpeedMPS: s.MaxImpliedSpeedMPS,
			StartLat: s.StartLat, StartLon: s.StartLon, EndLat: s.EndLat, EndLon: s.EndLon, StartH3Cell: segStartH3, EndH3Cell: segEndH3, MinLat: s.MinLat, MinLon: s.MinLon, MaxLat: s.MaxLat, MaxLon: s.MaxLon,
			StartReason: s.StartReason, EndReason: s.EndReason, ObservationsWithHA: int64(s.ObservationsWithHA), ObservationsWithoutHA: int64(s.ObservationsWithoutHA), HACoveragePct: s.HACoveragePct, AccuracySupport: s.AccuracySupport,
			StartHAState: s.StartHAState, EndHAState: s.EndHAState, EdgeBothKnownCount: int64(s.EdgeBothKnownCount), EdgePreviousMissingCount: int64(s.EdgePreviousMissingCount), EdgeCurrentMissingCount: int64(s.EdgeCurrentMissingCount), EdgeBothMissingCount: int64(s.EdgeBothMissingCount),
			PathComponentCount: int64(len(s.PathRuns)), EventizerVersion: config.EventizerVersion, Geometry: geo.WKBPath(s.PathRuns),
		}); err != nil {
			return err
		}
	}
	for _, p := range r.Presence {
		presH3, err := h3Index.Cell(p.CentroidLat, p.CentroidLon)
		if err != nil {
			return fmt.Errorf("H3 presence cell %s: %w", p.ID, err)
		}
		if err := w.presence.Write(PresenceRow{
			PresenceIntervalID: p.ID, TrackID: p.TrackID, SegmentID: p.SegmentID, EntityID: p.EntityID,
			StartTS: millis(p.Start), EndTS: millis(p.End), DurationS: p.DurationS, Classification: p.Classification,
			CentroidLat: p.CentroidLat, CentroidLon: p.CentroidLon, H3Cell: presH3, ObservationCount: int64(p.ObservationCount), ScatterM: p.ScatterM,
			MedianHAM: nullableMedianHA(p.MedianHAM, p.HasHAStats), MinHAM: nullableMedianHA(p.MinHAM, p.HasHAStats), MaxHAM: nullableMedianHA(p.MaxHAM, p.HasHAStats),
			ObservationsWithHA: int64(p.ObservationsWithHA), ObservationsWithoutHA: int64(p.ObservationsWithoutHA), HACoveragePct: p.HACoveragePct, AccuracySupport: p.AccuracySupport, StartHAState: p.StartHAState, EndHAState: p.EndHAState, StartReason: p.StartReason, EndReason: p.EndReason,
			IntervalEventID: p.IntervalEventID, EnterEventID: p.EnterEventID, LeaveEventID: p.LeaveEventID,
			EventizerVersion: config.EventizerVersion, Geometry: geo.WKBPoint(p.CentroidLon, p.CentroidLat),
		}); err != nil {
			return err
		}
	}
	for _, t := range r.Transitions {
		originH3, err := h3Index.Cell(t.OriginLat, t.OriginLon)
		if err != nil {
			return fmt.Errorf("H3 transition origin cell %s: %w", t.ID, err)
		}
		destH3, err := h3Index.Cell(t.DestinationLat, t.DestinationLon)
		if err != nil {
			return fmt.Errorf("H3 transition destination cell %s: %w", t.ID, err)
		}
		coords := [][2]float64{{t.OriginLon, t.OriginLat}, {t.DestinationLon, t.DestinationLat}}
		if err := w.transitions.Write(TransitionRow{
			TransitionID: t.ID, EntityID: t.EntityID, TrackID: t.TrackID, SegmentID: t.SegmentID,
			OriginPresenceID: t.OriginPresenceID, DestinationPresenceID: t.DestinationPresenceID,
			DepartTS: millis(t.Depart), ArriveTS: millis(t.Arrive), DurationS: t.DurationS,
			OriginLat: t.OriginLat, OriginLon: t.OriginLon, DestinationLat: t.DestinationLat, DestinationLon: t.DestinationLon, OriginH3Cell: originH3, DestinationH3Cell: destH3, SameH3Cell: originH3 == destH3,
			StraightLineDistanceM: t.StraightLineDistanceM, ObservedPathDistanceM: t.ObservedPathDistanceM,
			ObservationCount: int64(t.ObservationCount), ObservationsWithHA: int64(t.ObservationsWithHA), ObservationsWithoutHA: int64(t.ObservationsWithoutHA), HACoveragePct: t.HACoveragePct, AccuracySupport: t.AccuracySupport,
			OriginObservationsWithHA: int64(t.OriginObservationsWithHA), OriginObservationsWithoutHA: int64(t.OriginObservationsWithoutHA), OriginHACoveragePct: t.OriginHACoveragePct, OriginAccuracySupport: t.OriginAccuracySupport,
			MovementObservationsWithHA: int64(t.MovementObservationsWithHA), MovementObservationsWithoutHA: int64(t.MovementObservationsWithoutHA), MovementHACoveragePct: t.MovementHACoveragePct, MovementAccuracySupport: t.MovementAccuracySupport,
			DestinationObservationsWithHA: int64(t.DestinationObservationsWithHA), DestinationObservationsWithoutHA: int64(t.DestinationObservationsWithoutHA), DestinationHACoveragePct: t.DestinationHACoveragePct, DestinationAccuracySupport: t.DestinationAccuracySupport,
			EventizerVersion: config.EventizerVersion, Geometry: geo.WKBLineString(coords),
		}); err != nil {
			return err
		}
	}
	e := r.Entity
	return w.entities.Write(EntityRow{
		EntityID: e.EntityID, Subject: e.Subject, FirstSeen: millis(e.FirstSeen), LastSeen: millis(e.LastSeen), CoverageS: e.CoverageS,
		ObservationCount: int64(e.ObservationCount), TrackCount: int64(e.TrackCount), SegmentCount: int64(e.SegmentCount), PresenceCount: int64(e.PresenceCount),
		TransitionCount: int64(e.TransitionCount), StayCount: int64(e.StayCount), DwellCount: int64(e.DwellCount), GapCount: int64(e.GapCount),
		DiscontinuityCount: int64(e.DiscontinuityCount), TotalObservedPathM: e.TotalObservedPathM, TotalPresenceS: e.TotalPresenceS,
		TotalDwellS: e.TotalDwellS, UniquePresenceLocations: int64(e.UniquePresenceLocations), UniqueGeohashCells: int64(e.UniqueGeohashCells), UniqueH3Cells: int64(len(uniqueH3)), MedianHAM: nullableMedianHA(e.MedianHAM, e.HasMedianHA),
		ObservationsWithHA: int64(e.ObservationsWithHA), ObservationsWithoutHA: int64(e.ObservationsWithoutHA), HACoveragePct: e.HACoveragePct, HAClass: e.HAClass,
		StartLat: e.StartLat, StartLon: e.StartLon, EndLat: e.EndLat, EndLon: e.EndLon, StartH3Cell: startH3, EndH3Cell: endH3,
		SameTimestampCollapsed: int64(r.Diagnostics.SameTimestampCollapsed),
		PresenceEndGap:         int64(r.Diagnostics.PresenceEndGap), PresenceEndDiscontinuity: int64(r.Diagnostics.PresenceEndDiscontinuity),
		PresenceEndMovement: int64(r.Diagnostics.PresenceEndConfirmedMovement), DepartureCandidateSequences: int64(r.Diagnostics.DepartureCandidateSequences),
		DepartureCandidateWindowExpired: int64(r.Diagnostics.DepartureCandidateWindowExpired),
		DepartureCandidatesCancelledGap: int64(r.Diagnostics.DepartureCandidatesCancelledGap), DepartureCandidatesCancelledDisc: int64(r.Diagnostics.DepartureCandidatesCancelledDiscontinuity),
		DepartureCandidatesCancelledEOF: int64(r.Diagnostics.DepartureCandidatesCancelledEOF), DepartureCandidateUnaccounted: int64(r.Diagnostics.DepartureCandidateUnaccounted),
		EnterSuppressedNoPending:  int64(r.Diagnostics.EnterSuppressedNoPendingTransition),
		DeparturesConfirmed:       int64(r.Diagnostics.DeparturesConfirmed),
		PendingTransitionsStarted: int64(r.Diagnostics.PendingTransitionsStarted), TransitionsConfirmed: int64(r.Diagnostics.TransitionsConfirmed),
		TransitionsCancelledGap: int64(r.Diagnostics.PendingTransitionsCancelledGap), TransitionsCancelledDiscontinuity: int64(r.Diagnostics.PendingTransitionsCancelledDiscontinuity),
		TransitionsUnresolvedEOF: int64(r.Diagnostics.PendingTransitionsUnresolvedEOF), PendingTransitionUnaccounted: int64(r.Diagnostics.PendingTransitionUnaccounted), EventizerVersion: config.EventizerVersion,
	})
}

func nullableHA(o model.Observation) *float64 {
	if !o.HasHA {
		return nil
	}
	v := o.HA
	return &v
}

func nullableMedianHA(v float64, ok bool) *float64 {
	if !ok {
		return nil
	}
	x := v
	return &x
}

func observationHAIDPart(o model.Observation) string {
	if !o.HasHA {
		return "missing"
	}
	return fmt.Sprintf("%.3f", o.HA)
}

func eventAccuracySupport(e core.Event) (string, int64, int64) {
	if with, ok := contextInt64(e.Context, "observations_with_ha"); ok {
		without, _ := contextInt64(e.Context, "observations_without_ha")
		return model.AccuracySupportClass(int(with), int(without)), with, without
	}
	if state, ok := e.Context["edge_accuracy_state"].(string); ok {
		switch state {
		case "both_known":
			return "all_known", 2, 0
		case "both_missing":
			return "all_missing", 0, 2
		case "previous_missing", "current_missing":
			return "mixed", 1, 1
		}
	}
	if state, ok := e.Context["accuracy_state"].(string); ok {
		if state == "known" {
			return "all_known", 1, 0
		}
		if state == "missing" {
			return "all_missing", 0, 1
		}
	}
	if e.Location != nil && e.Location.AccuracyM != nil {
		return "all_known", 1, 0
	}
	return "none", 0, 0
}

func contextInt64(ctx map[string]any, key string) (int64, bool) {
	v, ok := ctx[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

func eventRow(e core.Event, entityID string, h3Index *h3Indexer) (EventRow, error) {
	t, err := time.Parse(time.RFC3339Nano, e.EventTime)
	if err != nil {
		return EventRow{}, err
	}
	r := EventRow{EventID: e.ID, EntityID: entityID, Subject: e.Subject, EventTime: millis(t), Source: e.Source, Modality: e.Modality,
		Class: e.Class, Profile: e.Profile, Feature: e.Feature, Action: e.Action, State: e.State, Polarity: int64(e.Polarity), Unit: e.Unit,
		EventizerVersion: config.EventizerVersion}
	if e.EndTime != "" {
		et, err := time.Parse(time.RFC3339Nano, e.EndTime)
		if err != nil {
			return EventRow{}, err
		}
		r.HasEndTime, r.EndTime = true, millis(et)
	}
	if e.Magnitude != nil {
		r.HasMagnitude, r.Magnitude = true, *e.Magnitude
	}
	if e.Location != nil {
		r.Lat, r.Lon = e.Location.Lat, e.Location.Lon
		if e.Location.AccuracyM != nil {
			v := *e.Location.AccuracyM
			r.AccuracyM = &v
			r.AccuracyState = "known"
		} else if v, ok := e.Context["accuracy_state"].(string); ok {
			r.AccuracyState = v
		} else {
			r.AccuracyState = "not_asserted"
		}
		r.Geometry = geo.WKBPoint(e.Location.Lon, e.Location.Lat)
		cell, err := h3Index.Cell(e.Location.Lat, e.Location.Lon)
		if err != nil {
			return EventRow{}, fmt.Errorf("H3 event cell for %s: %w", e.ID, err)
		}
		r.H3Cell = cell
	}
	if v, ok := e.Context["source_event"].(string); ok {
		r.EventType = v
	}
	if v, ok := e.Context["track_id"].(string); ok {
		r.TrackID = v
	}
	if v, ok := e.Context["segment_id"].(string); ok {
		r.SegmentID = v
	}
	if v, ok := e.Context["presence_interval_id"].(string); ok {
		r.PresenceIntervalID = v
	}
	r.AccuracySupport, r.AccuracyKnownCount, r.AccuracyMissingCount = eventAccuracySupport(e)
	if b, err := json.Marshal(e.Context); err == nil {
		r.ContextJSON = string(b)
	}
	if e.Provenance != nil {
		if b, err := json.Marshal(e.Provenance.Parameters); err == nil {
			r.ParametersJSON = string(b)
		}
	}
	return r, nil
}

func millis(t time.Time) int64 { return t.UTC().UnixMilli() }

func (w *Writers) Close() error {
	if w == nil {
		return nil
	}
	var first error
	closeSink := func(path string, closeFn func() error, statsFn func() ParquetFileStats) {
		if closeFn == nil {
			return
		}
		if err := closeFn(); err != nil && first == nil {
			first = err
		}
		if statsFn != nil {
			st := statsFn()
			if st.Rows > 0 || st.RowGroups > 0 {
				w.parquetStats.Files[path] = st
			}
		}
	}
	if w.observations != nil {
		s := w.observations
		closeSink("analysis/observations.parquet", s.Close, func() ParquetFileStats { return sinkStats(s) })
		w.observations = nil
	}
	if w.events != nil {
		s := w.events
		closeSink("analysis/events.parquet", s.Close, func() ParquetFileStats { return sinkStats(s) })
		w.events = nil
	}
	if w.tracks != nil {
		s := w.tracks
		closeSink("analysis/tracks.parquet", s.Close, func() ParquetFileStats { return sinkStats(s) })
		w.tracks = nil
	}
	if w.segments != nil {
		s := w.segments
		closeSink("analysis/segments.parquet", s.Close, func() ParquetFileStats { return sinkStats(s) })
		w.segments = nil
	}
	if w.presence != nil {
		s := w.presence
		closeSink("analysis/presence_intervals.parquet", s.Close, func() ParquetFileStats { return sinkStats(s) })
		w.presence = nil
	}
	if w.transitions != nil {
		s := w.transitions
		closeSink("analysis/transitions.parquet", s.Close, func() ParquetFileStats { return sinkStats(s) })
		w.transitions = nil
	}
	if w.entities != nil {
		s := w.entities
		closeSink("analysis/entities.parquet", s.Close, func() ParquetFileStats { return sinkStats(s) })
		w.entities = nil
	}
	w.statsFinalized = true
	if w.jsonBuf != nil {
		if err := w.jsonBuf.Flush(); err != nil && first == nil {
			first = err
		}
		w.jsonBuf = nil
		w.jsonEnc = nil
	}
	if w.jsonFile != nil {
		if err := w.jsonFile.Close(); err != nil && first == nil {
			first = err
		}
		w.jsonFile = nil
	}
	return first
}
