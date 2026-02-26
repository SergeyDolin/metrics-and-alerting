package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// AuditEvent represents an audit log entry containing information about
// when metrics were accessed and by whom. This is useful for compliance,
// debugging, and monitoring access patterns.
type AuditEvent struct {
	Timestamp int64    `json:"ts"`         // Unix timestamp when the event occurred
	Metrics   []string `json:"metrics"`    // Names of metrics that were accessed
	IPAddress string   `json:"ip_address"` // IP address of the client that accessed the metrics
}

// Observer defines the interface for components that want to receive
// audit event notifications. This follows the Observer design pattern
// allowing multiple audit logging mechanisms to be plugged in dynamically.
type Observer interface {
	// Notify sends an audit event to the observer for processing.
	// Returns an error if the notification fails.
	Notify(AuditEvent) error

	// Close performs any necessary cleanup when the observer is no longer needed.
	Close() error
}

// Publisher manages a collection of observers and broadcasts audit events
// to all registered observers. It provides thread-safe registration and
// notification capabilities.
type Publisher struct {
	observers []Observer   // Slice of registered observers
	mutex     sync.RWMutex // Mutex for thread-safe operations
}

// NewPublisher creates a new Publisher instance with the provided initial observers.
//
// Parameters:
//   - observers: Initial slice of observers to register
//
// Returns:
//   - *Publisher: A configured publisher ready to use
func NewPublisher(observers []Observer) *Publisher {
	return &Publisher{
		observers: observers,
	}
}

// Register adds a new observer to the publisher's list.
// This operation is thread-safe.
//
// Parameters:
//   - observer: The observer to register for future notifications
func (p *Publisher) Register(observer Observer) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.observers = append(p.observers, observer)
}

// Notify broadcasts an audit event to all registered observers.
// Each observer is notified sequentially, and any errors are logged
// to stderr but do not interrupt notification of other observers.
//
// Parameters:
//   - event: The audit event to broadcast to all observers
func (p *Publisher) Notify(event AuditEvent) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	// Notify each observer, logging errors but continuing
	for _, observer := range p.observers {
		if err := observer.Notify(event); err != nil {
			fmt.Fprintf(os.Stderr, "Error notifying observer: %v\n", err)
		}
	}
}

// Close gracefully shuts down all registered observers.
// This should be called when the application is shutting down
// to ensure all observers can perform cleanup.
func (p *Publisher) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Close each observer
	for _, observer := range p.observers {
		observer.Close()
	}
}

// FileWriterObserver implements the Observer interface by writing
// audit events to a file in JSON format, one event per line.
// This is useful for persistent audit logging and offline analysis.
type FileWriterObserver struct {
	filePath string     // Path to the audit log file
	mutex    sync.Mutex // Mutex to prevent concurrent file writes
}

// NewFileWriterObserver creates a new FileWriterObserver that writes
// audit events to the specified file path.
//
// Parameters:
//   - filePath: Path where audit logs will be written
//
// Returns:
//   - *FileWriterObserver: A configured file writer observer
func NewFileWriterObserver(filePath string) *FileWriterObserver {
	return &FileWriterObserver{
		filePath: filePath,
	}
}

// Notify writes an audit event to the configured file in JSON format.
// Each event is written on a new line, making the file easily parsable.
//
// Parameters:
//   - event: The audit event to write to the file
//
// Returns:
//   - error: nil if successful, otherwise an error describing what went wrong
func (fw *FileWriterObserver) Notify(event AuditEvent) error {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	// Marshal the event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	// Open the file with create/write/append flags and appropriate permissions
	file, err := os.OpenFile(fw.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open audit file: %w", err)
	}
	defer file.Close()

	// Write the JSON data to the file
	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to audit file: %w", err)
	}

	// Add a newline to separate events
	_, err = file.WriteString("\n")
	if err != nil {
		return fmt.Errorf("failed to write newline to audit file: %w", err)
	}

	return nil
}

// Close implements the Observer interface for FileWriterObserver.
// No cleanup is needed for file operations as the file is closed after each write.
func (fw *FileWriterObserver) Close() error {
	return nil
}

// HTTPSenderObserver implements the Observer interface by sending
// audit events to a remote HTTP endpoint. This enables centralized
// audit logging across multiple services.
type HTTPSenderObserver struct {
	url    string       // HTTP endpoint URL for sending audit events
	client *http.Client // HTTP client with configured timeout
}

// NewHTTPSenderObserver creates a new HTTPSenderObserver that sends
// audit events to the specified URL.
//
// Parameters:
//   - url: The HTTP endpoint URL (e.g., "http://audit-server:8080/events")
//
// Returns:
//   - *HTTPSenderObserver: A configured HTTP sender observer with a 10-second timeout
func NewHTTPSenderObserver(url string) *HTTPSenderObserver {
	return &HTTPSenderObserver{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second, // Prevent hanging requests
		},
	}
}

// Notify sends an audit event to the configured HTTP endpoint as a POST request
// with JSON content type.
//
// Parameters:
//   - event: The audit event to send to the remote endpoint
//
// Returns:
//   - error: nil if successful (HTTP 2xx status code), otherwise an error describing
//     what went wrong (network issues, non-2xx response, etc.)
func (hs *HTTPSenderObserver) Notify(event AuditEvent) error {
	// Marshal the event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	// Send POST request with JSON body
	resp, err := hs.client.Post(hs.url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to send audit event to %s: %w", hs.url, err)
	}
	defer resp.Body.Close()

	// Check for successful response (2xx status code)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received non-success status code %d from %s", resp.StatusCode, hs.url)
	}

	return nil
}

// Close implements the Observer interface for HTTPSenderObserver.
// The HTTP client doesn't require explicit cleanup.
func (hs *HTTPSenderObserver) Close() error {
	return nil
}
