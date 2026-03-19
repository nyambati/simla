// Package metrics provides lightweight in-process invocation metrics for
// Lambda services. It tracks invocation count, total/average duration, and
// error count per service name. All operations are safe for concurrent use.
package metrics

import (
	"sync"
	"time"
)

// ServiceMetrics holds the accumulated metrics for a single service.
type ServiceMetrics struct {
	Invocations  int64
	Errors       int64
	TotalLatency time.Duration
	LastInvoked  time.Time
}

// AvgLatency returns the mean invocation duration, or zero if no invocations
// have been recorded.
func (m *ServiceMetrics) AvgLatency() time.Duration {
	if m.Invocations == 0 {
		return 0
	}
	return m.TotalLatency / time.Duration(m.Invocations)
}

// ErrorRate returns the fraction of invocations that resulted in an error,
// in the range [0.0, 1.0]. Returns 0 when there are no invocations.
func (m *ServiceMetrics) ErrorRate() float64 {
	if m.Invocations == 0 {
		return 0
	}
	return float64(m.Errors) / float64(m.Invocations)
}

// Recorder accumulates per-service invocation metrics.
type Recorder struct {
	mu      sync.RWMutex
	metrics map[string]*ServiceMetrics
}

// NewRecorder returns an initialised Recorder.
func NewRecorder() *Recorder {
	return &Recorder{
		metrics: make(map[string]*ServiceMetrics),
	}
}

// Record registers the result of one Lambda invocation.
// duration is the wall-clock time from sending the request to receiving the
// response. errored should be true when the invocation returned an error.
func (r *Recorder) Record(serviceName string, duration time.Duration, errored bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.metrics[serviceName]
	if !ok {
		m = &ServiceMetrics{}
		r.metrics[serviceName] = m
	}

	m.Invocations++
	m.TotalLatency += duration
	m.LastInvoked = time.Now()
	if errored {
		m.Errors++
	}
}

// Get returns a snapshot of metrics for the named service.
// The second return value is false when no invocations have been recorded yet.
func (r *Recorder) Get(serviceName string) (ServiceMetrics, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.metrics[serviceName]
	if !ok {
		return ServiceMetrics{}, false
	}
	return *m, true
}

// All returns a snapshot of metrics for every tracked service.
func (r *Recorder) All() map[string]ServiceMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]ServiceMetrics, len(r.metrics))
	for k, v := range r.metrics {
		out[k] = *v
	}
	return out
}
