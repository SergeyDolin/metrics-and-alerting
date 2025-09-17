package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi"
	"github.com/stretchr/testify/assert"
)

func Test_postHandler(t *testing.T) {
	ms := createMetricStorage()

	router := chi.NewRouter()

	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Only POST request allowed!", http.StatusMethodNotAllowed)
	})

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Invalid path format", http.StatusNotFound)
	})

	router.Post("/update/{type}/{name}/{value}", http.HandlerFunc(postHandler(ms)))

	tests := []struct {
		name            string
		method          string
		contentType     string
		url             string
		expectedStatus  int
		expectedBody    string
		expectedGauge   map[string]float64
		expectedCounter map[string]int64
	}{
		{
			name:            "Valid gauge update",
			method:          http.MethodPost,
			contentType:     "text/plain",
			url:             "/update/gauge/temp/25.5",
			expectedStatus:  http.StatusOK,
			expectedGauge:   map[string]float64{"temp": 25.5},
			expectedCounter: map[string]int64{},
		},
		{
			name:            "Valid counter update",
			method:          http.MethodPost,
			contentType:     "text/plain",
			url:             "/update/counter/req/10",
			expectedStatus:  http.StatusOK,
			expectedGauge:   map[string]float64{},
			expectedCounter: map[string]int64{"req": 10},
		},
		{
			name:            "Counter increment twice",
			method:          http.MethodPost,
			contentType:     "text/plain",
			url:             "/update/counter/hits/7",
			expectedStatus:  http.StatusOK,
			expectedCounter: map[string]int64{"hits": 7},
		},
		{
			name:           "Invalid method",
			method:         http.MethodGet,
			contentType:    "text/plain",
			url:            "/update/gauge/temp/1.0",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   "Only POST request allowed!",
		},
		// {
		// 	name:           "Invalid content type",
		// 	method:         http.MethodPost,
		// 	contentType:    "application/json",
		// 	url:            "/update/gauge/temp/1.0",
		// 	expectedStatus: http.StatusBadRequest,
		// 	expectedBody:   "Invalid Content-Type",
		// },
		{
			name:           "Invalid path format",
			method:         http.MethodPost,
			contentType:    "text/plain",
			url:            "/update/gauge/temp",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Invalid path format",
		},
		{
			name:           "Unknown metric type",
			method:         http.MethodPost,
			contentType:    "text/plain",
			url:            "/update/unknown/test/123",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Unknown metric type",
		},
		{
			name:           "Invalid gauge value",
			method:         http.MethodPost,
			contentType:    "text/plain",
			url:            "/update/gauge/temp/abc",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Only Float type for Gauge allowed!",
		},
		{
			name:           "Invalid counter value",
			method:         http.MethodPost,
			contentType:    "text/plain",
			url:            "/update/counter/req/xyz",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Only Int type for Counter allowed!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.url, nil)
			req.Header.Set("Content-Type", tt.contentType)

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code, "HTTP status not equal")

			body := rr.Body.String()
			assert.Contains(t, body, tt.expectedBody, "Body not include expected text")

			for name, value := range tt.expectedGauge {
				got, exists := ms.gauge[name]
				assert.True(t, exists, "Expect metric gauge %s", name)
				assert.Equal(t, value, got, "Value gauge %s not contain", name)
			}

			for name, value := range tt.expectedCounter {
				got, exists := ms.counter[name]
				assert.True(t, exists, "Expect metric counter %s", name)
				assert.Equal(t, value, got, "Value counter %s not contain", name)
			}
		})
	}
}
