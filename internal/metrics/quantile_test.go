package metrics

import (
	"math"
	"testing"
)

func TestP2Quantiles(t *testing.T) {
	d := NewDistributionEstimator()
	for i := 1; i <= 10000; i++ {
		d.Add(float64(i))
	}
	s := d.Summary()
	if s.Count != 10000 {
		t.Fatalf("count=%d", s.Count)
	}
	checks := []struct{ got, want, tol float64 }{
		{s.P50, 5000.5, 80},
		{s.P90, 9000.1, 100},
		{s.P99, 9900.01, 120},
	}
	for _, c := range checks {
		if math.Abs(c.got-c.want) > c.tol {
			t.Fatalf("got %.2f want %.2f tol %.2f", c.got, c.want, c.tol)
		}
	}
}

func TestP2SmallExactInterpolation(t *testing.T) {
	q := NewP2(0.5)
	q.Add(1)
	q.Add(3)
	q.Add(2)
	if got := q.Value(); got != 2 {
		t.Fatalf("median=%v", got)
	}
}
