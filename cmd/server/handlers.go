package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
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

func postHandler(ms *MetricStorage) http.HandlerFunc {
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
				},
			)(res, req)

		case MetricTypeCounter:
			metricHandler(ms, MetricTypeCounter,
				func(s string) (interface{}, error) {
					return strconv.ParseInt(s, 10, 64)
				},
				func(name string, i interface{}) {
					ms.updateCounter(name, i.(int64))
				},
			)(res, req)

		default:
			http.Error(res, "Unknown metric type", http.StatusBadRequest)
		}
	})
}

func updateJSONHandler(ms *MetricStorage) http.HandlerFunc {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		var msJSON Metrics

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

		default:
			http.Error(res, "Unknown metric type", http.StatusBadRequest)
		}

		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusOK)
	})
}

func valueJSONHandler(ms *MetricStorage) http.HandlerFunc {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		var msJSON Metrics

		if err := json.NewDecoder(req.Body).Decode(&msJSON); err != nil {
			http.Error(res, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if msJSON.ID == "" {
			http.Error(res, "Missing metric ID", http.StatusBadRequest)
			return
		}
		if MetricType(msJSON.MType) == "" {
			http.Error(res, "Missing metric type", http.StatusBadRequest)
			return
		}

		switch MetricType(msJSON.MType) {
		case MetricTypeGauge:
			resp, err := json.Marshal(ms.gauge[msJSON.ID])
			if err != nil {
				http.Error(res, err.Error(), http.StatusInternalServerError)
				return
			}
			res.Write(resp)
		case MetricTypeCounter:
			resp, err := json.Marshal(ms.counter[msJSON.ID])
			if err != nil {
				http.Error(res, err.Error(), http.StatusInternalServerError)
				return
			}
			res.Write(resp)
		default:
			http.Error(res, "Unknown metric type", http.StatusBadRequest)
		}

		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusOK)

	})
}
