package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"time"
)

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

func main() {
	client := http.Client{}

	parseArgs()
	ms := createMetricStorage()

	serverAddr := *sAddr

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	pollInterval := time.Duration(*pInterval) * time.Second
	reportInterval := time.Duration(*rInterval) * time.Second
	lastReport := time.Now()

	ms.getMetrics(&m)

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

			err := retryWithBackoff(func() error {
				return sendBatchJSON(&client, batch, serverAddr)
			})
			if err != nil {
				fmt.Printf("Batch send failed after reties: %v\n", err)
				for _, m := range batch {
					mR := m
					retryErr := retryWithBackoff(func() error {
						if mR.MType == "gauge" {
							return sendMetricJSON(&client, mR.ID, mR.MType, serverAddr, mR.Value, nil)
						} else {
							return sendMetricJSON(&client, mR.ID, mR.MType, serverAddr, nil, mR.Delta)
						}
					})
					if retryErr != nil {
						fmt.Printf("Failed to send metric %s: %v\n", mR.ID, retryErr)
					}
				}
			} else {
				for _, m := range batch {
					if m.MType == "gauge" {
						sendMetricJSON(&client, m.ID, m.MType, serverAddr, m.Value, nil)
					} else {
						sendMetricJSON(&client, m.ID, m.MType, serverAddr, nil, m.Delta)
					}
				}
			}
			lastReport = time.Now()
		}
	}
}
