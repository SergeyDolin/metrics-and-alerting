package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_sendMetric(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "text/plain", r.Header.Get("Content-Type"))

		path := r.URL.Path
		assert.True(t, strings.HasPrefix(path, "/update/"))

		parts := strings.Split(strings.TrimPrefix(path, "/update/"), "/")
		assert.Len(t, parts, 3)

		metricType, metricName, metricValue := parts[0], parts[1], parts[2]
		assert.NotEmpty(t, metricType)
		assert.NotEmpty(t, metricName)
		assert.NotEmpty(t, metricValue)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverAddr := server.Listener.Addr().String()

	tests := []struct {
		name       string
		metricName string
		typeMetric string
		value      string
		serverAddr string
		wantErr    bool
	}{
		{
			name:       "Valid gauge metric",
			metricName: "Alloc",
			typeMetric: "gauge",
			value:      "12345.678",
			serverAddr: serverAddr,
			wantErr:    false,
		},
		{
			name:       "Valid counter metric",
			metricName: "PollCount",
			typeMetric: "counter",
			value:      "5",
			serverAddr: serverAddr,
			wantErr:    false,
		},
		{
			name:       "Invalid server address",
			metricName: "Alloc",
			typeMetric: "gauge",
			value:      "1.0",
			serverAddr: "localhost:12345",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sendMetric(tt.metricName, tt.typeMetric, tt.value, tt.serverAddr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_sendMetricJSON(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "gzip", r.Header.Get("Content-Encoding"))
		gz, err := gzip.NewReader(r.Body)
		assert.NoError(t, err)
		defer gz.Close()
		body, _ := io.ReadAll(gz)
		var m Metrics
		assert.NoError(t, json.Unmarshal(body, &m))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(m)
	}))
	defer okServer.Close()

	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errServer.Close()

	okAddr := strings.TrimPrefix(okServer.URL, "http://")
	errAddr := strings.TrimPrefix(errServer.URL, "http://")

	tests := []struct {
		name        string
		metricName  string
		metricType  string
		value       *float64
		delta       *int64
		serverAddr  string
		expectError bool
	}{
		{
			name:        "Valid gauge metric",
			metricName:  "Temperature",
			metricType:  "gauge",
			value:       func() *float64 { v := 25.5; return &v }(),
			delta:       nil,
			serverAddr:  okAddr,
			expectError: false,
		},
		{
			name:        "Valid counter metric",
			metricName:  "PollCount",
			metricType:  "counter",
			value:       nil,
			delta:       func() *int64 { v := int64(10); return &v }(),
			serverAddr:  okAddr,
			expectError: false,
		},
		{
			name:        "Server returns error",
			metricName:  "RandomValue",
			metricType:  "gauge",
			value:       func() *float64 { v := 0.123; return &v }(),
			delta:       nil,
			serverAddr:  errAddr,
			expectError: true,
		},
		{
			name:        "Invalid server address",
			metricName:  "Alloc",
			metricType:  "gauge",
			value:       func() *float64 { v := 12345.0; return &v }(),
			delta:       nil,
			serverAddr:  "localhost:12345",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sendMetricJSON(tt.metricName, tt.metricType, tt.serverAddr, tt.value, tt.delta)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_sendMetricJSON_GzipCompression(t *testing.T) {
	var receivedBody []byte
	var contentEncoding string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentEncoding = r.Header.Get("Content-Encoding")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverAddr := strings.TrimPrefix(server.URL, "http://")

	metricName := "TestGauge"
	metricType := "gauge"
	value := 42.0

	err := sendMetricJSON(metricName, metricType, serverAddr, &value, nil)
	assert.NoError(t, err)

	assert.Equal(t, "gzip", contentEncoding)

	gz, err := gzip.NewReader(bytes.NewReader(receivedBody))
	assert.NoError(t, err)
	defer gz.Close()

	decompressed, err := io.ReadAll(gz)
	assert.NoError(t, err)

	var metric Metrics
	err = json.Unmarshal(decompressed, &metric)
	assert.NoError(t, err)
	assert.Equal(t, metricName, metric.ID)
	assert.Equal(t, metricType, metric.MType)
	assert.Equal(t, value, *metric.Value)
}
func Test_sendBatchJSON_Success(t *testing.T) {
	var receivedMetrics []Metrics
	var contentEncoding string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/updates", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		contentEncoding = r.Header.Get("Content-Encoding")

		gz, err := gzip.NewReader(r.Body)
		assert.NoError(t, err)
		defer gz.Close()

		body, err := io.ReadAll(gz)
		assert.NoError(t, err)

		err = json.Unmarshal(body, &receivedMetrics)
		assert.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(receivedMetrics)
	}))
	defer server.Close()

	serverAddr := strings.TrimPrefix(server.URL, "http://")

	batch := []Metrics{
		{ID: "Gauge1", MType: "gauge", Value: func(v float64) *float64 { return &v }(1.1)},
		{ID: "Counter1", MType: "counter", Delta: func(v int64) *int64 { return &v }(2)},
	}

	err := sendBatchJSON(batch, serverAddr)
	assert.NoError(t, err)
	assert.Equal(t, "gzip", contentEncoding)
	assert.Len(t, receivedMetrics, 2)

	gotGauge := false
	gotCounter := false
	for _, m := range receivedMetrics {
		if m.ID == "Gauge1" && m.MType == "gauge" && *m.Value == 1.1 {
			gotGauge = true
		}
		if m.ID == "Counter1" && m.MType == "counter" && *m.Delta == 2 {
			gotCounter = true
		}
	}
	assert.True(t, gotGauge)
	assert.True(t, gotCounter)
}
func Test_sendBatchJSON_EmptyBatch(t *testing.T) {
	// Запускаем сервер, но он не должен получить запрос
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("Server should not receive request for empty batch")
	}))
	defer server.Close()

	serverAddr := strings.TrimPrefix(server.URL, "http://")

	// Пустой срез
	err := sendBatchJSON([]Metrics{}, serverAddr)
	assert.NoError(t, err) // должно завершиться без ошибки и без запроса
}
