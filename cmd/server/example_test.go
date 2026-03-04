package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
	"github.com/SergeyDolin/metrics-and-alerting/internal/storage"
	"github.com/go-chi/chi"
)

// This example demonstrates how to update a gauge metric using the legacy URL path format.
// URL pattern: POST /update/{type}/{name}/{value}
func Example_postHandler_gauge() {
	// Setup: Create a test server with in-memory storage
	store := storage.NewMemStorage()
	router := chi.NewRouter()
	router.Post("/update/{type}/{name}/{value}", postHandler(store, func() {}, nil))

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Send request to update a gauge metric
	url := fmt.Sprintf("%s/update/gauge/Alloc/42.5", ts.URL)
	resp, err := http.Post(url, "text/plain", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %s\n", resp.Status)

	// Verify the metric was stored
	if value, exists := store.GetGauge("Alloc"); exists {
		fmt.Printf("Alloc = %f\n", value)
	}

	// Output:
	// Status: 200 OK
	// Alloc = 42.500000
}

// This example demonstrates how to update a counter metric using the legacy URL path format.
// URL pattern: POST /update/{type}/{name}/{value}
func Example_postHandler_counter() {
	// Setup: Create a test server with in-memory storage
	store := storage.NewMemStorage()
	router := chi.NewRouter()
	router.Post("/update/{type}/{name}/{value}", postHandler(store, func() {}, nil))

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Send first update to increment counter
	url := fmt.Sprintf("%s/update/counter/PollCount/10", ts.URL)
	resp, err := http.Post(url, "text/plain", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	resp.Body.Close()

	// Send second update to increment further
	url = fmt.Sprintf("%s/update/counter/PollCount/5", ts.URL)
	resp, err = http.Post(url, "text/plain", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	resp.Body.Close()

	// Verify the counter was incremented (10 + 5 = 15)
	if value, exists := store.GetCounter("PollCount"); exists {
		fmt.Printf("PollCount = %d\n", value)
	}

	// Output:
	// PollCount = 15
}

// This example demonstrates how to update a metric using JSON format.
// URL pattern: POST /update
func Example_updateJSONHandler() {
	// Setup: Create a test server with in-memory storage
	store := storage.NewMemStorage()
	router := chi.NewRouter()
	router.Post("/update", updateJSONHandler(store, func() {}, nil))

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Create a gauge metric
	gaugeMetric := metrics.Metrics{
		ID:    "CPUUsage",
		MType: "gauge",
		Value: func() *float64 { v := 75.5; return &v }(),
	}

	// Marshal to JSON
	body, _ := json.Marshal(gaugeMetric)

	// Send request
	url := fmt.Sprintf("%s/update", ts.URL)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %s\n", resp.Status)

	// Read and parse response
	var response metrics.Metrics
	json.NewDecoder(resp.Body).Decode(&response)

	fmt.Printf("Metric: %s, Type: %s, Value: %f\n",
		response.ID, response.MType, *response.Value)

	// Output:
	// Status: 200 OK
	// Metric: CPUUsage, Type: gauge, Value: 75.500000
}

// This example demonstrates batch updating multiple metrics in a single request.
// URL pattern: POST /updates
func Example_updatesBatchHandler() {
	// Setup: Create a test server with in-memory storage
	store := storage.NewMemStorage()
	router := chi.NewRouter()
	router.Post("/updates", updatesBatchHandler(store, func() {}, nil))

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Create a batch of metrics
	gaugeVal := 42.5
	counterVal := int64(10)

	batch := []metrics.Metrics{
		{
			ID:    "Alloc",
			MType: "gauge",
			Value: &gaugeVal,
		},
		{
			ID:    "PollCount",
			MType: "counter",
			Delta: &counterVal,
		},
	}

	// Marshal batch to JSON
	body, _ := json.Marshal(batch)

	// Send request
	url := fmt.Sprintf("%s/updates", ts.URL)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %s\n", resp.Status)

	// Verify metrics were stored
	if value, exists := store.GetGauge("Alloc"); exists {
		fmt.Printf("Alloc = %f\n", value)
	}
	if value, exists := store.GetCounter("PollCount"); exists {
		fmt.Printf("PollCount = %d\n", value)
	}

	// Output:
	// Status: 200 OK
	// Alloc = 42.500000
	// PollCount = 10
}

// This example demonstrates retrieving a metric value using JSON format.
// URL pattern: POST /value
func Example_valueJSONHandler() {
	// Setup: Create a test server with pre-populated storage
	store := storage.NewMemStorage()
	store.UpdateGauge("Alloc", 42.5)
	store.UpdateCounter("PollCount", 10)

	router := chi.NewRouter()
	router.Post("/value", valueJSONHandler(store, nil))

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Request a gauge metric
	gaugeRequest := metrics.Metrics{
		ID:    "Alloc",
		MType: "gauge",
	}

	body, _ := json.Marshal(gaugeRequest)
	url := fmt.Sprintf("%s/value", ts.URL)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// Parse response
	var response metrics.Metrics
	json.NewDecoder(resp.Body).Decode(&response)

	fmt.Printf("Metric: %s, Type: %s, Value: %f\n",
		response.ID, response.MType, *response.Value)

	// Output:
	// Metric: Alloc, Type: gauge, Value: 42.500000
}

// This example demonstrates retrieving a metric value using the legacy URL path format.
// URL pattern: GET /value/{type}/{name}
func Example_getHandler() {
	// Setup: Create a test server with pre-populated storage
	store := storage.NewMemStorage()
	store.UpdateGauge("Alloc", 42.5)

	router := chi.NewRouter()
	router.Get("/value/{type}/{name}", getHandler(store, nil))

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Request the metric
	url := fmt.Sprintf("%s/value/gauge/Alloc", ts.URL)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Value: %s\n", strings.TrimSpace(string(body)))

	// Output:
	// Status: 200 OK
	// Value: 42.5
}

// This example demonstrates listing all metrics as HTML.
// URL pattern: GET /
func Example_indexHandler() {
	// Setup: Create a test server with pre-populated storage
	store := storage.NewMemStorage()
	store.UpdateGauge("Alloc", 42.5)
	store.UpdateGauge("CPUUsage", 75.5)
	store.UpdateCounter("PollCount", 10)

	router := chi.NewRouter()
	router.Get("/", indexHandler(store))

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Request the index page
	resp, err := http.Get(ts.URL)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))

	// Output:
	// Status: 200 OK
	// Content-Type: text/html; charset=utf-8
}

// This example demonstrates the complete workflow of the metrics agent and server.
// It shows how metrics are collected, sent, and retrieved.
func Example_workflow() {
	// Setup server with storage
	store := storage.NewMemStorage()
	router := chi.NewRouter()

	// Register handlers
	router.Post("/update", updateJSONHandler(store, func() {}, nil))
	router.Post("/value", valueJSONHandler(store, nil))

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Step 1: Agent sends a gauge metric
	gaugeMetric := metrics.Metrics{
		ID:    "Alloc",
		MType: "gauge",
		Value: func() *float64 { v := 42.5; return &v }(),
	}

	body, _ := json.Marshal(gaugeMetric)
	url := fmt.Sprintf("%s/update", ts.URL)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Error sending gauge: %v\n", err)
		return
	}
	resp.Body.Close()
	fmt.Println("Gauge metric sent")

	// Step 2: Agent sends a counter metric
	counterMetric := metrics.Metrics{
		ID:    "PollCount",
		MType: "counter",
		Delta: func() *int64 { v := int64(5); return &v }(),
	}

	body, _ = json.Marshal(counterMetric)
	resp, err = http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Error sending counter: %v\n", err)
		return
	}
	resp.Body.Close()
	fmt.Println("Counter metric sent")

	// Step 3: Client retrieves the gauge metric
	request := metrics.Metrics{
		ID:    "Alloc",
		MType: "gauge",
	}

	body, _ = json.Marshal(request)
	url = fmt.Sprintf("%s/value", ts.URL)
	resp, err = http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Error retrieving: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var response metrics.Metrics
	json.NewDecoder(resp.Body).Decode(&response)

	fmt.Printf("Retrieved %s = %f\n", response.ID, *response.Value)

	// Output:
	// Gauge metric sent
	// Counter metric sent
	// Retrieved Alloc = 42.500000
}
