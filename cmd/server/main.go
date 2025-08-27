package main

import (
	"net/http"
	"strconv"
	"strings"
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

func (ms *MetricStorage) updateGauge(name string, value float64) {
	if _, ok := ms.gauge[name]; !ok {
		ms.gauge[name] = 0
	}
	ms.gauge[name] = value
}

func (ms *MetricStorage) updateCounter(name string, value int64) {
	if _, ok := ms.counter[name]; !ok {
		ms.counter[name] = 0
	}
	ms.counter[name] += value
}

func middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(res, req)
	})
}

func postHandler(ms *MetricStorage) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(res, "Only POST request allowed!", http.StatusMethodNotAllowed)
			return
		}

		if req.Header.Get("Content-Type") != "text/plain" {
			http.Error(res, "Invalid Content-Type", http.StatusBadRequest)
			return
		}

		pathParts := strings.Split(req.URL.Path, "/")

		if len(pathParts) != 5 {
			http.Error(res, "Invalid path format", http.StatusNotFound)
			return
		}

		typeOfMetric := pathParts[2]
		nameOfMetric := pathParts[3]
		valueOfMetric := pathParts[4]

		switch typeOfMetric {
		case "gauge":
			value, err := strconv.ParseFloat(valueOfMetric, 64)
			if err != nil {
				http.Error(res, "Only Float type for Gauge allowed!", http.StatusBadRequest)
				return
			}
			ms.updateGauge(nameOfMetric, value)
			res.WriteHeader(http.StatusOK)
			// fmt.Fprintf(res, "Gauge metric %s updated to %f", nameOfMetric, value)
		case "counter":
			value, err := strconv.ParseInt(valueOfMetric, 10, 64)
			if err != nil {
				http.Error(res, "Only Int type for Counter allowed!", http.StatusBadRequest)
				return
			}
			ms.updateCounter(nameOfMetric, value)
			res.WriteHeader(http.StatusOK)
			// fmt.Fprintf(res, "Counter metric %s updated to %d", nameOfMetric, value)
		default:
			http.Error(res, "Unknown metric type", http.StatusBadRequest)
			return
		}
	}
}

func main() {
	mux := http.NewServeMux()
	ms := createMetricStorage()

	mux.Handle(`/update`, middleware(http.HandlerFunc(postHandler(ms))))

	err := http.ListenAndServe(`:8080`, mux)
	if err != nil {
		panic(err)
	}
}
