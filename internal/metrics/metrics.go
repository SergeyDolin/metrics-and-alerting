package metrics

// Metrics represents a single metric that can be exchanged between the agent and server.
// It follows the JSON format required by the API and uses pointers for optional fields
// to distinguish between zero values and omitted fields in JSON serialization.
//
// The struct supports two types of metrics:
//   - gauge: A floating-point value that can go up and down (e.g., CPU usage, memory usage)
//   - counter: A monotonically increasing integer value (e.g., request count, poll count)
//
// Example JSON representations:
//
// Gauge metric:
//
//	{"id":"Alloc","type":"gauge","value":42.5}
//
// Counter metric:
//
//	{"id":"PollCount","type":"counter","delta":10}
//
// Signed metric (with HMAC):
//
//	{"id":"Alloc","type":"gauge","value":42.5,"hash":"5d4f3c8e2a1b9f7d6c5e4a3b2c1d0e9f8a7b6c5d"}
//
// generate:reset
type Metrics struct {
	// ID is the unique identifier/name of the metric.
	// Examples: "Alloc", "PollCount", "CPUUtilization1", "TotalMemory"
	ID string `json:"id"`

	// MType specifies the metric type - either "gauge" or "counter".
	// This field determines which of Delta or Value should be used.
	MType string `json:"type"`

	// Delta is used for counter metrics and represents the change/increment value.
	// It's a pointer to distinguish between a zero value (0) and no value being provided.
	// This field is omitted from JSON when nil (using omitempty tag).
	Delta *int64 `json:"delta,omitempty"`

	// Value is used for gauge metrics and represents the current value.
	// It's a pointer to distinguish between a zero value (0.0) and no value being provided.
	// This field is omitted from JSON when nil (using omitempty tag).
	Value *float64 `json:"value,omitempty"`

	// Hash contains an optional HMAC-SHA256 signature of the metric data.
	// Used for data integrity verification between agent and server.
	// When present, the receiver should verify that the hash matches
	// the calculated hash using the shared secret key.
	// This field is omitted from JSON when empty (using omitempty tag).
	Hash string `json:"hash,omitempty"`
}
