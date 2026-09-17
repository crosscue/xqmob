package analyst

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/pipeline"
	_ "github.com/duckdb/duckdb-go/v2"
)

const (
	DefaultLimit = 100
	MaxLimit     = 1000
)

type Options struct {
	Threads int
}

type Dataset struct {
	Root     string
	Manifest pipeline.Manifest
	db       *sql.DB
	views    map[string]bool
	hasH3    bool
}

type Page struct {
	Rows       []map[string]any `json:"rows"`
	Returned   int              `json:"returned"`
	HasMore    bool             `json:"has_more"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type DatasetSummary struct {
	Tool                   string  `json:"tool"`
	ToolVersion            string  `json:"tool_version"`
	Eventizer              string  `json:"eventizer"`
	AnalysisContract       string  `json:"analysis_contract"`
	Source                 string  `json:"source"`
	InputRows              int64   `json:"input_rows"`
	NormalizedObservations int64   `json:"normalized_observations"`
	Entities               int64   `json:"entities"`
	Events                 int64   `json:"events"`
	Segments               int64   `json:"segments"`
	PresenceIntervals      int64   `json:"presence_intervals"`
	Transitions            int64   `json:"transitions"`
	Gaps                   int64   `json:"gaps"`
	Discontinuities        int64   `json:"discontinuities"`
	HACoveragePct          float64 `json:"ha_coverage_pct"`
	H3Available            bool    `json:"h3_available"`
	H3Resolution           int     `json:"h3_resolution"`
	GeohashPrecision       int     `json:"geohash_precision"`
	AnalysisBytes          int64   `json:"analysis_bytes"`
	CanonicalBytes         int64   `json:"canonical_bytes"`
	AnalysisExpansionRatio float64 `json:"analysis_expansion_ratio"`
}

type SearchEntitiesInput struct {
	MinObservations      int64   `json:"min_observations,omitempty" jsonschema:"minimum normalized observation count"`
	MinPresenceIntervals int64   `json:"min_presence_intervals,omitempty" jsonschema:"minimum inferred presence interval count"`
	MinTransitions       int64   `json:"min_transitions,omitempty" jsonschema:"minimum confirmed transition count"`
	MinDiscontinuities   int64   `json:"min_discontinuities,omitempty" jsonschema:"minimum discontinuity event count"`
	MinDwellSeconds      float64 `json:"min_dwell_seconds,omitempty" jsonschema:"minimum total dwell seconds"`
	HAClass              string  `json:"ha_class,omitempty" jsonschema:"HA support class: all_known, mixed, or all_missing"`
	MinHACoveragePct     float64 `json:"min_ha_coverage_pct,omitempty" jsonschema:"minimum percentage of normalized observations with supplied HA"`
	SortBy               string  `json:"sort_by,omitempty" jsonschema:"sort field: observation_count, transition_count, discontinuity_count, presence_count, total_dwell_s, first_seen, or last_seen"`
	Ascending            bool    `json:"ascending,omitempty" jsonschema:"sort ascending instead of the default descending"`
	Limit                int     `json:"limit,omitempty" jsonschema:"maximum rows to return; default 100, hard cap 1000"`
	Cursor               string  `json:"cursor,omitempty" jsonschema:"opaque next_cursor returned by a previous call"`
}

type EntityInput struct {
	EntityID string `json:"entity_id" jsonschema:"raw xqmob entity identifier"`
}

type TimePageInput struct {
	EntityID string `json:"entity_id,omitempty" jsonschema:"raw xqmob entity identifier"`
	From     string `json:"from,omitempty" jsonschema:"inclusive RFC3339 UTC lower time bound"`
	To       string `json:"to,omitempty" jsonschema:"inclusive RFC3339 UTC upper time bound"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum rows to return; default 100, hard cap 1000"`
	Cursor   string `json:"cursor,omitempty" jsonschema:"opaque next_cursor returned by a previous call"`
}

type PresenceInput struct {
	EntityID       string `json:"entity_id,omitempty" jsonschema:"raw xqmob entity identifier"`
	H3Cell         string `json:"h3_cell,omitempty" jsonschema:"materialized H3 cell string at the dataset resolution"`
	Classification string `json:"classification,omitempty" jsonschema:"presence classification: stay or dwell"`
	From           string `json:"from,omitempty" jsonschema:"inclusive RFC3339 UTC lower time bound"`
	To             string `json:"to,omitempty" jsonschema:"inclusive RFC3339 UTC upper time bound"`
	Limit          int    `json:"limit,omitempty" jsonschema:"maximum rows to return; default 100, hard cap 1000"`
	Cursor         string `json:"cursor,omitempty" jsonschema:"opaque next_cursor returned by a previous call"`
}

type TransitionInput struct {
	EntityID          string `json:"entity_id,omitempty" jsonschema:"raw xqmob entity identifier"`
	OriginH3Cell      string `json:"origin_h3_cell,omitempty" jsonschema:"transition origin H3 cell"`
	DestinationH3Cell string `json:"destination_h3_cell,omitempty" jsonschema:"transition destination H3 cell"`
	AccuracySupport   string `json:"accuracy_support,omitempty" jsonschema:"derived HA support class: all_known, mixed, or all_missing"`
	From              string `json:"from,omitempty" jsonschema:"inclusive RFC3339 UTC lower time bound"`
	To                string `json:"to,omitempty" jsonschema:"inclusive RFC3339 UTC upper time bound"`
	Limit             int    `json:"limit,omitempty" jsonschema:"maximum rows to return; default 100, hard cap 1000"`
	Cursor            string `json:"cursor,omitempty" jsonschema:"opaque next_cursor returned by a previous call"`
}

type EventInput struct {
	EntityID  string `json:"entity_id,omitempty" jsonschema:"raw xqmob entity identifier"`
	EventType string `json:"event_type,omitempty" jsonschema:"mobility semantic event type such as DWELL, GAP, or DISCONTINUITY"`
	H3Cell    string `json:"h3_cell,omitempty" jsonschema:"materialized H3 cell string at the dataset resolution"`
	From      string `json:"from,omitempty" jsonschema:"inclusive RFC3339 UTC lower time bound"`
	To        string `json:"to,omitempty" jsonschema:"inclusive RFC3339 UTC upper time bound"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum rows to return; default 100, hard cap 1000"`
	Cursor    string `json:"cursor,omitempty" jsonschema:"opaque next_cursor returned by a previous call"`
}

type ObservationInput struct {
	EntityID string `json:"entity_id,omitempty" jsonschema:"raw xqmob entity identifier"`
	H3Cell   string `json:"h3_cell,omitempty" jsonschema:"materialized H3 cell string at the dataset resolution"`
	From     string `json:"from,omitempty" jsonschema:"inclusive RFC3339 UTC lower time bound"`
	To       string `json:"to,omitempty" jsonschema:"inclusive RFC3339 UTC upper time bound"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum rows to return; default 100, hard cap 1000"`
	Cursor   string `json:"cursor,omitempty" jsonschema:"opaque next_cursor returned by a previous call"`
}

type H3CellInput struct {
	H3Cell    string `json:"h3_cell" jsonschema:"materialized H3 cell string at the dataset resolution"`
	FlowLimit int    `json:"flow_limit,omitempty" jsonschema:"maximum top origins/destinations and entities; default 10, maximum 50"`
}

type H3Flow struct {
	H3Cell          string `json:"h3_cell" jsonschema:"materialized H3 cell string at the dataset resolution"`
	TransitionCount int64  `json:"transition_count"`
	EntityCount     int64  `json:"entity_count"`
}

type H3CellDescription struct {
	H3Cell              string   `json:"h3_cell" jsonschema:"materialized H3 cell string at the dataset resolution"`
	H3Resolution        int      `json:"h3_resolution"`
	ObservationCount    int64    `json:"observation_count"`
	EntityCount         int64    `json:"entity_count"`
	PresenceCount       int64    `json:"presence_count"`
	DwellCount          int64    `json:"dwell_count"`
	TotalPresenceS      float64  `json:"total_presence_s"`
	TotalDwellS         float64  `json:"total_dwell_s"`
	IncomingTransitions int64    `json:"incoming_transitions"`
	OutgoingTransitions int64    `json:"outgoing_transitions"`
	FirstSeen           any      `json:"first_seen,omitempty"`
	LastSeen            any      `json:"last_seen,omitempty"`
	TopOrigins          []H3Flow `json:"top_origins"`
	TopDestinations     []H3Flow `json:"top_destinations"`
	TopEntities         Page     `json:"top_entities"`
}

type EventIDInput struct {
	EventID string `json:"event_id" jsonschema:"xqmob semantic event identifier"`
}

type EventExplanation struct {
	Event              map[string]any   `json:"event"`
	NearbyObservations []map[string]any `json:"nearby_observations"`
	Presence           map[string]any   `json:"presence,omitempty"`
	Segment            map[string]any   `json:"segment,omitempty"`
	EvidenceNote       string           `json:"evidence_note"`
}

type DiscontinuityExplanation struct {
	Event               map[string]any `json:"event"`
	PreviousObservation map[string]any `json:"previous_observation,omitempty"`
	CurrentObservation  map[string]any `json:"current_observation,omitempty"`
	Reason              string         `json:"reason"`
	JumpThresholdM      float64        `json:"jump_threshold_m"`
	SpeedThresholdMPS   float64        `json:"speed_threshold_mps"`
	EffectiveDistanceM  float64        `json:"effective_distance_m"`
	ImpliedSpeedMPS     float64        `json:"implied_speed_mps"`
	DTS                 float64        `json:"dt_s"`
	EdgeAccuracyState   string         `json:"edge_accuracy_state,omitempty"`
	Interpretation      string         `json:"interpretation"`
}

func Open(root string, opts Options) (*Dataset, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("MCP dataset must be an xqmob output directory")
	}
	manifestPath := filepath.Join(abs, "manifest.json")
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m pipeline.Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if m.Tool != "xqmob" {
		return nil, fmt.Errorf("manifest tool is %q, expected xqmob", m.Tool)
	}
	switch m.AnalysisContract {
	case "xqmob-analysis-v0.1", config.AnalysisContractVersion:
		// v0.1 predates materialized H3 columns. The analyst API exposes
		// compatibility views with nullable H3 fields so non-H3 tools can
		// query older datasets without re-eventization.
	default:
		return nil, fmt.Errorf("analyst API supports analysis contracts xqmob-analysis-v0.1 and %s; dataset has %s", config.AnalysisContractVersion, m.AnalysisContract)
	}
	threads := opts.Threads
	if threads <= 0 {
		threads = 4
	}
	// In-memory DuckDB cannot use access_mode=READ_ONLY: CREATE VIEW still
	// needs a writable catalog. Source parquet files are never imported.
	// Filesystem access is restricted to the opened dataset, then locked.
	dsn := fmt.Sprintf("?threads=%d", threads)
	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("open DuckDB: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	d := &Dataset{Root: abs, Manifest: m, db: db, views: make(map[string]bool), hasH3: m.AnalysisContract == config.AnalysisContractVersion}
	if err := d.restrictFilesystem(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := d.initViews(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := d.lockConfiguration(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return d, nil
}

func (d *Dataset) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

func (d *Dataset) initViews(ctx context.Context) error {
	names := []struct{ name, file string }{
		{"entities", "entities.parquet"}, {"observations", "observations.parquet"}, {"events", "events.parquet"},
		{"segments", "segments.parquet"}, {"presence_intervals", "presence_intervals.parquet"}, {"transitions", "transitions.parquet"}, {"tracks", "tracks.parquet"},
	}
	for _, v := range names {
		p := filepath.Join(d.Root, "analysis", v.file)
		if _, err := os.Stat(p); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		selectExpr := "*"
		if !d.hasH3 {
			selectExpr = compatibilitySelect(v.name)
		}
		q := fmt.Sprintf("CREATE OR REPLACE VIEW %s AS SELECT %s FROM read_parquet('%s')", v.name, selectExpr, sqlQuote(filepath.ToSlash(p)))
		if _, err := d.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("create DuckDB view %s: %w", v.name, err)
		}
		d.views[v.name] = true
	}
	required := []string{"entities", "observations", "events", "segments", "presence_intervals", "transitions"}
	for _, n := range required {
		if !d.views[n] {
			return fmt.Errorf("dataset missing analysis/%s.parquet", n)
		}
	}
	return nil
}

func (d *Dataset) restrictFilesystem(ctx context.Context) error {
	dir := filepath.ToSlash(d.Root)
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	q := fmt.Sprintf("SET allowed_directories=['%s']", sqlQuote(dir))
	if _, err := d.db.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("restrict DuckDB filesystem: %w", err)
	}
	if _, err := d.db.ExecContext(ctx, "SET enable_external_access=false"); err != nil {
		return fmt.Errorf("disable DuckDB external access: %w", err)
	}
	return nil
}

func (d *Dataset) lockConfiguration(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, "SET lock_configuration=true"); err != nil {
		return fmt.Errorf("lock DuckDB configuration: %w", err)
	}
	return nil
}

func sqlQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

func compatibilitySelect(view string) string {
	switch view {
	case "entities", "tracks":
		return "*, NULL::BIGINT AS unique_h3_cells, NULL::VARCHAR AS start_h3_cell, NULL::VARCHAR AS end_h3_cell"
	case "observations", "events", "presence_intervals":
		return "*, NULL::VARCHAR AS h3_cell"
	case "segments":
		return "*, NULL::VARCHAR AS start_h3_cell, NULL::VARCHAR AS end_h3_cell"
	case "transitions":
		return "*, NULL::VARCHAR AS origin_h3_cell, NULL::VARCHAR AS destination_h3_cell, NULL::BOOLEAN AS same_h3_cell"
	default:
		return "*"
	}
}

func (d *Dataset) H3Available() bool { return d != nil && d.hasH3 }

func (d *Dataset) requireH3() error {
	if d.H3Available() {
		return nil
	}
	return fmt.Errorf("H3 is not materialized in analysis contract %s; use entity/time/geohash-independent tools, or re-eventize with xqmob >= 0.1.1 for H3 analytical indexing", d.Manifest.AnalysisContract)
}

func (d *Dataset) Summary() DatasetSummary {
	s := d.Manifest.Stats
	h3Resolution := d.Manifest.Config.H3Resolution
	if !d.hasH3 {
		h3Resolution = -1
	}
	return DatasetSummary{
		Tool: d.Manifest.Tool, ToolVersion: d.Manifest.ToolVersion, Eventizer: d.Manifest.Eventizer,
		AnalysisContract: d.Manifest.AnalysisContract, Source: d.Manifest.Source,
		InputRows: s.InputRows, NormalizedObservations: s.NormalizedRows, Entities: s.EntityCount, Events: s.EventCount,
		Segments: s.SegmentCount, PresenceIntervals: s.PresenceCount, Transitions: s.TransitionCount, Gaps: s.GapCount,
		Discontinuities: s.DiscontinuityCount, HACoveragePct: s.Accuracy.NormalizedHACoveragePct,
		H3Available: d.hasH3, H3Resolution: h3Resolution, GeohashPrecision: d.Manifest.Config.GeohashPrecision,
		AnalysisBytes: d.Manifest.Storage.AnalysisBytes, CanonicalBytes: d.Manifest.Storage.CanonicalBytes,
		AnalysisExpansionRatio: d.Manifest.Storage.AnalysisExpansionRatio,
	}
}

func (d *Dataset) SearchEntities(ctx context.Context, in SearchEntitiesInput) (Page, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, v any) { where = append(where, cond); args = append(args, v) }
	if in.MinObservations > 0 {
		add("observation_count >= ?", in.MinObservations)
	}
	if in.MinPresenceIntervals > 0 {
		add("presence_count >= ?", in.MinPresenceIntervals)
	}
	if in.MinTransitions > 0 {
		add("transition_count >= ?", in.MinTransitions)
	}
	if in.MinDiscontinuities > 0 {
		add("discontinuity_count >= ?", in.MinDiscontinuities)
	}
	if in.MinDwellSeconds > 0 {
		add("total_dwell_s >= ?", in.MinDwellSeconds)
	}
	if in.HAClass != "" {
		add("ha_class = ?", in.HAClass)
	}
	if in.MinHACoveragePct > 0 {
		add("ha_coverage_pct >= ?", in.MinHACoveragePct)
	}
	sortBy := in.SortBy
	if sortBy == "" {
		sortBy = "observation_count"
	}
	allowed := map[string]bool{"observation_count": true, "transition_count": true, "discontinuity_count": true, "presence_count": true, "total_dwell_s": true, "first_seen": true, "last_seen": true}
	if !allowed[sortBy] {
		return Page{}, fmt.Errorf("unsupported sort_by %q", sortBy)
	}
	dir := "DESC"
	if in.Ascending {
		dir = "ASC"
	}
	q := `SELECT entity_id, subject, first_seen, last_seen, coverage_s, observation_count, segment_count, presence_count, transition_count, stay_count, dwell_count, gap_count, discontinuity_count, total_observed_path_m, total_presence_s, total_dwell_s, unique_geohash_cells, unique_h3_cells, median_ha_m, observations_with_ha, observations_without_ha, ha_coverage_pct, ha_class, start_h3_cell, end_h3_cell FROM entities WHERE ` + strings.Join(where, " AND ") + ` ORDER BY ` + sortBy + ` ` + dir + `, entity_id ASC`
	return d.queryPage(ctx, q, args, in.Limit, in.Cursor)
}

func (d *Dataset) GetEntity(ctx context.Context, id string) (map[string]any, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("entity_id is required")
	}
	q := `SELECT * EXCLUDE (eventizer_version) FROM entities WHERE entity_id = ? LIMIT 1`
	rows, err := d.queryRows(ctx, q, []any{id})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("entity %q not found", id)
	}
	return rows[0], nil
}

func (d *Dataset) GetPresence(ctx context.Context, in PresenceInput) (Page, error) {
	where, args, err := buildTimeWhere("start_ts", in.EntityID, in.From, in.To)
	if err != nil {
		return Page{}, err
	}
	if in.H3Cell != "" {
		if err := d.requireH3(); err != nil {
			return Page{}, err
		}
		where = append(where, "h3_cell = ?")
		args = append(args, in.H3Cell)
	}
	if in.Classification != "" {
		where = append(where, "classification = ?")
		args = append(args, in.Classification)
	}
	q := `SELECT presence_interval_id, track_id, segment_id, entity_id, start_ts, end_ts, duration_s, classification, centroid_lat, centroid_lon, h3_cell, observation_count, scatter_m, median_ha_m, min_ha_m, max_ha_m, observations_with_ha, observations_without_ha, ha_coverage_pct, accuracy_support, start_reason, end_reason, interval_event_id, enter_event_id, leave_event_id FROM presence_intervals WHERE ` + strings.Join(where, " AND ") + ` ORDER BY start_ts, presence_interval_id`
	return d.queryPage(ctx, q, args, in.Limit, in.Cursor)
}

func (d *Dataset) GetTransitions(ctx context.Context, in TransitionInput) (Page, error) {
	where, args, err := buildTimeWhere("depart_ts", in.EntityID, in.From, in.To)
	if err != nil {
		return Page{}, err
	}
	if in.OriginH3Cell != "" {
		if err := d.requireH3(); err != nil {
			return Page{}, err
		}
		where = append(where, "origin_h3_cell = ?")
		args = append(args, in.OriginH3Cell)
	}
	if in.DestinationH3Cell != "" {
		if err := d.requireH3(); err != nil {
			return Page{}, err
		}
		where = append(where, "destination_h3_cell = ?")
		args = append(args, in.DestinationH3Cell)
	}
	if in.AccuracySupport != "" {
		where = append(where, "accuracy_support = ?")
		args = append(args, in.AccuracySupport)
	}
	q := `SELECT transition_id, entity_id, track_id, segment_id, origin_presence_id, destination_presence_id, depart_ts, arrive_ts, duration_s, origin_lat, origin_lon, destination_lat, destination_lon, origin_h3_cell, destination_h3_cell, same_h3_cell, straight_line_distance_m, observed_path_distance_m, observation_count, observations_with_ha, observations_without_ha, ha_coverage_pct, accuracy_support, origin_accuracy_support, movement_accuracy_support, destination_accuracy_support FROM transitions WHERE ` + strings.Join(where, " AND ") + ` ORDER BY depart_ts, transition_id`
	return d.queryPage(ctx, q, args, in.Limit, in.Cursor)
}

func (d *Dataset) GetSegments(ctx context.Context, in TimePageInput) (Page, error) {
	where, args, err := buildTimeWhere("start_ts", in.EntityID, in.From, in.To)
	if err != nil {
		return Page{}, err
	}
	q := `SELECT segment_id, track_id, entity_id, segment_index, start_ts, end_ts, observation_count, gap_count, duration_s, observed_path_m, displacement_m, mean_observed_speed_mps, max_implied_speed_mps, start_lat, start_lon, end_lat, end_lon, start_h3_cell, end_h3_cell, start_reason, end_reason, observations_with_ha, observations_without_ha, ha_coverage_pct, accuracy_support, edge_both_known_count, edge_previous_missing_count, edge_current_missing_count, edge_both_missing_count, path_component_count FROM segments WHERE ` + strings.Join(where, " AND ") + ` ORDER BY start_ts, segment_id`
	return d.queryPage(ctx, q, args, in.Limit, in.Cursor)
}

func (d *Dataset) GetEvents(ctx context.Context, in EventInput) (Page, error) {
	where, args, err := buildTimeWhere("event_time", in.EntityID, in.From, in.To)
	if err != nil {
		return Page{}, err
	}
	if in.EventType != "" {
		where = append(where, "event_type = ?")
		args = append(args, in.EventType)
	}
	if in.H3Cell != "" {
		if err := d.requireH3(); err != nil {
			return Page{}, err
		}
		where = append(where, "h3_cell = ?")
		args = append(args, in.H3Cell)
	}
	q := `SELECT event_id, entity_id, subject, event_type, event_time, has_end_time, end_time, source, modality, class, profile, feature, action, state, polarity, has_magnitude, magnitude, unit, lat, lon, accuracy_m, accuracy_state, accuracy_support, accuracy_known_count, accuracy_missing_count, h3_cell, track_id, segment_id, presence_interval_id, context_json, parameters_json FROM events WHERE ` + strings.Join(where, " AND ") + ` ORDER BY event_time, event_id`
	return d.queryPage(ctx, q, args, in.Limit, in.Cursor)
}

func (d *Dataset) GetObservations(ctx context.Context, in ObservationInput) (Page, error) {
	if in.EntityID == "" && in.H3Cell == "" {
		return Page{}, fmt.Errorf("get_observations requires entity_id or h3_cell")
	}
	where, args, err := buildTimeWhere("ts", in.EntityID, in.From, in.To)
	if err != nil {
		return Page{}, err
	}
	if in.H3Cell != "" {
		if err := d.requireH3(); err != nil {
			return Page{}, err
		}
		where = append(where, "h3_cell = ?")
		args = append(args, in.H3Cell)
	}
	q := `SELECT observation_id, entity_id, subject, ts, lat, lon, ha_m, ha_state, step_accuracy_state, geohash, h3_cell, track_id, segment_id, presence_interval_id, sample_count, same_ts_scatter_m, has_step, step_dt_s, step_distance_m, effective_distance_m, implied_speed_mps, is_gap, is_discontinuity, movement_state, speed_class, source_row FROM observations WHERE ` + strings.Join(where, " AND ") + ` ORDER BY ts, observation_id`
	return d.queryPage(ctx, q, args, in.Limit, in.Cursor)
}

func (d *Dataset) GetTimeline(ctx context.Context, in TimePageInput) (Page, error) {
	if in.EntityID == "" {
		return Page{}, fmt.Errorf("entity_id is required")
	}
	from, to, err := parseBounds(in.From, in.To)
	if err != nil {
		return Page{}, err
	}
	clauses := []string{"entity_id = ?"}
	eventArgs := []any{in.EntityID}
	pclauses := []string{"entity_id = ?"}
	pArgs := []any{in.EntityID}
	tclauses := []string{"entity_id = ?"}
	tArgs := []any{in.EntityID}
	if !from.IsZero() {
		clauses = append(clauses, "event_time >= ?")
		eventArgs = append(eventArgs, from)
		pclauses = append(pclauses, "start_ts >= ?")
		pArgs = append(pArgs, from)
		tclauses = append(tclauses, "depart_ts >= ?")
		tArgs = append(tArgs, from)
	}
	if !to.IsZero() {
		clauses = append(clauses, "event_time <= ?")
		eventArgs = append(eventArgs, to)
		pclauses = append(pclauses, "start_ts <= ?")
		pArgs = append(pArgs, to)
		tclauses = append(tclauses, "depart_ts <= ?")
		tArgs = append(tArgs, to)
	}
	q := `SELECT event_time AS ts, 'event' AS kind, event_id AS id, event_type AS label, CASE WHEN has_end_time THEN end_time ELSE NULL END AS end_ts, CASE WHEN has_magnitude AND unit='s' THEN magnitude ELSE NULL END AS duration_s, h3_cell, NULL::VARCHAR AS destination_h3_cell, segment_id, presence_interval_id, accuracy_support FROM events WHERE ` + strings.Join(clauses, " AND ") + `
UNION ALL SELECT start_ts AS ts, 'presence' AS kind, presence_interval_id AS id, classification AS label, end_ts, duration_s, h3_cell, NULL::VARCHAR AS destination_h3_cell, segment_id, presence_interval_id, accuracy_support FROM presence_intervals WHERE ` + strings.Join(pclauses, " AND ") + `
UNION ALL SELECT depart_ts AS ts, 'transition' AS kind, transition_id AS id, 'transition' AS label, arrive_ts AS end_ts, duration_s, origin_h3_cell AS h3_cell, destination_h3_cell, segment_id, destination_presence_id AS presence_interval_id, accuracy_support FROM transitions WHERE ` + strings.Join(tclauses, " AND ") + `
ORDER BY ts, kind, id`
	args := append(append(eventArgs, pArgs...), tArgs...)
	return d.queryPage(ctx, q, args, in.Limit, in.Cursor)
}

func (d *Dataset) DescribeH3Cell(ctx context.Context, in H3CellInput) (H3CellDescription, error) {
	if err := d.requireH3(); err != nil {
		return H3CellDescription{}, err
	}
	if strings.TrimSpace(in.H3Cell) == "" {
		return H3CellDescription{}, fmt.Errorf("h3_cell is required")
	}
	out := H3CellDescription{H3Cell: in.H3Cell, H3Resolution: d.Manifest.Config.H3Resolution}
	r, err := d.queryRows(ctx, `SELECT count(*) AS observation_count, count(DISTINCT entity_id) AS entity_count, min(ts) AS first_seen, max(ts) AS last_seen FROM observations WHERE h3_cell = ?`, []any{in.H3Cell})
	if err != nil {
		return out, err
	}
	if len(r) > 0 {
		out.ObservationCount = toInt64(r[0]["observation_count"])
		out.EntityCount = toInt64(r[0]["entity_count"])
		out.FirstSeen = r[0]["first_seen"]
		out.LastSeen = r[0]["last_seen"]
	}
	r, err = d.queryRows(ctx, `SELECT count(*) AS presence_count, count(*) FILTER (WHERE classification='dwell') AS dwell_count, coalesce(sum(duration_s),0) AS total_presence_s, coalesce(sum(duration_s) FILTER (WHERE classification='dwell'),0) AS total_dwell_s FROM presence_intervals WHERE h3_cell = ?`, []any{in.H3Cell})
	if err != nil {
		return out, err
	}
	if len(r) > 0 {
		out.PresenceCount = toInt64(r[0]["presence_count"])
		out.DwellCount = toInt64(r[0]["dwell_count"])
		out.TotalPresenceS = toFloat64(r[0]["total_presence_s"])
		out.TotalDwellS = toFloat64(r[0]["total_dwell_s"])
	}
	r, err = d.queryRows(ctx, `SELECT count(*) FILTER (WHERE destination_h3_cell = ?) AS incoming, count(*) FILTER (WHERE origin_h3_cell = ?) AS outgoing FROM transitions WHERE destination_h3_cell = ? OR origin_h3_cell = ?`, []any{in.H3Cell, in.H3Cell, in.H3Cell, in.H3Cell})
	if err != nil {
		return out, err
	}
	if len(r) > 0 {
		out.IncomingTransitions = toInt64(r[0]["incoming"])
		out.OutgoingTransitions = toInt64(r[0]["outgoing"])
	}
	n := in.FlowLimit
	if n <= 0 {
		n = 10
	}
	if n > 50 {
		n = 50
	}
	origins, err := d.queryRows(ctx, `SELECT origin_h3_cell AS h3_cell, count(*) AS transition_count, count(DISTINCT entity_id) AS entity_count FROM transitions WHERE destination_h3_cell = ? GROUP BY origin_h3_cell ORDER BY transition_count DESC, h3_cell LIMIT ?`, []any{in.H3Cell, n})
	if err != nil {
		return out, err
	}
	out.TopOrigins = rowsToFlows(origins)
	dests, err := d.queryRows(ctx, `SELECT destination_h3_cell AS h3_cell, count(*) AS transition_count, count(DISTINCT entity_id) AS entity_count FROM transitions WHERE origin_h3_cell = ? GROUP BY destination_h3_cell ORDER BY transition_count DESC, h3_cell LIMIT ?`, []any{in.H3Cell, n})
	if err != nil {
		return out, err
	}
	out.TopDestinations = rowsToFlows(dests)
	top, err := d.queryPage(ctx, `SELECT entity_id, count(*) AS observation_count, min(ts) AS first_seen, max(ts) AS last_seen FROM observations WHERE h3_cell = ? GROUP BY entity_id ORDER BY observation_count DESC, entity_id`, []any{in.H3Cell}, n, "")
	if err != nil {
		return out, err
	}
	out.TopEntities = top
	return out, nil
}

func rowsToFlows(rows []map[string]any) []H3Flow {
	out := make([]H3Flow, 0, len(rows))
	for _, r := range rows {
		out = append(out, H3Flow{H3Cell: fmt.Sprint(r["h3_cell"]), TransitionCount: toInt64(r["transition_count"]), EntityCount: toInt64(r["entity_count"])})
	}
	return out
}

func (d *Dataset) ExplainEvent(ctx context.Context, id string) (EventExplanation, error) {
	if id == "" {
		return EventExplanation{}, fmt.Errorf("event_id is required")
	}
	rows, err := d.queryRows(ctx, `SELECT event_id, entity_id, subject, event_type, event_time, epoch_ms(event_time) AS event_time_ms, has_end_time, end_time, source, feature, action, state, polarity, has_magnitude, magnitude, unit, lat, lon, accuracy_m, accuracy_state, accuracy_support, h3_cell, track_id, segment_id, presence_interval_id, context_json, parameters_json FROM events WHERE event_id = ? LIMIT 1`, []any{id})
	if err != nil {
		return EventExplanation{}, err
	}
	if len(rows) == 0 {
		return EventExplanation{}, fmt.Errorf("event %q not found", id)
	}
	ev := rows[0]
	entity := fmt.Sprint(ev["entity_id"])
	eventTimeMS := toInt64(ev["event_time_ms"])
	near, err := d.queryRows(ctx, `SELECT observation_id, entity_id, ts, lat, lon, ha_m, ha_state, step_accuracy_state, geohash, h3_cell, track_id, segment_id, presence_interval_id, step_dt_s, step_distance_m, effective_distance_m, implied_speed_mps, is_gap, is_discontinuity FROM observations WHERE entity_id = ? ORDER BY abs(epoch_ms(ts) - ?) LIMIT 6`, []any{entity, eventTimeMS})
	if err != nil {
		return EventExplanation{}, err
	}
	out := EventExplanation{Event: ev, NearbyObservations: near, EvidenceNote: "Observations are source evidence; events, presence, segments, and transitions are algorithmically derived interpretations."}
	if pid := fmt.Sprint(ev["presence_interval_id"]); pid != "" && pid != "<nil>" {
		p, _ := d.queryRows(ctx, `SELECT * EXCLUDE (geometry,eventizer_version) FROM presence_intervals WHERE presence_interval_id = ? LIMIT 1`, []any{pid})
		if len(p) > 0 {
			out.Presence = p[0]
		}
	}
	if sid := fmt.Sprint(ev["segment_id"]); sid != "" && sid != "<nil>" {
		s, _ := d.queryRows(ctx, `SELECT * EXCLUDE (geometry,eventizer_version) FROM segments WHERE segment_id = ? LIMIT 1`, []any{sid})
		if len(s) > 0 {
			out.Segment = s[0]
		}
	}
	return out, nil
}

func (d *Dataset) ExplainDiscontinuity(ctx context.Context, id string) (DiscontinuityExplanation, error) {
	ex, err := d.ExplainEvent(ctx, id)
	if err != nil {
		return DiscontinuityExplanation{}, err
	}
	if fmt.Sprint(ex.Event["event_type"]) != "DISCONTINUITY" {
		return DiscontinuityExplanation{}, fmt.Errorf("event %q is %s, not DISCONTINUITY", id, fmt.Sprint(ex.Event["event_type"]))
	}
	var c map[string]any
	if raw, ok := ex.Event["context_json"].(string); ok {
		_ = json.Unmarshal([]byte(raw), &c)
	}
	eff := toFloat64(c["effective_distance_m"])
	speed := toFloat64(c["implied_speed_mps"])
	dt := toFloat64(c["travel_seconds"])
	jump := eff > d.Manifest.Config.MaxJumpM
	fast := dt > 0 && speed > d.Manifest.Config.MaxSpeedMPS
	reason := "unknown"
	switch {
	case jump && fast:
		reason = "jump_and_speed"
	case jump:
		reason = "jump_only"
	case fast:
		reason = "speed_only"
	}
	entity := fmt.Sprint(ex.Event["entity_id"])
	tms := toInt64(ex.Event["event_time_ms"])
	obs, err := d.queryRows(ctx, `SELECT observation_id, entity_id, ts, lat, lon, ha_m, ha_state, step_accuracy_state, geohash, h3_cell, track_id, segment_id, step_dt_s, step_distance_m, effective_distance_m, implied_speed_mps, is_gap, is_discontinuity FROM observations WHERE entity_id = ? AND epoch_ms(ts) <= ? ORDER BY ts DESC, observation_id DESC LIMIT 2`, []any{entity, tms})
	if err != nil {
		return DiscontinuityExplanation{}, err
	}
	var cur, prev map[string]any
	if len(obs) > 0 {
		cur = obs[0]
	}
	if len(obs) > 1 {
		prev = obs[1]
	}
	edge := fmt.Sprint(c["edge_accuracy_state"])
	if edge == "<nil>" {
		edge = ""
	}
	return DiscontinuityExplanation{Event: ex.Event, PreviousObservation: prev, CurrentObservation: cur, Reason: reason, JumpThresholdM: d.Manifest.Config.MaxJumpM, SpeedThresholdMPS: d.Manifest.Config.MaxSpeedMPS, EffectiveDistanceM: eff, ImpliedSpeedMPS: speed, DTS: dt, EdgeAccuracyState: edge, Interpretation: "A discontinuity means the adjacent observations violated the configured continuity model; it does not prove physical travel between the two positions."}, nil
}

func buildTimeWhere(col, entity, fromS, toS string) ([]string, []any, error) {
	where := []string{"1=1"}
	args := []any{}
	if entity != "" {
		where = append(where, "entity_id = ?")
		args = append(args, entity)
	}
	from, to, err := parseBounds(fromS, toS)
	if err != nil {
		return nil, nil, err
	}
	if !from.IsZero() {
		where = append(where, col+" >= ?")
		args = append(args, from)
	}
	if !to.IsZero() {
		where = append(where, col+" <= ?")
		args = append(args, to)
	}
	return where, args, nil
}
func parseBounds(fromS, toS string) (time.Time, time.Time, error) {
	var from, to time.Time
	var err error
	if fromS != "" {
		from, err = time.Parse(time.RFC3339, fromS)
		if err != nil {
			return from, to, fmt.Errorf("from must be RFC3339: %w", err)
		}
	}
	if toS != "" {
		to, err = time.Parse(time.RFC3339, toS)
		if err != nil {
			return from, to, fmt.Errorf("to must be RFC3339: %w", err)
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return from, to, fmt.Errorf("from must not be after to")
	}
	return from, to, nil
}

func (d *Dataset) queryPage(ctx context.Context, q string, args []any, limit int, cursor string) (Page, error) {
	limit = normalizeLimit(limit)
	offset, err := decodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	args = append(args, limit+1, offset)
	rows, err := d.queryRows(ctx, q+` LIMIT ? OFFSET ?`, args)
	if err != nil {
		return Page{}, err
	}
	p := Page{}
	if len(rows) > limit {
		p.HasMore = true
		rows = rows[:limit]
		p.NextCursor = encodeCursor(offset + limit)
	}
	p.Rows = rows
	p.Returned = len(rows)
	return p, nil
}
func normalizeLimit(n int) int {
	if n <= 0 {
		return DefaultLimit
	}
	if n > MaxLimit {
		return MaxLimit
	}
	return n
}
func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}
func decodeCursor(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor")
	}
	n, err := strconv.Atoi(string(b))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid cursor")
	}
	return n, nil
}

func (d *Dataset) queryRows(ctx context.Context, q string, args []any) ([]map[string]any, error) {
	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = normalizeSQLValue(vals[i])
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
func normalizeSQLValue(v any) any {
	switch x := v.(type) {
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	case []byte:
		return string(x)
	default:
		return x
	}
}
func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int32:
		return int64(x)
	case int:
		return int64(x)
	case uint64:
		return int64(x)
	case float64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}
func toFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int32:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

func (d *Dataset) ManifestJSON() ([]byte, error) { return json.MarshalIndent(d.Manifest, "", "  ") }

func StableMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
