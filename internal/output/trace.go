package output

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/crosscue/xqmob/internal/core"
	"github.com/crosscue/xqmob/internal/model"
)

// WriteTrace writes a targeted per-entity eventizer decision trace and returns
// its slash-separated path relative to the output root.
func WriteTrace(root string, records []model.TraceRecord) (string, error) {
	if len(records) == 0 {
		return "", nil
	}
	entityID := records[0].EntityID
	dir := filepath.Join(root, "diagnostics", "traces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := traceFileName(entityID)
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	header := []string{
		"entity_id", "observation_index", "source_row", "ts", "lat", "lon", "ha_m", "ha_state", "step_accuracy_state", "sample_count",
		"segment_id", "presence_interval_id", "step_dt_s", "step_distance_m", "effective_distance_m", "implied_speed_mps",
		"is_gap", "is_discontinuity", "phase_before", "phase_after", "move_candidates_before", "move_candidates_after",
		"pending_transition_before", "pending_transition_after", "decision", "events_emitted", "detail",
	}
	if err := w.Write(header); err != nil {
		return "", err
	}
	for _, r := range records {
		ha := ""
		if r.HasHA {
			ha = fmt.Sprintf("%.3f", r.HA)
		}
		row := []string{
			r.EntityID,
			strconv.Itoa(r.ObservationIndex),
			strconv.FormatInt(r.SourceRow, 10),
			core.RFC3339(r.TS),
			fmt.Sprintf("%.9f", r.Lat),
			fmt.Sprintf("%.9f", r.Lon),
			ha,
			r.AccuracyState,
			r.StepAccuracyState,
			strconv.Itoa(r.SampleCount),
			r.SegmentID,
			r.PresenceIntervalID,
			fmt.Sprintf("%.3f", r.StepDTS),
			fmt.Sprintf("%.3f", r.StepDistanceM),
			fmt.Sprintf("%.3f", r.EffectiveDistanceM),
			fmt.Sprintf("%.6f", r.ImpliedSpeedMPS),
			strconv.FormatBool(r.IsGap),
			strconv.FormatBool(r.IsDiscontinuity),
			r.PhaseBefore,
			r.PhaseAfter,
			strconv.Itoa(r.MoveCandidatesBefore),
			strconv.Itoa(r.MoveCandidatesAfter),
			strconv.FormatBool(r.PendingTransitionBefore),
			strconv.FormatBool(r.PendingTransitionAfter),
			r.Decision,
			r.EventsEmitted,
			r.Detail,
		}
		if err := w.Write(row); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("diagnostics", "traces", name)), nil
}

func traceFileName(entityID string) string {
	var b strings.Builder
	for _, r := range entityID {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
		if b.Len() >= 64 {
			break
		}
	}
	base := strings.Trim(b.String(), "._-")
	if base == "" {
		base = "entity"
	}
	sum := sha256.Sum256([]byte(entityID))
	suffix := hex.EncodeToString(sum[:4])
	return fmt.Sprintf("%s-%s.csv", base, suffix)
}
