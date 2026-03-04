// Package storage provides interfaces and implementations for metrics persistence.
// It supports multiple storage backends: in-memory, file-based, and PostgreSQL.
package storage

import "github.com/SergeyDolin/metrics-and-alerting/internal/metrics"

// Storage defines the interface for metrics storage backends.
// It provides methods for updating and retrieving both gauge and counter metrics.
// This interface allows the application to work with different storage implementations
// (in-memory, file-based, database) without changing the business logic.
//
// Implementations must be thread-safe as they will be accessed concurrently
// by multiple HTTP handlers and background goroutines.
//
// Example usage:
//
//	var store storage.Storage
//
//	// Using in-memory storage
//	store = storage.NewMemStorage()
//
//	// Update a gauge metric
//	store.UpdateGauge("CPUUsage", 42.5)
//
//	// Update a counter metric
//	store.UpdateCounter("Requests", 1)
//
//	// Retrieve values
//	if value, exists := store.GetGauge("CPUUsage"); exists {
//	    fmt.Printf("CPU Usage: %f\n", value)
//	}
type Storage interface {
	// UpdateGauge updates or creates a gauge metric with the given name and value.
	// Gauge metrics represent values that can go up and down (e.g., CPU usage, memory usage).
	// If the metric already exists, its value is overwritten with the new value.
	//
	// Parameters:
	//   - name: The unique identifier of the metric
	//   - value: The new floating-point value for the gauge metric
	//
	// Returns:
	//   - error: nil if successful, otherwise an error describing what went wrong
	UpdateGauge(name string, value float64) error

	// UpdateCounter increments a counter metric by the given delta.
	// Counter metrics represent monotonically increasing values (e.g., request count, poll count).
	// If the metric doesn't exist, it is created with the delta as its initial value.
	// For existing counters, the delta is added to the current value.
	//
	// Parameters:
	//   - name: The unique identifier of the metric
	//   - delta: The amount to increment the counter by (must be positive in typical use)
	//
	// Returns:
	//   - error: nil if successful, otherwise an error describing what went wrong
	UpdateCounter(name string, delta int64) error

	// SetCounter sets a counter metric to an absolute value.
	// Unlike UpdateCounter which increments, this method sets the exact value.
	// This is useful for restoring counters from backups or when you need
	// to set a specific value rather than incrementing.
	//
	// Parameters:
	//   - name: The unique identifier of the metric
	//   - value: The new absolute value for the counter metric
	//
	// Returns:
	//   - error: nil if successful, otherwise an error describing what went wrong
	SetCounter(name string, value int64) error

	// GetGauge retrieves the current value of a gauge metric.
	//
	// Parameters:
	//   - name: The unique identifier of the metric to retrieve
	//
	// Returns:
	//   - float64: The current value of the gauge metric
	//   - bool: true if the metric exists, false if it doesn't
	GetGauge(name string) (float64, bool)

	// GetCounter retrieves the current value of a counter metric.
	//
	// Parameters:
	//   - name: The unique identifier of the metric to retrieve
	//
	// Returns:
	//   - int64: The current value of the counter metric
	//   - bool: true if the metric exists, false if it doesn't
	GetCounter(name string) (int64, bool)

	// GetAll returns all metrics (both gauge and counter) currently stored.
	// The metrics are returned as a slice of metrics.Metrics objects,
	// which contain both the type information and the value.
	//
	// Returns:
	//   - []metrics.Metrics: Slice containing all stored metrics
	//   - error: nil if successful, otherwise an error describing what went wrong
	GetAll() ([]metrics.Metrics, error)
}
