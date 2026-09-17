package pipeline

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"github.com/crosscue/xqmob/internal/core"
	"github.com/crosscue/xqmob/internal/model"
	"github.com/crosscue/xqmob/internal/output"
)

type MemoryPhaseStats struct {
	Samples             int64 `json:"samples"`
	MaxCurrentRSSBytes  int64 `json:"max_current_rss_bytes,omitempty"`
	MaxGoHeapAllocBytes int64 `json:"max_go_heap_alloc_bytes"`
	MaxGoHeapInuseBytes int64 `json:"max_go_heap_inuse_bytes"`
	MaxGoSysBytes       int64 `json:"max_go_sys_bytes"`
}

type WorkingSetStats struct {
	MaxPartitionRows                    int64 `json:"max_partition_rows"`
	MaxPartitionObservationShallowBytes int64 `json:"max_partition_observation_shallow_bytes"`
	PartitionWriterBufferBytes          int64 `json:"partition_writer_buffer_bytes"`
	CanonicalJSONBufferBytes            int64 `json:"canonical_json_buffer_bytes"`
	MaxEntityObservations               int64 `json:"max_entity_observations"`
	MaxEntityEvents                     int64 `json:"max_entity_events"`
	MaxEntitySegments                   int64 `json:"max_entity_segments"`
	MaxEntityPresenceIntervals          int64 `json:"max_entity_presence_intervals"`
	MaxEntityTransitions                int64 `json:"max_entity_transitions"`
	MaxEntityResultShallowBytes         int64 `json:"max_entity_result_shallow_bytes"`
	ParquetApplicationBatchRows         int64 `json:"parquet_application_batch_rows"`
}

type MemoryDiagnostics struct {
	ProfilingEnabled     bool                        `json:"profiling_enabled"`
	ProfilingForcesGC    bool                        `json:"profiling_forces_gc"`
	HeapProfiles         []HeapProfileInfo           `json:"heap_profiles,omitempty"`
	SampleStrideEntities int64                       `json:"sample_stride_entities"`
	ByPhase              map[string]MemoryPhaseStats `json:"by_phase"`
	WorkingSet           WorkingSetStats             `json:"working_set"`
}

type memoryTracker struct {
	stride  int64
	byPhase map[string]MemoryPhaseStats
}

func newMemoryTracker(stride int64) *memoryTracker {
	return &memoryTracker{stride: stride, byPhase: map[string]MemoryPhaseStats{}}
}

func (m *memoryTracker) Sample(phase string) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	rss, _ := currentRSSBytes()
	s := m.byPhase[phase]
	s.Samples++
	if rss > s.MaxCurrentRSSBytes {
		s.MaxCurrentRSSBytes = rss
	}
	if int64(ms.HeapAlloc) > s.MaxGoHeapAllocBytes {
		s.MaxGoHeapAllocBytes = int64(ms.HeapAlloc)
	}
	if int64(ms.HeapInuse) > s.MaxGoHeapInuseBytes {
		s.MaxGoHeapInuseBytes = int64(ms.HeapInuse)
	}
	if int64(ms.Sys) > s.MaxGoSysBytes {
		s.MaxGoSysBytes = int64(ms.Sys)
	}
	m.byPhase[phase] = s
}

func (m *memoryTracker) Summary(ws WorkingSetStats) MemoryDiagnostics {
	ws.ParquetApplicationBatchRows = int64(output.ParquetBatchRows)
	ws.CanonicalJSONBufferBytes = int64(output.CanonicalJSONBufferBytes)
	return MemoryDiagnostics{SampleStrideEntities: m.stride, ByPhase: m.byPhase, WorkingSet: ws}
}

func currentRSSBytes() (int64, bool) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, false
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}

func partitionObservationShallowBytes(rows int) int64 {
	return int64(rows) * int64(unsafe.Sizeof(model.Observation{}))
}

func entityResultShallowBytes(r model.EntityResult) int64 {
	return int64(len(r.Observations))*int64(unsafe.Sizeof(model.Observation{})) +
		int64(len(r.Events))*int64(unsafe.Sizeof(core.Event{})) +
		int64(len(r.Segments))*int64(unsafe.Sizeof(model.Segment{})) +
		int64(len(r.Presence))*int64(unsafe.Sizeof(model.PresenceInterval{})) +
		int64(len(r.Transitions))*int64(unsafe.Sizeof(model.Transition{})) +
		int64(unsafe.Sizeof(r.Track)) + int64(unsafe.Sizeof(r.Entity))
}

func updateWorkingSet(ws *WorkingSetStats, r model.EntityResult) bool {
	changed := false
	if int64(len(r.Observations)) > ws.MaxEntityObservations {
		ws.MaxEntityObservations = int64(len(r.Observations))
		changed = true
	}
	if int64(len(r.Events)) > ws.MaxEntityEvents {
		ws.MaxEntityEvents = int64(len(r.Events))
		changed = true
	}
	if int64(len(r.Segments)) > ws.MaxEntitySegments {
		ws.MaxEntitySegments = int64(len(r.Segments))
		changed = true
	}
	if int64(len(r.Presence)) > ws.MaxEntityPresenceIntervals {
		ws.MaxEntityPresenceIntervals = int64(len(r.Presence))
		changed = true
	}
	if int64(len(r.Transitions)) > ws.MaxEntityTransitions {
		ws.MaxEntityTransitions = int64(len(r.Transitions))
		changed = true
	}
	if b := entityResultShallowBytes(r); b > ws.MaxEntityResultShallowBytes {
		ws.MaxEntityResultShallowBytes = b
		changed = true
	}
	return changed
}
