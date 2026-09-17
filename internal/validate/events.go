package validate

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/core"
)

type Report struct {
	Events int
	Errors []string
}

func EventsFile(path string) (Report, error) {
	var report Report
	f, err := os.Open(path)
	if err != nil {
		return report, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	s.Buffer(buf, 8*1024*1024)
	line := 0
	for s.Scan() {
		line++
		if len(s.Bytes()) == 0 {
			continue
		}
		var e core.Event
		if err := json.Unmarshal(s.Bytes(), &e); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("line %d: invalid JSON: %v", line, err))
			continue
		}
		report.Events++
		for _, msg := range Event(e) {
			report.Errors = append(report.Errors, fmt.Sprintf("line %d (%s): %s", line, e.ID, msg))
		}
	}
	if err := s.Err(); err != nil {
		return report, err
	}
	if len(report.Errors) > 0 {
		return report, errors.New("validation failed")
	}
	return report, nil
}

func Event(e core.Event) []string {
	var errs []string
	if e.XQVersion != config.WireVersion {
		errs = append(errs, "xq_version must be 0.1")
	}
	if e.ID == "" || e.Source == "" || e.EventTime == "" || e.Modality == "" || e.Class == "" || e.Feature == "" || e.Action == "" {
		errs = append(errs, "missing required Core field")
	}
	if e.Modality != config.Modality {
		errs = append(errs, "modality must be xq:mobility")
	}
	if e.Profile != config.ProfileID {
		errs = append(errs, "profile must be xq.mob:profile-0.1")
	}
	if e.Class != "transition" {
		errs = append(errs, "reference eventizer events must use class=transition")
	}
	et, err := time.Parse(time.RFC3339Nano, e.EventTime)
	if err != nil {
		errs = append(errs, "event_time is not RFC3339")
	}
	if e.EndTime != "" {
		if end, err := time.Parse(time.RFC3339Nano, e.EndTime); err != nil || (!et.IsZero() && end.Before(et)) {
			errs = append(errs, "invalid end_time")
		}
	}
	if e.Provenance == nil {
		return append(errs, "missing provenance")
	}
	if e.Provenance.Producer == "" || e.Provenance.ProducerVersion == "" {
		errs = append(errs, "missing provenance producer/version")
	}
	for _, key := range []string{"geohash_precision", "move_radius_m", "dwell_threshold_s", "gap_threshold_s", "max_speed_mps", "max_jump_m", "confirm_moves", "confirm_window_s", "walk_max_speed_mps", "walk_max_jump_m"} {
		if _, ok := e.Provenance.Parameters[key]; !ok {
			errs = append(errs, "missing provenance parameter "+key)
		}
	}
	se, _ := e.Context["source_event"].(string)
	expected := map[string]struct {
		f, a, s string
		p       int
	}{
		"START": {"xq:track", "xq:start", "xq:active", 1}, "STAY": {"xq:presence", "xq.mob:stay", "xq:present", 0},
		"DWELL": {"xq:presence", "xq.mob:dwell", "xq:present", 0}, "LEAVE": {"xq:presence", "xq:leave", "xq:absent", -1},
		"ENTER": {"xq:presence", "xq:enter", "xq:present", 1}, "GAP": {"xq:observation", "xq:gap", "xq:interrupted", 0},
		"DISCONTINUITY": {"xq:trajectory", "xq:discontinuity", "xq:discontinuous", 0}, "END": {"xq:track", "xq:end", "xq:inactive", -1},
	}
	m, ok := expected[se]
	if !ok {
		errs = append(errs, "missing or unknown context.source_event")
		return errs
	}
	if e.Feature != m.f || e.Action != m.a || e.State != m.s || e.Polarity != m.p {
		errs = append(errs, "profile mapping feature/action/state/polarity mismatch")
	}
	if se == "STAY" || se == "DWELL" {
		if e.Magnitude == nil || e.Unit != "s" || e.EndTime == "" {
			errs = append(errs, "STAY/DWELL require magnitude seconds and end_time")
		} else {
			end, _ := time.Parse(time.RFC3339Nano, e.EndTime)
			dur := end.Sub(et).Seconds()
			if math.Abs(dur-*e.Magnitude) > 1e-6 {
				errs = append(errs, "interval magnitude does not equal end_time-event_time")
			}
			threshold, _ := number(e.Provenance.Parameters["dwell_threshold_s"])
			if se == "STAY" && *e.Magnitude >= threshold {
				errs = append(errs, "STAY duration is not strictly below dwell threshold")
			}
			if se == "DWELL" && *e.Magnitude < threshold {
				errs = append(errs, "DWELL duration is below dwell threshold")
			}
		}
	}
	if se == "GAP" && (e.Magnitude == nil || e.Unit != "s") {
		errs = append(errs, "GAP requires magnitude in seconds")
	}
	if se == "DISCONTINUITY" && (e.Magnitude == nil || e.Unit != "m") {
		errs = append(errs, "DISCONTINUITY requires magnitude in metres")
	}
	return errs
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	default:
		return 0, false
	}
}
