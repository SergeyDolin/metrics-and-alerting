package storage

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"syscall"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
)

// FileStorage implements the Storage interface by persisting metrics to a JSON file on disk.
// It embeds MemStorage to provide in-memory storage and adds file persistence capabilities.
// The storage supports both synchronous saves (after each update) and can be used with
// periodic save strategies by calling Save() manually.
//
// File format: JSON array of metrics.Metrics objects, one per line or pretty-printed.
// Example:
// [
//
//	{"id":"Alloc","type":"gauge","value":42.5},
//	{"id":"PollCount","type":"counter","delta":10}
//
// ]
//
// generate:reset
type FileStorage struct {
	*MemStorage            // Embedded in-memory storage for fast access
	filePath    string     // Path to the JSON file for persistence
	mu          sync.Mutex // Mutex to prevent concurrent file writes
}

// NewFileStorage creates a new FileStorage instance and loads existing data from the specified file.
// If the file doesn't exist, it creates an empty storage.
//
// Parameters:
//   - filePath: Path to the JSON file for persistence (e.g., "/tmp/metrics.json")
//
// Returns:
//   - *FileStorage: Initialized file storage with loaded data
//   - error: Any error during file loading (except file not found)
func NewFileStorage(filePath string) (*FileStorage, error) {
	s := &FileStorage{
		MemStorage: NewMemStorage(),
		filePath:   filePath,
	}
	// Load existing data from file (if it exists)
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateGauge updates a gauge metric and immediately persists the change to disk.
// This provides synchronous persistence guarantee.
//
// Parameters:
//   - name: Metric name
//   - value: New gauge value
//
// Returns:
//   - error: Any error during in-memory update or file save
func (s *FileStorage) UpdateGauge(ctx context.Context, name string, value float64) error {
	// Update in-memory storage first
	if err := s.MemStorage.UpdateGauge(ctx, name, value); err != nil {
		return err
	}
	// Persist to disk synchronously
	return s.Save()
}

// UpdateCounter increments a counter metric and immediately persists the change to disk.
// This provides synchronous persistence guarantee.
//
// Parameters:
//   - name: Metric name
//   - delta: Amount to increment by
//
// Returns:
//   - error: Any error during in-memory update or file save
func (s *FileStorage) UpdateCounter(ctx context.Context, name string, delta int64) error {
	// Update in-memory storage first
	if err := s.MemStorage.UpdateCounter(ctx, name, delta); err != nil {
		return err
	}
	// Persist to disk synchronously
	return s.Save()
}

// SetCounter sets a counter metric to an absolute value and immediately persists the change to disk.
// This provides synchronous persistence guarantee.
//
// Parameters:
//   - name: Metric name
//   - value: New absolute value
//
// Returns:
//   - error: Any error during in-memory update or file save
func (s *FileStorage) SetCounter(ctx context.Context, name string, value int64) error {
	// Update in-memory storage first
	if err := s.MemStorage.SetCounter(ctx, name, value); err != nil {
		return err
	}
	// Persist to disk synchronously
	return s.Save()
}

// Save persists all current metrics from memory to the JSON file.
// This method is thread-safe and uses a mutex to prevent concurrent file writes.
// The file is written with O_SYNC flag to ensure data is written to disk immediately.
//
// Returns:
//   - error: Any error during JSON marshaling or file write
func (s *FileStorage) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get all metrics from in-memory storage
	metricsList, err := s.MemStorage.GetAll()
	if err != nil {
		return err
	}

	// Marshal to JSON with indentation for human readability
	data, err := json.MarshalIndent(metricsList, "", "  ")
	if err != nil {
		return err
	}

	// Open file with O_SYNC flag for immediate disk write
	// O_TRUNC ensures we overwrite the file completely
	file, err := os.OpenFile(s.filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_SYNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write JSON data to file
	_, err = file.Write(data)
	return err
}

// load reads metrics from the JSON file and populates the in-memory storage.
// If the file doesn't exist, it silently returns nil (starting with empty storage).
// If the file exists but is corrupted, it returns an error.
//
// Returns:
//   - error: Any error during file read or JSON unmarshaling (except file not found)
func (s *FileStorage) load() error {
	// Read file contents
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		// If file doesn't exist, start with empty storage
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Unmarshal JSON array
	var metricsList []metrics.Metrics
	if err := json.Unmarshal(data, &metricsList); err != nil {
		return err
	}

	// Populate in-memory storage
	for _, m := range metricsList {
		switch m.MType {
		case "gauge":
			if m.Value != nil {
				s.gauge[m.ID] = *m.Value
			}
		case "counter":
			if m.Delta != nil {
				s.counter[m.ID] = *m.Delta
			}
		}
	}
	return nil
}
