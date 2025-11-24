package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/go-chi/chi"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
	"github.com/SergeyDolin/metrics-and-alerting/internal/storage"
)

type MetricType string

const (
	MetricTypeGauge   MetricType = "gauge"
	MetricTypeCounter MetricType = "counter"
)

func computeHMACSHA256(data, key []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// indexHandler — возвращает HTTP-обработчик, который выводит все метрики (gauge и counter) в виде строки.
// Формат: "metric1=value1, metric2=value2, ..."
// Поддерживает только GET-запросы. При других методах возвращает ошибку 405.
func indexHandler(store storage.Storage) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(res, "Only GET request allowed!", http.StatusMethodNotAllowed)
			return
		}
		metrics, err := store.GetAll()
		if err != nil {
			http.Error(res, "Failed to fetch metrics", http.StatusInternalServerError)
			return
		}

		res.Header().Set("Content-Type", "text/html; charset=utf-8")

		html := `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Metrics</title></head>
<body><h1>Metrics</h1><ul>`
		for _, m := range metrics {
			switch m.MType {
			case "gauge":
				if m.Value != nil {
					html += fmt.Sprintf("<li><strong>%s</strong>: %v (gauge)</li>", m.ID, *m.Value)
				}
			case "counter":
				if m.Delta != nil {
					html += fmt.Sprintf("<li><strong>%s</strong>: %v (counter)</li>", m.ID, *m.Delta)
				}
			}
		}
		html += `</ul></body></html>`
		io.WriteString(res, html)
	}
}

// getHandler — возвращает HTTP-обработчик для получения значения конкретной метрики по типу и имени.
// URL: /value/{type}/{name}
// Поддерживает только GET. Возвращает значение метрики или ошибку 404, если метрика не найдена.
// Типы: "gauge" или "counter". Регистр не важен.
func getHandler(store storage.Storage) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(res, "Only GET request allowed!", http.StatusMethodNotAllowed)
			return
		}

		metricType := strings.ToLower(chi.URLParam(req, "type"))
		metricName := chi.URLParam(req, "name")

		switch metricType {
		case "gauge":
			if value, exists := store.GetGauge(metricName); exists {
				io.WriteString(res, fmt.Sprintf("%v", value))
				return
			}
			http.Error(res, "Unknown metric name", http.StatusNotFound)
			return

		case "counter":
			if value, exists := store.GetCounter(metricName); exists {
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
func metricHandler(metricType MetricType, parser func(string) (interface{}, error), updateFunc func(string, interface{})) http.HandlerFunc {
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

func postHandler(store storage.Storage, saveFunc func()) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(res, "Only POST request allowed!", http.StatusMethodNotAllowed)
			return
		}

		metricType := strings.ToLower(chi.URLParam(req, "type"))
		name := chi.URLParam(req, "name")
		valueStr := chi.URLParam(req, "value")

		var err error
		switch metricType {
		case "gauge":
			var v float64
			if v, err = strconv.ParseFloat(valueStr, 64); err != nil {
				http.Error(res, "Only Float type for Gauge allowed!", http.StatusBadRequest)
				return
			}
			if err = store.UpdateGauge(name, v); err != nil {
				http.Error(res, "Failed to update metric", http.StatusInternalServerError)
				return
			}

		case "counter":
			var d int64
			if d, err = strconv.ParseInt(valueStr, 10, 64); err != nil {
				http.Error(res, "Only Int type for Counter allowed!", http.StatusBadRequest)
				return
			}
			if err = store.UpdateCounter(name, d); err != nil {
				http.Error(res, "Failed to update metric", http.StatusInternalServerError)
				return
			}

		default:
			http.Error(res, "Unknown metric type", http.StatusBadRequest)
			return
		}

		saveFunc()
		res.WriteHeader(http.StatusOK)
	}
}

func updateJSONHandler(store storage.Storage, saveFunc func()) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var m metrics.Metrics
		if err := json.NewDecoder(req.Body).Decode(&m); err != nil {
			writeJSONError(res, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if m.ID == "" {
			writeJSONError(res, http.StatusBadRequest, "Missing metric ID")
			return
		}

		switch m.MType {
		case "gauge":
			if m.Value == nil {
				writeJSONError(res, http.StatusBadRequest, "Missing 'value' for gauge metric")
				return
			}
			if m.Delta != nil {
				writeJSONError(res, http.StatusBadRequest, "Unexpected 'delta' for gauge metric")
				return
			}
			if err := store.UpdateGauge(m.ID, *m.Value); err != nil {
				writeJSONError(res, http.StatusInternalServerError, "Storage error")
				return
			}

		case "counter":
			if m.Delta == nil {
				writeJSONError(res, http.StatusBadRequest, "Missing 'delta' for counter metric")
				return
			}
			if m.Value != nil {
				writeJSONError(res, http.StatusBadRequest, "Unexpected 'value' for counter metric")
				return
			}
			if err := store.SetCounter(m.ID, *m.Delta); err != nil {
				writeJSONError(res, http.StatusInternalServerError, "Storage error")
				return
			}

		default:
			writeJSONError(res, http.StatusBadRequest, "Unknown metric type")
			return
		}

		saveFunc()
		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(m)
	}
}

func valueJSONHandler(store storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var r metrics.Metrics
		if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
			writeJSONError(res, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if r.ID == "" || r.MType == "" {
			writeJSONError(res, http.StatusBadRequest, "Missing ID or type")
			return
		}

		var resp metrics.Metrics
		found := false

		switch r.MType {
		case "gauge":
			if v, ok := store.GetGauge(r.ID); ok {
				resp = metrics.Metrics{ID: r.ID, MType: "gauge", Value: &v}
				found = true
			}
		case "counter":
			if d, ok := store.GetCounter(r.ID); ok {
				resp = metrics.Metrics{ID: r.ID, MType: "counter", Delta: &d}
				found = true
			}
		default:
			writeJSONError(res, http.StatusBadRequest, "Unknown metric type")
			return
		}

		if !found {
			writeJSONError(res, http.StatusNotFound, "Metric not found")
			return
		}

		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(resp)
	}
}

func pingSQLHandler(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if dbStore, ok := store.(*storage.DBStorage); ok {
			if err := dbStore.Ping(context.Background()); err != nil {
				http.Error(w, "Couldn't connect to the database: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "DATABASE_DSN is not configured", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func updatesBatchHandler(store storage.Storage, saveFunc func()) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var batch []metrics.Metrics
		if err := json.NewDecoder(req.Body).Decode(&batch); err != nil {
			writeJSONError(res, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if len(batch) == 0 {
			writeJSONError(res, http.StatusBadRequest, "Empty batch not allowed")
			return
		}

		for _, m := range batch {
			if m.ID == "" {
				writeJSONError(res, http.StatusBadRequest, "Missing metric ID in batch")
				return
			}
			switch m.MType {
			case "gauge":
				if m.Value == nil {
					writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Missing 'value' for gauge metric %s", m.ID))
					return
				}
				if m.Delta != nil {
					writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Unexpected 'delta' for gauge metric %s", m.ID))
					return
				}
			case "counter":
				if m.Delta == nil {
					writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Missing 'delta' for counter metric %s", m.ID))
					return
				}
				if m.Value != nil {
					writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Unexpected 'value' for counter metric %s", m.ID))
					return
				}
			default:
				writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Unknown metric type for %s", m.ID))
				return
			}
		}

		for _, m := range batch {
			var err error
			switch m.MType {
			case "gauge":
				err = store.UpdateGauge(m.ID, *m.Value)
			case "counter":
				err = store.SetCounter(m.ID, *m.Delta)
			}
			if err != nil {
				writeJSONError(res, http.StatusBadRequest, fmt.Sprintf("Storage error during batch update %s", m.ID))
				return
			}
		}

		saveFunc()
		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(batch)
	}
}
