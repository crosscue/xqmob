package input

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crosscue/xqmob/internal/config"
)

func TestParseTimestamp(t *testing.T) {
	cases := []struct{ input, want string }{
		{"2026-09-01T12:34:56+01:00", "2026-09-01T11:34:56Z"},
		{"1788266096", "2026-09-01T12:34:56Z"},
		{"1788266096000", "2026-09-01T12:34:56Z"},
		{"1788266096000000", "2026-09-01T12:34:56Z"},
		{"1788266096000000001", "2026-09-01T12:34:56.000000001Z"},
		{"1788266096000000002", "2026-09-01T12:34:56.000000002Z"},
		{"1788266096.123456789", "2026-09-01T12:34:56.123456789Z"},
		{"1788266096123.456789", "2026-09-01T12:34:56.123456789Z"},
		{"1788266096123456.789", "2026-09-01T12:34:56.123456789Z"},
		{"1.788266096000000001e18", "2026-09-01T12:34:56.000000001Z"},
		{"-0.000000001", "1969-12-31T23:59:59.999999999Z"},
		{"-0.0000000005", "1969-12-31T23:59:59.999999999Z"},
		{"0.0000000005", "1970-01-01T00:00:00.000000001Z"},
		{"0.0000000004", "1970-01-01T00:00:00Z"},
		{"-1000000000000000001", "1938-04-24T22:13:19.999999999Z"},
		{"9223372036854775807", "2262-04-11T23:47:16.854775807Z"},
		{"-9223372036854775808", "1677-09-21T00:12:43.145224192Z"},
	}
	for _, c := range cases {
		got, err := ParseTimestamp(c.input)
		if err != nil {
			t.Fatalf("%s: %v", c.input, err)
		}
		if got.Location() != time.UTC || got.Format(time.RFC3339Nano) != c.want {
			t.Errorf("%s: got %s, want %s", c.input, got.Format(time.RFC3339Nano), c.want)
		}
	}
}

func TestRejectInvalidTimestamps(t *testing.T) {
	for _, s := range []string{"NaN", "Inf", "+Inf", "-Infinity", "1e309", "1e-999999999", "9223372036854775808", "-9223372036854775809", "9.223372036854775808e18", "1/2", "0x1p2", "-99999999999", strings.Repeat("1", 129)} {
		if _, err := ParseTimestamp(s); err == nil {
			t.Errorf("accepted invalid timestamp %q", s)
		}
		_, err := parseObservation("a", s, "51", "0", "5", 2, config.Default())
		if rejectCode(err) != "invalid_ts" {
			t.Errorf("%q: expected invalid_ts, got %v", s, err)
		}
	}
}

type failingOutput struct {
	bytes.Buffer
	writeErr, closeErr error
	closed             bool
}

func (f *failingOutput) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.Buffer.Write(p)
}

func (f *failingOutput) Close() error {
	f.closed = true
	return f.closeErr
}

func TestPartitionCSVPropagatesOutputFailures(t *testing.T) {
	failure := errors.New("simulated disk failure")
	for _, target := range []string{"partition flush", "partition close", "rejects flush", "rejects close", "rejects write", "strict and flush"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "input.csv")
			id := "a"
			if target == "rejects write" {
				id = strings.Repeat("a", 8192)
			}
			data := "id,ts,lat,lon,ha\n" + id + ",2026-09-01T00:00:00Z,51,0,5\n" + id + ",bad,51,0,5\n"
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg := config.Default()
			cfg.Partitions = 2
			cfg.Strict = target == "strict and flush"
			var outputs []*failingOutput
			create := func(path string) (io.WriteCloser, error) {
				f := &failingOutput{}
				isRejects := filepath.Base(path) == "rejects.csv"
				if (isRejects && strings.HasPrefix(target, "rejects")) || (!isRejects && !strings.HasPrefix(target, "rejects")) {
					if strings.HasSuffix(target, "close") {
						f.closeErr = failure
					} else {
						f.writeErr = failure
					}
				}
				outputs = append(outputs, f)
				return f, nil
			}
			_, err := partitionCSV(path, filepath.Join(dir, "parts"), filepath.Join(dir, "rejects.csv"), cfg, create)
			if !errors.Is(err, failure) {
				t.Fatalf("lost write/close failure: %v", err)
			}
			if cfg.Strict && !strings.Contains(err.Error(), "invalid ts") {
				t.Fatalf("lost original parse failure: %v", err)
			}
			for _, f := range outputs {
				if !f.closed {
					t.Fatal("output left open after failure")
				}
			}
		})
	}
}

func TestRejectCodes(t *testing.T) {
	cfg := config.Default()
	cases := []struct {
		name                 string
		id, ts, lat, lon, ha string
		want                 string
	}{
		{"empty id", "", "2026-09-01T00:00:00Z", "51", "0", "5", "empty_id"},
		{"bad ts", "a", "not-a-time", "51", "0", "5", "invalid_ts"},
		{"bad lat", "a", "2026-09-01T00:00:00Z", "91", "0", "5", "invalid_lat"},
		{"bad lon", "a", "2026-09-01T00:00:00Z", "51", "181", "5", "invalid_lon"},
		{"missing ha", "a", "2026-09-01T00:00:00Z", "51", "0", "", "missing_ha"},
		{"non numeric ha", "a", "2026-09-01T00:00:00Z", "51", "0", "bad", "non_numeric_ha"},
		{"non finite ha", "a", "2026-09-01T00:00:00Z", "51", "0", "NaN", "non_finite_ha"},
		{"negative ha", "a", "2026-09-01T00:00:00Z", "51", "0", "-1", "negative_ha"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testCfg := cfg
			if tc.want == "missing_ha" {
				testCfg.HAPolicy = "require"
			}
			_, err := parseObservation(tc.id, tc.ts, tc.lat, tc.lon, tc.ha, 2, testCfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := rejectCode(err); got != tc.want {
				t.Fatalf("reject code=%q want %q: %v", got, tc.want, err)
			}
		})
	}

	if _, err := parseObservation("a", "2026-09-01T00:00:00Z", "51", "0", "0", 2, cfg); err != nil {
		t.Fatalf("zero HA remains valid: %v", err)
	}

	preserve := cfg
	o, err := parseObservation("a", "2026-09-01T00:00:00Z", "51", "0", "", 2, preserve)
	if err != nil {
		t.Fatalf("default preserve rejected blank HA: %v", err)
	}
	if o.HasHA || o.AccuracyState != "missing" {
		t.Fatalf("missing HA was imputed instead of preserved: %+v", o)
	}

	cfg.MaxHAM = 10
	_, err = parseObservation("a", "2026-09-01T00:00:00Z", "51", "0", "20", 2, cfg)
	if got := rejectCode(err); got != "ha_exceeds_max" {
		t.Fatalf("reject code=%q want ha_exceeds_max", got)
	}
}

func TestCoordinateDecimalPlaces(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"51", 0},
		{"51.5", 1},
		{"51.500000", 6},
		{"-0.100000", 6},
		{"5.15e1", -1},
	}
	for _, tc := range cases {
		if got := coordinateDecimalPlaces(tc.in); got != tc.want {
			t.Fatalf("coordinateDecimalPlaces(%q)=%d want %d", tc.in, got, tc.want)
		}
	}
}
