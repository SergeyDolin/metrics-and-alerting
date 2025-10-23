package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
)

type MetricType string

const (
	MetricTypeGauge   MetricType = "gauge"
	MetricTypeCounter MetricType = "counter"
)

// indexHandler — возвращает HTTP-обработчик, который выводит все метрики (gauge и counter) в виде строки.
// Формат: "metric1=value1, metric2=value2, ..."
// Поддерживает только GET-запросы. При других методах возвращает ошибку 405.
func indexHandler(ms *MetricStorage) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(res, "Only GET request allowed!", http.StatusMethodNotAllowed)
			return
		}
		ms.mu.Lock()
		defer ms.mu.Unlock()

		res.Header().Set("Content-Type", "text/html; charset=utf-8")

		html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Metrics</title>
</head>
<body>
    <h1>Metrics</h1>
    <ul>`
		for name, value := range ms.gauge {
			html += fmt.Sprintf("<li><strong>%s</strong>: %v (gauge)</li>", name, value)
		}
		for name, value := range ms.counter {
			html += fmt.Sprintf("<li><strong>%s</strong>: %v (counter)</li>", name, value)
		}
		html += `</ul></body></html>`

		io.WriteString(res, html)
	}
}

// getHandler — возвращает HTTP-обработчик для получения значения конкретной метрики по типу и имени.
// URL: /value/{type}/{name}
// Поддерживает только GET. Возвращает значение метрики или ошибку 404, если метрика не найдена.
// Типы: "gauge" или "counter". Регистр не важен.
func getHandler(ms *MetricStorage) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(res, "Only GET request allowed!", http.StatusMethodNotAllowed)
			return
		}

		metricType := strings.ToLower(chi.URLParam(req, "type"))
		metricName := chi.URLParam(req, "name")

		ms.mu.Lock()
		defer ms.mu.Unlock()

		switch metricType {
		case "gauge":
			if value, exists := ms.gauge[metricName]; exists {
				io.WriteString(res, fmt.Sprintf("%v", value))
				return
			}
			http.Error(res, "Unknown metric name", http.StatusNotFound)
			return

		case "counter":
			if value, exists := ms.counter[metricName]; exists {
				io.WriteString(res, fmt.Sprintf("%v", value))
				return
			}
			http.Error(res, "Unknown metric name", http.StatusNotFound)
			return

		default:
			http.Error(res, "Unknown metric type", http.StatusNotFound)
			return
		}
	}
}

// postHandler — возвращает HTTP-обработчик для обновления метрик через POST-запрос.
// URL: /update/{type}/{name}/{value}
// Поддерживает только POST. Валидирует тип значения в зависимости от типа метрики:
// - gauge: требует float64
// - counter: требует int64
// При успехе возвращает 200 OK, при ошибках — соответствующие HTTP-ошибки.
func metricHandler(ms *MetricStorage, metricType MetricType, parser func(string) (interface{}, error), updateFunc func(string, interface{})) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(res, "Only POST request allowed!", http.StatusMethodNotAllowed)
			return
		}

		typeOfMetric := strings.ToLower(chi.URLParam(req, "type"))
		nameOfMetric := chi.URLParam(req, "name")
		valueOfMetric := chi.URLParam(req, "value")

		if MetricType(typeOfMetric) != metricType {
			http.Error(res, "Invalid type of metric!", http.StatusBadRequest)
			return
		}

		parseValue, err := parser(valueOfMetric)
		if err != nil {
			var errorMes string
			switch metricType {
			case MetricTypeGauge:
				errorMes = "Only Float type for Gauge allowed!"
			case MetricTypeCounter:
				errorMes = "Only Int type for Counter allowed!"
			default:
				errorMes = "Unknown metric type"
			}
			http.Error(res, errorMes, http.StatusBadRequest)
			return
		}
		updateFunc(nameOfMetric, parseValue)
		res.WriteHeader(http.StatusOK)
	}
}

func postHandler(ms *MetricStorage, saveFunc func()) http.HandlerFunc {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		typeOfMetric := strings.ToLower(chi.URLParam(req, "type"))

		switch MetricType(typeOfMetric) {
		case MetricTypeGauge:
			metricHandler(ms, MetricTypeGauge,
				func(s string) (interface{}, error) {
					return strconv.ParseFloat(s, 64)
				},
				func(name string, i interface{}) {
					ms.updateGauge(name, i.(float64))
					saveFunc()
				},
			)(res, req)

		case MetricTypeCounter:
			metricHandler(ms, MetricTypeCounter,
				func(s string) (interface{}, error) {
					return strconv.ParseInt(s, 10, 64)
				},
				func(name string, i interface{}) {
					ms.updateCounter(name, i.(int64))
					saveFunc()
				},
			)(res, req)

		default:
			http.Error(res, "Unknown metric type", http.StatusBadRequest)
		}
	})
}

func updateJSONHandler(ms *MetricStorage, saveFunc func()) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var msJSON metrics.Metrics

		if err := json.NewDecoder(req.Body).Decode(&msJSON); err != nil {
			http.Error(res, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if msJSON.ID == "" {
			http.Error(res, "Missing metric ID", http.StatusBadRequest)
			return
		}

		switch MetricType(msJSON.MType) {
		case MetricTypeGauge:
			if msJSON.Value == nil {
				http.Error(res, "Missing 'value' for gauge metric", http.StatusBadRequest)
				return
			}
			if msJSON.Delta != nil {
				http.Error(res, "Unexpected 'delta' for gauge metric", http.StatusBadRequest)
				return
			}
			ms.updateGauge(msJSON.ID, *msJSON.Value)
			saveFunc()

		case MetricTypeCounter:
			if msJSON.Delta == nil {
				http.Error(res, "Missing 'delta' for counter metric", http.StatusBadRequest)
				return
			}
			if msJSON.Value != nil {
				http.Error(res, "Unexpected 'value' for counter metric", http.StatusBadRequest)
				return
			}
			ms.updateCounter(msJSON.ID, *msJSON.Delta)
			saveFunc()

		default:
			http.Error(res, "Unknown metric type", http.StatusBadRequest)
			return
		}

		res.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(res).Encode(msJSON); err != nil {
			http.Error(res, "Bad encode", http.StatusBadRequest)
			return
		}
	}
}

func valueJSONHandler(ms *MetricStorage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var reqMetric metrics.Metrics

		res.Header().Set("Content-Type", "application/json")

		if err := json.NewDecoder(req.Body).Decode(&reqMetric); err != nil {
			http.Error(res, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if reqMetric.ID == "" {
			http.Error(res, "Missing metric ID", http.StatusBadRequest)
			return
		}
		if reqMetric.MType == "" {
			http.Error(res, "Missing metric type", http.StatusBadRequest)
			return
		}

		switch MetricType(reqMetric.MType) {
		case MetricTypeGauge:
			if value, ok := ms.gauge[reqMetric.ID]; ok {
				respMetric := metrics.Metrics{
					ID:    reqMetric.ID,
					MType: "gauge",
					Value: &value,
				}
				if err := json.NewEncoder(res).Encode(respMetric); err != nil {
					http.Error(res, "Failed to encode response", http.StatusInternalServerError)
				}
				return
			}

		case MetricTypeCounter:
			if delta, ok := ms.counter[reqMetric.ID]; ok {
				respMetric := metrics.Metrics{
					ID:    reqMetric.ID,
					MType: "counter",
					Delta: &delta,
				}
				if err := json.NewEncoder(res).Encode(respMetric); err != nil {
					http.Error(res, "Failed to encode response", http.StatusInternalServerError)
				}
				return
			}

		default:
			http.Error(res, "Unknown metric type", http.StatusBadRequest)
			return
		}

		http.Error(res, "Metric not found", http.StatusNotFound)
	}
}
