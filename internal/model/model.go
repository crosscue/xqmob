package model

import (
	"time"

	"github.com/crosscue/xqmob/internal/core"
)

type Observation struct {
	EntityID         string
	TS               time.Time
	Lat              float64
	Lon              float64
	HA               float64
	HasHA            bool
	AccuracyState    string
	SourceRow        int64
	SampleCount      int
	SameTSScatterM   float64
	Geohash          string
	LatDecimalPlaces int
	LonDecimalPlaces int

	TrackID            string
	SegmentID          string
	PresenceIntervalID string

	StepDTS                float64
	StepDistanceM          float64
	StepEffectiveDistanceM float64
	StepAccuracyState      string
	ImpliedSpeedMPS        float64
	IsGap                  bool
	IsDiscontinuity        bool
	MovementState          string
	SpeedClass             string
}

type PresenceInterval struct {
	ID                    string
	TrackID               string
	SegmentID             string
	EntityID              string
	Start                 time.Time
	End                   time.Time
	DurationS             float64
	Classification        string
	CentroidLat           float64
	CentroidLon           float64
	ObservationCount      int
	ScatterM              float64
	MedianHAM             float64
	MinHAM                float64
	MaxHAM                float64
	HasHAStats            bool
	ObservationsWithHA    int
	ObservationsWithoutHA int
	HACoveragePct         float64
	AccuracySupport       string
	StartHAState          string
	EndHAState            string
	StartReason           string
	EndReason             string
	IntervalEventID       string
	EnterEventID          string
	LeaveEventID          string
	StartObsIndex         int
	EndObsIndex           int
}

type Transition struct {
	ID                               string
	EntityID                         string
	TrackID                          string
	SegmentID                        string
	OriginPresenceID                 string
	DestinationPresenceID            string
	Depart                           time.Time
	Arrive                           time.Time
	DurationS                        float64
	OriginLat                        float64
	OriginLon                        float64
	DestinationLat                   float64
	DestinationLon                   float64
	StraightLineDistanceM            float64
	ObservedPathDistanceM            float64
	ObservationCount                 int
	ObservationsWithHA               int
	ObservationsWithoutHA            int
	HACoveragePct                    float64
	AccuracySupport                  string
	OriginObservationsWithHA         int
	OriginObservationsWithoutHA      int
	OriginHACoveragePct              float64
	OriginAccuracySupport            string
	MovementObservationsWithHA       int
	MovementObservationsWithoutHA    int
	MovementHACoveragePct            float64
	MovementAccuracySupport          string
	DestinationObservationsWithHA    int
	DestinationObservationsWithoutHA int
	DestinationHACoveragePct         float64
	DestinationAccuracySupport       string
}

type Segment struct {
	ID                       string
	TrackID                  string
	EntityID                 string
	Index                    int
	Start                    time.Time
	End                      time.Time
	ObservationCount         int
	GapCount                 int
	DurationS                float64
	ObservedPathM            float64
	DisplacementM            float64
	MeanObservedSpeedMPS     float64
	MaxImpliedSpeedMPS       float64
	StartLat                 float64
	StartLon                 float64
	EndLat                   float64
	EndLon                   float64
	MinLat                   float64
	MinLon                   float64
	MaxLat                   float64
	MaxLon                   float64
	StartReason              string
	EndReason                string
	ObservationsWithHA       int
	ObservationsWithoutHA    int
	HACoveragePct            float64
	AccuracySupport          string
	StartHAState             string
	EndHAState               string
	EdgeBothKnownCount       int
	EdgePreviousMissingCount int
	EdgeCurrentMissingCount  int
	EdgeBothMissingCount     int
	Coordinates              [][2]float64 // lon,lat
	PathRuns                 [][][2]float64
}

type Track struct {
	ID                    string
	EntityID              string
	Subject               string
	Start                 time.Time
	End                   time.Time
	CoverageS             float64
	ObservationCount      int
	EventCount            int
	SegmentCount          int
	PresenceIntervalCount int
	TransitionCount       int
	GapCount              int
	DiscontinuityCount    int
	ObservedPathM         float64
	UniqueGeohashCells    int
	DwellCount            int
	StayCount             int
	TotalDwellS           float64
	MedianHAM             float64
	HasMedianHA           bool
	ObservationsWithHA    int
	ObservationsWithoutHA int
	HACoveragePct         float64
	HAClass               string
	StartLat              float64
	StartLon              float64
	EndLat                float64
	EndLon                float64
	Coordinates           [][2]float64
	PathRuns              [][][2]float64
}

type EntitySummary struct {
	EntityID                string
	Subject                 string
	FirstSeen               time.Time
	LastSeen                time.Time
	CoverageS               float64
	ObservationCount        int
	TrackCount              int
	SegmentCount            int
	PresenceCount           int
	TransitionCount         int
	StayCount               int
	DwellCount              int
	GapCount                int
	DiscontinuityCount      int
	TotalObservedPathM      float64
	TotalPresenceS          float64
	TotalDwellS             float64
	UniquePresenceLocations int
	UniqueGeohashCells      int
	MedianHAM               float64
	HasMedianHA             bool
	ObservationsWithHA      int
	ObservationsWithoutHA   int
	HACoveragePct           float64
	HAClass                 string
	StartLat                float64
	StartLon                float64
	EndLat                  float64
	EndLon                  float64
}

type EventizerDiagnostics struct {
	InputObservations                          int                       `json:"input_observations"`
	NormalizedObservations                     int                       `json:"normalized_observations"`
	SameTimestampCollapsed                     int                       `json:"same_timestamp_collapsed"`
	ContinuousEdges                            int                       `json:"continuous_edges"`
	GapEdges                                   int                       `json:"gap_edges"`
	DiscontinuityEdges                         int                       `json:"discontinuity_edges"`
	GapAndDiscontinuityEdges                   int                       `json:"gap_and_discontinuity_edges"`
	PresenceConfirmations                      int                       `json:"presence_confirmations"`
	PresenceStartDataset                       int                       `json:"presence_start_dataset"`
	PresenceStartContinuityReset               int                       `json:"presence_start_continuity_reset"`
	PresenceStartCandidate                     int                       `json:"presence_start_candidate"`
	PresenceStartMovementArrival               int                       `json:"presence_start_movement_arrival"`
	PresenceEndDataset                         int                       `json:"presence_end_dataset"`
	PresenceEndGap                             int                       `json:"presence_end_gap"`
	PresenceEndDiscontinuity                   int                       `json:"presence_end_discontinuity"`
	PresenceEndConfirmedMovement               int                       `json:"presence_end_confirmed_movement"`
	DepartureCandidateSequences                int                       `json:"departure_candidate_sequences"`
	DepartureCandidateOutsideFixes             int                       `json:"departure_candidate_outside_fixes"`
	DepartureCandidatesReturnedInside          int                       `json:"departure_candidates_returned_inside"`
	DepartureCandidateWindowExpired            int                       `json:"departure_candidate_window_expired"`
	DepartureCandidatesCancelledGap            int                       `json:"departure_candidates_cancelled_gap"`
	DepartureCandidatesCancelledDiscontinuity  int                       `json:"departure_candidates_cancelled_discontinuity"`
	DepartureCandidatesCancelledEOF            int                       `json:"departure_candidates_cancelled_eof"`
	DepartureCandidateUnaccounted              int                       `json:"departure_candidate_unaccounted"`
	DeparturesConfirmed                        int                       `json:"departures_confirmed"`
	PendingTransitionsStarted                  int                       `json:"pending_transitions_started"`
	TransitionsConfirmed                       int                       `json:"transitions_confirmed"`
	PendingTransitionsCancelledGap             int                       `json:"pending_transitions_cancelled_gap"`
	PendingTransitionsCancelledDiscontinuity   int                       `json:"pending_transitions_cancelled_discontinuity"`
	PendingTransitionsCancelledSegmentMismatch int                       `json:"pending_transitions_cancelled_segment_mismatch"`
	PendingTransitionsUnresolvedEOF            int                       `json:"pending_transitions_unresolved_eof"`
	PendingTransitionUnaccounted               int                       `json:"pending_transition_unaccounted"`
	EnterSuppressedNoPendingTransition         int                       `json:"enter_suppressed_no_pending_transition"`
	AccuracyAttribution                        map[string]map[string]int `json:"accuracy_attribution,omitempty"`
	DiscontinuityReasons                       map[string]int            `json:"discontinuity_reasons,omitempty"`
	DiscontinuityReasonAccuracy                map[string]map[string]int `json:"discontinuity_reason_accuracy,omitempty"`
}

func (d *EventizerDiagnostics) Add(o EventizerDiagnostics) {
	d.InputObservations += o.InputObservations
	d.NormalizedObservations += o.NormalizedObservations
	d.SameTimestampCollapsed += o.SameTimestampCollapsed
	d.ContinuousEdges += o.ContinuousEdges
	d.GapEdges += o.GapEdges
	d.DiscontinuityEdges += o.DiscontinuityEdges
	d.GapAndDiscontinuityEdges += o.GapAndDiscontinuityEdges
	d.PresenceConfirmations += o.PresenceConfirmations
	d.PresenceStartDataset += o.PresenceStartDataset
	d.PresenceStartContinuityReset += o.PresenceStartContinuityReset
	d.PresenceStartCandidate += o.PresenceStartCandidate
	d.PresenceStartMovementArrival += o.PresenceStartMovementArrival
	d.PresenceEndDataset += o.PresenceEndDataset
	d.PresenceEndGap += o.PresenceEndGap
	d.PresenceEndDiscontinuity += o.PresenceEndDiscontinuity
	d.PresenceEndConfirmedMovement += o.PresenceEndConfirmedMovement
	d.DepartureCandidateSequences += o.DepartureCandidateSequences
	d.DepartureCandidateOutsideFixes += o.DepartureCandidateOutsideFixes
	d.DepartureCandidatesReturnedInside += o.DepartureCandidatesReturnedInside
	d.DepartureCandidateWindowExpired += o.DepartureCandidateWindowExpired
	d.DepartureCandidatesCancelledGap += o.DepartureCandidatesCancelledGap
	d.DepartureCandidatesCancelledDiscontinuity += o.DepartureCandidatesCancelledDiscontinuity
	d.DepartureCandidatesCancelledEOF += o.DepartureCandidatesCancelledEOF
	d.DepartureCandidateUnaccounted += o.DepartureCandidateUnaccounted
	d.DeparturesConfirmed += o.DeparturesConfirmed
	d.PendingTransitionsStarted += o.PendingTransitionsStarted
	d.TransitionsConfirmed += o.TransitionsConfirmed
	d.PendingTransitionsCancelledGap += o.PendingTransitionsCancelledGap
	d.PendingTransitionsCancelledDiscontinuity += o.PendingTransitionsCancelledDiscontinuity
	d.PendingTransitionsCancelledSegmentMismatch += o.PendingTransitionsCancelledSegmentMismatch
	d.PendingTransitionsUnresolvedEOF += o.PendingTransitionsUnresolvedEOF
	d.PendingTransitionUnaccounted += o.PendingTransitionUnaccounted
	d.EnterSuppressedNoPendingTransition += o.EnterSuppressedNoPendingTransition
	if len(o.AccuracyAttribution) > 0 {
		if d.AccuracyAttribution == nil {
			d.AccuracyAttribution = map[string]map[string]int{}
		}
		for kind, states := range o.AccuracyAttribution {
			if d.AccuracyAttribution[kind] == nil {
				d.AccuracyAttribution[kind] = map[string]int{}
			}
			for state, n := range states {
				d.AccuracyAttribution[kind][state] += n
			}
		}
	}
	if len(o.DiscontinuityReasons) > 0 {
		if d.DiscontinuityReasons == nil {
			d.DiscontinuityReasons = map[string]int{}
		}
		for reason, n := range o.DiscontinuityReasons {
			d.DiscontinuityReasons[reason] += n
		}
	}
	if len(o.DiscontinuityReasonAccuracy) > 0 {
		if d.DiscontinuityReasonAccuracy == nil {
			d.DiscontinuityReasonAccuracy = map[string]map[string]int{}
		}
		for reason, states := range o.DiscontinuityReasonAccuracy {
			if d.DiscontinuityReasonAccuracy[reason] == nil {
				d.DiscontinuityReasonAccuracy[reason] = map[string]int{}
			}
			for state, n := range states {
				d.DiscontinuityReasonAccuracy[reason][state] += n
			}
		}
	}
}

func (d *EventizerDiagnostics) AddDiscontinuityReason(reason, accuracyState string) {
	if reason == "" {
		return
	}
	if d.DiscontinuityReasons == nil {
		d.DiscontinuityReasons = map[string]int{}
	}
	d.DiscontinuityReasons[reason]++
	if accuracyState == "" {
		return
	}
	if d.DiscontinuityReasonAccuracy == nil {
		d.DiscontinuityReasonAccuracy = map[string]map[string]int{}
	}
	if d.DiscontinuityReasonAccuracy[reason] == nil {
		d.DiscontinuityReasonAccuracy[reason] = map[string]int{}
	}
	d.DiscontinuityReasonAccuracy[reason][accuracyState]++
}

func (d EventizerDiagnostics) DepartureCandidateOutcomes() int {
	return d.DeparturesConfirmed + d.DepartureCandidatesReturnedInside + d.DepartureCandidateWindowExpired +
		d.DepartureCandidatesCancelledGap + d.DepartureCandidatesCancelledDiscontinuity + d.DepartureCandidatesCancelledEOF
}

func (d EventizerDiagnostics) PendingTransitionOutcomes() int {
	return d.TransitionsConfirmed + d.PendingTransitionsCancelledGap + d.PendingTransitionsCancelledDiscontinuity +
		d.PendingTransitionsCancelledSegmentMismatch + d.PendingTransitionsUnresolvedEOF
}

func (d *EventizerDiagnostics) AddAccuracyAttribution(kind, state string) {
	if kind == "" || state == "" {
		return
	}
	if d.AccuracyAttribution == nil {
		d.AccuracyAttribution = map[string]map[string]int{}
	}
	if d.AccuracyAttribution[kind] == nil {
		d.AccuracyAttribution[kind] = map[string]int{}
	}
	d.AccuracyAttribution[kind][state]++
}

func AccuracySupportClass(withHA, withoutHA int) string {
	switch {
	case withHA > 0 && withoutHA == 0:
		return "all_known"
	case withHA == 0 && withoutHA > 0:
		return "all_missing"
	case withHA > 0 && withoutHA > 0:
		return "mixed"
	default:
		return "none"
	}
}

type TraceRecord struct {
	EntityID                string
	ObservationIndex        int
	SourceRow               int64
	TS                      time.Time
	Lat                     float64
	Lon                     float64
	HA                      float64
	HasHA                   bool
	AccuracyState           string
	StepAccuracyState       string
	SampleCount             int
	SegmentID               string
	StepDTS                 float64
	StepDistanceM           float64
	EffectiveDistanceM      float64
	ImpliedSpeedMPS         float64
	IsGap                   bool
	IsDiscontinuity         bool
	PhaseBefore             string
	PhaseAfter              string
	MoveCandidatesBefore    int
	MoveCandidatesAfter     int
	PendingTransitionBefore bool
	PendingTransitionAfter  bool
	Decision                string
	EventsEmitted           string
	PresenceIntervalID      string
	Detail                  string
}

type EntityResult struct {
	Observations []Observation
	Events       []core.Event
	Segments     []Segment
	Presence     []PresenceInterval
	Transitions  []Transition
	Track        Track
	Entity       EntitySummary
	Diagnostics  EventizerDiagnostics
	Trace        []TraceRecord
}
