package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_sendMetric(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method, "Expected POST req")

		assert.Equal(t, "text/plain", r.Header.Get("Content-Type"), "Expected Content-Type: text/plain")

		path := r.URL.Path
		assert.True(t, len(path) > 8, "Path so small")
		assert.Contains(t, path, "/update/", "Path must beginning /update/")

		if r.URL.Query().Get("fail") == "true" {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	serverAddr := server.Listener.Addr().String()

	type metric struct {
		name       string
		typeMetric string
		value      string
		serverAddr string
	}

	tests := []struct {
		name       string
		metric     metric
		wantErr    bool
		serverFail bool
	}{
		{
			name: "Valid gauge metric",
			metric: metric{
				name:       "Alloc",
				typeMetric: "gauge",
				value:      "12345.678",
				serverAddr: serverAddr,
			},
			wantErr:    false,
			serverFail: false,
		},
		{
			name: "Valid counter metric",
			metric: metric{
				name:       "PollCount",
				typeMetric: "counter",
				value:      "5",
				serverAddr: serverAddr,
			},
			wantErr:    false,
			serverFail: false,
		},
		{
			name: "Server returns error",
			metric: metric{
				name:       "RandomValue",
				typeMetric: "gauge",
				value:      "0.123",
				serverAddr: serverAddr,
			},
			wantErr:    true,
			serverFail: true,
		},
		{
			name: "Empty name",
			metric: metric{
				name:       "",
				typeMetric: "gauge",
				value:      "1.0",
				serverAddr: serverAddr,
			},
			wantErr:    false,
			serverFail: false,
		},
		{
			name: "Invalid server address",
			metric: metric{
				name:       "Alloc",
				typeMetric: "gauge",
				value:      "1.0",
				serverAddr: "localhost:12345",
			},
			wantErr:    true,
			serverFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServerAddr := tt.metric.serverAddr

			err := sendMetric(tt.metric.name, tt.metric.typeMetric, tt.metric.value, testServerAddr)

			if !tt.wantErr {
				assert.NoError(t, err, "Error not expected: %v", err)
			}

			if tt.metric.serverAddr == "localhost:12345" && err != nil {
				assert.Contains(t, err.Error(), "connection refused")
			}
		})
	}
}

func Test_sendMetricJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "/update", r.URL.Path) // ← теперь должно быть /update

		var received Metrics
		err := json.NewDecoder(r.Body).Decode(&received)
		assert.NoError(t, err)

		if r.URL.Query().Get("fail") == "true" {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(received)
		}
	}))
	defer server.Close()

	// Извлекаем хост:порт из server.URL
	serverAddr := strings.TrimPrefix(server.URL, "http://")

	tests := []struct {
		name        string
		metricName  string
		metricType  string
		value       *float64
		delta       *int64
		queryFail   bool
		expectError bool
	}{
		{
			name:        "Valid gauge metric",
			metricName:  "Temperature",
			metricType:  "gauge",
			value:       func() *float64 { v := 25.5; return &v }(),
			delta:       nil,
			queryFail:   false,
			expectError: false,
		},
		{
			name:        "Invalid server address",
			metricName:  "Alloc",
			metricType:  "gauge",
			value:       func() *float64 { v := 12345.0; return &v }(),
			delta:       nil,
			queryFail:   false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServerAddr := serverAddr
			if tt.name == "Invalid server address" {
				testServerAddr = "localhost:12345"
			}

			err := sendMetricJSON(tt.metricName, tt.metricType, testServerAddr, tt.value, tt.delta)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
