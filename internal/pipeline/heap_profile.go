package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"
)

type HeapProfileInfo struct {
	Label            string `json:"label"`
	Path             string `json:"path"`
	Bytes            int64  `json:"bytes"`
	CurrentRSSBytes  int64  `json:"current_rss_bytes,omitempty"`
	GoHeapAllocBytes int64  `json:"go_heap_alloc_bytes"`
	GoHeapInuseBytes int64  `json:"go_heap_inuse_bytes"`
	GoSysBytes       int64  `json:"go_sys_bytes"`
}

type heapProfiler struct {
	dir      string
	enabled  bool
	profiles []HeapProfileInfo
	seconds  float64
}

func newHeapProfiler(dir string) (*heapProfiler, error) {
	h := &heapProfiler{}
	if strings.TrimSpace(dir) == "" {
		return h, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve --profile-memory: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create --profile-memory directory: %w", err)
	}
	h.dir = abs
	h.enabled = true
	return h, nil
}

func (h *heapProfiler) Capture(label string) error {
	if h == nil || !h.enabled {
		return nil
	}
	start := time.Now()
	// Heap profiles describe live allocations as of the most recent GC. In
	// profiling mode we force a collection so snapshots are useful for retained
	// heap diagnosis. This intentionally perturbs benchmark timings.
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	rss, _ := currentRSSBytes()
	name := sanitizeProfileLabel(label) + ".heap"
	path := filepath.Join(h.dir, name)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create heap profile %s: %w", label, err)
	}
	if err := pprof.WriteHeapProfile(f); err != nil {
		_ = f.Close()
		return fmt.Errorf("write heap profile %s: %w", label, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close heap profile %s: %w", label, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	h.profiles = append(h.profiles, HeapProfileInfo{
		Label: label, Path: path, Bytes: info.Size(), CurrentRSSBytes: rss,
		GoHeapAllocBytes: int64(ms.HeapAlloc), GoHeapInuseBytes: int64(ms.HeapInuse), GoSysBytes: int64(ms.Sys),
	})
	h.seconds += time.Since(start).Seconds()
	return nil
}

func sanitizeProfileLabel(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
