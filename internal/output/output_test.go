package output

import (
	"testing"
	"time"
)

func TestMillisPreservesPreEpochAndDistantDates(t *testing.T) {
	for _, input := range []string{"1969-12-31T23:59:59.999999999Z", "2500-01-01T00:00:00Z", "1600-01-01T00:00:00Z"} {
		ts, err := time.Parse(time.RFC3339Nano, input)
		if err != nil {
			t.Fatal(err)
		}
		got := time.UnixMilli(millis(ts)).UTC()
		if !got.Equal(ts.Truncate(time.Millisecond)) {
			t.Errorf("%s projected as %s", input, got)
		}
	}
}
