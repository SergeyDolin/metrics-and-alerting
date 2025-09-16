package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
)

const (
	metricCount = 26
)

var MetricNames = [metricCount]string{
	"Alloc",
	"BuckHashSys",
	"Frees",
	"GCCPUFraction",
	"GCSys",
	"HeapAlloc",
	"HeapIdle",
	"HeapObjects",
	"HeapReleased",
	"HeapSys",
	"LastGC",
	"Lookups",
	"MCacheInuse",
	"MCacheSys",
	"MSpanSys",
	"Mallocs",
	"NextGC",
	"NumForcedGC",
	"NumGC",
	"OtherSys",
	"PauseTotalNs",
	"StackInuse",
	"Sys",
	"TotalAlloc",
	"PollCount",
	"RandomValue",
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

func indexHandler(ms *MetricStorage) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(res, "Only GET request allowed!", http.StatusMethodNotAllowed)
			return
		}
		list := make([]string, 0)
		for name, value := range ms.gauge {
			list = append(list, fmt.Sprintf("%s=%v", name, value))
		}
		for name, value := range ms.counter {
			list = append(list, fmt.Sprintf("%s=%v", name, value))
		}
		io.WriteString(res, strings.Join(list, ", "))
	}
}

func getHandler(ms *MetricStorage) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(res, "Only GET request allowed!", http.StatusMethodNotAllowed)
			return
		}

		metricType := strings.ToLower(chi.URLParam(req, "type"))
		metricName := chi.URLParam(req, "name")

		switch metricType {
		case "gauge":
			for _, metric := range MetricNames {
				if metric == metricName {
					io.WriteString(res, fmt.Sprintf("%s=%v", metricName, ms.gauge[metricName]))
					return
				}
			}
			http.Error(res, "Unknown metric name", http.StatusNotFound)
			return

		case "counter":
			for _, metric := range MetricNames {
				if metric == metricName {
					io.WriteString(res, fmt.Sprintf("%s=%v", metricName, ms.counter[metricName]))
					return
				}
			}
			http.Error(res, "Unknown metric name", http.StatusNotFound)
			return

		default:
			http.Error(res, "Unknown metric type", http.StatusNotFound)
			return
		}

	}
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

		typeOfMetric := strings.ToLower(chi.URLParam(req, "type"))
		nameOfMetric := chi.URLParam(req, "name")
		valueOfMetric := chi.URLParam(req, "value")

		if typeOfMetric == "" || nameOfMetric == "" || valueOfMetric == "" {
			http.Error(res, "Missing required parameters", http.StatusBadRequest)
			return
		}

		switch typeOfMetric {
		case "gauge":
			value, err := strconv.ParseFloat(valueOfMetric, 64)
			if err != nil {
				http.Error(res, "Only Float type for Gauge allowed!", http.StatusBadRequest)
				return
			}
			ms.updateGauge(nameOfMetric, value)
			res.WriteHeader(http.StatusOK)

		case "counter":
			value, err := strconv.ParseInt(valueOfMetric, 10, 64)
			if err != nil {
				http.Error(res, "Only Int type for Counter allowed!", http.StatusBadRequest)
				return
			}
			ms.updateCounter(nameOfMetric, value)
			res.WriteHeader(http.StatusOK)

		default:
			http.Error(res, "Unknown metric type", http.StatusBadRequest)
			return
		}
	}
}

func main() {
	router := chi.NewRouter()
	ms := createMetricStorage()

	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Only POST request allowed!", http.StatusMethodNotAllowed)
	})

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Invalid path format", http.StatusNotFound)
	})

	router.Route("/", func(r chi.Router) {
		r.Get("/", indexHandler(ms))
		r.Route("/update", func(r chi.Router) {
			r.Post("/{type}/{name}/{value}", http.HandlerFunc(postHandler(ms)))
		})
		// http://<АДРЕС_СЕРВЕРА>/value/<ТИП_МЕТРИКИ>/<ИМЯ_МЕТРИКИ>
		r.Route("/value", func(r chi.Router) {
			r.Get("/{type}/{name}", http.HandlerFunc(getHandler(ms)))
		})
	})

	log.Fatal(http.ListenAndServe(":8080", router))

}
