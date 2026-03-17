package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/go-chi/chi"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
	"github.com/SergeyDolin/metrics-and-alerting/internal/sha256"
	"github.com/SergeyDolin/metrics-and-alerting/internal/storage"
	"github.com/SergeyDolin/metrics-and-alerting/internal/subnet"
)

// MetricType represents the type of metric (gauge or counter).
type MetricType string

const (
	// MetricTypeGauge represents a gauge metric type that stores floating-point values.
	// Gauge metrics can go up and down (e.g., CPU usage, memory usage)
	MetricTypeGauge MetricType = "gauge"

	// MetricTypeCounter represents a counter metric type that stores integer values.
	// Counter metrics are monotonically increasing (e.g., request count, poll count)
	MetricTypeCounter MetricType = "counter"
)

// writeJSONError writes an error response in JSON format.
// It sets the Content-Type header to application/json and writes the specified code and message.
//
// Parameters:
//   - w: HTTP response writer
//   - code: HTTP status code to return
//   - message: Error message to include in the JSON response
func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// indexHandler returns an HTTP handler that displays all metrics (gauge and counter) as HTML.
// The format is a list of metric names with their values.
// Supports only GET requests; returns 405 Method Not Allowed for other methods.
//
// Parameters:
//   - store: Storage interface for retrieving all metrics
//
// Returns:
//   - http.HandlerFunc: Handler function for the index endpoint
func indexHandler(store storage.Storage) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(res, "Only GET request allowed!", http.StatusMethodNotAllowed)
			return
		}
		metrics, err := store.GetAll()
		if err != nil {
			http.Error(res, "Failed to fetch metrics", http.StatusInternalServerError)
			return
		}

		res.Header().Set("Content-Type", "text/html; charset=utf-8")

		// Build HTML response with all metrics
		html := `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Metrics</title></head>
<body><h1>Metrics</h1><ul>`
		for _, m := range metrics {
			switch m.MType {
			case "gauge":
				if m.Value != nil {
					html += fmt.Sprintf("<li><strong>%s</strong>: %v (gauge)</li>", m.ID, *m.Value)
				}
			case "counter":
				if m.Delta != nil {
					html += fmt.Sprintf("<li><strong>%s</strong>: %v (counter)</li>", m.ID, *m.Delta)
				}
			}
		}
		html += `</ul></body></html>`
		io.WriteString(res, html)
	}
}

// getHandler returns an HTTP handler for retrieving the value of a specific metric by type and name.
// URL pattern: /value/{type}/{name}
// Supports only GET requests; returns 404 if the metric is not found or if the type is invalid.
// Valid types: "gauge" or "counter" (case-insensitive).
//
// Parameters:
//   - store: Storage interface for retrieving metrics
//   - auditPublisher: Optional publisher for audit logging (can be nil)
//
// Returns:
//   - http.HandlerFunc: Handler function for the value endpoint
func getHandler(store storage.Storage, auditPublisher *Publisher) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(res, "Only GET request allowed!", http.StatusMethodNotAllowed)
			return
		}

		metricType := strings.ToLower(chi.URLParam(req, "type"))
		metricName := chi.URLParam(req, "name")

		if auditPublisher != nil {
			ipAddress := subnet.ExtractIPFromRequest(req)
			event := AuditEvent{
				Timestamp: time.Now().Unix(),
				Metrics:   []string{metricName},
				IPAddress: ipAddress,
			}
			auditPublisher.Notify(event)
		}

		switch metricType {
		case "gauge":
			if value, exists := store.GetGauge(metricName); exists {
				// Log audit event if publisher is configured
				if auditPublisher != nil {
					ipAddress := getRealIP(req)
					event := AuditEvent{
						Timestamp: time.Now().Unix(),
						Metrics:   []string{metricName},
						IPAddress: ipAddress,
					}
					auditPublisher.Notify(event)
				}
				io.WriteString(res, fmt.Sprintf("%v", value))
				return
			}
			http.Error(res, "Unknown metric name", http.StatusNotFound)
			return

		case "counter":
			if value, exists := store.GetCounter(metricName); exists {
				// Log audit event if publisher is configured
				if auditPublisher != nil {
					ipAddress := getRealIP(req)
					event := AuditEvent{
						Timestamp: time.Now().Unix(),
						Metrics:   []string{metricName},
						IPAddress: ipAddress,
					}
					auditPublisher.Notify(event)
				}
				io.WriteString(res, fmt.Sprintf("%v", value))
				return
			}
			http.Error(res, "Unknown metric name", http.StatusNotFound)
			return

		default:
			http.Error(res, "Unknown metric type", http.StatusNotFound)
			return
		}
	}
}

// postHandler returns an HTTP handler for updating metrics via POST requests.
// URL pattern: /update/{type}/{name}/{value}
// Supports only POST requests; validates the value type based on the metric type:
// - gauge: requires float64
// - counter: requires int64
// On success, returns 200 OK; on errors, returns appropriate HTTP error codes.
//
// Parameters:
//   - store: Storage interface for updating metrics
//   - saveFunc: Function to persist metrics to disk/database
//   - auditPublisher: Optional publisher for audit logging (can be nil)
//
// Returns:
//   - http.HandlerFunc: Handler function for the update endpoint
func postHandler(ctx context.Context, store storage.Storage, saveFunc func(), auditPublisher *Publisher) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(res, "Only POST request allowed!", http.StatusMethodNotAllowed)
			return
		}

		metricType := strings.ToLower(chi.URLParam(req, "type"))
		name := chi.URLParam(req, "name")
		valueStr := chi.URLParam(req, "value")

		var err error
		switch metricType {
		case "gauge":
			var v float64
			if v, err = strconv.ParseFloat(valueStr, 64); err != nil {
				http.Error(res, "Only Float type for Gauge allowed!", http.StatusBadRequest)
				return
			}
			if err = store.UpdateGauge(ctx, name, v); err != nil {
				http.Error(res, "Failed to update metric", http.StatusInternalServerError)
				return
			}

		case "counter":
			var d int64
			if d, err = strconv.ParseInt(valueStr, 10, 64); err != nil {
				http.Error(res, "Only Int type for Counter allowed!", http.StatusBadRequest)
				return
			}
			if err = store.UpdateCounter(ctx, name, d); err != nil {
				http.Error(res, "Failed to update metric", http.StatusInternalServerError)
				return
			}

		default:
			http.Error(res, "Unknown metric type", http.StatusBadRequest)
			return
		}

		// Log audit event if publisher is configured
		if auditPublisher != nil {
			ipAddress := getRealIP(req)
			event := AuditEvent{
				Timestamp: time.Now().Unix(),
				Metrics:   []string{name},
				IPAddress: ipAddress,
			}
			auditPublisher.Notify(event)
		}

		saveFunc()
		res.WriteHeader(http.StatusOK)
	}
}

// updateJSONHandler handles updating metrics via JSON payload.
// Accepts a JSON object representing a metric with its type, name, and value.
// Validates the metric type and value format before updating the storage.
// Returns the updated metric in the response body along with appropriate HTTP status codes.
//
// Parameters:
//   - store: Storage interface for updating metrics
//   - saveFunc: Function to persist metrics to disk/database
//   - auditPublisher: Optional publisher for audit logging (can be nil)
//
// Returns:
//   - http.HandlerFunc: Handler function for the JSON update endpoint
func updateJSONHandler(ctx context.Context, store storage.Storage, saveFunc func(), auditPublisher *Publisher) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var m metrics.Metrics
		if err := json.NewDecoder(req.Body).Decode(&m); err != nil {
			writeJSONError(res, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if m.ID == "" {
			writeJSONError(res, http.StatusBadRequest, "Missing metric ID")
			return
		}

		// Validate and process based on metric type
		switch m.MType {
		case "gauge":
			if m.Value == nil {
				writeJSONError(res, http.StatusBadRequest, "Missing 'value' for gauge metric")
				return
			}
			if m.Delta != nil {
				writeJSONError(res, http.StatusBadRequest, "Unexpected 'delta' for gauge metric")
				return
			}
			if err := store.UpdateGauge(ctx, m.ID, *m.Value); err != nil {
				writeJSONError(res, http.StatusInternalServerError, "Storage error")
				return
			}

		case "counter":
			if m.Delta == nil {
				writeJSONError(res, http.StatusBadRequest, "Missing 'delta' for counter metric")
				return
			}
			if m.Value != nil {
				writeJSONError(res, http.StatusBadRequest, "Unexpected 'value' for counter metric")
				return
			}
			if err := store.UpdateCounter(ctx, m.ID, *m.Delta); err != nil {
				writeJSONError(res, http.StatusInternalServerError, "Storage error")
				return
			}

		default:
			writeJSONError(res, http.StatusBadRequest, "Unknown metric type")
			return
		}

		// Log audit event if publisher is configured
		if auditPublisher != nil {
			ipAddress := getRealIP(req)
			event := AuditEvent{
				Timestamp: time.Now().Unix(),
				Metrics:   []string{m.ID},
				IPAddress: ipAddress,
			}
			auditPublisher.Notify(event)
		}

		saveFunc()

		// Add HMAC signature if key is configured
		responseBody, _ := json.Marshal(m)
		if flagKey != "" {
			hash := sha256.ComputeHMACSHA256(responseBody, flagKey)
			res.Header().Set("HashSHA256", hash)
		}
		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(m)
	}
}

// valueJSONHandler handles retrieving metric values via JSON payload.
// Accepts a JSON object with metric ID and type, returns the current value.
// Returns 404 if the metric is not found.
//
// Parameters:
//   - store: Storage interface for retrieving metrics
//   - auditPublisher: Optional publisher for audit logging (can be nil)
//
// Returns:
//   - http.HandlerFunc: Handler function for the JSON value endpoint
func valueJSONHandler(store storage.Storage, auditPublisher *Publisher) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var r metrics.Metrics
		if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
			writeJSONError(res, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if r.ID == "" || r.MType == "" {
			writeJSONError(res, http.StatusBadRequest, "Missing ID or type")
			return
		}

		var resp metrics.Metrics
		found := false

		// Retrieve based on metric type
		switch r.MType {
		case "gauge":
			if v, ok := store.GetGauge(r.ID); ok {
				resp = metrics.Metrics{ID: r.ID, MType: "gauge", Value: &v}
				found = true
			}
		case "counter":
			if d, ok := store.GetCounter(r.ID); ok {
				resp = metrics.Metrics{ID: r.ID, MType: "counter", Delta: &d}
				found = true
			}
		default:
			writeJSONError(res, http.StatusBadRequest, "Unknown metric type")
			return
		}

		if !found {
			writeJSONError(res, http.StatusNotFound, "Metric not found")
			return
		}

		// Add HMAC signature if key is configured
		responseBody, _ := json.Marshal(resp)
		if flagKey != "" {
			hash := sha256.ComputeHMACSHA256(responseBody, flagKey)
			res.Header().Set("HashSHA256", hash)
		}

		// Log audit event if publisher is configured
		if auditPublisher != nil {
			ipAddress := getRealIP(req)
			event := AuditEvent{
				Timestamp: time.Now().Unix(),
				Metrics:   []string{r.ID},
				IPAddress: ipAddress,
			}
			auditPublisher.Notify(event)
		}

		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(resp)
	}
}

// pingSQLHandler checks the database connection health.
// Returns 200 OK if the database is reachable, 500 Internal Server Error otherwise.
//
// Parameters:
//   - store: Storage interface (must be *storage.DBStorage to check database connection)
//
// Returns:
//   - http.HandlerFunc: Handler function for the ping endpoint
func pingSQLHandler(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if storage is database-backed
		if dbStore, ok := store.(*storage.DBStorage); ok {
			if err := dbStore.Ping(context.Background()); err != nil {
				http.Error(w, "Couldn't connect to the database: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "DATABASE_DSN is not configured", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// updatesBatchHandler handles batch updates of multiple metrics in a single request.
// Accepts a JSON array of metrics and updates all of them atomically.
// Returns the updated batch in the response body.
//
// Parameters:
//   - store: Storage interface for updating metrics
//   - saveFunc: Function to persist metrics to disk/database
//   - auditPublisher: Optional publisher for audit logging (can be nil)
//
// Returns:
//   - http.HandlerFunc: Handler function for the batch update endpoint
func updatesBatchHandler(ctx context.Context, store storage.Storage, saveFunc func(), auditPublisher *Publisher) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var batch []metrics.Metrics
		if err := json.NewDecoder(req.Body).Decode(&batch); err != nil {
			writeJSONError(res, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if len(batch) == 0 {
			writeJSONError(res, http.StatusBadRequest, "Empty batch not allowed")
			return
		}

		// Validate each metric in the batch
		for _, m := range batch {
			if m.ID == "" {
				writeJSONError(res, http.StatusBadRequest, "Missing metric ID in batch")
				return
			}
			switch m.MType {
			case "gauge":
				if m.Value == nil {
					writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Missing 'value' for gauge metric %s", m.ID))
					return
				}
				if m.Delta != nil {
					writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Unexpected 'delta' for gauge metric %s", m.ID))
					return
				}
			case "counter":
				if m.Delta == nil {
					writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Missing 'delta' for counter metric %s", m.ID))
					return
				}
				if m.Value != nil {
					writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Unexpected 'value' for counter metric %s", m.ID))
					return
				}
			default:
				writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Unknown metric type for %s", m.ID))
				return
			}
		}

		// Update all metrics
		for _, m := range batch {
			var err error
			switch m.MType {
			case "gauge":
				err = store.UpdateGauge(ctx, m.ID, *m.Value)
			case "counter":
				err = store.UpdateCounter(ctx, m.ID, *m.Delta)
			}
			if err != nil {
				writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Storage error during batch update %s", m.ID))
				return
			}
		}

		// Log batch audit event if publisher is configured
		if auditPublisher != nil {
			ipAddress := getRealIP(req)
			metricNames := make([]string, len(batch))
			for i, m := range batch {
				metricNames[i] = m.ID
			}
			event := AuditEvent{
				Timestamp: time.Now().Unix(),
				Metrics:   metricNames,
				IPAddress: ipAddress,
			}
			auditPublisher.Notify(event)
		}

		saveFunc()

		// Add HMAC signature if key is configured
		responseBody, _ := json.Marshal(batch)
		if flagKey != "" {
			hash := sha256.ComputeHMACSHA256(responseBody, flagKey)
			res.Header().Set("HashSHA256", hash)
		}
		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(batch)
	}
}

// getRealIP extracts the real client IP address from the request headers.
// It checks in order:
//  1. X-Forwarded-For header (taking the first IP in case of a list)
//  2. X-Real-IP header
//  3. RemoteAddr as fallback
//
// This is useful when the server is behind a reverse proxy.
//
// Parameters:
//   - req: HTTP request object
//
// Returns:
//   - string: The client's IP address
func getRealIP(req *http.Request) string {
	// Check X-Forwarded-For header
	if forwarded := req.Header.Get("X-Forwarded-For"); forwarded != "" {
		// Take the first IP in case there are multiple
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if realIP := req.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	// Fallback to RemoteAddr
	host, _, _ := net.SplitHostPort(req.RemoteAddr)
	return host
}
