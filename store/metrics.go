package store

import (
	"sync"
	"time"
)

// Query operation kinds tracked per dataset.
const (
	OpList   = "list"
	OpGet    = "get"
	OpCreate = "create"
	OpUpdate = "update"
	OpDelete = "delete"
	OpFilter = "filter"
)

// opStat accumulates call count and total latency for one (op, contentType)
// pair. Average latency is derived on snapshot.
type opStat struct {
	Count      int64
	TotalNanos int64
}

// Metrics records per-dataset query analytics: how many of each operation
// kind ran, broken down by content type, with latency aggregates. It is
// safe for concurrent use and lives entirely in memory (reset on unload).
type Metrics struct {
	mu sync.Mutex
	// op -> contentType -> stat
	ops map[string]map[string]*opStat
}

func newMetrics() *Metrics {
	return &Metrics{ops: make(map[string]map[string]*opStat)}
}

// Record adds one observation for (op, contentType) with the given latency.
func (m *Metrics) Record(op, contentType string, d time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	byType := m.ops[op]
	if byType == nil {
		byType = make(map[string]*opStat)
		m.ops[op] = byType
	}
	s := byType[contentType]
	if s == nil {
		s = &opStat{}
		byType[contentType] = s
	}
	s.Count++
	s.TotalNanos += d.Nanoseconds()
}

// OpStatReport is the JSON-friendly form of an opStat.
type OpStatReport struct {
	Count   int64   `json:"count"`
	AvgMs   float64 `json:"avg_ms"`
	TotalMs float64 `json:"total_ms"`
}

// MetricsReport is a point-in-time snapshot of a dataset's query metrics.
type MetricsReport struct {
	TotalQueries int64 `json:"total_queries"`
	// op -> contentType -> stat
	ByOp map[string]map[string]OpStatReport `json:"by_op"`
}

// Snapshot returns a copy of the current metrics in JSON-friendly form.
func (m *Metrics) Snapshot() MetricsReport {
	rep := MetricsReport{ByOp: make(map[string]map[string]OpStatReport)}
	if m == nil {
		return rep
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for op, byType := range m.ops {
		out := make(map[string]OpStatReport, len(byType))
		for ct, s := range byType {
			var avgMs float64
			if s.Count > 0 {
				avgMs = float64(s.TotalNanos) / float64(s.Count) / 1e6
			}
			out[ct] = OpStatReport{
				Count:   s.Count,
				AvgMs:   avgMs,
				TotalMs: float64(s.TotalNanos) / 1e6,
			}
			rep.TotalQueries += s.Count
		}
		rep.ByOp[op] = out
	}
	return rep
}
