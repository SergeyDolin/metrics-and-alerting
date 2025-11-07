package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-chi/chi"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
)

const testDB = "postgres://user:pass@localhost:5432/postgres?sslmode=disable"

func Test_postHandler(t *testing.T) {
	ms := &MetricStorage{
		gauge:   make(map[string]float64),
		counter: make(map[string]int64),
	}
	router := chi.NewRouter()
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Only POST request allowed!", http.StatusMethodNotAllowed)
	})
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Invalid path format", http.StatusNotFound)
	})
	router.Post("/update/{type}/{name}/{value}", postHandler(ms, func() {}))

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
			name:            "Valid gauge update",
			method:          http.MethodPost,
			url:             "/update/gauge/temp/25.5",
			expectedStatus:  http.StatusOK,
			expectedGauge:   map[string]float64{"temp": 25.5},
			expectedCounter: map[string]int64{},
		},
		{
			name:            "Valid counter update",
			method:          http.MethodPost,
			url:             "/update/counter/req/10",
			expectedStatus:  http.StatusOK,
			expectedGauge:   map[string]float64{},
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
			req := httptest.NewRequest(tt.method, tt.url, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code, "HTTP status mismatch")

			if tt.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedBody, "Response body mismatch")
			}

			for name, value := range tt.expectedGauge {
				got, exists := ms.gauge[name]
				assert.True(t, exists, "Expected gauge metric %s not found", name)
				assert.Equal(t, value, got, "Gauge %s value mismatch", name)
			}

			for name, value := range tt.expectedCounter {
				got, exists := ms.counter[name]
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
			name:            "Counter increment twice",
			jsonBody:        `{"id": "Hits", "type": "counter", "delta": 7}`,
			expectedStatus:  http.StatusOK,
			expectedCounter: map[string]int64{"Hits": 7},
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
		{
			name:           "Counter with null delta",
			jsonBody:       `{"id": "Test", "type": "counter", "delta": null}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Missing 'delta' for counter metric",
		},
		{
			name:           "Gauge with null value",
			jsonBody:       `{"id": "Test", "type": "gauge", "value": null}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Missing 'value' for gauge metric",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &MetricStorage{
				gauge:   make(map[string]float64),
				counter: make(map[string]int64),
			}
			router := chi.NewRouter()
			router.Post("/update", updateJSONHandler(ms, func() {}))

			req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code, "Unexpected status code")

			if tt.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedBody, "Response body does not contain expected message")
			}

			for name, expectedValue := range tt.expectedGauge {
				actualValue, exists := ms.gauge[name]
				assert.True(t, exists, "Gauge metric %s not found", name)
				assert.Equal(t, expectedValue, actualValue, "Gauge metric %s has wrong value", name)
			}

			for name, expectedValue := range tt.expectedCounter {
				actualValue, exists := ms.counter[name]
				assert.True(t, exists, "Counter metric %s not found", name)
				assert.Equal(t, expectedValue, actualValue, "Counter metric %s has wrong value", name)
			}
		})
	}
}

func Test_valueJSONHandler(t *testing.T) {
	ms := &MetricStorage{
		gauge:   make(map[string]float64),
		counter: make(map[string]int64),
	}
	ms.updateGauge("Temperature", 25.5)
	ms.updateCounter("PollCount", 42)

	router := chi.NewRouter()
	router.Post("/value", valueJSONHandler(ms))

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
		},
		{
			name:           "Get non-existing counter",
			jsonBody:       `{"id": "Unknown", "type": "counter"}`,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Invalid JSON",
			jsonBody:       `{"id": "Test"`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid JSON",
		},
		{
			name:           "Missing ID",
			jsonBody:       `{"type": "gauge"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Missing metric ID",
		},
		{
			name:           "Missing type",
			jsonBody:       `{"id": "Test"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Missing metric type",
		},
		{
			name:           "Unknown metric type",
			jsonBody:       `{"id": "Test", "type": "unknown"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Unknown metric type",
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

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	migrationsDir := filepath.Join(projectRoot, "migrations")
	migrationsDir, err := filepath.Abs(migrationsDir)
	require.NoError(t, err)

	t.Logf("Using migrations dir: %s", migrationsDir)
	_, err = os.Stat(migrationsDir)
	require.NoError(t, err, "migrations dir not found")

	adminDB, err := sql.Open("pgx", testDB)
	require.NoError(t, err)
	defer adminDB.Close()

	_, err = adminDB.ExecContext(context.Background(), "DROP DATABASE IF EXISTS test_metrics")
	require.NoError(t, err)
	_, err = adminDB.ExecContext(context.Background(), "CREATE DATABASE test_metrics")
	require.NoError(t, err)

	db, err := sql.Open("pgx", testDB)
	require.NoError(t, err)

	goose.SetLogger(goose.NopLogger())
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(db, migrationsDir))

	return db
}

func teardownTestDB(t *testing.T, db *sql.DB) {
	t.Helper()
	if db != nil {
		db.Close()
	}

	adminDB, err := sql.Open("pgx", "postgres://user:pass@localhost:5432/postgres?sslmode=disable")
	require.NoError(t, err)
	defer adminDB.Close()

	_, err = adminDB.ExecContext(context.Background(), "DROP DATABASE IF EXISTS test_metrics")
	require.NoError(t, err)
}
func Test_Migrations_Applied(t *testing.T) {
	db := setupTestDB(t)
	defer teardownTestDB(t, db)

	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'gauge'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists, "Table 'gauge' should exist after migrations")

	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'counter'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists, "Table 'counter' should exist after migrations")
}
