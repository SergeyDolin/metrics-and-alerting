package storage

import (
	"sync"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
)

// MemStorage implements the Storage interface using in-memory maps.
// It provides thread-safe storage for both gauge and counter metrics
// using read-write mutexes for concurrent access.
//
// This is the simplest storage implementation and serves as the foundation
// for other storage types (like FileStorage which embeds MemStorage).
// It's suitable for testing and scenarios where persistence is not required.
//
// generate:reset
type MemStorage struct {
	// gauge stores floating-point metrics with their names as keys
	gauge map[string]float64

	// counter stores integer counter metrics with their names as keys
	counter map[string]int64

	// mu protects both maps from concurrent access
	mu sync.RWMutex
}

// NewMemStorage creates and initializes a new in-memory storage.
// It initializes empty maps for both gauge and counter metrics.
//
// Returns:
//   - *MemStorage: A ready-to-use memory storage instance
func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauge:   make(map[string]float64),
		counter: make(map[string]int64),
	}
}

// UpdateGauge sets a gauge metric to the specified value.
// If the metric already exists, its value is overwritten.
// This operation is thread-safe and acquires a write lock.
//
// Parameters:
//   - name: The metric name/identifier
//   - value: The new floating-point value for the gauge
//
// Returns:
//   - error: Always nil (kept for interface compatibility)
func (s *MemStorage) UpdateGauge(name string, value float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gauge[name] = value
	return nil
}

// UpdateCounter increments a counter metric by the specified delta.
// If the metric doesn't exist, it's created with the delta as its value.
// This operation is thread-safe and acquires a write lock.
//
// Parameters:
//   - name: The metric name/identifier
//   - delta: The amount to add to the counter
//
// Returns:
//   - error: Always nil (kept for interface compatibility)
func (s *MemStorage) UpdateCounter(name string, delta int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter[name] += delta
	return nil
}

// SetCounter sets a counter metric to an absolute value.
// Unlike UpdateCounter which increments, this sets the exact value.
// This is useful for restoring counters from backups or when you need
// to set a specific value rather than incrementing.
//
// Parameters:
//   - name: The metric name/identifier
//   - value: The new absolute value for the counter
//
// Returns:
//   - error: Always nil (kept for interface compatibility)
func (s *MemStorage) SetCounter(name string, value int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter[name] = value
	return nil
}

// GetGauge retrieves the current value of a gauge metric.
// This operation is thread-safe and acquires a read lock.
//
// Parameters:
//   - name: The metric name/identifier to retrieve
//
// Returns:
//   - float64: The current value of the gauge metric
//   - bool: true if the metric exists, false otherwise
func (s *MemStorage) GetGauge(name string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.gauge[name]
	return v, ok
}

// GetCounter retrieves the current value of a counter metric.
// This operation is thread-safe and acquires a read lock.
//
// Parameters:
//   - name: The metric name/identifier to retrieve
//
// Returns:
//   - int64: The current value of the counter metric
//   - bool: true if the metric exists, false otherwise
func (s *MemStorage) GetCounter(name string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.counter[name]
	return v, ok
}

// GetAll returns all metrics currently stored in memory.
// The metrics are returned as a slice of metrics.Metrics objects,
// with separate entries for gauge and counter metrics.
// This operation is thread-safe and acquires a read lock.
//
// Returns:
//   - []metrics.Metrics: Slice containing all stored metrics
//   - error: Always nil (kept for interface compatibility)
func (s *MemStorage) GetAll() ([]metrics.Metrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []metrics.Metrics

	// Add all gauge metrics
	for name, v := range s.gauge {
		val := v // Create a copy to avoid pointer issues
		out = append(out, metrics.Metrics{ID: name, MType: "gauge", Value: &val})
	}

	// Add all counter metrics
	for name, d := range s.counter {
		delta := d // Create a copy to avoid pointer issues
		out = append(out, metrics.Metrics{ID: name, MType: "counter", Delta: &delta})
	}

	return out, nil
}
