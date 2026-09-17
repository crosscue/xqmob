package pipeline

import (
	"fmt"
	"math"
	"strconv"

	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/geo"
	"github.com/crosscue/xqmob/internal/metrics"
	"github.com/crosscue/xqmob/internal/model"
)

// ReversalDiagnostics describes consecutive target-population edges. These
// counters are descriptive only: they do not change eventization or continuity.
type ReversalDiagnostics struct {
	ConsecutiveTargetEdgePairs int64            `json:"consecutive_target_edge_pairs"`
	ExactABAReturns            int64            `json:"exact_a_b_a_returns"`
	ReturnWithin5M             int64            `json:"return_within_5m"`
	NearOppositeBearing        int64            `json:"near_opposite_bearing_22_5deg"`
	NearSymmetricDistance5M    int64            `json:"near_symmetric_distance_within_5m"`
	ABALikePairs               int64            `json:"a_b_a_like_pairs"`
	PrecisionABAReturns        int64            `json:"precision_a_b_a_returns"`
	PrecisionTriplets          map[string]int64 `json:"precision_triplets"`
}

// TargetPopulationDiagnostics characterises the structured population that
// emerged in large preserve-HA runs. It is descriptive only and does not alter
// eventization or continuity decisions.
type TargetPopulationDiagnostics struct {
	Definition                      string                      `json:"definition"`
	Count                           int64                       `json:"count"`
	RawDistanceM                    metrics.Distribution        `json:"raw_distance_m"`
	EffectiveDistanceM              metrics.Distribution        `json:"effective_distance_m"`
	StepDTS                         metrics.Distribution        `json:"step_dt_s"`
	AbsDeltaLatDeg                  metrics.Distribution        `json:"abs_delta_lat_deg"`
	AbsDeltaLonDeg                  metrics.Distribution        `json:"abs_delta_lon_deg"`
	Bearing16                       map[string]int64            `json:"bearing_16"`
	DistanceRounded1M               map[string]int64            `json:"distance_rounded_1m"`
	VectorRounded10M                map[string]int64            `json:"vector_rounded_10m"`
	PreviousCoordinateDecimalPlaces map[string]int64            `json:"previous_coordinate_decimal_places"`
	CurrentCoordinateDecimalPlaces  map[string]int64            `json:"current_coordinate_decimal_places"`
	PrecisionTransitions            map[string]int64            `json:"precision_transitions"`
	PrecisionTransitionBearing16    map[string]map[string]int64 `json:"precision_transition_bearing_16"`
	PrecisionTransitionDistance1M   map[string]map[string]int64 `json:"precision_transition_distance_rounded_1m"`
	PrecisionTransitionVector10M    map[string]map[string]int64 `json:"precision_transition_vector_rounded_10m"`
	Reversals                       ReversalDiagnostics         `json:"reversals"`
}

type SpatialDiagnostics struct {
	BothMissingSpeedOnlyShortDT50To500M TargetPopulationDiagnostics `json:"both_missing_speed_only_dt_lt_5s_distance_50_500m"`
}

type targetPopulationCollector struct {
	count                         int64
	raw                           *metrics.DistributionEstimator
	effective                     *metrics.DistributionEstimator
	dt                            *metrics.DistributionEstimator
	absDLat                       *metrics.DistributionEstimator
	absDLon                       *metrics.DistributionEstimator
	bearing16                     map[string]int64
	distance1m                    map[string]int64
	vector10m                     map[string]int64
	prevDP                        map[string]int64
	currDP                        map[string]int64
	precisionTransitions          map[string]int64
	precisionTransitionBearing16  map[string]map[string]int64
	precisionTransitionDistance1M map[string]map[string]int64
	precisionTransitionVector10M  map[string]map[string]int64
	reversals                     ReversalDiagnostics
}

func newTargetPopulationCollector() *targetPopulationCollector {
	return &targetPopulationCollector{
		raw: metrics.NewDistributionEstimator(), effective: metrics.NewDistributionEstimator(), dt: metrics.NewDistributionEstimator(),
		absDLat: metrics.NewDistributionEstimator(), absDLon: metrics.NewDistributionEstimator(), bearing16: map[string]int64{},
		distance1m: map[string]int64{}, vector10m: map[string]int64{}, prevDP: map[string]int64{}, currDP: map[string]int64{},
		precisionTransitions: map[string]int64{}, precisionTransitionBearing16: map[string]map[string]int64{},
		precisionTransitionDistance1M: map[string]map[string]int64{}, precisionTransitionVector10M: map[string]map[string]int64{},
		reversals: ReversalDiagnostics{PrecisionTriplets: map[string]int64{}},
	}
}

func isTargetPopulationObservation(cur model.Observation, cfg config.Config) bool {
	return cur.StepAccuracyState == "both_missing" && cur.IsDiscontinuity && discontinuityReason(cur, cfg) == "speed_only" &&
		cur.StepDTS > 0 && cur.StepDTS < 5 && cur.StepEffectiveDistanceM >= 50 && cur.StepEffectiveDistanceM < 500
}

func (c *targetPopulationCollector) AddObservations(obs []model.Observation, cfg config.Config) {
	for i := 1; i < len(obs); i++ {
		prev, cur := obs[i-1], obs[i]
		if !isTargetPopulationObservation(cur, cfg) {
			continue
		}
		c.count++
		c.raw.Add(cur.StepDistanceM)
		c.effective.Add(cur.StepEffectiveDistanceM)
		c.dt.Add(cur.StepDTS)
		c.absDLat.Add(math.Abs(cur.Lat - prev.Lat))
		c.absDLon.Add(math.Abs(shortestLonDeltaDeg(cur.Lon - prev.Lon)))
		bearing := geo.BearingDeg(prev.Lat, prev.Lon, cur.Lat, cur.Lon)
		bearingKey := bearing16(bearing)
		c.bearing16[bearingKey]++
		distanceKey := strconv.Itoa(int(math.Round(cur.StepEffectiveDistanceM)))
		c.distance1m[distanceKey]++
		vectorKey := roundedVector10M(cur.StepDistanceM, bearing)
		c.vector10m[vectorKey]++
		prevPrecision := coordinatePrecisionKey(prev.LatDecimalPlaces, prev.LonDecimalPlaces)
		curPrecision := coordinatePrecisionKey(cur.LatDecimalPlaces, cur.LonDecimalPlaces)
		c.prevDP[prevPrecision]++
		c.currDP[curPrecision]++
		transitionKey := prevPrecision + "->" + curPrecision
		c.precisionTransitions[transitionKey]++
		addNestedCount(c.precisionTransitionBearing16, transitionKey, bearingKey)
		addNestedCount(c.precisionTransitionDistance1M, transitionKey, distanceKey)
		addNestedCount(c.precisionTransitionVector10M, transitionKey, vectorKey)
	}

	// Characterise immediate A->B->A-like patterns using consecutive target
	// edges. Thresholds here are diagnostic constants, not eventizer settings.
	for i := 2; i < len(obs); i++ {
		a, b, d := obs[i-2], obs[i-1], obs[i]
		if !isTargetPopulationObservation(b, cfg) || !isTargetPopulationObservation(d, cfg) {
			continue
		}
		c.reversals.ConsecutiveTargetEdgePairs++
		if a.Lat == d.Lat && a.Lon == d.Lon {
			c.reversals.ExactABAReturns++
		}
		returnDistance := geo.DistanceM(a.Lat, a.Lon, d.Lat, d.Lon)
		returnWithin5 := returnDistance < 5
		if returnWithin5 {
			c.reversals.ReturnWithin5M++
		}
		b1 := geo.BearingDeg(a.Lat, a.Lon, b.Lat, b.Lon)
		b2 := geo.BearingDeg(b.Lat, b.Lon, d.Lat, d.Lon)
		nearOpposite := math.Abs(angularDifferenceDeg(b1, b2)-180) <= 22.5
		if nearOpposite {
			c.reversals.NearOppositeBearing++
		}
		nearSymmetric := math.Abs(b.StepEffectiveDistanceM-d.StepEffectiveDistanceM) <= 5
		if nearSymmetric {
			c.reversals.NearSymmetricDistance5M++
		}
		if returnWithin5 && nearOpposite && nearSymmetric {
			c.reversals.ABALikePairs++
		}
		pa := coordinatePrecisionKey(a.LatDecimalPlaces, a.LonDecimalPlaces)
		pb := coordinatePrecisionKey(b.LatDecimalPlaces, b.LonDecimalPlaces)
		pd := coordinatePrecisionKey(d.LatDecimalPlaces, d.LonDecimalPlaces)
		triplet := pa + "->" + pb + "->" + pd
		c.reversals.PrecisionTriplets[triplet]++
		if pa == pd && pa != pb {
			c.reversals.PrecisionABAReturns++
		}
	}
}

func shortestLonDeltaDeg(d float64) float64 {
	for d > 180 {
		d -= 360
	}
	for d < -180 {
		d += 360
	}
	return d
}

func angularDifferenceDeg(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 180 {
		d = 360 - d
	}
	return d
}

func roundedVector10M(distanceM, bearing float64) string {
	rad := bearing * math.Pi / 180
	east := distanceM * math.Sin(rad)
	north := distanceM * math.Cos(rad)
	eb := int(math.Round(east/10.0)) * 10
	nb := int(math.Round(north/10.0)) * 10
	return fmt.Sprintf("east_%+dm_north_%+dm", eb, nb)
}

func (c *targetPopulationCollector) Summary() SpatialDiagnostics {
	return SpatialDiagnostics{BothMissingSpeedOnlyShortDT50To500M: TargetPopulationDiagnostics{
		Definition: "step_accuracy_state=both_missing AND discontinuity_reason=speed_only AND 0<dt_s<5 AND 50<=effective_distance_m<500",
		Count:      c.count, RawDistanceM: c.raw.Summary(), EffectiveDistanceM: c.effective.Summary(), StepDTS: c.dt.Summary(),
		AbsDeltaLatDeg: c.absDLat.Summary(), AbsDeltaLonDeg: c.absDLon.Summary(), Bearing16: c.bearing16,
		DistanceRounded1M: c.distance1m, VectorRounded10M: c.vector10m,
		PreviousCoordinateDecimalPlaces: c.prevDP, CurrentCoordinateDecimalPlaces: c.currDP,
		PrecisionTransitions: c.precisionTransitions, PrecisionTransitionBearing16: c.precisionTransitionBearing16,
		PrecisionTransitionDistance1M: c.precisionTransitionDistance1M, PrecisionTransitionVector10M: c.precisionTransitionVector10M,
		Reversals: c.reversals,
	}}
}

func bearing16(b float64) string {
	dirs := [...]string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	return dirs[int(math.Floor((b+11.25)/22.5))%16]
}

func coordinatePrecisionKey(latDP, lonDP int) string {
	fmtDP := func(v int) string {
		if v < 0 {
			return "scientific"
		}
		return strconv.Itoa(v)
	}
	return "lat_" + fmtDP(latDP) + "_lon_" + fmtDP(lonDP)
}
