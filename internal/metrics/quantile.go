package metrics

import (
	"math"
	"sort"
)

// P2 implements the Jain-Chlamtac P² streaming quantile estimator.
// It uses constant memory after the first five observations.
type P2 struct {
	p       float64
	count   int64
	initial []float64
	q       [5]float64
	n       [5]int
	np      [5]float64
	dn      [5]float64
}

func NewP2(p float64) *P2 {
	if p <= 0 || p >= 1 {
		panic("P2 quantile must be between 0 and 1")
	}
	return &P2{p: p, initial: make([]float64, 0, 5)}
}

func (e *P2) Add(x float64) {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return
	}
	e.count++
	if len(e.initial) < 5 {
		e.initial = append(e.initial, x)
		if len(e.initial) == 5 {
			sort.Float64s(e.initial)
			copy(e.q[:], e.initial)
			e.n = [5]int{1, 2, 3, 4, 5}
			e.np = [5]float64{1, 1 + 2*e.p, 1 + 4*e.p, 3 + 2*e.p, 5}
			e.dn = [5]float64{0, e.p / 2, e.p, (1 + e.p) / 2, 1}
		}
		return
	}

	k := 0
	switch {
	case x < e.q[0]:
		e.q[0] = x
		k = 0
	case x < e.q[1]:
		k = 0
	case x < e.q[2]:
		k = 1
	case x < e.q[3]:
		k = 2
	case x <= e.q[4]:
		k = 3
	default:
		e.q[4] = x
		k = 3
	}
	for i := k + 1; i < 5; i++ {
		e.n[i]++
	}
	for i := 0; i < 5; i++ {
		e.np[i] += e.dn[i]
	}
	for i := 1; i <= 3; i++ {
		d := e.np[i] - float64(e.n[i])
		if (d >= 1 && e.n[i+1]-e.n[i] > 1) || (d <= -1 && e.n[i-1]-e.n[i] < -1) {
			ds := 1
			if d < 0 {
				ds = -1
			}
			qhat := e.parabolic(i, ds)
			if e.q[i-1] < qhat && qhat < e.q[i+1] {
				e.q[i] = qhat
			} else {
				e.q[i] = e.linear(i, ds)
			}
			e.n[i] += ds
		}
	}
}

func (e *P2) parabolic(i, d int) float64 {
	ni := float64(e.n[i])
	nim1 := float64(e.n[i-1])
	nip1 := float64(e.n[i+1])
	dd := float64(d)
	return e.q[i] + dd/(nip1-nim1)*((ni-nim1+dd)*(e.q[i+1]-e.q[i])/(nip1-ni)+
		(nip1-ni-dd)*(e.q[i]-e.q[i-1])/(ni-nim1))
}

func (e *P2) linear(i, d int) float64 {
	j := i + d
	return e.q[i] + float64(d)*(e.q[j]-e.q[i])/float64(e.n[j]-e.n[i])
}

func (e *P2) Count() int64 { return e.count }

func (e *P2) Value() float64 {
	if e.count == 0 {
		return 0
	}
	if e.count <= 5 {
		v := append([]float64(nil), e.initial...)
		sort.Float64s(v)
		pos := e.p * float64(len(v)-1)
		lo := int(math.Floor(pos))
		hi := int(math.Ceil(pos))
		if lo == hi {
			return v[lo]
		}
		f := pos - float64(lo)
		return v[lo]*(1-f) + v[hi]*f
	}
	return e.q[2]
}

type Distribution struct {
	Count int64   `json:"count"`
	P50   float64 `json:"p50"`
	P90   float64 `json:"p90"`
	P99   float64 `json:"p99"`
}

type DistributionEstimator struct {
	count int64
	p50   *P2
	p90   *P2
	p99   *P2
}

func NewDistributionEstimator() *DistributionEstimator {
	return &DistributionEstimator{p50: NewP2(0.50), p90: NewP2(0.90), p99: NewP2(0.99)}
}

func (d *DistributionEstimator) Add(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	d.count++
	d.p50.Add(v)
	d.p90.Add(v)
	d.p99.Add(v)
}

func (d *DistributionEstimator) Summary() Distribution {
	return Distribution{Count: d.count, P50: d.p50.Value(), P90: d.p90.Value(), P99: d.p99.Value()}
}
