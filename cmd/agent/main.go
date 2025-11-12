package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"time"
)

var сlient = &http.Client{}

type Metrics struct {
	ID    string   `json:"id"`
	MType string   `json:"type"`
	Delta *int64   `json:"delta,omitempty"`
	Value *float64 `json:"value,omitempty"`
}

type MetricStorage struct {
	gauge   map[string]float64
	counter map[string]int64
}

func createMetricStorage() *MetricStorage {
	return &MetricStorage{
		gauge:   make(map[string]float64),
		counter: make(map[string]int64),
	}
}

var fieldMap = map[string]func(*runtime.MemStats) float64{
	"Alloc":         func(m *runtime.MemStats) float64 { return float64(m.Alloc) },
	"BuckHashSys":   func(m *runtime.MemStats) float64 { return float64(m.BuckHashSys) },
	"Frees":         func(m *runtime.MemStats) float64 { return float64(m.Frees) },
	"GCCPUFraction": func(m *runtime.MemStats) float64 { return float64(m.GCCPUFraction) },
	"GCSys":         func(m *runtime.MemStats) float64 { return float64(m.GCSys) },
	"HeapAlloc":     func(m *runtime.MemStats) float64 { return float64(m.HeapAlloc) },
	"HeapIdle":      func(m *runtime.MemStats) float64 { return float64(m.HeapIdle) },
	"HeapObjects":   func(m *runtime.MemStats) float64 { return float64(m.HeapObjects) },
	"HeapReleased":  func(m *runtime.MemStats) float64 { return float64(m.HeapReleased) },
	"HeapSys":       func(m *runtime.MemStats) float64 { return float64(m.HeapSys) },
	"LastGC":        func(m *runtime.MemStats) float64 { return float64(m.LastGC) },
	"Lookups":       func(m *runtime.MemStats) float64 { return float64(m.Lookups) },
	"MCacheInuse":   func(m *runtime.MemStats) float64 { return float64(m.MCacheInuse) },
	"MCacheSys":     func(m *runtime.MemStats) float64 { return float64(m.MCacheSys) },
	"MSpanSys":      func(m *runtime.MemStats) float64 { return float64(m.MSpanSys) },
	"Mallocs":       func(m *runtime.MemStats) float64 { return float64(m.Mallocs) },
	"NextGC":        func(m *runtime.MemStats) float64 { return float64(m.NextGC) },
	"NumForcedGC":   func(m *runtime.MemStats) float64 { return float64(m.NumForcedGC) },
	"NumGC":         func(m *runtime.MemStats) float64 { return float64(m.NumGC) },
	"OtherSys":      func(m *runtime.MemStats) float64 { return float64(m.OtherSys) },
	"PauseTotalNs":  func(m *runtime.MemStats) float64 { return float64(m.PauseTotalNs) },
	"StackInuse":    func(m *runtime.MemStats) float64 { return float64(m.StackInuse) },
	"Sys":           func(m *runtime.MemStats) float64 { return float64(m.Sys) },
	"TotalAlloc":    func(m *runtime.MemStats) float64 { return float64(m.TotalAlloc) },
	"HeapInuse":     func(m *runtime.MemStats) float64 { return float64(m.HeapInuse) },
	"MSpanInuse":    func(m *runtime.MemStats) float64 { return float64(m.MSpanInuse) },
	"StackSys":      func(m *runtime.MemStats) float64 { return float64(m.StackSys) },
}

func (ms *MetricStorage) getMetrics(m *runtime.MemStats) {
	for name, metric := range fieldMap {
		ms.gauge[name] = metric(m)
	}
	ms.counter["PollCount"]++
	ms.gauge["RandomValue"] = rand.Float64()
}

func sendMetric(name, typeMetric string, value string, serverAddr string) error {
	// http://<АДРЕС_СЕРВЕРА>/update/<ТИП_МЕТРИКИ>/<ИМЯ_МЕТРИКИ>/<ЗНАЧЕНИЕ_МЕТРИКИ>
	url := fmt.Sprintf("http://%s/update/%s/%s/%s", serverAddr, typeMetric, name, value)

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("request error: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := сlient.Do(req)
	if err != nil {
		return fmt.Errorf("response error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server return code %d", resp.StatusCode)
	}
	return nil
}

func sendMetricJSON(name, metricType string, serverAddr string, value *float64, delta *int64) error {
	var b bytes.Buffer

	metric := Metrics{
		ID:    name,
		MType: metricType,
		Value: value,
		Delta: delta,
	}

	body, err := json.Marshal(metric)
	if err != nil {
		return fmt.Errorf("failed to marshal metric: %w", err)
	}

	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(body); err != nil {
		gz.Close()
		return fmt.Errorf("failed to compress body: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	url := fmt.Sprintf("http://%s/update", serverAddr)
	req, err := http.NewRequest(http.MethodPost, url, &b)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := сlient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}
	return nil
}

func sendBatchJSON(metricsList []Metrics, serverAddr string) error {
	if len(metricsList) == 0 {
		return nil
	}

	body, err := json.Marshal(metricsList)
	if err != nil {
		return fmt.Errorf("failed to marshal batch: %w", err)
	}

	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(body); err != nil {
		gz.Close()
		return fmt.Errorf("failed to compress batch: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	url := fmt.Sprintf("http://%s/updates", serverAddr)
	req, err := http.NewRequest(http.MethodPost, url, &b)
	if err != nil {
		return fmt.Errorf("failed to create batch request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := сlient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d for batch", resp.StatusCode)
	}
	return nil
}

func main() {
	parseArgs()
	ms := createMetricStorage()

	serverAddr := *sAddr

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	pollInterval := time.Duration(*pInterval) * time.Second
	reportInterval := time.Duration(*rInterval) * time.Second
	lastReport := time.Now()

	ms.getMetrics(&m)
	useBatch := true

	for {
		time.Sleep(pollInterval)
		runtime.ReadMemStats(&m)
		ms.getMetrics(&m)

		if time.Since(lastReport) >= reportInterval {
			var batch []Metrics
			for name, v := range ms.gauge {
				batch = append(batch, Metrics{ID: name, MType: "gauge", Value: &v})
			}
			for name, d := range ms.counter {
				batch = append(batch, Metrics{ID: name, MType: "counter", Delta: &d})
			}

			var err error
			if useBatch {
				err = sendBatchJSON(batch, serverAddr)
				if err != nil {
					fmt.Println("New API /updates/ not available, falling back to old API")
					useBatch = false
					for _, m := range batch {
						if m.MType == "gauge" {
							sendMetricJSON(m.ID, m.MType, serverAddr, m.Value, nil)
						} else {
							sendMetricJSON(m.ID, m.MType, serverAddr, nil, m.Delta)
						}
					}
				}
			} else {
				for _, m := range batch {
					if m.MType == "gauge" {
						sendMetricJSON(m.ID, m.MType, serverAddr, m.Value, nil)
					} else {
						sendMetricJSON(m.ID, m.MType, serverAddr, nil, m.Delta)
					}
				}
			}

			lastReport = time.Now()
		}
	}
}
