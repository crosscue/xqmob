package input

import (
	"bufio"
	"encoding/csv"
	"encoding/gob"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/model"
)

type PartitionResult struct {
	Files              []string
	ValidRows          int64
	RejectedRows       int64
	InputRows          int64
	RejectsPath        string
	RejectReasons      map[string]int64
	InputHAPresentRows int64
	InputHAMissingRows int64
	AcceptedWithHA     int64
	AcceptedWithoutHA  int64
}

type RejectError struct {
	Code    string
	Message string
}

func (e *RejectError) Error() string { return e.Message }

func reject(code, message string) error {
	return &RejectError{Code: code, Message: message}
}

func rejectCode(err error) string {
	var r *RejectError
	if errors.As(err, &r) {
		return r.Code
	}
	return "unknown"
}

type partitionWriter struct {
	file io.WriteCloser
	buf  *bufio.Writer
	enc  *gob.Encoder
}

// PartitionWriterBufferBytes is the application-level buffered writer size per
// deterministic hash partition.
const PartitionWriterBufferBytes = 1 << 20

func PartitionCSV(inputPath, tempDir, rejectsPath string, cfg config.Config) (PartitionResult, error) {
	return partitionCSV(inputPath, tempDir, rejectsPath, cfg, func(path string) (io.WriteCloser, error) {
		return os.Create(path)
	})
}

func partitionCSV(inputPath, tempDir, rejectsPath string, cfg config.Config, create func(string) (io.WriteCloser, error)) (result PartitionResult, retErr error) {
	result.RejectReasons = map[string]int64{}
	f, err := os.Open(inputPath)
	if err != nil {
		return result, err
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return result, fmt.Errorf("read CSV header: %w", err)
	}
	idx, err := requiredColumns(header)
	if err != nil {
		return result, err
	}
	if cfg.Partitions < 1 {
		return result, errors.New("partitions must be >= 1")
	}

	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return result, err
	}
	writers := make([]partitionWriter, cfg.Partitions)
	defer func() { retErr = errors.Join(retErr, closePartitions(writers)) }()
	result.Files = make([]string, cfg.Partitions)
	for i := range writers {
		path := filepath.Join(tempDir, fmt.Sprintf("part-%04d.gob", i))
		pf, err := create(path)
		if err != nil {
			return result, err
		}
		bw := bufio.NewWriterSize(pf, PartitionWriterBufferBytes)
		writers[i] = partitionWriter{file: pf, buf: bw, enc: gob.NewEncoder(bw)}
		result.Files[i] = path
	}

	rf, err := create(rejectsPath)
	if err != nil {
		return result, err
	}
	rw := csv.NewWriter(rf)
	defer func() {
		rw.Flush()
		if err := rw.Error(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("flush rejects: %w", err))
		}
		if err := rf.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close rejects: %w", err))
		}
	}()
	writeReject := func(record []string) error {
		if err := rw.Write(record); err != nil {
			return fmt.Errorf("write rejects: %w", err)
		}
		return nil
	}
	if err := writeReject([]string{"source_row", "reason_code", "reason", "id", "ts", "lat", "lon", "ha"}); err != nil {
		return result, err
	}
	result.RejectsPath = rejectsPath

	rowNum := int64(1)
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		rowNum++
		result.InputRows++
		if err != nil {
			var parseErr *csv.ParseError
			if !errors.As(err, &parseErr) {
				return result, fmt.Errorf("read CSV row %d: %w", rowNum, err)
			}
			if cfg.Strict {
				return result, fmt.Errorf("CSV row %d: %w", rowNum, err)
			}
			result.RejectedRows++
			result.RejectReasons["csv_parse_error"]++
			if err := writeReject([]string{strconv.FormatInt(rowNum, 10), "csv_parse_error", err.Error(), "", "", "", "", ""}); err != nil {
				return result, err
			}
			continue
		}
		get := func(name string) string {
			i := idx[name]
			if i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}
		id, tsRaw, latRaw, lonRaw, haRaw := get("id"), get("ts"), get("lat"), get("lon"), get("ha")
		if strings.TrimSpace(haRaw) == "" {
			result.InputHAMissingRows++
		} else {
			result.InputHAPresentRows++
		}
		obs, parseErr := parseObservation(id, tsRaw, latRaw, lonRaw, haRaw, rowNum, cfg)
		if parseErr != nil {
			if cfg.Strict {
				return result, fmt.Errorf("CSV row %d: %w", rowNum, parseErr)
			}
			result.RejectedRows++
			code := rejectCode(parseErr)
			result.RejectReasons[code]++
			if err := writeReject([]string{strconv.FormatInt(rowNum, 10), code, parseErr.Error(), id, tsRaw, latRaw, lonRaw, haRaw}); err != nil {
				return result, err
			}
			continue
		}
		p := partitionIndex(obs.EntityID, cfg.Partitions)
		if err := writers[p].enc.Encode(obs); err != nil {
			return result, err
		}
		result.ValidRows++
		if obs.HasHA {
			result.AcceptedWithHA++
		} else {
			result.AcceptedWithoutHA++
		}
	}
	return result, nil
}

func closePartitions(ws []partitionWriter) error {
	var result error
	for i := range ws {
		if ws[i].buf != nil {
			if err := ws[i].buf.Flush(); err != nil {
				result = errors.Join(result, fmt.Errorf("flush partition %d: %w", i, err))
			}
		}
		if ws[i].file != nil {
			if err := ws[i].file.Close(); err != nil {
				result = errors.Join(result, fmt.Errorf("close partition %d: %w", i, err))
			}
		}
	}
	return result
}

func requiredColumns(header []string) (map[string]int, error) {
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, name := range []string{"id", "ts", "lat", "lon", "ha"} {
		if _, ok := idx[name]; !ok {
			return nil, fmt.Errorf("missing required CSV header %q", name)
		}
	}
	return idx, nil
}

func parseObservation(id, tsRaw, latRaw, lonRaw, haRaw string, row int64, cfg config.Config) (model.Observation, error) {
	var o model.Observation
	if id == "" {
		return o, reject("empty_id", "empty id")
	}
	ts, err := ParseTimestamp(tsRaw)
	if err != nil {
		return o, reject("invalid_ts", fmt.Sprintf("invalid ts: %v", err))
	}
	lat, err := strconv.ParseFloat(latRaw, 64)
	if err != nil || math.IsNaN(lat) || math.IsInf(lat, 0) || lat < -90 || lat > 90 {
		return o, reject("invalid_lat", "invalid lat")
	}
	lon, err := strconv.ParseFloat(lonRaw, 64)
	if err != nil || math.IsNaN(lon) || math.IsInf(lon, 0) || lon < -180 || lon > 180 {
		return o, reject("invalid_lon", "invalid lon")
	}
	if strings.TrimSpace(haRaw) == "" {
		if cfg.HAPolicy == "require" {
			return o, reject("missing_ha", "missing ha")
		}
		return model.Observation{EntityID: id, TS: ts.UTC(), Lat: lat, Lon: lon, HasHA: false, AccuracyState: "missing", SourceRow: row, SampleCount: 1,
			LatDecimalPlaces: coordinateDecimalPlaces(latRaw), LonDecimalPlaces: coordinateDecimalPlaces(lonRaw)}, nil
	}
	ha, err := strconv.ParseFloat(haRaw, 64)
	if err != nil {
		return o, reject("non_numeric_ha", "non-numeric ha")
	}
	if math.IsNaN(ha) || math.IsInf(ha, 0) {
		return o, reject("non_finite_ha", "non-finite ha")
	}
	if ha < 0 {
		return o, reject("negative_ha", "negative ha")
	}
	if cfg.MaxHAM > 0 && ha > cfg.MaxHAM {
		return o, reject("ha_exceeds_max", fmt.Sprintf("ha exceeds max-ha-m %.3f", cfg.MaxHAM))
	}
	return model.Observation{EntityID: id, TS: ts.UTC(), Lat: lat, Lon: lon, HA: ha, HasHA: true, AccuracyState: "known", SourceRow: row, SampleCount: 1,
		LatDecimalPlaces: coordinateDecimalPlaces(latRaw), LonDecimalPlaces: coordinateDecimalPlaces(lonRaw)}, nil
}

// coordinateDecimalPlaces records the lexical fractional precision supplied by
// the source export. Scientific notation is represented as -1 because its
// displayed decimal places are not a useful proxy for coordinate precision.
func coordinateDecimalPlaces(s string) int {
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, "eE") {
		return -1
	}
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		return len(s) - dot - 1
	}
	return 0
}

var decimalTimestamp = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func ParseTimestamp(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty timestamp")
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if len(s) > 128 {
		return time.Time{}, errors.New("numeric timestamp exceeds 128 characters")
	}
	// Keep the common integer path exact and allocation-light, including
	// nanosecond epochs that cannot be represented by float64.
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		units := int64(1)
		switch {
		case v >= 1e17 || v <= -1e17:
			units = 1_000_000_000
		case v >= 1e14 || v <= -1e14:
			units = 1_000_000
		case v >= 1e11 || v <= -1e11:
			units = 1_000
		}
		return epochTime(v/units, (v%units)*(1_000_000_000/units))
	}
	// Decimal fractions and scientific notation are parsed exactly too. Bound
	// their size/exponent before big.Rat parsing to avoid unbounded allocation.
	if !decimalTimestamp.MatchString(s) {
		return time.Time{}, errors.New("expected RFC3339 or Unix epoch numeric timestamp")
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		exp, err := strconv.Atoi(s[i+1:])
		if err != nil || exp < -400 || exp > 400 {
			return time.Time{}, errors.New("timestamp exponent out of range")
		}
	}
	v, ok := new(big.Rat).SetString(s)
	if !ok || v.Cmp(new(big.Rat).SetInt64(math.MinInt64)) < 0 || v.Cmp(new(big.Rat).SetInt64(math.MaxInt64)) > 0 {
		return time.Time{}, errors.New("numeric timestamp outside signed 64-bit range")
	}
	av := new(big.Rat).Abs(v)
	nanosPerUnit := int64(1_000_000_000)
	switch {
	case av.Cmp(big.NewRat(1e17, 1)) >= 0:
		nanosPerUnit = 1
	case av.Cmp(big.NewRat(1e14, 1)) >= 0:
		nanosPerUnit = 1_000
	case av.Cmp(big.NewRat(1e11, 1)) >= 0:
		nanosPerUnit = 1_000_000
	}
	v.Mul(v, big.NewRat(nanosPerUnit, 1))
	// Round only precision finer than one nanosecond, with ties away from zero.
	nanos, remainder := new(big.Int), new(big.Int)
	nanos.QuoRem(v.Num(), v.Denom(), remainder)
	if new(big.Int).Lsh(new(big.Int).Abs(remainder), 1).Cmp(v.Denom()) >= 0 {
		nanos.Add(nanos, big.NewInt(int64(v.Sign())))
	}
	sec, nsec := new(big.Int), new(big.Int)
	sec.QuoRem(nanos, big.NewInt(1_000_000_000), nsec)
	if !sec.IsInt64() {
		return time.Time{}, errors.New("timestamp seconds out of range")
	}
	return epochTime(sec.Int64(), nsec.Int64())
}

func epochTime(sec, nsec int64) (time.Time, error) {
	t := time.Unix(sec, nsec).UTC()
	if t.Year() < 0 || t.Year() > 9999 {
		return time.Time{}, errors.New("timestamp outside RFC3339 year range")
	}
	return t, nil
}

func partitionIndex(id string, n int) int {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return int(h.Sum64() % uint64(n))
}
