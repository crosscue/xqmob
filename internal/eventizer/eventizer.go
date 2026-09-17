package eventizer

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/core"
	"github.com/crosscue/xqmob/internal/geo"
	"github.com/crosscue/xqmob/internal/idgen"
	"github.com/crosscue/xqmob/internal/model"
)

type phase int

const (
	seekingPresence phase = iota
	present
)

type presenceState struct {
	phase          phase
	anchorIdx      int
	members        []int
	lastInsideIdx  int
	moveCandidates []int
	startReason    string
	enterEventID   string
}

type pendingTransition struct {
	origin       model.PresenceInterval
	departIdx    int
	leaveEventID string
}

func Eventize(sortedInput []model.Observation, cfg config.Config) (model.EntityResult, error) {
	var result model.EntityResult
	if len(sortedInput) == 0 {
		return result, nil
	}
	diag := model.EventizerDiagnostics{InputObservations: len(sortedInput)}
	obs := collapseSameTimestamp(sortedInput)
	diag.NormalizedObservations = len(obs)
	diag.SameTimestampCollapsed = len(sortedInput) - len(obs)
	entityID := obs[0].EntityID
	for i := range obs {
		if obs[i].EntityID != entityID {
			return result, fmt.Errorf("eventizer received multiple entity ids: %q and %q", entityID, obs[i].EntityID)
		}
		obs[i].Geohash = geo.Geohash(obs[i].Lat, obs[i].Lon, cfg.GeohashPrecision)
	}

	trackID := idgen.Stable("trk", cfg.Source, entityID)
	subject := cfg.SubjectPrefix + entityID
	assignTrajectoryMetrics(obs, trackID, cfg)
	for i := 1; i < len(obs); i++ {
		if !obs[i].IsGap && !obs[i].IsDiscontinuity {
			diag.ContinuousEdges++
			diag.AddAccuracyAttribution("continuous_edges", obs[i].StepAccuracyState)
		}
		if obs[i].IsGap {
			diag.GapEdges++
			diag.AddAccuracyAttribution("gap_edges", obs[i].StepAccuracyState)
		}
		if obs[i].IsDiscontinuity {
			diag.DiscontinuityEdges++
			diag.AddAccuracyAttribution("discontinuity_edges", obs[i].StepAccuracyState)
			diag.AddDiscontinuityReason(discontinuityReason(obs[i], cfg), obs[i].StepAccuracyState)
		}
		if obs[i].IsGap && obs[i].IsDiscontinuity {
			diag.GapAndDiscontinuityEdges++
		}
	}

	segments := buildSegments(obs, trackID)
	events := make([]core.Event, 0, len(obs)/2+4)
	presenceIntervals := make([]model.PresenceInterval, 0)
	transitions := make([]model.Transition, 0)
	traceEnabled := cfg.ShouldTrace(entityID)
	trace := make([]model.TraceRecord, 0)

	events = append(events, makeSimpleEvent("START", obs[0].TS, obs[0], cfg, subject, trackID, obs[0].SegmentID, nil, ""))

	state := presenceState{
		phase:       seekingPresence,
		anchorIdx:   0,
		members:     []int{0},
		startReason: "dataset_start",
	}
	var pending *pendingTransition

	recordPresenceStart := func(reason string) {
		diag.PresenceConfirmations++
		switch reason {
		case "dataset_start":
			diag.PresenceStartDataset++
		case "continuity_reset":
			diag.PresenceStartContinuityReset++
		case "candidate_presence":
			diag.PresenceStartCandidate++
		case "movement_arrival":
			diag.PresenceStartMovementArrival++
		}
	}
	recordPresenceEnd := func(reason string) {
		switch reason {
		case "dataset_end":
			diag.PresenceEndDataset++
		case "gap":
			diag.PresenceEndGap++
		case "discontinuity":
			diag.PresenceEndDiscontinuity++
		case "confirmed_movement":
			diag.PresenceEndConfirmedMovement++
		}
	}
	startPendingTransition := func(p model.PresenceInterval, departIdx int, leaveEventID string) {
		pending = &pendingTransition{origin: p, departIdx: departIdx, leaveEventID: leaveEventID}
		diag.PendingTransitionsStarted++
	}
	appendTrace := func(i int, beforePhase string, beforeCandidates int, beforePending bool, decision, detail string, eventStart int) {
		if !traceEnabled {
			return
		}
		trace = append(trace, model.TraceRecord{
			EntityID: entityID, ObservationIndex: i + 1, SourceRow: obs[i].SourceRow, TS: obs[i].TS,
			Lat: obs[i].Lat, Lon: obs[i].Lon, HA: obs[i].HA, HasHA: obs[i].HasHA, AccuracyState: obs[i].AccuracyState, StepAccuracyState: obs[i].StepAccuracyState, SampleCount: obs[i].SampleCount,
			SegmentID: obs[i].SegmentID, StepDTS: obs[i].StepDTS, StepDistanceM: obs[i].StepDistanceM,
			EffectiveDistanceM: obs[i].StepEffectiveDistanceM, ImpliedSpeedMPS: obs[i].ImpliedSpeedMPS,
			IsGap: obs[i].IsGap, IsDiscontinuity: obs[i].IsDiscontinuity,
			PhaseBefore: beforePhase, PhaseAfter: phaseName(state.phase), MoveCandidatesBefore: beforeCandidates,
			MoveCandidatesAfter: len(state.moveCandidates), PendingTransitionBefore: beforePending, PendingTransitionAfter: pending != nil,
			Decision: decision, EventsEmitted: emittedEventNames(events[eventStart:]), Detail: detail,
		})
	}

	if traceEnabled {
		trace = append(trace, model.TraceRecord{
			EntityID: entityID, ObservationIndex: 1, SourceRow: obs[0].SourceRow, TS: obs[0].TS,
			Lat: obs[0].Lat, Lon: obs[0].Lon, HA: obs[0].HA, HasHA: obs[0].HasHA, AccuracyState: obs[0].AccuracyState, StepAccuracyState: obs[0].StepAccuracyState, SampleCount: obs[0].SampleCount,
			SegmentID: obs[0].SegmentID, PhaseBefore: "none", PhaseAfter: phaseName(state.phase),
			Decision: "dataset_start", EventsEmitted: "START", Detail: "initialized presence search",
		})
	}

	for i := 1; i < len(obs); i++ {
		beforePhase := phaseName(state.phase)
		beforeCandidates := len(state.moveCandidates)
		beforePending := pending != nil
		eventStart := len(events)
		decision := ""
		detail := ""

		if obs[i].IsGap || obs[i].IsDiscontinuity {
			if state.phase == present && len(state.moveCandidates) > 0 {
				if obs[i].IsDiscontinuity {
					diag.DepartureCandidatesCancelledDiscontinuity++
				} else {
					diag.DepartureCandidatesCancelledGap++
				}
			}
			if state.phase == present {
				reason := "gap"
				if obs[i].IsDiscontinuity {
					reason = "discontinuity"
				}
				interval, intervalEvent := closePresence(&state, obs, cfg, subject, trackID, reason, false)
				if interval.ID != "" {
					presenceIntervals = append(presenceIntervals, interval)
					events = append(events, intervalEvent)
					recordPresenceEnd(reason)
				}
			}
			if pending != nil {
				if obs[i].IsDiscontinuity {
					diag.PendingTransitionsCancelledDiscontinuity++
				} else {
					diag.PendingTransitionsCancelledGap++
				}
			}
			pending = nil
			state = presenceState{phase: seekingPresence, anchorIdx: i, members: []int{i}, startReason: "continuity_reset"}

			if obs[i].IsGap {
				prev := obs[i-1]
				events = append(events, makeGapEvent(prev, obs[i], cfg, subject, trackID))
			}
			if obs[i].IsDiscontinuity {
				prev := obs[i-1]
				events = append(events, makeDiscontinuityEvent(prev, obs[i], cfg, subject, trackID))
			}
			switch {
			case obs[i].IsGap && obs[i].IsDiscontinuity:
				decision = "continuity_break_gap_discontinuity"
			case obs[i].IsDiscontinuity:
				decision = "continuity_break_discontinuity"
			default:
				decision = "continuity_break_gap"
			}
			appendTrace(i, beforePhase, beforeCandidates, beforePending, decision, detail, eventStart)
			continue
		}

		switch state.phase {
		case seekingPresence:
			if withinPresence(obs[state.anchorIdx], obs[i], cfg) {
				state.members = append(state.members, i)
				state.lastInsideIdx = i
				if len(state.members) >= 2 {
					state.phase = present
					recordPresenceStart(state.startReason)
					switch state.startReason {
					case "movement_arrival":
						if pending != nil && pending.origin.SegmentID == obs[state.anchorIdx].SegmentID {
							enter := makeEnterEvent(obs[state.anchorIdx], cfg, subject, trackID, obs[state.anchorIdx].SegmentID)
							state.enterEventID = enter.ID
							events = append(events, enter)
							transition := makeTransition(*pending, state.anchorIdx, obs, trackID, entityID)
							transitions = append(transitions, transition)
							diag.TransitionsConfirmed++
							diag.AddAccuracyAttribution("transitions_confirmed", transition.AccuracySupport)
							decision = "presence_confirmed_arrival_transition"
						} else if pending != nil {
							diag.PendingTransitionsCancelledSegmentMismatch++
							decision = "presence_confirmed_arrival_segment_mismatch"
						} else {
							diag.EnterSuppressedNoPendingTransition++
							decision = "presence_confirmed_enter_suppressed"
						}
						pending = nil
					case "candidate_presence":
						diag.EnterSuppressedNoPendingTransition++
						decision = "presence_confirmed_enter_suppressed"
					default:
						decision = "presence_confirmed_" + state.startReason
					}
				} else {
					decision = "presence_support_added"
				}
			} else {
				state.anchorIdx = i
				state.members = []int{i}
				state.lastInsideIdx = i
				if pending != nil {
					state.startReason = "movement_arrival"
				} else {
					state.startReason = "candidate_presence"
				}
				decision = "presence_candidate_reanchored"
				detail = "new anchor outside previous presence search radius"
			}

		case present:
			anchor := obs[state.anchorIdx]
			if withinPresence(anchor, obs[i], cfg) {
				if len(state.moveCandidates) > 0 {
					diag.DepartureCandidatesReturnedInside++
					decision = "departure_candidate_returned_inside"
				} else {
					decision = "presence_inside"
				}
				state.members = append(state.members, i)
				state.lastInsideIdx = i
				state.moveCandidates = state.moveCandidates[:0]
			} else if !cfg.ConfirmMoves {
				interval, intervalEvent := closePresence(&state, obs, cfg, subject, trackID, "confirmed_movement", true)
				presenceIntervals = append(presenceIntervals, interval)
				events = append(events, intervalEvent)
				recordPresenceEnd("confirmed_movement")
				leave := makeLeaveEvent(obs[interval.EndObsIndex], interval, cfg, subject)
				presenceIntervals[len(presenceIntervals)-1].LeaveEventID = leave.ID
				events = append(events, leave)
				diag.DeparturesConfirmed++
				diag.AddAccuracyAttribution("departures_confirmed", obs[i].StepAccuracyState)
				startPendingTransition(presenceIntervals[len(presenceIntervals)-1], interval.EndObsIndex, leave.ID)
				state = presenceState{phase: seekingPresence, anchorIdx: i, members: []int{i}, lastInsideIdx: i, startReason: "movement_arrival"}
				decision = "departure_confirmed_unconfirmed_mode"
			} else {
				diag.DepartureCandidateOutsideFixes++
				if len(state.moveCandidates) == 0 {
					diag.DepartureCandidateSequences++
					diag.AddAccuracyAttribution("departure_candidates", obs[i].StepAccuracyState)
					state.moveCandidates = append(state.moveCandidates, i)
					decision = "departure_candidate_started"
				} else {
					firstCandidate := state.moveCandidates[0]
					if obs[i].TS.Sub(obs[firstCandidate].TS).Seconds() > cfg.ConfirmWindowS {
						diag.DepartureCandidateWindowExpired++
						diag.DepartureCandidateSequences++
						diag.AddAccuracyAttribution("departure_candidates", obs[i].StepAccuracyState)
						state.moveCandidates = []int{i}
						decision = "departure_candidate_window_expired_restart"
					} else {
						interval, intervalEvent := closePresence(&state, obs, cfg, subject, trackID, "confirmed_movement", true)
						presenceIntervals = append(presenceIntervals, interval)
						events = append(events, intervalEvent)
						recordPresenceEnd("confirmed_movement")
						leave := makeLeaveEvent(obs[interval.EndObsIndex], interval, cfg, subject)
						presenceIntervals[len(presenceIntervals)-1].LeaveEventID = leave.ID
						events = append(events, leave)
						diag.DeparturesConfirmed++
						diag.AddAccuracyAttribution("departures_confirmed", obs[i].StepAccuracyState)
						startPendingTransition(presenceIntervals[len(presenceIntervals)-1], interval.EndObsIndex, leave.ID)
						state = presenceState{phase: seekingPresence, anchorIdx: i, members: []int{i}, lastInsideIdx: i, startReason: "movement_arrival"}
						decision = "departure_confirmed"
					}
				}
			}
		}
		appendTrace(i, beforePhase, beforeCandidates, beforePending, decision, detail, eventStart)
	}

	finalEventStart := len(events)
	if state.phase == present {
		if len(state.moveCandidates) > 0 {
			diag.DepartureCandidatesCancelledEOF++
		}
		interval, intervalEvent := closePresence(&state, obs, cfg, subject, trackID, "dataset_end", false)
		if interval.ID != "" {
			presenceIntervals = append(presenceIntervals, interval)
			events = append(events, intervalEvent)
			recordPresenceEnd("dataset_end")
		}
	}
	if pending != nil {
		diag.PendingTransitionsUnresolvedEOF++
	}

	events = append(events, makeSimpleEvent("END", obs[len(obs)-1].TS, obs[len(obs)-1], cfg, subject, trackID, obs[len(obs)-1].SegmentID, nil, ""))
	if traceEnabled && len(trace) > 0 {
		last := &trace[len(trace)-1]
		last.Decision = appendDecision(last.Decision, "dataset_end")
		last.EventsEmitted = appendEventNames(last.EventsEmitted, emittedEventNames(events[finalEventStart:]))
		last.PendingTransitionAfter = pending != nil
	}
	sortEvents(events)

	for pi := range presenceIntervals {
		p := &presenceIntervals[pi]
		for oi := p.StartObsIndex; oi <= p.EndObsIndex; oi++ {
			if obs[oi].SegmentID == p.SegmentID && withinCentroid(obs[oi], p.CentroidLat, p.CentroidLon, cfg.MoveRadiusM) {
				obs[oi].PresenceIntervalID = p.ID
			}
		}
	}
	if traceEnabled {
		for i := range trace {
			idx := trace[i].ObservationIndex - 1
			if idx >= 0 && idx < len(obs) {
				trace[i].PresenceIntervalID = obs[idx].PresenceIntervalID
			}
		}
	}

	diag.DepartureCandidateUnaccounted = diag.DepartureCandidateSequences - diag.DepartureCandidateOutcomes()
	diag.PendingTransitionUnaccounted = diag.PendingTransitionsStarted - diag.PendingTransitionOutcomes()
	enrichTransitionDestinationSupport(transitions, presenceIntervals)
	track := buildTrack(obs, events, segments, presenceIntervals, transitions, trackID, subject)
	entity := buildEntity(track, presenceIntervals, transitions)
	result.Observations = obs
	result.Events = events
	result.Segments = segments
	result.Presence = presenceIntervals
	result.Transitions = transitions
	result.Track = track
	result.Entity = entity
	result.Diagnostics = diag
	result.Trace = trace
	return result, nil
}

func phaseName(p phase) string {
	if p == present {
		return "present"
	}
	return "seeking_presence"
}

func emittedEventNames(events []core.Event) string {
	parts := make([]string, 0, len(events))
	for _, e := range events {
		if v, ok := e.Context["source_event"].(string); ok && v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ";")
}

func appendEventNames(existing, added string) string {
	if existing == "" {
		return added
	}
	if added == "" {
		return existing
	}
	return existing + ";" + added
}

func appendDecision(existing, added string) string {
	if existing == "" {
		return added
	}
	return existing + ";" + added
}

func betterTimestampRepresentative(cand, best model.Observation) bool {
	if cand.HasHA != best.HasHA {
		return cand.HasHA
	}
	if cand.HasHA && cand.HA != best.HA {
		return cand.HA < best.HA
	}
	if cand.Lat != best.Lat {
		return cand.Lat < best.Lat
	}
	if cand.Lon != best.Lon {
		return cand.Lon < best.Lon
	}
	return cand.SourceRow < best.SourceRow
}

func observationAccuracyState(o model.Observation) string {
	if o.HasHA {
		return "known"
	}
	return "missing"
}

func edgeAccuracyState(prev, cur model.Observation) string {
	switch {
	case prev.HasHA && cur.HasHA:
		return "both_known"
	case !prev.HasHA && cur.HasHA:
		return "previous_missing"
	case prev.HasHA && !cur.HasHA:
		return "current_missing"
	default:
		return "both_missing"
	}
}

func effectiveDistance(raw float64, prev, cur model.Observation) float64 {
	return geo.EffectiveDistanceKnownM(raw, prev.HA, prev.HasHA, cur.HA, cur.HasHA)
}

func observationLocationContext(o model.Observation) map[string]any {
	m := map[string]any{"lat": o.Lat, "lon": o.Lon, "accuracy_state": observationAccuracyState(o)}
	if o.HasHA {
		m["accuracy_m"] = o.HA
	}
	return m
}

func addObservationAccuracyContext(ctx map[string]any, o model.Observation) {
	ctx["accuracy_state"] = observationAccuracyState(o)
	if o.HasHA {
		ctx["accuracy_m"] = o.HA
	}
}

func observationHAFingerprint(o model.Observation) string {
	if !o.HasHA {
		return "missing"
	}
	return fmt.Sprintf("%.3f", o.HA)
}

func collapseSameTimestamp(input []model.Observation) []model.Observation {
	out := make([]model.Observation, 0, len(input))
	for i := 0; i < len(input); {
		j := i + 1
		for j < len(input) && input[j].TS.Equal(input[i].TS) {
			j++
		}
		best := input[i]
		for k := i + 1; k < j; k++ {
			cand := input[k]
			if betterTimestampRepresentative(cand, best) {
				best = cand
			}
		}
		best.SampleCount = j - i
		var scatter float64
		for k := i; k < j; k++ {
			d := geo.DistanceM(best.Lat, best.Lon, input[k].Lat, input[k].Lon)
			if d > scatter {
				scatter = d
			}
		}
		best.SameTSScatterM = scatter
		out = append(out, best)
		i = j
	}
	return out
}

func assignTrajectoryMetrics(obs []model.Observation, trackID string, cfg config.Config) {
	segmentIndex := 1
	segmentID := idgen.Stable("seg", trackID, strconv.Itoa(segmentIndex))
	obs[0].TrackID = trackID
	obs[0].SegmentID = segmentID
	obs[0].MovementState = "initial"
	obs[0].SpeedClass = "unknown"
	obs[0].AccuracyState = observationAccuracyState(obs[0])
	obs[0].StepAccuracyState = "initial"
	for i := 1; i < len(obs); i++ {
		prev, cur := &obs[i-1], &obs[i]
		dt := cur.TS.Sub(prev.TS).Seconds()
		raw := geo.DistanceM(prev.Lat, prev.Lon, cur.Lat, cur.Lon)
		eff := effectiveDistance(raw, *prev, *cur)
		var speed float64
		if dt > 0 {
			speed = eff / dt
		}
		gap := dt > cfg.GapThresholdS
		disc := eff > cfg.MaxJumpM || (dt > 0 && speed > cfg.MaxSpeedMPS)
		if disc {
			segmentIndex++
			segmentID = idgen.Stable("seg", trackID, strconv.Itoa(segmentIndex))
		}
		cur.TrackID = trackID
		cur.SegmentID = segmentID
		cur.StepDTS = dt
		cur.StepDistanceM = raw
		cur.StepEffectiveDistanceM = eff
		cur.AccuracyState = observationAccuracyState(*cur)
		cur.StepAccuracyState = edgeAccuracyState(*prev, *cur)
		cur.ImpliedSpeedMPS = speed
		cur.IsGap = gap
		cur.IsDiscontinuity = disc
		if gap || disc {
			cur.MovementState = "continuity_break"
		} else if eff <= cfg.MoveRadiusM {
			cur.MovementState = "within_deadband"
		} else {
			cur.MovementState = "movement"
		}
		if gap || dt <= 0 {
			cur.SpeedClass = "unknown"
		} else if eff <= cfg.WalkMaxJumpM && speed <= cfg.WalkMaxSpeedMPS {
			cur.SpeedClass = "walk-compatible"
		} else {
			cur.SpeedClass = "non-walk-compatible"
		}
	}
}

func discontinuityReason(cur model.Observation, cfg config.Config) string {
	jump := cur.StepEffectiveDistanceM > cfg.MaxJumpM
	speed := cur.StepDTS > 0 && cur.ImpliedSpeedMPS > cfg.MaxSpeedMPS
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

func withinPresence(anchor, cur model.Observation, cfg config.Config) bool {
	raw := geo.DistanceM(anchor.Lat, anchor.Lon, cur.Lat, cur.Lon)
	eff := effectiveDistance(raw, anchor, cur)
	return eff <= cfg.MoveRadiusM
}

func withinCentroid(o model.Observation, lat, lon, radius float64) bool {
	allowed := radius
	if o.HasHA {
		allowed += o.HA
	}
	return geo.DistanceM(lat, lon, o.Lat, o.Lon) <= allowed
}

func closePresence(state *presenceState, obs []model.Observation, cfg config.Config, subject, trackID, reason string, movement bool) (model.PresenceInterval, core.Event) {
	if state.phase != present || len(state.members) < 2 {
		return model.PresenceInterval{}, core.Event{}
	}
	members := state.members
	startIdx := members[0]
	endIdx := members[len(members)-1]
	points := make([][2]float64, 0, len(members))
	has := make([]float64, 0, len(members))
	minHA := math.Inf(1)
	maxHA := 0.0
	for _, idx := range members {
		points = append(points, [2]float64{obs[idx].Lat, obs[idx].Lon})
		if obs[idx].HasHA {
			has = append(has, obs[idx].HA)
			if obs[idx].HA < minHA {
				minHA = obs[idx].HA
			}
			if obs[idx].HA > maxHA {
				maxHA = obs[idx].HA
			}
		}
	}
	c := geo.MeanCentroid(points)
	scatter := geo.MaxScatterM(points, c)
	duration := obs[endIdx].TS.Sub(obs[startIdx].TS).Seconds()
	classification := "stay"
	sourceEvent := "STAY"
	if duration >= cfg.DwellThresholdS {
		classification = "dwell"
		sourceEvent = "DWELL"
	}
	id := presenceID(trackID, obs[startIdx])
	p := model.PresenceInterval{
		ID:                    id,
		TrackID:               trackID,
		SegmentID:             obs[startIdx].SegmentID,
		EntityID:              obs[startIdx].EntityID,
		Start:                 obs[startIdx].TS,
		End:                   obs[endIdx].TS,
		DurationS:             duration,
		Classification:        classification,
		CentroidLat:           c.Lat,
		CentroidLon:           c.Lon,
		ObservationCount:      len(members),
		ScatterM:              scatter,
		MedianHAM:             geo.Median(has),
		MinHAM:                minHA,
		MaxHAM:                maxHA,
		HasHAStats:            len(has) > 0,
		ObservationsWithHA:    len(has),
		ObservationsWithoutHA: len(members) - len(has),
		HACoveragePct:         100 * float64(len(has)) / float64(len(members)),
		AccuracySupport:       model.AccuracySupportClass(len(has), len(members)-len(has)),
		StartHAState:          observationAccuracyState(obs[startIdx]),
		EndHAState:            observationAccuracyState(obs[endIdx]),
		StartReason:           state.startReason,
		EndReason:             reason,
		EnterEventID:          state.enterEventID,
		StartObsIndex:         startIdx,
		EndObsIndex:           endIdx,
	}
	if len(has) == 0 {
		minHA, maxHA = 0, 0
		p.MinHAM, p.MaxHAM = 0, 0
	}
	mag := duration
	ctx := map[string]any{
		"source_event":            sourceEvent,
		"track_id":                trackID,
		"segment_id":              p.SegmentID,
		"presence_interval_id":    id,
		"cell":                    geo.Geohash(c.Lat, c.Lon, cfg.GeohashPrecision),
		"state_seconds":           duration,
		"observation_samples":     len(members),
		"observation_scatter_m":   scatter,
		"observations_with_ha":    p.ObservationsWithHA,
		"observations_without_ha": p.ObservationsWithoutHA,
		"ha_coverage_pct":         p.HACoveragePct,
		"accuracy_support":        p.AccuracySupport,
		"start_reason":            p.StartReason,
		"end_reason":              p.EndReason,
	}
	if p.HasHAStats {
		ctx["median_accuracy_m"] = p.MedianHAM
	}
	if movement {
		ctx["closed_by_confirmed_movement"] = true
	}
	ev := baseEvent(sourceEvent, obs[startIdx].TS, cfg, subject)
	ev.EndTime = core.RFC3339(obs[endIdx].TS)
	ev.Magnitude = &mag
	ev.Unit = "s"
	ev.Location = &core.Location{Lat: c.Lat, Lon: c.Lon}
	ev.Context = ctx
	ev.Provenance.SourceRecords = sourceRows(obs[startIdx], obs[endIdx])
	if sourceEvent == "STAY" {
		ev.Feature, ev.Action, ev.State, ev.Polarity = "xq:presence", "xq.mob:stay", "xq:present", 0
	} else {
		ev.Feature, ev.Action, ev.State, ev.Polarity = "xq:presence", "xq.mob:dwell", "xq:present", 0
	}
	ev.ID = idgen.Stable("evt", cfg.Source, subject, sourceEvent, ev.EventTime, ev.EndTime, id)
	p.IntervalEventID = ev.ID
	return p, ev
}

func makeSimpleEvent(sourceEvent string, ts time.Time, o model.Observation, cfg config.Config, subject, trackID, segmentID string, magnitude *float64, unit string) core.Event {
	ev := baseEvent(sourceEvent, ts, cfg, subject)
	ev.Location = locationFromObs(o)
	ev.Magnitude = magnitude
	ev.Unit = unit
	ev.Context["track_id"] = trackID
	ev.Context["segment_id"] = segmentID
	ev.Context["cell"] = o.Geohash
	addObservationAccuracyContext(ev.Context, o)
	ev.Provenance.SourceRecords = []string{fmt.Sprintf("row:%d", o.SourceRow)}
	switch sourceEvent {
	case "START":
		ev.Feature, ev.Action, ev.State, ev.Polarity = "xq:track", "xq:start", "xq:active", 1
	case "END":
		ev.Feature, ev.Action, ev.State, ev.Polarity = "xq:track", "xq:end", "xq:inactive", -1
	}
	ev.ID = idgen.Stable("evt", cfg.Source, subject, sourceEvent, ev.EventTime, trackID)
	return ev
}

func makeEnterEvent(o model.Observation, cfg config.Config, subject, trackID, segmentID string) core.Event {
	ev := baseEvent("ENTER", o.TS, cfg, subject)
	ev.Feature, ev.Action, ev.State, ev.Polarity = "xq:presence", "xq:enter", "xq:present", 1
	ev.Location = locationFromObs(o)
	ev.Context["track_id"] = trackID
	ev.Context["segment_id"] = segmentID
	ev.Context["presence_interval_id"] = presenceID(trackID, o)
	ev.Context["cell"] = o.Geohash
	addObservationAccuracyContext(ev.Context, o)
	ev.Provenance.SourceRecords = []string{fmt.Sprintf("row:%d", o.SourceRow)}
	ev.ID = idgen.Stable("evt", cfg.Source, subject, "ENTER", ev.EventTime, segmentID, observationFingerprint(o))
	return ev
}

func makeLeaveEvent(o model.Observation, p model.PresenceInterval, cfg config.Config, subject string) core.Event {
	ev := baseEvent("LEAVE", o.TS, cfg, subject)
	ev.Feature, ev.Action, ev.State, ev.Polarity = "xq:presence", "xq:leave", "xq:absent", -1
	ev.Location = locationFromObs(o)
	ev.Context["track_id"] = p.TrackID
	ev.Context["segment_id"] = p.SegmentID
	ev.Context["presence_interval_id"] = p.ID
	ev.Context["cell"] = o.Geohash
	addObservationAccuracyContext(ev.Context, o)
	ev.Provenance.SourceRecords = []string{fmt.Sprintf("row:%d", o.SourceRow)}
	ev.ID = idgen.Stable("evt", cfg.Source, subject, "LEAVE", ev.EventTime, p.ID)
	return ev
}

func makeGapEvent(prev, cur model.Observation, cfg config.Config, subject, trackID string) core.Event {
	gap := cur.StepDTS
	ev := baseEvent("GAP", cur.TS, cfg, subject)
	ev.Feature, ev.Action, ev.State, ev.Polarity = "xq:observation", "xq:gap", "xq:interrupted", 0
	ev.Magnitude, ev.Unit = &gap, "s"
	ev.Location = locationFromObs(cur)
	ev.Context["track_id"] = trackID
	ev.Context["segment_id"] = cur.SegmentID
	ev.Context["cell"] = cur.Geohash
	ev.Context["from_cell"] = prev.Geohash
	ev.Context["to_cell"] = cur.Geohash
	ev.Context["from_location"] = observationLocationContext(prev)
	ev.Context["to_location"] = observationLocationContext(cur)
	ev.Context["travel_seconds"] = gap
	ev.Context["distance_m"] = cur.StepDistanceM
	addObservationAccuracyContext(ev.Context, cur)
	ev.Context["edge_accuracy_state"] = cur.StepAccuracyState
	ev.Provenance.SourceRecords = sourceRows(prev, cur)
	ev.ID = idgen.Stable("evt", cfg.Source, subject, "GAP", ev.EventTime, observationFingerprint(prev), observationFingerprint(cur))
	return ev
}

func makeDiscontinuityEvent(prev, cur model.Observation, cfg config.Config, subject, trackID string) core.Event {
	jump := cur.StepDistanceM
	bearing := geo.BearingDeg(prev.Lat, prev.Lon, cur.Lat, cur.Lon)
	ev := baseEvent("DISCONTINUITY", cur.TS, cfg, subject)
	ev.Feature, ev.Action, ev.State, ev.Polarity = "xq:trajectory", "xq:discontinuity", "xq:discontinuous", 0
	ev.Magnitude, ev.Unit = &jump, "m"
	ev.Location = locationFromObs(cur)
	ev.Context["track_id"] = trackID
	ev.Context["segment_id"] = cur.SegmentID
	ev.Context["from_cell"] = prev.Geohash
	ev.Context["to_cell"] = cur.Geohash
	ev.Context["from_location"] = observationLocationContext(prev)
	ev.Context["to_location"] = observationLocationContext(cur)
	ev.Context["travel_seconds"] = cur.StepDTS
	ev.Context["distance_m"] = cur.StepDistanceM
	ev.Context["effective_distance_m"] = cur.StepEffectiveDistanceM
	addObservationAccuracyContext(ev.Context, cur)
	ev.Context["edge_accuracy_state"] = cur.StepAccuracyState
	ev.Context["implied_speed_mps"] = cur.ImpliedSpeedMPS
	ev.Context["bearing_deg"] = bearing
	ev.Context["bearing_8"] = geo.Bearing8(bearing)
	ev.Provenance.SourceRecords = sourceRows(prev, cur)
	ev.ID = idgen.Stable("evt", cfg.Source, subject, "DISCONTINUITY", ev.EventTime, observationFingerprint(prev), observationFingerprint(cur))
	return ev
}

func baseEvent(sourceEvent string, ts time.Time, cfg config.Config, subject string) core.Event {
	return core.Event{
		XQVersion: config.WireVersion,
		EventTime: core.RFC3339(ts),
		Source:    cfg.Source,
		Modality:  config.Modality,
		Class:     "transition",
		Profile:   config.ProfileID,
		Subject:   subject,
		Provenance: &core.Provenance{
			Producer:        "xqmob",
			ProducerVersion: config.Version,
			Method:          "algorithmic",
			Parameters:      cfg.Parameters(),
		},
		Context: map[string]any{"source_event": sourceEvent},
	}
}

func locationFromObs(o model.Observation) *core.Location {
	loc := &core.Location{Lat: o.Lat, Lon: o.Lon}
	if o.HasHA {
		ha := o.HA
		loc.AccuracyM = &ha
	}
	return loc
}

func sourceRows(a, b model.Observation) []string {
	if a.SourceRow == b.SourceRow {
		return []string{fmt.Sprintf("row:%d", a.SourceRow)}
	}
	return []string{fmt.Sprintf("row:%d", a.SourceRow), fmt.Sprintf("row:%d", b.SourceRow)}
}

func makeTransition(p pendingTransition, destinationIdx int, obs []model.Observation, trackID, entityID string) model.Transition {
	dest := obs[destinationIdx]
	duration := dest.TS.Sub(obs[p.departIdx].TS).Seconds()
	path := 0.0
	count := 0
	withHA, withoutHA := 0, 0
	for i := p.departIdx; i <= destinationIdx; i++ {
		if obs[i].HasHA {
			withHA++
		} else {
			withoutHA++
		}
	}
	for i := p.departIdx + 1; i <= destinationIdx; i++ {
		if obs[i].IsGap || obs[i].IsDiscontinuity {
			continue
		}
		path += obs[i].StepDistanceM
		count++
	}
	straight := geo.DistanceM(p.origin.CentroidLat, p.origin.CentroidLon, dest.Lat, dest.Lon)
	id := idgen.Stable("trn", trackID, p.origin.ID, presenceID(trackID, dest))
	return model.Transition{
		ID:                            id,
		EntityID:                      entityID,
		TrackID:                       trackID,
		SegmentID:                     dest.SegmentID,
		OriginPresenceID:              p.origin.ID,
		DestinationPresenceID:         presenceID(trackID, dest),
		Depart:                        obs[p.departIdx].TS,
		Arrive:                        dest.TS,
		DurationS:                     duration,
		OriginLat:                     p.origin.CentroidLat,
		OriginLon:                     p.origin.CentroidLon,
		DestinationLat:                dest.Lat,
		DestinationLon:                dest.Lon,
		StraightLineDistanceM:         straight,
		ObservedPathDistanceM:         path,
		ObservationCount:              count + 1,
		ObservationsWithHA:            withHA,
		ObservationsWithoutHA:         withoutHA,
		HACoveragePct:                 100 * float64(withHA) / float64(withHA+withoutHA),
		AccuracySupport:               model.AccuracySupportClass(withHA, withoutHA),
		OriginObservationsWithHA:      p.origin.ObservationsWithHA,
		OriginObservationsWithoutHA:   p.origin.ObservationsWithoutHA,
		OriginHACoveragePct:           p.origin.HACoveragePct,
		OriginAccuracySupport:         p.origin.AccuracySupport,
		MovementObservationsWithHA:    withHA,
		MovementObservationsWithoutHA: withoutHA,
		MovementHACoveragePct:         100 * float64(withHA) / float64(withHA+withoutHA),
		MovementAccuracySupport:       model.AccuracySupportClass(withHA, withoutHA),
		DestinationAccuracySupport:    "none",
	}
}

func enrichTransitionDestinationSupport(transitions []model.Transition, presence []model.PresenceInterval) {
	byID := make(map[string]model.PresenceInterval, len(presence))
	for _, p := range presence {
		byID[p.ID] = p
	}
	for i := range transitions {
		p, ok := byID[transitions[i].DestinationPresenceID]
		if !ok {
			continue
		}
		transitions[i].DestinationObservationsWithHA = p.ObservationsWithHA
		transitions[i].DestinationObservationsWithoutHA = p.ObservationsWithoutHA
		transitions[i].DestinationHACoveragePct = p.HACoveragePct
		transitions[i].DestinationAccuracySupport = p.AccuracySupport
	}
}

func buildSegments(obs []model.Observation, trackID string) []model.Segment {
	segments := make([]model.Segment, 0)
	for start := 0; start < len(obs); {
		segID := obs[start].SegmentID
		end := start
		for end+1 < len(obs) && obs[end+1].SegmentID == segID {
			end++
		}
		s := model.Segment{
			ID:               segID,
			TrackID:          trackID,
			EntityID:         obs[start].EntityID,
			Index:            len(segments) + 1,
			Start:            obs[start].TS,
			End:              obs[end].TS,
			ObservationCount: end - start + 1,
			DurationS:        obs[end].TS.Sub(obs[start].TS).Seconds(),
			StartLat:         obs[start].Lat,
			StartLon:         obs[start].Lon,
			EndLat:           obs[end].Lat,
			EndLon:           obs[end].Lon,
			MinLat:           obs[start].Lat,
			MaxLat:           obs[start].Lat,
			MinLon:           obs[start].Lon,
			MaxLon:           obs[start].Lon,
			StartReason:      "track_start",
			EndReason:        "track_end",
			StartHAState:     observationAccuracyState(obs[start]),
			EndHAState:       observationAccuracyState(obs[end]),
		}
		if start > 0 {
			s.StartReason = "discontinuity"
		}
		if end+1 < len(obs) {
			s.EndReason = "discontinuity"
		}
		var run [][2]float64
		for i := start; i <= end; i++ {
			if obs[i].HasHA {
				s.ObservationsWithHA++
			} else {
				s.ObservationsWithoutHA++
			}
			coord := [2]float64{obs[i].Lon, obs[i].Lat}
			s.Coordinates = append(s.Coordinates, coord)
			if i == start || !obs[i].IsGap {
				run = append(run, coord)
			} else {
				if len(run) > 0 {
					s.PathRuns = append(s.PathRuns, run)
				}
				run = [][2]float64{coord}
			}
			if obs[i].Lat < s.MinLat {
				s.MinLat = obs[i].Lat
			}
			if obs[i].Lat > s.MaxLat {
				s.MaxLat = obs[i].Lat
			}
			if obs[i].Lon < s.MinLon {
				s.MinLon = obs[i].Lon
			}
			if obs[i].Lon > s.MaxLon {
				s.MaxLon = obs[i].Lon
			}
			if i > start {
				switch obs[i].StepAccuracyState {
				case "both_known":
					s.EdgeBothKnownCount++
				case "previous_missing":
					s.EdgePreviousMissingCount++
				case "current_missing":
					s.EdgeCurrentMissingCount++
				case "both_missing":
					s.EdgeBothMissingCount++
				}
				if obs[i].IsGap {
					s.GapCount++
				}
				if !obs[i].IsGap {
					s.ObservedPathM += obs[i].StepDistanceM
				}
				if obs[i].ImpliedSpeedMPS > s.MaxImpliedSpeedMPS {
					s.MaxImpliedSpeedMPS = obs[i].ImpliedSpeedMPS
				}
			}
		}
		if len(run) > 0 {
			s.PathRuns = append(s.PathRuns, run)
		}
		s.DisplacementM = geo.DistanceM(s.StartLat, s.StartLon, s.EndLat, s.EndLon)
		observedSeconds := 0.0
		for i := start + 1; i <= end; i++ {
			if !obs[i].IsGap {
				observedSeconds += obs[i].StepDTS
			}
		}
		if observedSeconds > 0 {
			s.MeanObservedSpeedMPS = s.ObservedPathM / observedSeconds
		}
		if s.ObservationCount > 0 {
			s.HACoveragePct = 100 * float64(s.ObservationsWithHA) / float64(s.ObservationCount)
		}
		s.AccuracySupport = model.AccuracySupportClass(s.ObservationsWithHA, s.ObservationsWithoutHA)
		segments = append(segments, s)
		start = end + 1
	}
	return segments
}

func buildTrack(obs []model.Observation, events []core.Event, segments []model.Segment, presence []model.PresenceInterval, transitions []model.Transition, trackID, subject string) model.Track {
	t := model.Track{
		ID:                    trackID,
		EntityID:              obs[0].EntityID,
		Subject:               subject,
		Start:                 obs[0].TS,
		End:                   obs[len(obs)-1].TS,
		CoverageS:             obs[len(obs)-1].TS.Sub(obs[0].TS).Seconds(),
		ObservationCount:      len(obs),
		EventCount:            len(events),
		SegmentCount:          len(segments),
		PresenceIntervalCount: len(presence),
		TransitionCount:       len(transitions),
		StartLat:              obs[0].Lat,
		StartLon:              obs[0].Lon,
		EndLat:                obs[len(obs)-1].Lat,
		EndLon:                obs[len(obs)-1].Lon,
	}
	cells := map[string]struct{}{}
	has := make([]float64, 0, len(obs))
	var run [][2]float64
	for i, o := range obs {
		cells[o.Geohash] = struct{}{}
		if o.HasHA {
			has = append(has, o.HA)
			t.ObservationsWithHA++
		} else {
			t.ObservationsWithoutHA++
		}
		coord := [2]float64{o.Lon, o.Lat}
		t.Coordinates = append(t.Coordinates, coord)
		if i == 0 || (!o.IsGap && !o.IsDiscontinuity) {
			run = append(run, coord)
		} else {
			if len(run) > 0 {
				t.PathRuns = append(t.PathRuns, run)
			}
			run = [][2]float64{coord}
		}
		if i > 0 {
			if o.IsGap {
				t.GapCount++
			}
			if o.IsDiscontinuity {
				t.DiscontinuityCount++
			}
			if !o.IsGap && !o.IsDiscontinuity {
				t.ObservedPathM += o.StepDistanceM
			}
		}
	}
	if len(run) > 0 {
		t.PathRuns = append(t.PathRuns, run)
	}
	t.UniqueGeohashCells = len(cells)
	if len(has) > 0 {
		t.MedianHAM = geo.Median(has)
		t.HasMedianHA = true
	}
	if t.ObservationCount > 0 {
		t.HACoveragePct = 100 * float64(t.ObservationsWithHA) / float64(t.ObservationCount)
	}
	t.HAClass = model.AccuracySupportClass(t.ObservationsWithHA, t.ObservationsWithoutHA)
	for _, p := range presence {
		if p.Classification == "dwell" {
			t.DwellCount++
			t.TotalDwellS += p.DurationS
		} else {
			t.StayCount++
		}
	}
	return t
}

func buildEntity(t model.Track, presence []model.PresenceInterval, transitions []model.Transition) model.EntitySummary {
	e := model.EntitySummary{
		EntityID:              t.EntityID,
		Subject:               t.Subject,
		FirstSeen:             t.Start,
		LastSeen:              t.End,
		CoverageS:             t.CoverageS,
		ObservationCount:      t.ObservationCount,
		TrackCount:            1,
		SegmentCount:          t.SegmentCount,
		PresenceCount:         len(presence),
		TransitionCount:       len(transitions),
		StayCount:             t.StayCount,
		DwellCount:            t.DwellCount,
		GapCount:              t.GapCount,
		DiscontinuityCount:    t.DiscontinuityCount,
		TotalObservedPathM:    t.ObservedPathM,
		TotalDwellS:           t.TotalDwellS,
		UniqueGeohashCells:    t.UniqueGeohashCells,
		MedianHAM:             t.MedianHAM,
		HasMedianHA:           t.HasMedianHA,
		ObservationsWithHA:    t.ObservationsWithHA,
		ObservationsWithoutHA: t.ObservationsWithoutHA,
		HACoveragePct:         t.HACoveragePct,
		HAClass:               t.HAClass,
		StartLat:              t.StartLat,
		StartLon:              t.StartLon,
		EndLat:                t.EndLat,
		EndLon:                t.EndLon,
	}
	locs := map[string]struct{}{}
	for _, p := range presence {
		e.TotalPresenceS += p.DurationS
		key := fmt.Sprintf("%.5f,%.5f", p.CentroidLat, p.CentroidLon)
		locs[key] = struct{}{}
	}
	e.UniquePresenceLocations = len(locs)
	return e
}

func sortEvents(events []core.Event) {
	rank := map[string]int{"START": 0, "ENTER": 1, "STAY": 2, "DWELL": 2, "LEAVE": 3, "GAP": 4, "DISCONTINUITY": 5, "END": 6}
	sort.SliceStable(events, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339Nano, events[i].EventTime)
		tj, _ := time.Parse(time.RFC3339Nano, events[j].EventTime)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		si, _ := events[i].Context["source_event"].(string)
		sj, _ := events[j].Context["source_event"].(string)
		if rank[si] != rank[sj] {
			return rank[si] < rank[sj]
		}
		return events[i].ID < events[j].ID
	})
}

func observationFingerprint(o model.Observation) string {
	return idgen.Stable("fix", o.EntityID, core.RFC3339(o.TS), fmt.Sprintf("%.9f", o.Lat), fmt.Sprintf("%.9f", o.Lon), observationHAFingerprint(o))
}

func presenceID(trackID string, o model.Observation) string {
	return idgen.Stable("prs", trackID, observationFingerprint(o))
}

func StableIDForTest(prefix string, parts ...string) string { return idgen.Stable(prefix, parts...) }

func NormalizeEventSource(e core.Event) string {
	if e.Context == nil {
		return ""
	}
	v, _ := e.Context["source_event"].(string)
	return strings.ToUpper(v)
}
