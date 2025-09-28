package main

import (
	"net/http"
	"net/http/httptest"
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
