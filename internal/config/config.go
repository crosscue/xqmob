package config

const (
	Version                 = "0.1.4-rc1"
	EventizerVersion        = "mobility-reference-v1.2"
	ProfileID               = "xq.mob:profile-0.1"
	WireVersion             = "0.1"
	AnalysisContractVersion = "xqmob-analysis-v0.2"
	MCPContractVersion      = "xqmob-mcp-v0.2"
	Modality                = "xq:mobility"
	H3Library               = "github.com/dimchansky/h3-go"
	H3LibraryVersion        = "v0.4.0"
	H3CoreVersion           = "4.5.0"
)

type Config struct {
	ReplaceOutput             bool     `json:"replace_output,omitempty"`
	OutputProfile             string   `json:"output_profile"`
	ParquetCompression        string   `json:"parquet_compression"`
	ParquetRowGroupRows       int64    `json:"parquet_row_group_rows"`
	ParquetDictionaryMaxBytes int64    `json:"parquet_dictionary_max_bytes"`
	Source                    string   `json:"source"`
	SubjectPrefix             string   `json:"subject_prefix"`
	GeohashPrecision          int      `json:"geohash_precision"`
	H3Resolution              int      `json:"h3_resolution"`
	MoveRadiusM               float64  `json:"move_radius_m"`
	DwellThresholdS           float64  `json:"dwell_threshold_s"`
	GapThresholdS             float64  `json:"gap_threshold_s"`
	MaxSpeedMPS               float64  `json:"max_speed_mps"`
	MaxJumpM                  float64  `json:"max_jump_m"`
	ConfirmMoves              bool     `json:"confirm_moves"`
	ConfirmWindowS            float64  `json:"confirm_window_s"`
	WalkMaxSpeedMPS           float64  `json:"walk_max_speed_mps"`
	WalkMaxJumpM              float64  `json:"walk_max_jump_m"`
	AccuracyPolicy            string   `json:"accuracy_policy"`
	HAPolicy                  string   `json:"ha_policy"`
	Partitions                int      `json:"partitions"`
	MaxHAM                    float64  `json:"max_ha_m,omitempty"`
	Strict                    bool     `json:"strict"`
	KeepTemporary             bool     `json:"keep_temporary"`
	TempDir                   string   `json:"temp_dir,omitempty"`
	MemoryProfileDir          string   `json:"memory_profile_dir,omitempty"`
	TraceIDs                  []string `json:"trace_ids,omitempty"`
}

func Default() Config {
	return Config{
		OutputProfile:             "standard",
		ParquetCompression:        "zstd",
		ParquetRowGroupRows:       131072,
		ParquetDictionaryMaxBytes: 8 << 20,
		SubjectPrefix:             "device:",
		GeohashPrecision:          8,
		H3Resolution:              11,
		MoveRadiusM:               100,
		DwellThresholdS:           900,
		GapThresholdS:             7200,
		MaxSpeedMPS:               50,
		MaxJumpM:                  50000,
		ConfirmMoves:              true,
		ConfirmWindowS:            900,
		WalkMaxSpeedMPS:           6,
		WalkMaxJumpM:              8000,
		AccuracyPolicy:            "subtract_radii",
		HAPolicy:                  "preserve",
		Partitions:                64,
	}
}

func (c Config) Parameters() map[string]any {
	return map[string]any{
		"geohash_precision":             c.GeohashPrecision,
		"move_radius_m":                 c.MoveRadiusM,
		"dwell_threshold_s":             c.DwellThresholdS,
		"gap_threshold_s":               c.GapThresholdS,
		"max_speed_mps":                 c.MaxSpeedMPS,
		"max_jump_m":                    c.MaxJumpM,
		"confirm_moves":                 c.ConfirmMoves,
		"confirm_window_s":              c.ConfirmWindowS,
		"walk_max_speed_mps":            c.WalkMaxSpeedMPS,
		"walk_max_jump_m":               c.WalkMaxJumpM,
		"eventizer_algorithm":           EventizerVersion,
		"accuracy_policy":               c.AccuracyPolicy,
		"ha_policy":                     c.HAPolicy,
		"minimum_presence_observations": 2,
		"same_timestamp_policy":         "known_ha_first_then_lowest_ha_then_lat_lon_then_source_row",
		"distance_model":                "haversine_wgs84_mean_radius",
	}
}

// ShouldTrace reports whether decision tracing is enabled for an entity.
func (c Config) ShouldTrace(entityID string) bool {
	for _, id := range c.TraceIDs {
		if id == entityID {
			return true
		}
	}
	return false
}
