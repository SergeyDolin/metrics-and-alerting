package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"time"
)

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

func (ms *MetricStorage) getMetrics(m *runtime.MemStats) {
	ms.gauge["Alloc"] = float64(m.Alloc)
	ms.gauge["BuckHashSys"] = float64(m.BuckHashSys)
	ms.gauge["Frees"] = float64(m.Frees)
	ms.gauge["GCCPUFraction"] = float64(m.GCCPUFraction)
	ms.gauge["GCSys"] = float64(m.GCSys)
	ms.gauge["HeapAlloc"] = float64(m.HeapAlloc)
	ms.gauge["HeapIdle"] = float64(m.HeapIdle)
	ms.gauge["HeapObjects"] = float64(m.HeapObjects)
	ms.gauge["HeapReleased"] = float64(m.HeapReleased)
	ms.gauge["HeapSys"] = float64(m.HeapSys)
	ms.gauge["LastGC"] = float64(m.LastGC)
	ms.gauge["Lookups"] = float64(m.Lookups)
	ms.gauge["MCacheInuse"] = float64(m.MCacheInuse)
	ms.gauge["MCacheSys"] = float64(m.MCacheSys)
	ms.gauge["MSpanSys"] = float64(m.MSpanSys)
	ms.gauge["Mallocs"] = float64(m.Mallocs)
	ms.gauge["NextGC"] = float64(m.NextGC)
	ms.gauge["NumForcedGC"] = float64(m.NumForcedGC)
	ms.gauge["NumGC"] = float64(m.NumGC)
	ms.gauge["OtherSys"] = float64(m.OtherSys)
	ms.gauge["PauseTotalNs"] = float64(m.PauseTotalNs)
	ms.gauge["StackInuse"] = float64(m.StackInuse)
	ms.gauge["Sys"] = float64(m.Sys)
	ms.gauge["TotalAlloc"] = float64(m.TotalAlloc)
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

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("response error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server return code %d", resp.StatusCode)
	}
	return nil
}

func main() {
	ms := createMetricStorage()

	serverAddr := "localhost:8080"

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	pollInterval := 2 * time.Second
	reportInterval := 10 * time.Second
	lastReport := time.Now()

	ms.getMetrics(&m)
	for {
		time.Sleep(pollInterval)
		ms.getMetrics(&m)

		if time.Since(lastReport) >= reportInterval {
			sent := 0
			for name, value := range ms.gauge {
				err := sendMetric(name, "gauge", fmt.Sprintf("%f", value), serverAddr)
				if err != nil {
					fmt.Printf("error send gauge %v", err)
				} else {
					sent++
				}
			}
			for name, value := range ms.counter {
				err := sendMetric(name, "counter", fmt.Sprintf("%d", value), serverAddr)
				if err != nil {
					fmt.Printf("error send counter %v", err)
				} else {
					sent++
				}
			}
			lastReport = time.Now()
		}
	}

}
