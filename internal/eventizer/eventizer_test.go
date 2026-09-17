package eventizer

import (
	"testing"
	"time"

	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/model"
)

func obs(id string, t time.Time, lat, lon, ha float64, row int64) model.Observation {
	return model.Observation{EntityID: id, TS: t, Lat: lat, Lon: lon, HA: ha, HasHA: true, AccuracyState: "known", SourceRow: row, SampleCount: 1}
}

func sourceEvents(r model.EntityResult) []string {
	out := make([]string, 0, len(r.Events))
	for _, e := range r.Events {
		v, _ := e.Context["source_event"].(string)
		out = append(out, v)
	}
	return out
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestStationaryProducesDwellWithoutBoundaryEnterLeave(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5, -0.1, 10, 2),
		obs("a", t0.Add(10*time.Minute), 51.5, -0.1, 10, 3),
		obs("a", t0.Add(20*time.Minute), 51.5, -0.1, 10, 4),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	events := sourceEvents(r)
	if !contains(events, "DWELL") {
		t.Fatalf("expected DWELL, got %v", events)
	}
	if contains(events, "ENTER") || contains(events, "LEAVE") {
		t.Fatalf("dataset boundaries must not invent ENTER/LEAVE: %v", events)
	}
	if len(r.Presence) != 1 || r.Presence[0].DurationS != 1200 {
		t.Fatalf("unexpected presence: %+v", r.Presence)
	}
}

func TestConfirmedMovementProducesLeaveEnterAndTransition(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5000, -0.1000, 5, 2),
		obs("a", t0.Add(5*time.Minute), 51.5000, -0.1000, 5, 3),
		obs("a", t0.Add(10*time.Minute), 51.5050, -0.1000, 5, 4),
		obs("a", t0.Add(12*time.Minute), 51.5060, -0.1000, 5, 5),
		obs("a", t0.Add(14*time.Minute), 51.5060, -0.1000, 5, 6),
		obs("a", t0.Add(20*time.Minute), 51.5060, -0.1000, 5, 7),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	events := sourceEvents(r)
	for _, want := range []string{"LEAVE", "ENTER"} {
		if !contains(events, want) {
			t.Fatalf("expected %s in %v", want, events)
		}
	}
	if len(r.Transitions) != 1 {
		t.Fatalf("expected one transition, got %+v", r.Transitions)
	}
	if r.Transitions[0].OriginPresenceID == "" || r.Transitions[0].DestinationPresenceID == "" {
		t.Fatal("transition must link presence intervals")
	}
	if len(r.Presence) < 2 || r.Presence[1].StartReason != "movement_arrival" {
		t.Fatalf("destination presence must preserve movement_arrival start reason: %+v", r.Presence)
	}
}

func TestGapClosesPresenceWithoutLeaveOrEnter(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5, -0.1, 5, 2),
		obs("a", t0.Add(10*time.Minute), 51.5, -0.1, 5, 3),
		obs("a", t0.Add(3*time.Hour), 51.5, -0.1, 5, 4),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	events := sourceEvents(r)
	if !contains(events, "GAP") {
		t.Fatalf("expected GAP: %v", events)
	}
	if contains(events, "LEAVE") || contains(events, "ENTER") {
		t.Fatalf("gap must not invent LEAVE/ENTER: %v", events)
	}
	if r.Track.SegmentCount != 1 {
		t.Fatalf("gap alone must not break segment: %+v", r.Track)
	}
}

func TestDiscontinuityBreaksSegment(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5, -0.1, 5, 2),
		obs("a", t0.Add(time.Minute), 51.5, -0.1, 5, 3),
		obs("a", t0.Add(61*time.Second), 52.0, -0.1, 5, 4),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(sourceEvents(r), "DISCONTINUITY") {
		t.Fatalf("expected discontinuity: %v", sourceEvents(r))
	}
	if len(r.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(r.Segments))
	}
	if contains(sourceEvents(r), "LEAVE") {
		t.Fatalf("discontinuity must not infer leave")
	}
	if r.Diagnostics.DiscontinuityReasons["jump_and_speed"] != 1 {
		t.Fatalf("unexpected discontinuity reasons: %+v", r.Diagnostics.DiscontinuityReasons)
	}
	if r.Diagnostics.DiscontinuityReasonAccuracy["jump_and_speed"]["both_known"] != 1 {
		t.Fatalf("unexpected reason/accuracy cross-tab: %+v", r.Diagnostics.DiscontinuityReasonAccuracy)
	}
}

func TestSameTimestampSelectsLowestHA(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5, -0.1, 50, 2),
		obs("a", t0, 51.5001, -0.1001, 5, 3),
		obs("a", t0.Add(time.Minute), 51.5001, -0.1001, 5, 4),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Observations) != 2 {
		t.Fatalf("expected collapse to 2 observations, got %d", len(r.Observations))
	}
	if r.Observations[0].HA != 5 || r.Observations[0].SourceRow != 3 || r.Observations[0].SampleCount != 2 {
		t.Fatalf("wrong representative: %+v", r.Observations[0])
	}
}

func TestStableIDs(t *testing.T) {
	a := StableIDForTest("evt", "a", "b")
	b := StableIDForTest("evt", "a", "b")
	c := StableIDForTest("evt", "a", "c")
	if a != b || a == c {
		t.Fatalf("unexpected stable id behavior: %s %s %s", a, b, c)
	}
}

func TestSemanticIDsIgnoreSourceRowPosition(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	makeInput := func(offset int64) []model.Observation {
		return []model.Observation{
			obs("a", t0, 51.5000, -0.1000, 5, 2+offset),
			obs("a", t0.Add(5*time.Minute), 51.5000, -0.1000, 5, 3+offset),
			obs("a", t0.Add(10*time.Minute), 51.5050, -0.1000, 5, 4+offset),
			obs("a", t0.Add(12*time.Minute), 51.5060, -0.1000, 5, 5+offset),
			obs("a", t0.Add(14*time.Minute), 51.5060, -0.1000, 5, 6+offset),
			obs("a", t0.Add(20*time.Minute), 51.5060, -0.1000, 5, 7+offset),
		}
	}
	a, err := Eventize(makeInput(0), cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Eventize(makeInput(100), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if a.Track.ID != b.Track.ID {
		t.Fatalf("track id changed with source row position: %s != %s", a.Track.ID, b.Track.ID)
	}
	if len(a.Events) != len(b.Events) {
		t.Fatalf("event count differs: %d != %d", len(a.Events), len(b.Events))
	}
	for i := range a.Events {
		if a.Events[i].ID != b.Events[i].ID {
			t.Fatalf("event id %d changed with source row position: %s != %s", i, a.Events[i].ID, b.Events[i].ID)
		}
	}
	if len(a.Presence) != len(b.Presence) || len(a.Transitions) != len(b.Transitions) {
		t.Fatalf("projection counts changed")
	}
	for i := range a.Presence {
		if a.Presence[i].ID != b.Presence[i].ID {
			t.Fatalf("presence id %d changed", i)
		}
	}
	for i := range a.Transitions {
		if a.Transitions[i].ID != b.Transitions[i].ID {
			t.Fatalf("transition id %d changed", i)
		}
	}
}

func TestTransitionDestinationMatchesPresenceInterval(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5000, -0.1000, 5, 2),
		obs("a", t0.Add(5*time.Minute), 51.5000, -0.1000, 5, 3),
		obs("a", t0.Add(10*time.Minute), 51.5050, -0.1000, 5, 4),
		obs("a", t0.Add(12*time.Minute), 51.5060, -0.1000, 5, 5),
		obs("a", t0.Add(14*time.Minute), 51.5060, -0.1000, 5, 6),
		obs("a", t0.Add(20*time.Minute), 51.5060, -0.1000, 5, 7),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Transitions) != 1 || len(r.Presence) != 2 {
		t.Fatalf("unexpected result: transitions=%d presence=%d", len(r.Transitions), len(r.Presence))
	}
	tr := r.Transitions[0]
	if tr.OriginPresenceID != r.Presence[0].ID || tr.DestinationPresenceID != r.Presence[1].ID {
		t.Fatalf("transition links do not match presence rows: %+v vs %s -> %s", tr, r.Presence[0].ID, r.Presence[1].ID)
	}
	if r.Presence[1].EnterEventID == "" {
		t.Fatal("destination presence should retain ENTER event id")
	}
	var enterFound bool
	for _, ev := range r.Events {
		if NormalizeEventSource(ev) == "ENTER" {
			enterFound = true
			if got, _ := ev.Context["presence_interval_id"].(string); got != r.Presence[1].ID {
				t.Fatalf("ENTER event presence_interval_id=%q want %q", got, r.Presence[1].ID)
			}
		}
	}
	if !enterFound {
		t.Fatal("missing ENTER event")
	}
}

func TestDiagnosticsConfirmedTransition(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5000, -0.1000, 5, 2),
		obs("a", t0.Add(5*time.Minute), 51.5000, -0.1000, 5, 3),
		obs("a", t0.Add(10*time.Minute), 51.5050, -0.1000, 5, 4),
		obs("a", t0.Add(12*time.Minute), 51.5060, -0.1000, 5, 5),
		obs("a", t0.Add(14*time.Minute), 51.5060, -0.1000, 5, 6),
		obs("a", t0.Add(20*time.Minute), 51.5060, -0.1000, 5, 7),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	d := r.Diagnostics
	if d.DeparturesConfirmed != 1 || d.PendingTransitionsStarted != 1 || d.TransitionsConfirmed != 1 {
		t.Fatalf("unexpected transition diagnostics: %+v", d)
	}
	if d.DepartureCandidateUnaccounted != 0 || d.PendingTransitionUnaccounted != 0 {
		t.Fatalf("diagnostic closure failed: %+v", d)
	}
	if d.PresenceEndConfirmedMovement != 1 {
		t.Fatalf("expected one movement-closed presence: %+v", d)
	}
}

func TestDiagnosticsPendingTransitionCancelledByGap(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5000, -0.1000, 5, 2),
		obs("a", t0.Add(5*time.Minute), 51.5000, -0.1000, 5, 3),
		obs("a", t0.Add(10*time.Minute), 51.5050, -0.1000, 5, 4),
		obs("a", t0.Add(12*time.Minute), 51.5060, -0.1000, 5, 5),
		obs("a", t0.Add(3*time.Hour), 51.5060, -0.1000, 5, 6),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	d := r.Diagnostics
	if d.DeparturesConfirmed != 1 || d.PendingTransitionsStarted != 1 || d.PendingTransitionsCancelledGap != 1 || d.TransitionsConfirmed != 0 {
		t.Fatalf("unexpected cancelled transition diagnostics: %+v", d)
	}
}

func TestCandidatePresenceSuppressesEnterWithoutPendingTransition(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5000, -0.1000, 5, 2),
		obs("a", t0.Add(5*time.Minute), 51.5020, -0.1000, 5, 3),
		obs("a", t0.Add(10*time.Minute), 51.5020, -0.1000, 5, 4),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if contains(sourceEvents(r), "ENTER") {
		t.Fatalf("candidate presence without prior confirmed departure must not emit ENTER: %v", sourceEvents(r))
	}
	if r.Diagnostics.EnterSuppressedNoPendingTransition != 1 {
		t.Fatalf("expected one suppressed ENTER diagnostic: %+v", r.Diagnostics)
	}
	if len(r.Presence) != 1 || r.Presence[0].EnterEventID != "" {
		t.Fatalf("unexpected presence ENTER linkage: %+v", r.Presence)
	}
}

func TestDepartureCandidateAccountingClosesAtGap(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5000, -0.1000, 5, 2),
		obs("a", t0.Add(5*time.Minute), 51.5000, -0.1000, 5, 3),
		obs("a", t0.Add(10*time.Minute), 51.5020, -0.1000, 5, 4),
		obs("a", t0.Add(3*time.Hour), 51.5020, -0.1000, 5, 5),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	d := r.Diagnostics
	if d.DepartureCandidateSequences != 1 || d.DepartureCandidatesCancelledGap != 1 || d.DepartureCandidateUnaccounted != 0 {
		t.Fatalf("candidate gap accounting did not close: %+v", d)
	}
}

func TestDepartureCandidateAccountingClosesAtDiscontinuity(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5000, -0.1000, 5, 2),
		obs("a", t0.Add(5*time.Minute), 51.5000, -0.1000, 5, 3),
		obs("a", t0.Add(10*time.Minute), 51.5020, -0.1000, 5, 4),
		obs("a", t0.Add(10*time.Minute+time.Second), 52.5000, -0.1000, 5, 5),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	d := r.Diagnostics
	if d.DepartureCandidateSequences != 1 || d.DepartureCandidatesCancelledDiscontinuity != 1 || d.DepartureCandidateUnaccounted != 0 {
		t.Fatalf("candidate discontinuity accounting did not close: %+v", d)
	}
}

func TestDepartureCandidateAccountingClosesAtEOF(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5000, -0.1000, 5, 2),
		obs("a", t0.Add(5*time.Minute), 51.5000, -0.1000, 5, 3),
		obs("a", t0.Add(10*time.Minute), 51.5020, -0.1000, 5, 4),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	d := r.Diagnostics
	if d.DepartureCandidateSequences != 1 || d.DepartureCandidatesCancelledEOF != 1 || d.DepartureCandidateUnaccounted != 0 {
		t.Fatalf("candidate EOF accounting did not close: %+v", d)
	}
}

func TestTargetedTraceRecordsDecisions(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	cfg.TraceIDs = []string{"a"}
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5000, -0.1000, 5, 2),
		obs("a", t0.Add(5*time.Minute), 51.5000, -0.1000, 5, 3),
		obs("a", t0.Add(10*time.Minute), 51.5050, -0.1000, 5, 4),
		obs("a", t0.Add(12*time.Minute), 51.5060, -0.1000, 5, 5),
		obs("a", t0.Add(14*time.Minute), 51.5060, -0.1000, 5, 6),
	}
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Trace) != len(r.Observations) {
		t.Fatalf("trace rows=%d want %d", len(r.Trace), len(r.Observations))
	}
	var found bool
	for _, tr := range r.Trace {
		if tr.Decision == "departure_confirmed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("trace missing departure confirmation: %+v", r.Trace)
	}

	cfg.TraceIDs = []string{"other"}
	r, err = Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Trace) != 0 {
		t.Fatalf("unexpected trace for unselected entity: %+v", r.Trace)
	}
}

func TestMissingHAUsesOnlyKnownRadius(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	cfg.HAPolicy = "allow-missing"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	a := obs("a", t0, 51.5000, -0.1000, 50, 2)
	b := model.Observation{EntityID: "a", TS: t0.Add(time.Minute), Lat: 51.5020, Lon: -0.1000, HasHA: false, AccuracyState: "missing", SourceRow: 3, SampleCount: 1}
	r, err := Eventize([]model.Observation{a, b}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Observations) != 2 {
		t.Fatalf("observations=%d", len(r.Observations))
	}
	o := r.Observations[1]
	if o.StepAccuracyState != "current_missing" {
		t.Fatalf("step accuracy state=%q", o.StepAccuracyState)
	}
	want := o.StepDistanceM - 50
	if want < 0 {
		want = 0
	}
	if diff := o.StepEffectiveDistanceM - want; diff < -0.001 || diff > 0.001 {
		t.Fatalf("effective distance %.3f want %.3f (raw %.3f)", o.StepEffectiveDistanceM, want, o.StepDistanceM)
	}
	for _, ev := range r.Events {
		if NormalizeEventSource(ev) == "END" && ev.Location != nil && ev.Location.AccuracyM != nil {
			t.Fatalf("missing HA was imputed into Core location: %+v", ev.Location)
		}
	}
}

func TestSameTimestampPrefersKnownHAOverMissing(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	cfg.HAPolicy = "allow-missing"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	missing := model.Observation{EntityID: "a", TS: t0, Lat: 51.4990, Lon: -0.1000, HasHA: false, AccuracyState: "missing", SourceRow: 2, SampleCount: 1}
	known := obs("a", t0, 51.5000, -0.1000, 500, 3)
	later := obs("a", t0.Add(time.Minute), 51.5000, -0.1000, 5, 4)
	r, err := Eventize([]model.Observation{missing, known, later}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Observations) != 2 || !r.Observations[0].HasHA || r.Observations[0].SourceRow != 3 {
		t.Fatalf("known HA was not preferred at identical timestamp: %+v", r.Observations)
	}
}

func TestAccuracySupportIsAttributedWithoutFiltering(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5000, -0.1000, 5, 2),
		obs("a", t0.Add(5*time.Minute), 51.5000, -0.1000, 5, 3),
		obs("a", t0.Add(10*time.Minute), 51.5050, -0.1000, 5, 4),
		obs("a", t0.Add(12*time.Minute), 51.5060, -0.1000, 5, 5),
		obs("a", t0.Add(14*time.Minute), 51.5060, -0.1000, 5, 6),
		obs("a", t0.Add(20*time.Minute), 51.5060, -0.1000, 5, 7),
	}
	in[3].HasHA = false
	in[3].HA = 0
	in[3].AccuracyState = "missing"

	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.Entity.HAClass != "mixed" {
		t.Fatalf("entity HA class=%q want mixed", r.Entity.HAClass)
	}
	if len(r.Transitions) != 1 || r.Transitions[0].AccuracySupport != "mixed" {
		t.Fatalf("transition accuracy support unexpected: %+v", r.Transitions)
	}
	tr := r.Transitions[0]
	if tr.OriginAccuracySupport != "all_known" {
		t.Fatalf("origin support=%q want all_known: %+v", tr.OriginAccuracySupport, tr)
	}
	if tr.MovementAccuracySupport != tr.AccuracySupport || tr.MovementHACoveragePct != tr.HACoveragePct {
		t.Fatalf("legacy transition support must remain movement-window compatible: %+v", tr)
	}
	if tr.DestinationAccuracySupport != "mixed" || tr.DestinationObservationsWithoutHA == 0 {
		t.Fatalf("destination support was not enriched from closed presence: %+v", tr)
	}
	if r.Diagnostics.AccuracyAttribution["transitions_confirmed"]["mixed"] != 1 {
		t.Fatalf("transition attribution missing: %+v", r.Diagnostics.AccuracyAttribution)
	}
	if r.Diagnostics.AccuracyAttribution["departure_candidates"]["current_missing"] == 0 &&
		r.Diagnostics.AccuracyAttribution["departure_candidates"]["previous_missing"] == 0 &&
		r.Diagnostics.AccuracyAttribution["departure_candidates"]["both_known"] == 0 {
		t.Fatalf("departure candidate attribution missing: %+v", r.Diagnostics.AccuracyAttribution)
	}
}

func TestSegmentAccuracyEdgeAccounting(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "test"
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	in := []model.Observation{
		obs("a", t0, 51.5000, -0.1000, 5, 2),
		obs("a", t0.Add(time.Minute), 51.5001, -0.1000, 5, 3),
		obs("a", t0.Add(2*time.Minute), 51.5002, -0.1000, 5, 4),
		obs("a", t0.Add(3*time.Minute), 51.5003, -0.1000, 5, 5),
	}
	in[1].HasHA = false
	in[1].HA = 0
	in[1].AccuracyState = "missing"
	in[2].HasHA = false
	in[2].HA = 0
	in[2].AccuracyState = "missing"
	r, err := Eventize(in, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Segments) != 1 {
		t.Fatalf("segments=%d want 1", len(r.Segments))
	}
	sg := r.Segments[0]
	gotEdges := sg.EdgeBothKnownCount + sg.EdgePreviousMissingCount + sg.EdgeCurrentMissingCount + sg.EdgeBothMissingCount
	if gotEdges != sg.ObservationCount-1 {
		t.Fatalf("edge support counts=%d want %d: %+v", gotEdges, sg.ObservationCount-1, sg)
	}
	if sg.EdgeCurrentMissingCount != 1 || sg.EdgeBothMissingCount != 1 || sg.EdgePreviousMissingCount != 1 {
		t.Fatalf("unexpected edge support distribution: %+v", sg)
	}
	if sg.StartHAState != "known" || sg.EndHAState != "known" {
		t.Fatalf("segment endpoint HA state unexpected: %+v", sg)
	}
}

func TestDiscontinuityReasonClassification(t *testing.T) {
	cfg := config.Default()
	cases := []struct {
		name string
		obs  model.Observation
		want string
	}{
		{"jump-only", model.Observation{StepEffectiveDistanceM: cfg.MaxJumpM + 1, StepDTS: 2000, ImpliedSpeedMPS: 25}, "jump_only"},
		{"speed-only", model.Observation{StepEffectiveDistanceM: 1000, StepDTS: 1, ImpliedSpeedMPS: cfg.MaxSpeedMPS + 1}, "speed_only"},
		{"both", model.Observation{StepEffectiveDistanceM: cfg.MaxJumpM + 1, StepDTS: 1, ImpliedSpeedMPS: cfg.MaxSpeedMPS + 1000}, "jump_and_speed"},
		{"none", model.Observation{StepEffectiveDistanceM: 100, StepDTS: 10, ImpliedSpeedMPS: 10}, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := discontinuityReason(tc.obs, cfg); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
