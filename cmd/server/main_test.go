package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
	"github.com/SergeyDolin/metrics-and-alerting/internal/storage"
)

func tempFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "metrics-*.json")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func Test_postHandler(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		url             string
		expectedStatus  int
		expectedBody    string
		expectedGauge   map[string]float64
		expectedCounter map[string]int64
	}{
		{
			name:           "Valid gauge update",
			method:         http.MethodPost,
			url:            "/update/gauge/temp/25.5",
			expectedStatus: http.StatusOK,
			expectedGauge:  map[string]float64{"temp": 25.5},
		},
		{
			name:            "Valid counter update",
			method:          http.MethodPost,
			url:             "/update/counter/req/10",
			expectedStatus:  http.StatusOK,
			expectedCounter: map[string]int64{"req": 10},
		},
		{
			name:            "Counter increment twice",
			method:          http.MethodPost,
			url:             "/update/counter/hits/7",
			expectedStatus:  http.StatusOK,
			expectedCounter: map[string]int64{"hits": 7},
		},
		{
			name:           "Invalid method",
			method:         http.MethodGet,
			url:            "/update/gauge/temp/1.0",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   "Only POST request allowed!",
		},
		{
			name:           "Invalid path format",
			method:         http.MethodPost,
			url:            "/update/gauge/temp",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Invalid path format",
		},
		{
			name:           "Unknown metric type",
			method:         http.MethodPost,
			url:            "/update/unknown/test/123",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Unknown metric type",
		},
		{
			name:           "Invalid gauge value",
			method:         http.MethodPost,
			url:            "/update/gauge/temp/abc",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Only Float type for Gauge allowed!",
		},
		{
			name:           "Invalid counter value",
			method:         http.MethodPost,
			url:            "/update/counter/req/xyz",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Only Int type for Counter allowed!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := storage.NewMemStorage()
			router := chi.NewRouter()
			router.Use(gzipMiddleware)
			router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "Only POST request allowed!", http.StatusMethodNotAllowed)
			})
			router.NotFound(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "Invalid path format", http.StatusNotFound)
			})
			router.Post("/update/{type}/{name}/{value}", postHandler(context.Background(), store, func() {}, nil))

			req := httptest.NewRequest(tt.method, tt.url, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code, "HTTP status mismatch")

			if tt.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedBody, "Response body mismatch")
			}

			for name, value := range tt.expectedGauge {
				got, exists := store.GetGauge(name)
				assert.True(t, exists, "Expected gauge metric %s not found", name)
				assert.Equal(t, value, got, "Gauge %s value mismatch", name)
			}

			for name, value := range tt.expectedCounter {
				got, exists := store.GetCounter(name)
				assert.True(t, exists, "Expected counter metric %s not found", name)
				assert.Equal(t, value, got, "Counter %s value mismatch", name)
			}
		})
	}
}

func Test_updateJSONHandler(t *testing.T) {
	tests := []struct {
		name            string
		jsonBody        string
		expectedStatus  int
		expectedBody    string
		expectedGauge   map[string]float64
		expectedCounter map[string]int64
	}{
		{
			name:           "Valid gauge metric",
			jsonBody:       `{"id": "Temperature", "type": "gauge", "value": 25.5}`,
			expectedStatus: http.StatusOK,
			expectedGauge:  map[string]float64{"Temperature": 25.5},
		},
		{
			name:            "Valid counter metric",
			jsonBody:        `{"id": "PollCount", "type": "counter", "delta": 10}`,
			expectedStatus:  http.StatusOK,
			expectedCounter: map[string]int64{"PollCount": 10},
		},
		{
			name:           "Invalid JSON",
			jsonBody:       `{"id": "Test", "type": "gauge", "value": `,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid JSON",
		},
		{
			name:           "Missing ID",
			jsonBody:       `{"type": "gauge", "value": 100}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Missing metric ID",
		},
		{
			name:           "Gauge without value",
			jsonBody:       `{"id": "Test", "type": "gauge"}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Missing 'value' for gauge metric",
		},
		{
			name:           "Gauge with delta (invalid)",
			jsonBody:       `{"id": "Test", "type": "gauge", "value": 1.0, "delta": 5}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Unexpected 'delta' for gauge metric",
		},
		{
			name:           "Counter without delta",
			jsonBody:       `{"id": "Test", "type": "counter"}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Missing 'delta' for counter metric",
		},
		{
			name:           "Counter with value (invalid)",
			jsonBody:       `{"id": "Test", "type": "counter", "delta": 1, "value": 5.0}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Unexpected 'value' for counter metric",
		},
		{
			name:           "Unknown metric type",
			jsonBody:       `{"id": "Test", "type": "unknown", "value": 100}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Unknown metric type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := storage.NewMemStorage()
			router := chi.NewRouter()
			oldFlagKey := flagKey
			flagKey = ""
			defer func() { flagKey = oldFlagKey }()

			router.Post("/update", updateJSONHandler(context.Background(), store, func() {}, nil))

			req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedBody)
			}

			for name, expectedValue := range tt.expectedGauge {
				actualValue, exists := store.GetGauge(name)
				assert.True(t, exists, "Gauge %s not found", name)
				assert.Equal(t, expectedValue, actualValue, "Gauge %s wrong value", name)
			}

			for name, expectedValue := range tt.expectedCounter {
				actualValue, exists := store.GetCounter(name)
				assert.True(t, exists, "Counter %s not found", name)
				assert.Equal(t, expectedValue, actualValue, "Counter %s wrong value", name)
			}
		})
	}
}

func Test_valueJSONHandler(t *testing.T) {
	store := storage.NewMemStorage()
	store.UpdateGauge(context.Background(), "Temperature", 25.5)
	store.UpdateCounter(context.Background(), "PollCount", 42)

	router := chi.NewRouter()
	oldFlagKey := flagKey
	flagKey = ""
	defer func() { flagKey = oldFlagKey }()

	router.Post("/value", valueJSONHandler(store, nil))

	tests := []struct {
		name           string
		jsonBody       string
		expectedStatus int
		expectedMetric *metrics.Metrics
		expectedError  string
	}{
		{
			name:           "Get existing gauge",
			jsonBody:       `{"id": "Temperature", "type": "gauge"}`,
			expectedStatus: http.StatusOK,
			expectedMetric: &metrics.Metrics{
				ID:    "Temperature",
				MType: "gauge",
				Value: func(v float64) *float64 { return &v }(25.5),
			},
		},
		{
			name:           "Get existing counter",
			jsonBody:       `{"id": "PollCount", "type": "counter"}`,
			expectedStatus: http.StatusOK,
			expectedMetric: &metrics.Metrics{
				ID:    "PollCount",
				MType: "counter",
				Delta: func(v int64) *int64 { return &v }(42),
			},
		},
		{
			name:           "Get non-existing gauge",
			jsonBody:       `{"id": "NonExistent", "type": "gauge"}`,
			expectedStatus: http.StatusNotFound,
			expectedError:  "Metric not found",
		},
		{
			name:           "Invalid JSON",
			jsonBody:       `{"id": "Test"`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid JSON",
		},
		{
			name:           "Missing type",
			jsonBody:       `{"id": "Temperature"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Missing ID or type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/value", strings.NewReader(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedError != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedError)
				return
			}

			if tt.expectedMetric != nil {
				var actual metrics.Metrics
				err := json.Unmarshal(rr.Body.Bytes(), &actual)
				require.NoError(t, err)

				assert.Equal(t, tt.expectedMetric.ID, actual.ID)
				assert.Equal(t, tt.expectedMetric.MType, actual.MType)

				if tt.expectedMetric.Value != nil {
					require.NotNil(t, actual.Value)
					assert.Equal(t, *tt.expectedMetric.Value, *actual.Value)
					assert.Nil(t, actual.Delta)
				}
				if tt.expectedMetric.Delta != nil {
					require.NotNil(t, actual.Delta)
					assert.Equal(t, *tt.expectedMetric.Delta, *actual.Delta)
					assert.Nil(t, actual.Value)
				}
			}
		})
	}
}

func Test_batchUpdateHandler(t *testing.T) {
	tests := []struct {
		name            string
		jsonBody        string
		expectedStatus  int
		expectedBody    string
		expectedGauge   map[string]float64
		expectedCounter map[string]int64
	}{
		{
			name: "Valid batch with gauge and counter",
			jsonBody: `[
				{"id": "Temperature", "type": "gauge", "value": 25.6},
				{"id": "PollCount", "type": "counter", "delta": 1}
			]`,
			expectedStatus:  http.StatusOK,
			expectedGauge:   map[string]float64{"Temperature": 25.6},
			expectedCounter: map[string]int64{"PollCount": 1},
		},
		{
			name:           "Empty batch",
			jsonBody:       `[]`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Empty batch not allowed",
		},
		{
			name: "Gauge without value",
			jsonBody: `[
				{"id": "Test", "type": "gauge"}
			]`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Missing 'value' for gauge metric Test",
		},
		{
			name: "Counter without delta",
			jsonBody: `[
				{"id": "Test", "type": "counter"}
			]`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Missing 'delta' for counter metric Test",
		},
		{
			name: "Invalid metric type",
			jsonBody: `[
				{"id": "Test", "type": "unknown", "value": 100}
			]`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Unknown metric type for Test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := storage.NewMemStorage()
			router := chi.NewRouter()
			oldFlagKey := flagKey
			flagKey = ""
			defer func() { flagKey = oldFlagKey }()

			router.Post("/updates", updatesBatchHandler(context.Background(), store, func() {}, nil))

			req := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedBody)
			}

			for name, expected := range tt.expectedGauge {
				actual, ok := store.GetGauge(name)
				assert.True(t, ok, "Gauge %s not found", name)
				assert.Equal(t, expected, actual, "Gauge %s value mismatch", name)
			}

			for name, expected := range tt.expectedCounter {
				actual, ok := store.GetCounter(name)
				assert.True(t, ok, "Counter %s not found", name)
				assert.Equal(t, expected, actual, "Counter %s value mismatch", name)
			}
		})
	}
}

func Test_batchUpdateHandler_Gzip(t *testing.T) {
	store := storage.NewMemStorage()
	router := chi.NewRouter()
	router.Use(gzipMiddleware)
	oldFlagKey := flagKey
	flagKey = ""
	defer func() { flagKey = oldFlagKey }()

	router.Post("/updates", updatesBatchHandler(context.Background(), store, func() {}, nil))

	data := `[{"id": "GzipTest", "type": "gauge", "value": 42.0}]`
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write([]byte(data))
	require.NoError(t, err)
	gz.Close()

	req := httptest.NewRequest(http.MethodPost, "/updates", &buf)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	got, ok := store.GetGauge("GzipTest")
	assert.True(t, ok)
	assert.Equal(t, 42.0, got)
}

func Test_getHandler(t *testing.T) {
	store := storage.NewMemStorage()
	store.UpdateGauge(context.Background(), "Temperature", 25.5)
	store.UpdateCounter(context.Background(), "PollCount", 42)

	router := chi.NewRouter()
	router.Get("/value/{type}/{name}", getHandler(store, nil))

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Get existing gauge",
			url:            "/value/gauge/Temperature",
			expectedStatus: http.StatusOK,
			expectedBody:   "25.5",
		},
		{
			name:           "Get existing counter",
			url:            "/value/counter/PollCount",
			expectedStatus: http.StatusOK,
			expectedBody:   "42",
		},
		{
			name:           "Get non-existing gauge",
			url:            "/value/gauge/NonExistent",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Unknown metric name",
		},
		{
			name:           "Invalid metric type",
			url:            "/value/unknown/Test",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Unknown metric type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.expectedBody)
		})
	}
}

func Test_indexHandler(t *testing.T) {
	store := storage.NewMemStorage()
	store.UpdateGauge(context.Background(), "Temperature", 25.5)
	store.UpdateCounter(context.Background(), "PollCount", 42)

	router := chi.NewRouter()
	router.Get("/", indexHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "Temperature")
	assert.Contains(t, rr.Body.String(), "25.5")
	assert.Contains(t, rr.Body.String(), "PollCount")
	assert.Contains(t, rr.Body.String(), "42")
}

func TestAuditPublisher(t *testing.T) {
	tmpFile := tempFile(t)

	fileObserver := NewFileWriterObserver(tmpFile)

	publisher := NewPublisher([]Observer{fileObserver})
	defer publisher.Close()

	event := AuditEvent{
		Timestamp: time.Now().Unix(),
		Metrics:   []string{"test_metric"},
		IPAddress: "127.0.0.1",
	}

	publisher.Notify(event)
	data, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "test_metric")
	assert.Contains(t, string(data), "127.0.0.1")
}
