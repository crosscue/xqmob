package core

import "time"

type Location struct {
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	AccuracyM *float64 `json:"accuracy_m,omitempty"`
}

type Provenance struct {
	Producer        string         `json:"producer"`
	ProducerVersion string         `json:"producer_version"`
	Method          string         `json:"method,omitempty"`
	SourceRecords   []string       `json:"source_records,omitempty"`
	Parameters      map[string]any `json:"parameters"`
}

type Event struct {
	XQVersion  string         `json:"xq_version"`
	ID         string         `json:"id"`
	EventTime  string         `json:"event_time"`
	EndTime    string         `json:"end_time,omitempty"`
	Source     string         `json:"source"`
	Modality   string         `json:"modality"`
	Class      string         `json:"class"`
	Profile    string         `json:"profile,omitempty"`
	Subject    string         `json:"subject,omitempty"`
	Feature    string         `json:"feature"`
	Action     string         `json:"action"`
	State      string         `json:"state,omitempty"`
	Polarity   int            `json:"polarity"`
	Magnitude  *float64       `json:"magnitude,omitempty"`
	Unit       string         `json:"unit,omitempty"`
	Location   *Location      `json:"location,omitempty"`
	Provenance *Provenance    `json:"provenance,omitempty"`
	Context    map[string]any `json:"context,omitempty"`
}

func RFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
