package main

import (
	"log"
	"sync"
)

// Metrics represents a single metric that can be sent to the monitoring server.
// It follows the JSON format expected by the server API and uses pointers for
// optional fields to distinguish between zero values and omitted fields.
//
// The struct supports two types of metrics:
//   - gauge: A floating-point value that can go up and down (e.g., CPU usage, memory usage)
//   - counter: A monotonically increasing integer value (e.g., request count, poll count)
type Metrics struct {
	// ID is the unique identifier/name of the metric (e.g., "Alloc", "PollCount", "CPUUtilization")
	ID string `json:"id"`

	// MType specifies the metric type - either "gauge" or "counter"
	MType string `json:"type"`

	// Delta is used for counter metrics and represents the change in value.
	// It's a pointer to distinguish between a zero value and no value being provided.
	Delta *int64 `json:"delta,omitempty"`

	// Value is used for gauge metrics and represents the current value.
	// It's a pointer to distinguish between a zero value and no value being provided.
	Value *float64 `json:"value,omitempty"`
}

// MetricQueue provides a thread-safe, buffered queue for metrics with a simple
// producer-consumer pattern. It allows metrics collectors (producers) to push
// metrics while workers (consumers) pop them for processing.
//
// The queue includes a sync.Pool for Metrics object reuse to reduce garbage
// collection pressure, though currently the pool is not actively used in Push/Pop.
type MetricQueue struct {
	// queue is a buffered channel that holds metrics waiting to be processed
	queue chan Metrics

	// pool is a sync.Pool for reusing Metrics objects to reduce allocations
	// Note: Currently the pool is created but not utilized in the queue operations
	pool sync.Pool
}

// NewMetricQueue creates and initializes a new MetricQueue with the specified buffer size.
//
// Parameters:
//   - size: The maximum number of metrics that can be queued before blocking/dropping
//
// Returns:
//   - *MetricQueue: A pointer to the initialized queue ready for use
func NewMetricQueue(size int) *MetricQueue {
	return &MetricQueue{
		// Create a buffered channel with capacity of 100 (size parameter is ignored)
		// FIXME: The size parameter is passed but hardcoded to 100 - consider using size
		queue: make(chan Metrics, 100),
		pool: sync.Pool{
			New: func() interface{} {
				return &Metrics{}
			},
		},
	}
}

// Push adds a metric to the queue for processing. If the queue is full,
// the metric is dropped and a warning is logged to prevent blocking the
// collection goroutine.
//
// This is a non-blocking operation that uses a select statement with a default
// case to handle full queue scenarios gracefully.
//
// Parameters:
//   - metric: The Metrics object to be queued for processing
func (mq *MetricQueue) Push(metric Metrics) {
	select {
	case mq.queue <- metric:
		// Successfully added to queue
	default:
		// Queue is full - log warning and drop the metric
		log.Printf("Queue full, dropping metric %s\n", metric.ID)
	}
}

// Pop retrieves and removes a metric from the queue for processing.
// This is a blocking operation that will wait until a metric becomes available
// if the queue is empty.
//
// Returns:
//   - Metrics: The next metric in the queue to be processed
func (mq *MetricQueue) Pop() Metrics {
	return <-mq.queue
}

// IsEmpty checks whether the queue currently has any pending metrics.
// This can be used to determine if there's work to do without blocking.
//
// Returns:
//   - bool: true if there are no metrics in the queue, false otherwise
func (mq *MetricQueue) IsEmpty() bool {
	return len(mq.queue) == 0
}
